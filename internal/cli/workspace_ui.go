package cli

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

type keyHint struct{ key, label string }

type workspaceLayout struct {
	width, height, contentHeight int
	listWidth, detailWidth       int
	narrow                       bool
}

func terminalLayout(width, height int) workspaceLayout {
	w := max(1, width)
	inner := max(0, w-2)
	l := workspaceLayout{width: w, height: max(1, height), contentHeight: max(0, height-6), narrow: w < 80}
	l.listWidth, l.detailWidth = inner, inner
	if !l.narrow {
		l.listWidth = min(32, max(24, (inner-3)/3))
		l.detailWidth = inner - l.listWidth - 3
	}
	return l
}

type workspaceView struct {
	width, height          int
	view, context          string
	listTitle, detailTitle string
	list, detail           []string
	focusDetail            bool
	actions                []keyHint
	status                 string
}

// renderWorkspace owns all terminal bounds and shared chrome. The two work
// areas use one divider; state changes replace content rather than the frame.
func renderWorkspace(v workspaceView) string {
	l := terminalLayout(v.width, v.height)
	inner := max(0, l.width-2)
	header := edgeText(pkTitle.Render("mox / "+v.view), pkDim.Render(v.context), inner)
	rows := []string{" " + header + " ", pkBorder.Render(strings.Repeat("─", l.width))}
	listTitle, detailTitle := v.listTitle, v.detailTitle
	if v.focusDetail {
		detailTitle = pkSelected.Render(detailTitle)
	} else {
		listTitle = pkSelected.Render(listTitle)
	}
	if l.narrow {
		title, content := listTitle, v.list
		if v.focusDetail {
			title, content = detailTitle, v.detail
		}
		rows = append(rows, " "+fitCell(pkTitle.Render(title), inner)+" ")
		for i := 0; i < l.contentHeight; i++ {
			line := ""
			if i < len(content) {
				line = content[i]
			}
			rows = append(rows, " "+fitCell(line, inner)+" ")
		}
	} else {
		divider := " " + pkBorder.Render("│") + " "
		rows = append(rows, " "+fitCell(pkTitle.Render(listTitle), l.listWidth)+divider+fitCell(pkTitle.Render(detailTitle), l.detailWidth)+" ")
		for i := 0; i < l.contentHeight; i++ {
			left, right := "", ""
			if i < len(v.list) {
				left = v.list[i]
			}
			if i < len(v.detail) {
				right = v.detail[i]
			}
			rows = append(rows, " "+fitCell(left, l.listWidth)+divider+fitCell(right, l.detailWidth)+" ")
		}
	}
	rows = append(rows, pkBorder.Render(strings.Repeat("─", l.width)), " "+fitCell(renderHints(v.actions, inner), inner)+" ", " "+fitCell(v.status, inner)+" ")
	if l.height == 1 {
		rows = rows[:1]
	} else if len(rows) > l.height {
		rows = append(rows[:l.height-2], rows[len(rows)-2:]...)
	}
	for i, row := range rows {
		rows[i] = clipText(row, l.width)
	}
	view := strings.Join(rows, "\n")
	if lipgloss.ColorProfile() == termenv.Ascii {
		view = ansi.Strip(view)
	}
	return view
}

func clipText(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if ansi.StringWidth(s) > width {
		tail := ""
		if width > 1 {
			tail = "…"
		}
		s = ansi.Truncate(s, width, tail)
	}
	if strings.Contains(s, "\x1b") {
		s += "\x1b[0m"
	}
	return s
}

func fitCell(s string, width int) string {
	s = clipText(s, width)
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

func edgeText(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	if right == "" || ansi.StringWidth(left)+ansi.StringWidth(right)+3 > width {
		return fitCell(left, width)
	}
	return left + strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right)) + right
}

// Hints never cut a key/action in half. Help remains available when a narrow
// terminal cannot show the full action vocabulary.
func renderHints(actions []keyHint, width int) string {
	var visible []keyHint
	var help *keyHint
	for i := range actions {
		if actions[i].key == "?" {
			h := actions[i]
			help = &h
			break
		}
	}
	used, reserve := 0, 0
	if help != nil {
		reserve = ansi.StringWidth(help.key+" "+help.label) + 3
	}
	for _, action := range actions {
		if action.key == "?" {
			continue
		}
		n := ansi.StringWidth(action.key + " " + action.label)
		if len(visible) > 0 {
			n += 3
		}
		if used+n+reserve > width {
			continue
		}
		visible = append(visible, action)
		used += n
	}
	if help != nil && used+reserve <= width {
		visible = append(visible, *help)
	}
	pairs := make([]string, 0, len(visible)*2)
	for _, action := range visible {
		pairs = append(pairs, action.key, action.label)
	}
	return hints(pairs...)
}

func sessionRow(name, state string, selected bool, width int) string {
	dot := pkStopped.Render("○")
	stateStyle := pkDim
	switch state {
	case "running":
		dot, stateStyle = pkStateRunning.Render("●"), pkRunning
	case "tmux only":
		dot, stateStyle = pkStateForeign.Render("◆"), pkForeign
	case "draft", "delete":
		dot, stateStyle = pkForeign.Render("*"), pkForeign
	}
	marker := "  "
	if selected {
		marker = pkAccent.Render("▌ ")
	}
	if width < 20 {
		state = ""
	}
	nameWidth := max(1, width-4-ansi.StringWidth(state)-3)
	name = clipText(name, nameWidth)
	if selected {
		name = pkSelected.Render(name)
	}
	return marker + dot + " " + edgeText(name, stateStyle.Render(state), max(0, width-4))
}

func helpLines(actions []keyHint, width int) []string {
	lines := []string{pkTitle.Render("Keyboard help"), ""}
	seen := make(map[string]bool)
	for _, action := range actions {
		if seen[action.key] {
			continue
		}
		seen[action.key] = true
		if action.key == "?" {
			action.label = "close help"
		}
		lines = append(lines, pkTitle.Render(fitCell(action.key, min(10, max(1, width/3))))+" "+action.label)
	}
	return lines
}

// inputTail keeps the editing end and caret visible, including wide text.
func inputTail(input string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(input)+1 <= width {
		return input + pkTitle.Render("█")
	}
	runes := []rune(input)
	used, start := 0, len(runes)
	for start > 0 {
		n := ansi.StringWidth(string(runes[start-1]))
		if used+n > width-2 {
			break
		}
		used += n
		start--
	}
	if width == 1 {
		return pkTitle.Render("█")
	}
	return "…" + string(runes[start:]) + pkTitle.Render("█")
}
