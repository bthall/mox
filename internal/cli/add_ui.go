package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/bthall/mox/internal/config"
)

// The add wizard walks one field at a time toward a saved session: name,
// hosts, connection details, then a YAML preview to confirm. It only builds
// simple-mode sessions — custom pane layouts are better captured by building
// the window for real and running `mox import`.

type addStep int

const (
	stepName addStep = iota
	stepHosts
	stepUser
	stepSync
	stepArrange
	stepRoot
	stepCommands
	stepConfirm
)

type addAction int

const (
	addActionCancel addAction = iota
	addActionSave
	addActionSaveStart
)

// addResult is what the wizard leaves behind for the command to act on.
type addResult struct {
	action    addAction
	name      string
	sess      *config.Session
	overwrite bool
}

// arrangeChoices are the arrange-step options: the shared tmux layout list
// plus a no-rearrangement entry that maps to an unset arrange field.
var arrangeChoices = append(append([]string{}, arrangeLayouts...), "(none)")

// confirmChoices and confirmActions are parallel: the final-step menu and
// the action each entry maps to.
var (
	confirmChoices = []string{"save to config", "save + start now", "cancel"}
	confirmActions = []addAction{addActionSave, addActionSaveStart, addActionCancel}
)

type addModel struct {
	cfg      *config.Config      // existing config, for collision checks and @refs
	clusters map[string][]string // clusterssh clusters for @cluster expansion

	step  addStep
	input []rune // shared text buffer for the current text step

	name       string
	hostsRaw   string
	expanded   []string
	expandErr  string
	user       string
	sync       bool
	arrangeIdx int
	root       string
	commands   []string

	errMsg     string
	overwrite  bool // user confirmed overwriting an existing entry
	collision  bool // pending collision: next Enter confirms overwrite
	confirmIdx int
	preview    []string // rendered YAML for the confirm step

	width, height int
	embedded      bool
	focusList     bool
	listOff       int
	help          bool
	helpOff       int
	previewOff    int
	cancelConfirm bool
	cancelQuit    bool
	buffers       map[addStep][]rune
	done          addResult
	finished      bool // set exactly when done is populated and the wizard quits
}

func newAddModel(cfg *config.Config, clusters map[string][]string, prefillName string) addModel {
	if cfg == nil {
		cfg = &config.Config{Sessions: map[string]*config.Session{}}
	}
	return addModel{
		cfg:      cfg,
		clusters: clusters,
		input:    []rune(prefillName),
		width:    80,
		height:   24,
	}
}

func (m addModel) Init() tea.Cmd { return nil }

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.cancelConfirm {
			if msg.Type == tea.KeyEsc {
				m.cancelConfirm = false
			} else if string(msg.Runes) == "d" {
				m.done = addResult{action: addActionCancel}
				m.finished = true
				return m, tea.Quit
			}
			return m, nil
		}
		if msg.Type == tea.KeyCtrlC {
			return m.requestCancel(true)
		}
		if m.help {
			switch {
			case msg.Type == tea.KeyEsc || string(msg.Runes) == "?":
				m.help = false
			case msg.Type == tea.KeyDown || string(msg.Runes) == "j":
				m.helpOff = min(m.helpOff+1, max(0, len(m.helpActions())-1))
			case msg.Type == tea.KeyUp || string(msg.Runes) == "k":
				m.helpOff = max(0, m.helpOff-1)
			}
			return m, nil
		}
		if msg.Type == tea.KeyTab && !m.embedded {
			m.focusList = !m.focusList
			return m, nil
		}
		if m.focusList {
			switch {
			case msg.Type == tea.KeyEsc:
				m.focusList = false
			case msg.Type == tea.KeyDown || string(msg.Runes) == "j":
				m.listOff = min(m.listOff+1, max(0, len(m.cfg.Sessions)-1))
			case msg.Type == tea.KeyUp || string(msg.Runes) == "k":
				m.listOff = max(0, m.listOff-1)
			}
			return m, nil
		}
		if (m.step == stepSync || m.step == stepArrange || m.step == stepConfirm) && string(msg.Runes) == "?" {
			m.help, m.helpOff = true, 0
			return m, nil
		}
		switch msg.Type {
		case tea.KeyEsc:
			return m.back()
		case tea.KeyEnter:
			return m.advance()
		case tea.KeyUp, tea.KeyCtrlP:
			m.moveChoice(-1)
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN:
			m.moveChoice(1)
			return m, nil
		case tea.KeyCtrlU:
			m.setInput(nil)
			return m, nil
		case tea.KeyBackspace:
			if len(m.input) > 0 {
				m.setInput(m.input[:len(m.input)-1])
			} else if m.step == stepCommands && len(m.commands) > 0 {
				m.input = []rune(m.commands[len(m.commands)-1])
				m.commands = m.commands[:len(m.commands)-1]
			}
			return m, nil
		case tea.KeyPgUp:
			m.previewOff = max(0, m.previewOff-max(1, m.height-12))
			return m, nil
		case tea.KeyPgDown:
			m.previewOff = min(max(0, len(m.preview)-1), m.previewOff+max(1, m.height-12))
			return m, nil
		case tea.KeySpace:
			// bubbletea reports a lone space as KeySpace, not KeyRunes.
			m.setInput(append(m.input, ' '))
			return m, nil
		case tea.KeyRunes:
			if m.step == stepSync {
				return m.syncKey(msg.Runes)
			}
			if m.step == stepArrange || m.step == stepConfirm {
				return m, nil
			}
			buf := m.input
			for _, r := range msg.Runes {
				if unicode.IsPrint(r) {
					buf = append(buf, r)
				}
			}
			m.setInput(buf)
			return m, nil
		}
	}
	return m, nil
}

