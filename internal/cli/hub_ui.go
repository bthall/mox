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
	"github.com/charmbracelet/lipgloss"
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
	col := 0
	for len(s) > 0 {
		if loc := sgrRe.FindStringIndex(s); loc != nil && loc[0] == 0 {
			b.WriteString(s[:loc[1]])
			s = s[loc[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		switch {
		case r == '\t':
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
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

// clampView hard-clips every rendered line to the terminal width as the
// last line of defense: a line the terminal renders wider than we measured
// (width disagreement on exotic glyphs) would wrap and push the footer and
// status line off-screen.
func clampView(view string, w int) string {
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "") + "\x1b[0m"
		}
	}
	return strings.Join(lines, "\n")
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

	mode hubMode

	previewName string   // session the preview belongs to
	previewBody []string // captured buffer lines
	previewWin  string   // window summary
	previewErr  string
	ticking     bool

	pending   string // non-empty while a start/kill is in flight
	status    string
	statusErr bool
	statusOK  bool // success feedback renders green

	action hubAction
	choice string

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
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			// Key repeat batches runes; replay them one at a time.
			cur := m
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				nm, cmd := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				hm, ok := nm.(hubModel)
				if !ok {
					return nm, cmd
				}
				cur = hm
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			if len(cmds) == 0 {
				return cur, nil
			}
			return cur, tea.Batch(cmds...)
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
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyUp, tea.KeyCtrlP, tea.KeyCtrlK:
		return m.move(-1)
	case tea.KeyDown, tea.KeyCtrlN, tea.KeyCtrlJ:
		return m.move(1)
	case tea.KeyEnter:
		if c, ok := m.selected(); ok && m.pending == "" {
			m.choice = c.Name
			m.action = hubAttach
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyCtrlE:
		if c, ok := m.selected(); ok && m.pending == "" {
			if _, managed := m.sessions[c.Name]; managed {
				m.choice = c.Name
				m.action = hubEdit
				return m, tea.Quit
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
		return m, tea.Quit
	}
	switch string(msg.Runes) {
	case "q":
		if m.pending != "" {
			return m, nil
		}
		return m, tea.Quit
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
		return m, tea.Quit
	}
	return m, nil
}

func (m hubModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
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
		return m, tea.Quit
	}
	if msg.Type == tea.KeyEsc {
		m.mode = hubBrowse
		return m, nil
	}
	if string(msg.Runes) == "y" || msg.Type == tea.KeyEnter {
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
	h := m.height - 2
	if h < 8 {
		h = 8
	}
	return h
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
	h := m.panelHeight()
	if m.width < 60 {
		w := m.width
		if w < 24 {
			w = 24
		}
		return clampView(panel(m.listTitle(), m.footer(), m.listLines(w-4, h-2), w, h)+"\n"+m.statusLine(), w)
	}
	leftW := m.width * 2 / 5
	if leftW < 26 {
		leftW = 26
	}
	if leftW > 40 {
		leftW = 40
	}
	rightW := m.width - leftW - 1
	left := panel(m.listTitle(), "", m.listLines(leftW-4, h-2), leftW, h)
	right := panel(m.previewTitle(), m.footer(), m.previewPane(rightW-4, h-2), rightW, h)
	return clampView(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)+"\n"+m.statusLine(), m.width)
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
	switch m.mode {
	case hubFilter:
		return hints("↵", "keep", "esc", "clear")
	case hubConfirmKill:
		return hints("y", "kill", "esc", "cancel")
	}
	return hints("↵", "attach", "S", "start", "K", "kill", "^e", "edit", "i", "import", "q", "quit")
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
		meta := ""
		if c.Running {
			meta = fmt.Sprintf("%dw %s", c.Windows, relativeShort(m.now, c.LastActivity))
		}
		name := truncate(c.Name, w-6-len(meta))
		pad := w - 4 - lipgloss.Width(name) - len(meta)
		if pad < 1 {
			pad = 1
		}
		// mox list's vocabulary: running green, unmanaged-running yellow.
		styledName := name
		switch {
		case c.Running && !c.Managed:
			styledName = pkForeign.Render(name)
		case c.Running:
			styledName = pkRunning.Render(name)
		}
		if i == m.sel {
			styledName = pkSelected.Render(styledName)
		}
		row := statusDot(c) + " " + styledName + strings.Repeat(" ", pad) + pkDim.Render(meta)
		if i == m.sel {
			lines = append(lines, pkAccent.Render("▌")+" "+row)
		} else {
			lines = append(lines, "  "+row)
		}
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
		return []string{pkDim.Render("no session matches the filter")}
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
		lines = append(lines, pkBorder.Render(strings.Repeat("─", w)))
	}
	if m.previewErr != "" {
		lines = append(lines, m.summaryLines(c, w)...)
		lines = append(lines, "", pkDim.Render(truncate("preview unavailable: "+m.previewErr, w)))
		return lines
	}
	if m.previewName != c.Name {
		lines = append(lines, pkDim.Render("  …"))
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
			return " " + pkErr.Render("kill "+c.Name+"? ") + pkDim.Render("y / esc")
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
	return " " + truncate(strings.Join(parts, pkDim.Render(" · ")), m.width-2)
}
