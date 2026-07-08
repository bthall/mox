package cli

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

// The picker draws two bordered panes: a filterable session list on the
// left and a detail preview of the highlighted session on the right. All
// colors come from the terminal's 16-color palette so the user's theme
// decides how it looks.
var (
	pkBorder   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkTitle    = lipgloss.NewStyle().Bold(true)
	pkDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	pkSelected = lipgloss.NewStyle().Bold(true)
	pkRunning  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	pkStopped  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkForeign  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

// pickerModel is the Bubble Tea model for the two-pane picker.
type pickerModel struct {
	candidates []session.SessionInfo
	sessions   map[string]*config.Session // config bodies for preview detail
	now        time.Time

	query    []rune
	filtered []int // indices into candidates, ranked
	selected int   // index into filtered
	offset   int   // list scroll offset

	width  int
	height int
	choice string
}

func newPickerModel(candidates []session.SessionInfo, sessions map[string]*config.Session, now time.Time) pickerModel {
	m := pickerModel{
		candidates: candidates,
		sessions:   sessions,
		now:        now,
		width:      80,
		height:     24,
	}
	m.refilter()
	return m
}

// refilter recomputes the ranked candidate view for the current query.
func (m *pickerModel) refilter() {
	if len(m.query) == 0 {
		m.filtered = make([]int, len(m.candidates))
		for i := range m.candidates {
			m.filtered[i] = i
		}
	} else {
		names := make([]string, len(m.candidates))
		for i, c := range m.candidates {
			names[i] = c.Name
		}
		matches := fuzzy.Find(string(m.query), names)
		m.filtered = make([]int, len(matches))
		for i, match := range matches {
			m.filtered[i] = match.Index
		}
	}
	m.selected = 0
	m.offset = 0
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.choice = m.candidates[m.filtered[m.selected]].Name
			}
			return m, tea.Quit
		case tea.KeyEsc:
			if len(m.query) > 0 {
				m.query = nil
				m.refilter()
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyUp, tea.KeyCtrlP, tea.KeyCtrlK:
			m.move(-1)
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN, tea.KeyCtrlJ:
			m.move(1)
			return m, nil
		case tea.KeyCtrlU:
			m.query = nil
			m.refilter()
			return m, nil
		case tea.KeyBackspace:
			if len(m.query) > 0 {
				m.query = m.query[:len(m.query)-1]
				m.refilter()
			}
			return m, nil
		case tea.KeyRunes:
			for _, r := range msg.Runes {
				if unicode.IsPrint(r) {
					m.query = append(m.query, r)
				}
			}
			m.refilter()
			return m, nil
		}
	}
	return m, nil
}

// move shifts the selection by delta, clamping (no wrap: with a preview
// pane, wrapping reads as a glitch) and keeps it scrolled into view.
func (m *pickerModel) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected > len(m.filtered)-1 {
		m.selected = len(m.filtered) - 1
	}
	rows := m.listRows()
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+rows {
		m.offset = m.selected - rows + 1
	}
}

// listRows is how many session rows fit in the left pane.
func (m pickerModel) listRows() int {
	// panel height minus: borders (2), search line (1), blank line (1).
	h := m.panelHeight() - 4
	if h < 1 {
		h = 1
	}
	return h
}

// panelHeight sizes both panes: enough for the list plus chrome, capped by
// the terminal and an absolute ceiling so the inline UI stays compact.
func (m pickerModel) panelHeight() int {
	want := len(m.candidates) + 4
	if want > 16 {
		want = 16
	}
	if m.height > 0 && want > m.height-1 {
		want = m.height - 1
	}
	if want < 6 {
		want = 6
	}
	return want
}

func (m pickerModel) View() string {
	if m.width < 40 {
		return m.viewNarrow()
	}
	leftW := m.width * 2 / 5
	if leftW < 26 {
		leftW = 26
	}
	if leftW > 44 {
		leftW = 44
	}
	rightW := m.width - leftW - 1
	if rightW > 56 {
		rightW = 56
	}
	h := m.panelHeight()

	left := panel(m.listTitle(), "", m.listLines(leftW-4, h-2), leftW, h)
	right := panel(m.previewTitle(), "↵ attach · esc quit", m.previewLines(rightW-4), rightW, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right) + "\n"
}

// viewNarrow drops the preview pane when the terminal is too tight for two.
func (m pickerModel) viewNarrow() string {
	w := m.width
	if w < 20 {
		w = 20
	}
	h := m.panelHeight()
	return panel(m.listTitle(), "↵ attach · esc quit", m.listLines(w-4, h-2), w, h) + "\n"
}

func (m pickerModel) listTitle() string {
	if len(m.query) > 0 {
		return fmt.Sprintf("sessions · %d/%d", len(m.filtered), len(m.candidates))
	}
	return "sessions"
}