func (m addModel) hasDraft() bool {
	if strings.TrimSpace(string(m.input)) != "" || m.name != "" || m.hostsRaw != "" || m.user != "" || m.root != "" || len(m.commands) > 0 {
		return true
	}
	for _, input := range m.buffers {
		if strings.TrimSpace(string(input)) != "" {
			return true
		}
	}
	return false
}

func (m addModel) requestCancel(quit bool) (tea.Model, tea.Cmd) {
	m.cancelQuit = quit
	if m.hasDraft() {
		m.cancelConfirm = true
		return m, nil
	}
	m.done = addResult{action: addActionCancel}
	m.finished = true
	return m, tea.Quit
}

// setInput updates the text buffer plus anything derived from it.
func (m *addModel) setInput(buf []rune) {
	m.input = buf
	m.errMsg = ""
	m.collision = false
	if m.step == stepName {
		// An overwrite confirmation belongs to one specific name; editing
		// the name revokes it so a different session can't be clobbered.
		m.overwrite = false
	}
	if m.step == stepHosts {
		m.reexpand()
	}
}

// reexpand recomputes the live host expansion for the hosts step.
func (m *addModel) reexpand() {
	m.expandErr = ""
	fields := strings.Fields(string(m.input))
	expanded, err := expandHosts(fields, m.cfg, m.clusters)
	if err != nil {
		m.expanded = nil
		m.expandErr = err.Error()
		return
	}
	m.expanded = expanded
}

func (m *addModel) moveChoice(delta int) {
	switch m.step {
	case stepArrange:
		m.arrangeIdx = clampChoice(m.arrangeIdx+delta, len(arrangeChoices))
	case stepConfirm:
		m.confirmIdx = clampChoice(m.confirmIdx+delta, len(confirmChoices))
	case stepSync:
		m.sync = !m.sync
	}
}

func clampChoice(i, n int) int {
	if i < 0 {
		return 0
	}
	if i > n-1 {
		return n - 1
	}
	return i
}

// syncKey handles y/n on the sync toggle step.
func (m addModel) syncKey(runes []rune) (tea.Model, tea.Cmd) {
	for _, r := range runes {
		switch r {
		case 'y', 'Y':
			m.sync = true
		case 'n', 'N':
			m.sync = false
		}
	}
	return m, nil
}

