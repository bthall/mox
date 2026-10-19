package cli

// The session hub: bare `mox` on a terminal. Full-screen, sessions on the
// left, a preview of the highlighted session on the right — a live buffer
// capture for running sessions, the config summary for stopped ones. Enter
// attaches; S starts a configured session detached; K kills after a
// one-line confirm. ctrl+e hands off to the config editor; i hands off to
// the import flow for a running session that isn't in the config.

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

// hubAction is what the hub asks runPicker to do after it exits.
type hubAction int

const (
	hubQuit hubAction = iota
	hubAttach
	hubEdit
	hubImport
)

type hubMode int

const (
	hubBrowse hubMode = iota
	hubFilter
	hubConfirmKill
)

// hubManager is the slice of session.Manager the hub drives. Faked in tests.
type hubManager interface {
	Kill(name string) error
	Create(ctx context.Context, name string, force bool) error
	List() ([]session.SessionInfo, error)
}

// hubOrder re-sorts a refreshed listing the same way the initial candidate
// list was sorted (running by activity, stopped by recency).
type hubOrder func([]session.SessionInfo) []session.SessionInfo

// hubTickMsg drives the preview refresh cadence.
type hubTickMsg struct{}

// hubPreviewMsg carries one capture result. name guards against staleness:
// the result is dropped unless that session is still highlighted.
type hubPreviewMsg struct {
	name    string
	body    string
	windows string
	err     error
}

// hubActionMsg reports a finished start/kill plus the refreshed listing.
type hubActionMsg struct {
	verb    string // "started" / "killed"
	name    string
	err     error
	infos   []session.SessionInfo
	listErr error
}

// hubTickInterval is a var so tests can shrink it; the UI treats it as
// constant.
var hubTickInterval = time.Second

// sgrRe matches the SGR color/attribute sequences capture-pane -e emits.
var sgrRe = regexp.MustCompile("\x1b\\[[0-9;:]*m")