// listLines renders the search line and the visible slice of session rows.
func (m pickerModel) listLines(w, h int) []string {
	lines := []string{
		pkAccent.Render("▸ ") + string(m.query) + pkAccent.Render("█"),
		"",
	}
	if len(m.filtered) == 0 {
		lines = append(lines, pkDim.Render("  (no match)"))
		return lines
	}
	rows := h - 2
	if rows < 1 {
		rows = 1
	}
	end := m.offset + rows
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	for i := m.offset; i < end; i++ {
		c := m.candidates[m.filtered[i]]
		dot := statusDot(c)
		name := truncate(c.Name, w-4)
		if i == m.selected {
			lines = append(lines, pkAccent.Render("▌")+" "+dot+" "+pkSelected.Render(name))
		} else {
			lines = append(lines, "  "+dot+" "+name)
		}
	}
	if end < len(m.filtered) {
		lines = append(lines, pkDim.Render(fmt.Sprintf("  … %d more", len(m.filtered)-end)))
	}
	return lines
}

func statusDot(c session.SessionInfo) string {
	switch {
	case c.Running && !c.Managed:
		return pkForeign.Render("●")
	case c.Running:
		return pkRunning.Render("●")
	default:
		return pkStopped.Render("○")
	}
}

func (m pickerModel) previewTitle() string {
	if len(m.filtered) == 0 {
		return "mox"
	}
	return m.candidates[m.filtered[m.selected]].Name
}

// previewLines renders the detail key/value block for the highlighted
// session: live state first, then whatever the config knows about it.
func (m pickerModel) previewLines(w int) []string {
	if len(m.filtered) == 0 {
		return []string{pkDim.Render("no session matches the filter")}
	}
	c := m.candidates[m.filtered[m.selected]]

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

	if c.Running && c.Windows > 0 {
		lines = append(lines, kv("windows", fmt.Sprintf("%d", c.Windows)))
	}

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

// wrapKV wraps a word list under a single key label, indenting continuation
// lines to the value column.
func wrapKV(key string, words []string, w int, kv func(k, v string) string) []string {
	avail := w - 10
	if avail < 8 {
		avail = 8
	}
	var lines []string
	cur := ""
	flush := func() {
		if cur == "" {
			return
		}
		if len(lines) == 0 {
			lines = append(lines, kv(key, cur))
		} else {
			lines = append(lines, strings.Repeat(" ", 9)+cur)
		}
		cur = ""
	}
	for _, word := range words {
		if cur != "" && len(cur)+1+len(word) > avail {
			flush()
		}
		if cur != "" {
			cur += " "
		}
		cur += word
	}
	flush()
	return lines
}

// panel draws a rounded-border box with a title in the top border and an
// optional footer in the bottom border. Content lines are clipped and padded
// to the inner width; the pane is padded to the requested height.
func panel(title, footer string, content []string, w, h int) string {
	if w < 8 {
		w = 8
	}
	inner := w - 4 // "│ " + " │"

	top := "╭─ " + pkTitle.Render(title) + " "
	fill := w - lipgloss.Width(top) - 1
	if fill < 0 {
		fill = 0
	}
	topLine := pkBorder.Render("╭─ ") + pkTitle.Render(title) + pkBorder.Render(" "+strings.Repeat("─", fill)+"╮")

	var bottomLine string
	if footer != "" {
		fill := w - lipgloss.Width("╰─ "+footer+" ") - 1
		if fill < 0 {
			fill = 0
		}
		bottomLine = pkBorder.Render("╰─ ") + pkDim.Render(footer) + pkBorder.Render(" "+strings.Repeat("─", fill)+"╯")
	} else {
		bottomLine = pkBorder.Render("╰" + strings.Repeat("─", w-2) + "╯")
	}

	rows := make([]string, 0, h)
	rows = append(rows, topLine)
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		pad := inner - lipgloss.Width(line)
		if pad < 0 {
			line = truncate(line, inner) // best-effort; plain-text lines only
			pad = inner - lipgloss.Width(line)
			if pad < 0 {
				pad = 0
			}
		}
		rows = append(rows, pkBorder.Render("│")+" "+line+strings.Repeat(" ", pad)+" "+pkBorder.Render("│"))
	}
	rows = append(rows, bottomLine)
	return strings.Join(rows, "\n")
}

// runFuzzyPicker runs the interactive picker inline (no alt screen). ran is
// false when the terminal can't host it and the caller should fall back to
// the numbered prompt.
func runFuzzyPicker(candidates []session.SessionInfo, sessions map[string]*config.Session) (name string, ran bool) {
	final, err := tea.NewProgram(newPickerModel(candidates, sessions, time.Now())).Run()
	if err != nil {
		return "", false
	}
	m, ok := final.(pickerModel)
	if !ok {
		return "", false
	}
	return m.choice, true
}
