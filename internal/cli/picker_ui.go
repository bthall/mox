package cli

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bthall/mox/internal/session"
)

// Terminal-native styles and shared row/key helpers for the full-screen UIs.
// The responsive workspace frame lives in workspace_ui.go.
var (
	pkBorder = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkTitle  = lipgloss.NewStyle().Bold(true)
	// Secondary text follows the terminal foreground so it remains readable
	// on light, dark, and custom themes. Only decorative rules use color 8.
	pkDim      = lipgloss.NewStyle()
	pkAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	pkSelected = lipgloss.NewStyle().Bold(true)
	pkRunning  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	pkStopped  = lipgloss.NewStyle()
	pkForeign  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	pkOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
)

// hints renders alternating key/label pairs for a panel footer: keys in
// the accent color, labels dim — e.g. hints("↵", "attach", "q", "quit").
func hints(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(pkDim.Render(" · "))
		}
		b.WriteString(pkAccent.Render(pairs[i]))
		b.WriteString(pkDim.Render(" " + pairs[i+1]))
	}
	return b.String()
}

// statusDot is the per-session status glyph: shape carries the origin
// (● in the config, ◆ tmux only), color carries the state — so the
// distinction survives without color too.
func statusDot(c session.SessionInfo) string {
	switch {
	case c.Running && !c.Managed:
		return pkForeign.Render("◆")
	case c.Running:
		return pkRunning.Render("●")
	default:
		return pkStopped.Render("○")
	}
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

// leaver is a full-screen model that can report it has handed the terminal
// off: quit, or suspended for $EDITOR.
type leaver interface {
	tea.Model
	isLeaving() bool
}

// replayRunes feeds a batched KeyRunes message (key repeat, fast typing)
// back through the model one rune at a time, since hotkeys match single
// runes. Commands from every rune are batched together. Replay stops at the
// rune that hands the terminal off, so keys typed after a quit never act on
// a model that is already leaving.
func replayRunes[M leaver](m M, runes []rune) (tea.Model, tea.Cmd) {
	cur := m
	var cmds []tea.Cmd
	for _, r := range runes {
		nm, cmd := cur.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		next, ok := nm.(M)
		if !ok {
			return nm, tea.Batch(append(cmds, cmd)...)
		}
		cur = next
		cmds = append(cmds, cmd)
		if cur.isLeaving() {
			break
		}
	}
	return cur, tea.Batch(cmds...)
}