// sanitizeCaptureLine makes a captured buffer line safe to lay out: tabs
// expand to 8-column stops (capture-pane emits them literally, and a tab
// counted as one cell but rendered as eight wraps the line and pushes the
// panel footer off-screen), SGR sequences pass through, and every other
// control character is dropped.
func sanitizeCaptureLine(s string) string {
	var b strings.Builder
	// Optimization: preallocate builder memory based on string length to minimize reallocations.
	b.Grow(len(s))
	col := 0
	for len(s) > 0 {
		// Optimization: fast-path check for ANSI ESC sequence prefix '\x1b[' before running regex search.
		if strings.HasPrefix(s, "\x1b[") {
			if loc := sgrRe.FindStringIndex(s); loc != nil && loc[0] == 0 {
				b.WriteString(s[:loc[1]])
				s = s[loc[1]:]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == '\t':
			n := 8 - col%8
			// Optimization: avoid strings.Repeat heap allocation by slicing a constant padding string.
			b.WriteString("        "[:n])
			col += n
		case r < 0x20 || r == 0x7f: // stray ESC / CR / BS etc.
			// dropped
		default:
			b.WriteRune(r)
			col += ansi.StringWidth(string(r))
		}
		s = s[size:]
	}
	return b.String()
}

type hubModel struct {
	ctx      context.Context
	mgr      hubManager
	order    hubOrder
	capture  func(target string) (string, error) // active pane buffer
	windows  func(target string) (string, error) // window summary line
	sessions map[string]*config.Session
	now      time.Time

	candidates []session.SessionInfo
	filter     []rune
	visible    []int // indices into candidates
	sel        int   // index into visible
	offset     int

	mode        hubMode
	focusDetail bool
	help        bool
	helpOff     int

	previewName string   // session the preview belongs to
	previewBody []string // captured buffer lines
	previewWin  string   // window summary
	previewErr  string
	ticking     bool

	pending   string // non-empty while a start/kill is in flight
	status    string
	statusErr bool
	statusOK  bool // success feedback renders green

	action  hubAction
	choice  string
	leaving bool // set once the hub has asked to quit

	width, height int
}

func newHubModel(ctx context.Context, mgr hubManager, order hubOrder, capture, windows func(string) (string, error), candidates []session.SessionInfo, sessions map[string]*config.Session, now time.Time) hubModel {
	if order == nil {
		order = func(infos []session.SessionInfo) []session.SessionInfo { return infos }
	}
	m := hubModel{
		ctx:      ctx,
		mgr:      mgr,
		order:    order,
		capture:  capture,
		windows:  windows,
		sessions: sessions,
		now:      now,
		width:    100,
		height:   30,
	}
	m.candidates = candidates
	m.refilter()
	// Init starts the tick loop for a running initial highlight; record it
	// here so onHighlightChange never starts a second loop.
	if c, ok := m.selected(); ok && c.Running {
		m.ticking = true
	}
	return m
}

// Init kicks off the preview cycle when the first highlight is running.
func (m hubModel) Init() tea.Cmd {
	if c, ok := m.selected(); ok && c.Running {
		return tea.Batch(m.captureCmd(c.Name), m.tickCmd())
	}
	return nil
}

func (m *hubModel) refilter() {
	if len(m.filter) == 0 {
		m.visible = make([]int, len(m.candidates))
		for i := range m.candidates {
			m.visible[i] = i
		}
	} else {
		names := make([]string, len(m.candidates))
		for i, c := range m.candidates {
			names[i] = c.Name
		}
		matches := fuzzy.Find(string(m.filter), names)
		m.visible = make([]int, len(matches))
		for i, match := range matches {
			m.visible[i] = match.Index
		}
	}
	if m.sel > len(m.visible)-1 {
		m.sel = len(m.visible) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.offset > m.sel {
		m.offset = m.sel
	}
	m.keepVisible()
}

func (m hubModel) selected() (session.SessionInfo, bool) {
	if len(m.visible) == 0 {
		return session.SessionInfo{}, false
	}
	return m.candidates[m.visible[m.sel]], true
}

func (m hubModel) selectedName() string {
	c, ok := m.selected()
	if !ok {
		return ""
	}
	return c.Name
}

// captureCmd fetches the highlighted session's buffer off the UI thread.
func (m hubModel) captureCmd(name string) tea.Cmd {
	capture, windows := m.capture, m.windows
	return func() tea.Msg {
		body, err := capture(name)
		win := ""
		if err == nil && windows != nil {
			win, _ = windows(name) // summary is best-effort
		}
		return hubPreviewMsg{name: name, body: body, windows: win, err: err}
	}
}

func (m hubModel) tickCmd() tea.Cmd {
	return tea.Tick(hubTickInterval, func(time.Time) tea.Msg { return hubTickMsg{} })
}

// actionCmd runs a start or kill plus the follow-up listing refresh.
func (m hubModel) actionCmd(verb, name string) tea.Cmd {
	mgr, ctx := m.mgr, m.ctx
	return func() tea.Msg {
		var err error
		switch verb {
		case "started":
			err = mgr.Create(ctx, name, false)
		case "killed":
			err = mgr.Kill(name)
		}
		infos, listErr := mgr.List()
		return hubActionMsg{verb: verb, name: name, err: err, infos: infos, listErr: listErr}
	}
}

// onHighlightChange resets the preview and starts the cycle for a running
// session. Returns a capture command when one should fire immediately.
func (m *hubModel) onHighlightChange() tea.Cmd {
	m.previewName = ""
	m.previewBody = nil
	m.previewWin = ""
	m.previewErr = ""
	c, ok := m.selected()
	if !ok || !c.Running {
		return nil
	}
	cmds := []tea.Cmd{m.captureCmd(c.Name)}
	if !m.ticking {
		m.ticking = true
		cmds = append(cmds, m.tickCmd())
	}
	return tea.Batch(cmds...)
}

func (m hubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.keepVisible()
		return m, nil

	case hubTickMsg:
		c, ok := m.selected()
		if !ok || !c.Running {
			m.ticking = false // cycle stops; restarted on next highlight
			return m, nil
		}
		return m, tea.Batch(m.captureCmd(c.Name), m.tickCmd())

	case hubPreviewMsg:
		if msg.name != m.selectedName() {
			return m, nil // stale: highlight moved on
		}
		m.previewName = msg.name
		if msg.err != nil {
			m.previewBody = nil
			m.previewErr = msg.err.Error()
			return m, nil
		}
		m.previewErr = ""
		m.previewWin = msg.windows
		raw := strings.Split(strings.TrimRight(msg.body, "\n"), "\n")
		body := make([]string, len(raw))
		for i, l := range raw {
			body[i] = sanitizeCaptureLine(l)
		}
		m.previewBody = body
		return m, nil

	case hubActionMsg:
		m.pending = ""
		keep := msg.name
		if msg.listErr == nil {
			m.candidates = m.order(msg.infos)
		}
		m.refilter()
		for i, ci := range m.visible {
			if m.candidates[ci].Name == keep {
				m.sel = i
				break
			}
		}
		m.keepVisible()
		cmd := m.onHighlightChange()
		if msg.err != nil {
			verb := "start"
			if msg.verb == "killed" {
				verb = "kill"
			}
			m.status = verb + " " + msg.name + ": " + msg.err.Error()
			m.statusErr = true
			m.statusOK = false
			return m, cmd
		}
		m.status = msg.verb + " " + msg.name + " ✓"
		m.statusErr = false
		m.statusOK = true
		if msg.listErr != nil {
			m.status += " · list refresh failed: " + msg.listErr.Error()
			m.statusErr = true
			m.statusOK = false
		}
		return m, cmd

	case tea.KeyMsg:
		if m.help {
			switch {
			case msg.Type == tea.KeyCtrlC:
				return m.quit()
			case msg.Type == tea.KeyEsc || string(msg.Runes) == "?":
				m.help = false
			case msg.Type == tea.KeyDown || string(msg.Runes) == "j":
				m.helpOff = min(m.helpOff+1, max(0, len(m.helpActions())-1))
			case msg.Type == tea.KeyUp || string(msg.Runes) == "k":
				m.helpOff = max(0, m.helpOff-1)
			}
			return m, nil
		}
		if m.mode != hubFilter && string(msg.Runes) == "?" {
			m.help, m.helpOff = true, 0
			return m, nil
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			return replayRunes(m, msg.Runes)
		}
		switch m.mode {
		case hubBrowse:
			return m.updateBrowse(msg)
		case hubFilter:
			return m.updateFilter(msg)
		case hubConfirmKill:
			return m.updateConfirmKill(msg)
		}
	}
	return m, nil
}

func (m hubModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		m.focusDetail = !m.focusDetail
		return m, nil
	case tea.KeyCtrlC:
		return m.quit()
	case tea.KeyUp, tea.KeyCtrlP, tea.KeyCtrlK:
		return m.move(-1)
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyCtrlJ:
		return m.move(1)
	case tea.KeyEnter:
		if c, ok := m.selected(); ok && m.pending == "" {
			m.choice = c.Name
			m.action = hubAttach
			return m.quit()
		}
		return m, nil
	case tea.KeyCtrlE:
		if c, ok := m.selected(); ok && m.pending == "" {
			if _, managed := m.sessions[c.Name]; managed {
				m.choice = c.Name
				m.action = hubEdit
				return m.quit()
			}
			// Same feedback as S: a silent no-op reads as a dead key.
			m.status = c.Name + " is not in the config"
			m.statusErr = false
			m.statusOK = false
		}
		return m, nil
	case tea.KeyEsc:
		if len(m.filter) > 0 {
			m.filter = nil
			m.refilter()
			return m, m.onHighlightChange()
		}
		if m.pending != "" {
			return m, nil // let the in-flight action finish (ctrl+c overrides)
		}
		return m.quit()
	}
	switch string(msg.Runes) {
	case "q":
		if m.pending != "" {
			return m, nil
		}
		return m.quit()
	case "j":
		return m.move(1)
	case "k":
		return m.move(-1)
	case "/":
		m.mode = hubFilter
		return m, nil
	case "S":
		c, ok := m.selected()
		if !ok || m.pending != "" {
			return m, nil
		}
		if c.Running {
			m.status = c.Name + " is already running"
			m.statusErr = false
			return m, nil
		}
		if _, managed := m.sessions[c.Name]; !managed {
			m.status = c.Name + " is not in the config"
			m.statusErr = false
			return m, nil
		}
		m.pending = "starting " + c.Name + "…"
		m.status = ""
		return m, m.actionCmd("started", c.Name)
	case "K":
		c, ok := m.selected()
		if !ok || m.pending != "" {
			return m, nil
		}
		if !c.Running {
			m.status = c.Name + " is not running"
			m.statusErr = false
			return m, nil
		}
		m.mode = hubConfirmKill
		return m, nil
	case "i":
		c, ok := m.selected()
		if !ok || m.pending != "" {
			return m, nil
		}
		if _, managed := m.sessions[c.Name]; managed {
			m.status = c.Name + " is already in the config"
			m.statusErr = false
			return m, nil
		}
		if !c.Running {
			m.status = c.Name + " is not running"
			m.statusErr = false
			return m, nil
		}
		m.choice = c.Name
		m.action = hubImport
		return m.quit()
	}
	return m, nil
}