// advance commits the current step and moves forward.
func (m addModel) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepName:
		name := strings.TrimSpace(string(m.input))
		// Reuse the real config rule: an otherwise-empty session validates
		// only its name.
		if err := (&config.Session{}).Validate(name); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		if _, exists := m.cfg.GetSession(name); exists && !m.overwrite {
			if !m.collision {
				m.collision = true
				m.errMsg = fmt.Sprintf("session %q already exists — Enter again to overwrite it, or edit the name", name)
				return m, nil
			}
			m.overwrite = true
		}
		m.name = name
		return m.goTo(stepHosts), nil

	case stepHosts:
		if m.expandErr != "" {
			m.errMsg = m.expandErr
			return m, nil
		}
		if err := (&config.Session{Hosts: m.expanded}).Validate("probe"); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.hostsRaw = string(m.input)
		if len(m.expanded) == 0 {
			// No hosts: ssh user, sync, and arrange have nothing to act on.
			return m.goTo(stepRoot), nil
		}
		return m.goTo(stepUser), nil

	case stepUser:
		user := strings.TrimSpace(string(m.input))
		if err := (&config.Session{Hosts: []string{"probe"}, SSHUser: user}).Validate("probe"); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.user = user
		// Seed the sync default on the forward pass only — re-entering the
		// step via Esc must not discard an explicit toggle.
		m.sync = len(m.expanded) > 1
		return m.goTo(stepSync), nil

	case stepSync:
		return m.goTo(stepArrange), nil

	case stepArrange:
		return m.goTo(stepRoot), nil

	case stepRoot:
		m.root = strings.TrimSpace(string(m.input))
		return m.goTo(stepCommands), nil

	case stepCommands:
		line := strings.TrimSpace(string(m.input))
		if line != "" {
			m.commands = append(m.commands, line)
			m.input = nil
			return m, nil
		}
		return m.goTo(stepConfirm), nil

	case stepConfirm:
		if action := confirmActions[m.confirmIdx]; action == addActionCancel {
			return m.requestCancel(false)
		} else {
			m.done = addResult{action: action, name: m.name, sess: m.buildSession(), overwrite: m.overwrite}
		}
		m.finished = true
		return m, tea.Quit
	}
	return m, nil
}

// goTo enters a step, seeding the text buffer and step defaults.
func (m addModel) goTo(step addStep) addModel {
	if m.buffers == nil {
		m.buffers = make(map[addStep][]rune)
	}
	m.buffers[m.step] = slices.Clone(m.input)
	m.step = step
	m.errMsg = ""
	m.collision = false
	switch step {
	case stepName:
		m.input = []rune(m.name)
		// Returning to the name step re-arms the collision confirmation.
		m.overwrite = false
	case stepHosts:
		m.input = []rune(m.hostsRaw)
		m.reexpand()
	case stepUser:
		m.input = []rune(m.user)
	case stepRoot:
		m.input = []rune(m.root)
	case stepCommands:
		m.input = nil
	case stepConfirm:
		m.input = nil
		m.preview = m.renderPreview()
		m.previewOff = 0
	}
	if input, ok := m.buffers[step]; ok && step != stepConfirm {
		m.input = slices.Clone(input)
		if step == stepHosts {
			m.reexpand()
		}
	}
	return m
}

// back steps backwards; on the first step it cancels.
func (m addModel) back() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepName:
		return m.requestCancel(false)
	case stepHosts:
		return m.goTo(stepName), nil
	case stepUser:
		return m.goTo(stepHosts), nil
	case stepSync:
		return m.goTo(stepUser), nil
	case stepArrange:
		return m.goTo(stepSync), nil
	case stepRoot:
		if len(m.expanded) == 0 {
			return m.goTo(stepHosts), nil
		}
		return m.goTo(stepArrange), nil
	case stepCommands:
		return m.goTo(stepRoot), nil
	case stepConfirm:
		return m.goTo(stepCommands), nil
	}
	return m, nil
}

// buildSession assembles the config.Session from the collected answers.
func (m *addModel) buildSession() *config.Session {
	s := &config.Session{
		Root:     m.root,
		Commands: slices.Clone(m.commands),
	}
	if len(m.expanded) > 0 {
		s.Hosts = slices.Clone(m.expanded)
		s.SSHUser = m.user
		s.Sync = m.sync
		if a := arrangeChoices[m.arrangeIdx]; a != "(none)" {
			s.Arrange = a
		}
	}
	return s
}

// wrapWords greedily wraps words into 2-space-indented lines of at most
// width characters.
func wrapWords(words []string, width int) []string {
	var lines []string
	cur := " "
	for _, word := range words {
		if ansi.StringWidth(cur) > 1 && ansi.StringWidth(cur)+1+ansi.StringWidth(word) > width {
			lines = append(lines, cur)
			cur = " "
		}
		cur += " " + word
	}
	if len(cur) > 1 {
		lines = append(lines, cur)
	}
	return lines
}

