package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

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

	width int
	done  addResult
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
	}
}

func (m addModel) Init() tea.Cmd { return nil }

func (m addModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.done = addResult{action: addActionCancel}
			return m, tea.Quit
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
			}
			return m, nil
		case tea.KeySpace:
			// bubbletea reports a lone space as KeySpace, not KeyRunes.
			m.setInput(append(m.input, ' '))
			return m, nil
		case tea.KeyRunes:
			if m.step == stepSync {
				return m.syncKey(msg.Runes)
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
			m.done = addResult{action: addActionCancel}
		} else {
			m.done = addResult{action: action, name: m.name, sess: m.buildSession(), overwrite: m.overwrite}
		}
		return m, tea.Quit
	}
	return m, nil
}

// goTo enters a step, seeding the text buffer and step defaults.
func (m addModel) goTo(step addStep) addModel {
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
	}
	return m
}

// back steps backwards; on the first step it cancels.
func (m addModel) back() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepName:
		m.done = addResult{action: addActionCancel}
		return m, tea.Quit
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
		// Backing out of commands drops the collected lines so re-entry
		// starts clean rather than appending duplicates.
		m.commands = nil
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
		if len(cur) > 1 && len(cur)+1+len(word) > width {
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
	w := m.width
	if w > 64 {
		w = 64
	}
	if w < 32 {
		w = 32
	}

	lines := m.stepLines(w - 4)
	if m.errMsg != "" {
		lines = append(lines, "", pkForeign.Render(truncate(m.errMsg, w-4)))
	}
	title := fmt.Sprintf("add session · %s", m.stepTitle())
	return panel(title, m.stepFooter(), lines, w, len(lines)+2) + "\n"
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

func (m addModel) stepFooter() string {
	switch m.step {
	case stepName:
		return "↵ next · esc quit"
	case stepSync:
		return "y/n toggle · ↵ next · esc back"
	case stepArrange, stepConfirm:
		return "↑↓ choose · ↵ confirm · esc back"
	case stepCommands:
		return "↵ add line · empty ↵ done · esc back"
	default:
		return "↵ next · esc back"
	}
}

// stepLines renders the body of the current step.
func (m addModel) stepLines(w int) []string {
	prompt := pkAccent.Render("▸ ") + string(m.input) + pkAccent.Render("█")
	switch m.step {
	case stepName:
		return []string{pkDim.Render("Session name for the config."), "", prompt}

	case stepHosts:
		lines := []string{
			pkDim.Render("Hosts, space separated. @cluster expands; empty = local session."),
			"",
			prompt,
		}
		switch {
		case m.expandErr != "":
			lines = append(lines, "", pkForeign.Render(truncate(m.expandErr, w)))
		case len(m.expanded) > 0:
			lines = append(lines, "", pkDim.Render(fmt.Sprintf("%d host(s):", len(m.expanded))))
			lines = append(lines, wrapWords(m.expanded, w)...)
		}
		return lines

	case stepUser:
		return []string{pkDim.Render("SSH as this user (empty for your default)."), "", prompt}

	case stepSync:
		state := pkStopped.Render("off")
		if m.sync {
			state = pkRunning.Render("on")
		}
		return []string{
			pkDim.Render("Broadcast typed input to every pane (synchronize-panes)?"),
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
		return []string{pkDim.Render("Working directory for panes (empty to skip)."), "", prompt}

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