func (m hubModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.quit()
	case tea.KeyEsc:
		m.filter = nil
		m.mode = hubBrowse
		m.refilter()
		return m, m.onHighlightChange()
	case tea.KeyEnter:
		m.mode = hubBrowse
		return m, nil
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.refilter()
			return m, m.onHighlightChange()
		}
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				m.filter = append(m.filter, r)
			}
		}
		m.refilter()
		return m, m.onHighlightChange()
	}
	return m, nil
}

func (m hubModel) updateConfirmKill(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}
	if msg.Type == tea.KeyEsc || string(msg.Runes) == "n" || string(msg.Runes) == "N" {
		m.mode = hubBrowse
		return m, nil
	}
	if string(msg.Runes) == "y" || string(msg.Runes) == "Y" || msg.Type == tea.KeyEnter {
		c, ok := m.selected()
		m.mode = hubBrowse
		if !ok {
			return m, nil
		}
		m.pending = "killing " + c.Name + "…"
		m.status = ""
		return m, m.actionCmd("killed", c.Name)
	}
	return m, nil
}

// quit ends the hub, marking it so a batched rune replay stops here.
func (m hubModel) quit() (tea.Model, tea.Cmd) {
	m.leaving = true
	return m, tea.Quit
}

func (m hubModel) isLeaving() bool { return m.leaving }