// renderPreview produces the YAML snippet shown on the confirm step.
func (m *addModel) renderPreview() []string {
	var b strings.Builder
	if err := printSessionYAML(&b, m.name, m.buildSession()); err != nil {
		return []string{"(preview unavailable: " + err.Error() + ")"}
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func (m addModel) View() string {
	l := terminalLayout(m.width, m.height)
	lines := []string{sessionRow(orDash(m.name), "draft", true, l.listWidth), ""}
	names := m.cfg.ListSessionNames()
	for _, name := range names[min(m.listOff, len(names)):] {
		lines = append(lines, sessionRow(name, "config", false, l.listWidth))
	}
	status := pkForeign.Render("Draft not saved")
	if m.errMsg != "" {
		status = pkErr.Render(m.errMsg)
	}
	return renderWorkspace(workspaceView{width: m.width, height: m.height, view: "add",
		context: fmt.Sprintf("%d configured sessions", len(m.cfg.Sessions)), listTitle: "sessions",
		detailTitle: m.title(), list: lines, detail: m.detailLines(l.detailWidth, l.contentHeight),
		focusDetail: !m.focusList || m.cancelConfirm || m.help, actions: m.actionHints(), status: status})
}

func (m addModel) title() string {
	return fmt.Sprintf("add session · %s · step %d/8", m.stepTitle(), m.step+1)
}

func (m addModel) detailLines(w, h int) []string {
	if m.cancelConfirm {
		return []string{pkForeign.Render("Discard this session draft?"), "", "Your input is still here. Esc keeps editing.", "Press d to discard and leave."}
	}
	if m.help {
		lines := helpLines(m.helpActions(), w)
		return lines[min(m.helpOff, len(lines)):]
	}
	if m.step == stepConfirm {
		choices := slices.Clone(confirmChoices)
		if m.embedded {
			choices = []string{"keep as draft", "review + save + start", "cancel"}
		}
		if m.overwrite {
			choices[0], choices[1] = "overwrite config", "overwrite + start"
			if m.embedded {
				choices[0], choices[1] = "keep replacement draft", "review + replace + start"
			}
		}
		var lines []string
		for i, choice := range choices {
			prefix := "  "
			if i == m.confirmIdx {
				prefix = pkAccent.Render("▸ ")
				choice = pkSelected.Render(choice)
			}
			lines = append(lines, prefix+choice)
		}
		lines = append(lines, "", pkTitle.Render("YAML review · PgUp/PgDn to scroll"))
		rows := max(1, h-len(lines))
		start := min(m.previewOff, max(0, len(m.preview)-rows))
		end := min(len(m.preview), start+rows)
		return append(lines, m.preview[start:end]...)
	}
	if m.step == stepArrange {
		lines := []string{"Pane arrangement for the window.", ""}
		rows := max(1, h-2)
		start := max(0, m.arrangeIdx-rows+1)
		for i := start; i < min(len(arrangeChoices), start+rows); i++ {
			prefix, choice := "  ", arrangeChoices[i]
			if i == m.arrangeIdx {
				prefix, choice = pkAccent.Render("▌ "), pkSelected.Render(choice)
			}
			lines = append(lines, prefix+choice)
		}
		return lines
	}
	if m.step == stepCommands {
		lines := []string{"Commands after connect, one per line."}
		start := max(0, len(m.commands)-max(0, h-3))
		for _, command := range m.commands[start:] {
			lines = append(lines, "  "+clipText(command, w-2))
		}
		return append(lines, "", pkAccent.Render("▸ ")+inputTail(string(m.input), w-2))
	}
	lines := m.stepLines(w)
	if m.errMsg != "" {
		lines = append([]string{pkErr.Render(clipText(m.errMsg, w)), ""}, lines...)
	}
	return lines
}

func (m addModel) stepTitle() string {
	switch m.step {
	case stepName:
		return "name"
	case stepHosts:
		return "hosts"
	case stepUser:
		return "ssh user"
	case stepSync:
		return "broadcast"
	case stepArrange:
		return "layout"
	case stepRoot:
		return "directory"
	case stepCommands:
		return "commands"
	case stepConfirm:
		return "review"
	}
	return ""
}

func (m addModel) actionHints() []keyHint {
	actions := m.stepHints()
	if !m.embedded && !m.cancelConfirm && !m.help && !m.focusList {
		actions = append(actions, keyHint{"Tab", "sessions"})
	}
	return actions
}

func (m addModel) stepHints() []keyHint {
	if m.cancelConfirm {
		return []keyHint{{"d", "discard"}, {"Esc", "keep editing"}}
	}
	if m.help {
		return []keyHint{{"Esc", "back"}, {"j/k", "scroll"}}
	}
	if m.focusList {
		return []keyHint{{"Tab", "add session"}, {"j/k", "scroll"}, {"Esc", "add session"}}
	}
	switch m.step {
	case stepName:
		if m.collision {
			return []keyHint{{"Enter", "overwrite"}, {"Esc", "cancel"}}
		}
		return []keyHint{{"Enter", "next"}, {"Esc", "cancel"}}
	case stepSync:
		return []keyHint{{"Enter", "next"}, {"y/n", "toggle"}, {"Esc", "back"}, {"?", "help"}}
	case stepArrange, stepConfirm:
		return []keyHint{{"Enter", "confirm"}, {"↑/↓", "choose"}, {"Esc", "back"}, {"?", "help"}}
	case stepCommands:
		actions := []keyHint{{"Enter", "add line / finish"}, {"Esc", "back"}}
		if len(m.commands) > 0 && len(m.input) == 0 {
			actions = append(actions, keyHint{"Bksp", "edit last line"})
		}
		return actions
	default:
		return []keyHint{{"Enter", "next"}, {"Esc", "back"}}
	}
}

func (m addModel) helpActions() []keyHint {
	m.help = false
	actions := m.actionHints()
	if m.step == stepConfirm {
		actions = append(actions, keyHint{"PgUp/PgDn", "scroll the YAML review"})
	}
	return append(actions, keyHint{"Ctrl+C", "cancel / discard decision"})
}

// stepLines renders the body of the current step.
func (m addModel) stepLines(w int) []string {
	prompt := pkAccent.Render("▸ ") + inputTail(string(m.input), w-2)
	switch m.step {
	case stepName:
		return []string{"Session name for the config.", "", prompt}

	case stepHosts:
		lines := wrapWords(strings.Fields("Hosts or @cluster; empty creates a local session."), w)
		lines = append(lines, "", prompt)
		switch {
		case m.expandErr != "":
			lines = append(lines, "", pkForeign.Render(truncate(m.expandErr, w)))
		case len(m.expanded) > 0:
			words := append([]string{fmt.Sprintf("%d hosts:", len(m.expanded))}, m.expanded...)
			lines = append(lines, "")
			lines = append(lines, wrapWords(words, w)...)
		}
		return lines

	case stepUser:
		return []string{"SSH user; empty uses your default.", "", prompt}

	case stepSync:
		state := pkStopped.Render("off")
		if m.sync {
			state = pkRunning.Render("on")
		}
		return []string{
			"Broadcast input to every host pane?",
			"",
			"  sync: " + state,
		}

	case stepArrange:
		lines := []string{pkDim.Render("Pane arrangement for the window."), ""}
		for i, c := range arrangeChoices {
			if i == m.arrangeIdx {
				lines = append(lines, pkAccent.Render("▌ ")+pkSelected.Render(c))
			} else {
				lines = append(lines, "  "+c)
			}
		}
		return lines

	case stepRoot:
		return []string{"Directory; empty uses tmux's default.", "", prompt}

	case stepCommands:
		lines := []string{pkDim.Render("Commands run in each pane after connect, one per line.")}
		for _, c := range m.commands {
			lines = append(lines, "  "+truncate(c, w-2))
		}
		return append(lines, "", prompt)

	case stepConfirm:
		lines := make([]string, 0, len(m.preview)+len(confirmChoices)+2)
		for _, l := range m.preview {
			lines = append(lines, pkDim.Render(truncate(l, w)))
		}
		lines = append(lines, "")
		for i, c := range confirmChoices {
			if i == m.confirmIdx {
				lines = append(lines, pkAccent.Render("▌ ")+pkSelected.Render(c))
			} else {
				lines = append(lines, "  "+c)
			}
		}
		return lines
	}
	return nil
}
