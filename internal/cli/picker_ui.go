package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/bthall/mox/internal/session"
)

// Shared chrome for mox's full-screen UIs (the session hub and the config
// editor): the 16-color palette, the bordered panel primitive, and small
// row helpers. The inline two-pane picker that used to live here was
// replaced by the hub (hub_ui.go).
var (
	pkBorder   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkTitle    = lipgloss.NewStyle().Bold(true)
	pkDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pkAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	pkSelected = lipgloss.NewStyle().Bold(true)
	pkRunning  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	pkStopped  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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

// panel draws a rounded-border box with a title in the top border and an
// optional footer in the bottom border. Content lines are clipped and padded
// to the inner width; the pane is padded to the requested height.
func panel(title, footer string, content []string, w, h int) string {
	if w < 8 {
		w = 8
	}
	inner := w - 4 // "│ " + " │"

	if lipgloss.Width(title) > w-5 {
		title = ansi.Truncate(title, w-5, "…")
	}
	top := "╭─ " + pkTitle.Render(title) + " "
	fill := w - lipgloss.Width(top) - 1
	if fill < 0 {
		fill = 0
	}
	topLine := pkBorder.Render("╭─ ") + pkTitle.Render(title) + pkBorder.Render(" "+strings.Repeat("─", fill)+"╮")

	var bottomLine string
	if footer != "" {
		styled := pkDim.Render(footer)
		if strings.Contains(footer, "\x1b") {
			styled = footer // pre-styled footer (key/label hints)
		}
		if lipgloss.Width(styled) > w-5 {
			// Clip without breaking escape sequences; reset so a cut
			// style can't bleed into the border.
			styled = ansi.Truncate(styled, w-5, "…") + "\x1b[0m"
		}
		fill := w - lipgloss.Width("╰─ "+styled+" ") - 1
		if fill < 0 {
			fill = 0
		}
		bottomLine = pkBorder.Render("╰─ ") + styled + pkBorder.Render(" "+strings.Repeat("─", fill)+"╯")
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