// move shifts the selection and restarts the preview cycle.
func (m hubModel) move(delta int) (tea.Model, tea.Cmd) {
	if len(m.visible) == 0 {
		return m, nil
	}
	next := clampChoice(m.sel+delta, len(m.visible))
	if next == m.sel {
		return m, nil
	}
	m.sel = next
	m.keepVisible()
	m.status = ""
	m.statusErr = false
	return m, m.onHighlightChange()
}

func (m *hubModel) keepVisible() {
	rows := m.effectiveListRows()
	if m.sel < m.offset {
		m.offset = m.sel
	}
	if m.sel >= m.offset+rows {
		m.offset = m.sel - rows + 1
	}
}

func (m hubModel) panelHeight() int {
	return terminalLayout(m.width, m.height).contentHeight + 2
}

func (m hubModel) listRows() int {
	h := m.panelHeight() - 4
	if h < 1 {
		h = 1
	}
	return h
}

// effectiveListRows reserves the overflow-indicator line, same contract as
// the editor's.
func (m hubModel) effectiveListRows() int {
	rows := m.listRows()
	if len(m.visible) > rows {
		rows--
		if rows < 1 {
			rows = 1
		}
	}
	return rows
}

// --- view ---

func (m hubModel) View() string {
	l := terminalLayout(m.width, m.height)
	detail, title := m.previewPane(l.detailWidth, l.contentHeight), m.previewTitle()
	focus := m.focusDetail || m.mode == hubConfirmKill
	if m.mode == hubFilter {
		focus = false
	}
	if m.help {
		detail = helpLines(m.helpActions(), l.detailWidth)
		detail = detail[min(m.helpOff, len(detail)):]
		title, focus = "help / sessions", true
	}
	running := 0
	for _, c := range m.candidates {
		if c.Running {
			running++
		}
	}
	return renderWorkspace(workspaceView{width: m.width, height: m.height, view: "sessions",
		context:   fmt.Sprintf("%d running · %d sessions", running, len(m.candidates)),
		listTitle: m.listTitle(), detailTitle: title, list: m.listLines(l.listWidth, l.contentHeight),
		detail: detail, focusDetail: focus, actions: m.actionHints(), status: m.statusLine()})
}

func (m hubModel) listTitle() string {
	if len(m.filter) > 0 || m.mode == hubFilter {
		return fmt.Sprintf("sessions · %d/%d", len(m.visible), len(m.candidates))
	}
	return fmt.Sprintf("sessions · %d", len(m.candidates))
}

func (m hubModel) previewTitle() string {
	c, ok := m.selected()
	if !ok {
		return "mox"
	}
	if c.Running {
		if !c.Managed {
			return fmt.Sprintf("%s · tmux only · running · %s", c.Name, pluralize(c.Windows, "window"))
		}
		return fmt.Sprintf("%s · running · %s", c.Name, pluralize(c.Windows, "window"))
	}
	return c.Name + " · stopped"
}

func (m hubModel) footer() string {
	return renderHints(m.actionHints(), 1000)
}

func (m hubModel) actionHints() []keyHint {
	if m.help {
		return []keyHint{{"Esc", "back"}, {"j/k", "scroll"}}
	}
	switch m.mode {
	case hubFilter:
		return []keyHint{{"Enter", "keep filter"}, {"Esc", "clear"}}
	case hubConfirmKill:
		return []keyHint{{"Enter/y", "kill"}, {"Esc", "cancel"}, {"?", "help"}}
	}
	var actions []keyHint
	if _, ok := m.selected(); ok && m.pending == "" {
		actions = append(actions, keyHint{"Enter", "attach"})
	}
	label := "preview"
	if m.focusDetail {
		label = "sessions"
	}
	actions = append(actions, keyHint{"Tab", label})
	if c, ok := m.selected(); ok && m.pending == "" {
		if c.Running {
			actions = append(actions, keyHint{"K", "kill"})
		} else if c.Managed {
			actions = append(actions, keyHint{"S", "start"})
		}
		if c.Managed {
			actions = append(actions, keyHint{"^e", "edit"})
		} else if c.Running {
			actions = append(actions, keyHint{"i", "import"})
		}
	}
	actions = append(actions, keyHint{"/", "filter"})
	if m.pending == "" {
		actions = append(actions, keyHint{"q", "quit"})
	}
	return append(actions, keyHint{"?", "help"})
}

func (m hubModel) helpActions() []keyHint {
	actions := []keyHint{{"j/k, ↑/↓", "select a session"}}
	m.help = false
	actions = append(actions, m.actionHints()...)
	return actions
}

func (m hubModel) listLines(w, h int) []string {
	header := pkDim.Render("/ filter")
	if m.mode == hubFilter || len(m.filter) > 0 {
		header = pkAccent.Render("▸ ") + string(m.filter) + pkAccent.Render("█")
	}
	lines := []string{header, ""}
	if len(m.visible) == 0 {
		return append(lines, pkDim.Render("  (no match)"))
	}
	rows := m.effectiveListRows()
	end := m.offset + rows
	if end > len(m.visible) {
		end = len(m.visible)
	}
	for i := m.offset; i < end; i++ {
		c := m.candidates[m.visible[i]]
		state := "stopped"
		if c.Running {
			state = "running"
			if !c.Managed {
				state = "tmux only"
			}
		}
		lines = append(lines, sessionRow(c.Name, state, i == m.sel, w))
	}
	if end < len(m.visible) {
		lines = append(lines, pkDim.Render(fmt.Sprintf("  … %d more", len(m.visible)-end)))
	}
	return lines
}

// previewPane renders the right side: live buffer for running sessions,
// config summary for stopped ones.
func (m hubModel) previewPane(w, h int) []string {
	c, ok := m.selected()
	if !ok {
		return []string{"No session matches the filter.", "Esc clears it; / changes the search."}
	}
	if m.mode == hubConfirmKill {
		return []string{pkErr.Render("Kill " + c.Name + "?"), "", "The running session and its panes will close.", "Enter/y kills it. Esc keeps it running."}
	}
	if m.pending != "" {
		return []string{"", pkForeign.Render("  " + m.pending)}
	}
	if !c.Running {
		lines := m.summaryLines(c, w)
		if _, managed := m.sessions[c.Name]; managed {
			lines = append(lines, "", pkDim.Render("S starts it detached"))
		}
		return lines
	}
	var lines []string
	if m.previewWin != "" {
		lines = append(lines, pkDim.Render(truncate("win: "+m.previewWin, w)))
		lines = append(lines, pkBorder.Render(strings.Repeat("─", max(0, w))))
	}
	if m.previewErr != "" {
		lines = append(lines, pkForeign.Render("preview unavailable"), "Enter still attaches. Capture retries automatically.")
		lines = append(lines, wrapWords(strings.Fields(m.previewErr), w)...)
		lines = append(lines, "")
		lines = append(lines, m.summaryLines(c, w)...)
		return lines
	}
	if m.previewName != c.Name {
		lines = append(lines, pkDim.Render("Loading preview…"))
		return lines
	}
	// Tail-clip the buffer to the rows left under the header, dropping
	// trailing blank lines first so the tail is content, not padding.
	body := m.previewBody
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	rows := h - len(lines) - 1 // one line reserved for the live badge
	if rows < 1 {
		rows = 1
	}
	if len(body) > rows {
		body = body[len(body)-rows:]
	}
	for _, l := range body {
		// The session's own colors, clipped without breaking escape
		// sequences and reset-terminated so nothing bleeds into borders.
		lines = append(lines, ansi.Truncate(l, w, "")+"\x1b[0m")
	}
	lines = append(lines, pkOK.Render(fmt.Sprintf("live · %ds", int(hubTickInterval.Seconds()))))
	return lines
}

// summaryLines is the stopped-session config summary (state, hosts,
// connect template, …), carried over from the retired inline picker.
func (m hubModel) summaryLines(c session.SessionInfo, w int) []string {
	kv := func(k, v string) string {
		return pkDim.Render(fmt.Sprintf("%-8s", k)) + " " + v
	}
	var lines []string

	state := pkStopped.Render("stopped")
	if c.Running {
		state = pkRunning.Render("running")
		if c.Attached {
			state += " · attached"
		}
		state += pkDim.Render(" · " + relativeShort(m.now, c.LastActivity))
	}
	lines = append(lines, kv("state", state))

	origin := "mox config"
	if !c.Managed {
		origin = "tmux only"
	}
	lines = append(lines, kv("origin", origin))

	if len(c.Hosts) > 0 {
		lines = append(lines, wrapKV("hosts", c.Hosts, w, kv)...)
	}

	if sess := m.sessions[c.Name]; sess != nil {
		connect := sess.Connect
		if connect == "" && sess.SSHUser != "" {
			connect = "ssh " + sess.SSHUser + "@{{host}}"
		}
		if connect == "" && len(c.Hosts) > 0 {
			connect = "ssh {{host}}"
		}
		if connect != "" {
			lines = append(lines, kv("connect", truncate(connect, w-10)))
		}
		if sess.Sync {
			lines = append(lines, kv("sync", "on (broadcast typing)"))
		}
		if sess.Arrange != "" {
			lines = append(lines, kv("arrange", sess.Arrange))
		}
		if sess.Root != "" {
			lines = append(lines, kv("root", truncate(sess.Root, w-10)))
		}
		if len(sess.Windows) > 0 {
			lines = append(lines, kv("layout", fmt.Sprintf("%d configured windows", len(sess.Windows))))
		}
	}
	return lines
}

func (m hubModel) statusLine() string {
	if m.mode == hubConfirmKill {
		if c, ok := m.selected(); ok {
			return pkErr.Render("kill "+c.Name+"? ") + pkDim.Render("Enter/y / Esc")
		}
	}
	var parts []string
	if m.pending != "" {
		parts = append(parts, pkForeign.Render(m.pending))
	}
	if m.status != "" {
		style := pkDim
		switch {
		case m.statusErr:
			style = pkErr
		case m.statusOK:
			style = pkOK
		}
		parts = append(parts, style.Render(m.status))
	}
	if len(parts) == 0 {
		if m.previewErr != "" {
			return pkForeign.Render("Preview unavailable · Enter still attaches")
		}
		return "Ready"
	}
	return strings.Join(parts, pkDim.Render(" · "))
}
