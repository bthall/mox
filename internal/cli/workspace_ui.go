package cli

import (
	"strings"
	"sync"

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
	borderDash := pkBorder.Render(dashes(l.width))
	// Optimization: preallocate rows slice capacity to avoid repeated slice growth allocations.
	rows := make([]string, 0, l.contentHeight+6)
	rows = append(rows, " "+header+" ", borderDash)
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
		divider := getRowStyles().divider
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
	rows = append(rows, borderDash, " "+fitCell(renderHints(v.actions, inner), inner)+" ", " "+fitCell(v.status, inner)+" ")
	if l.height == 1 {
		rows = rows[:1]
	} else if len(rows) > l.height {
		rows = append(rows[:l.height-2], rows[len(rows)-2:]...)
	}
	for i, row := range rows {
		if len(row) > l.width || strings.IndexByte(row, '\n') >= 0 {
			rows[i] = clipText(row, l.width)
		}
	}
	view := strings.Join(rows, "\n")
	if lipgloss.ColorProfile() == termenv.Ascii && strings.Contains(view, "\x1b") {
		view = ansi.Strip(view)
	}
	return view
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

func clipText(s string, width int) string {
	s, _ = clipTextWidth(s, width)
	return s
}

// Optimization: single-pass truncation and cell width calculation that returns
// both the clipped string and its visible width, eliminating double ansi.StringWidth
// passes when formatting layout cells. Fast-path printable ASCII strings.
func clipTextWidth(s string, width int) (string, int) {
	if width <= 0 {
		return "", 0
	}
	if isPrintableASCII(s) {
		if len(s) <= width {
			return s, len(s)
		}
		tail := ""
		if width > 1 {
			tail = "…"
		}
		s = ansi.Truncate(s, width, tail)
		return s, ansi.StringWidth(s)
	}
	if strings.IndexByte(s, '\n') >= 0 {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	sw := ansi.StringWidth(s)
	if sw > width {
		tail := ""
		if width > 1 {
			tail = "…"
		}
		s = ansi.Truncate(s, width, tail)
		sw = ansi.StringWidth(s)
	}
	if strings.Contains(s, "\x1b") && !hasResetSuffix(s) {
		s += "\x1b[0m"
	}
	return s, sw
}

func hasResetSuffix(s string) bool {
	return strings.HasSuffix(s, "\x1b[0m") || strings.HasSuffix(s, "\x1b[m")
}

// Optimization: constant padding and dash strings for zero-allocation string slicing
// during TUI layout rendering.
const dashesStr = "────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────" // 200 dashes (600 bytes)

func padding(n int) string {
	if n <= 0 {
		return ""
	}
	if n <= len(spaces) {
		return spaces[:n]
	}
	return strings.Repeat(" ", n)
}

func dashes(n int) string {
	if n <= 0 {
		return ""
	}
	if n*3 <= len(dashesStr) {
		return dashesStr[:n*3]
	}
	return strings.Repeat("─", n)
}

// Reuse a measured width only when formatting does not change the text.
func fitCellWithWidth(s string, sw, width int) string {
	if width <= 0 {
		return ""
	}
	if sw > width || strings.Contains(s, "\n") {
		return fitCell(s, width)
	}
	if strings.Contains(s, "\x1b") && !strings.HasSuffix(s, "\x1b[0m") {
		s += "\x1b[0m"
	}
	return s + padding(width-sw)
}

func fitCell(s string, width int) string {
	s, sw := clipTextWidth(s, width)
	pad := width - sw
	if pad <= 0 {
		return s
	}
	// Optimization: slice static spaces string to avoid strings.Repeat heap allocations.
	return s + padding(pad)
}

func edgeText(left, right string, width int) string {
	return edgeTextWithWidths(left, ansi.StringWidth(left), right, ansi.StringWidth(right), width)
}

// Optimization: edgeTextWithWidths accepts precomputed component visible widths
// to eliminate redundant ansi.StringWidth sequence scans during session row formatting.
func edgeTextWithWidths(left string, leftW int, right string, rightW int, width int) string {
	if width <= 0 {
		return ""
	}
	if right == "" {
		return fitCellWithWidth(left, leftW, width)
	}
	if leftW+rightW+3 > width {
		return fitCellWithWidth(left, leftW, width)
	}
	pad := width - leftW - rightW
	return left + padding(pad) + right
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
		reserve = hintWidth(help.key, help.label) + 3
	}
	for _, action := range actions {
		if action.key == "?" {
			continue
		}
		n := hintWidth(action.key, action.label)
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
	return hintsActions(visible)
}

// Optimization: calculate hint component width without intermediate string concatenation
// and fast-path printable ASCII strings to avoid ansi.StringWidth scanning.
func hintWidth(key, label string) int {
	kw := len(key)
	if !isPrintableASCII(key) {
		kw = ansi.StringWidth(key)
	}
	lw := len(label)
	if !isPrintableASCII(label) {
		lw = ansi.StringWidth(label)
	}
	return kw + 1 + lw
}

// Optimization: format visible hints via hints helper.
func hintsActions(actions []keyHint) string {
	pairs := make([]string, 0, len(actions)*2)
	for _, action := range actions {
		pairs = append(pairs, action.key, action.label)
	}
	return hints(pairs...)
}

type sessionRowStyleCache struct {
	profile      termenv.Profile
	marker       string
	dotRunning   string
	stateRunning string
	dotForeign   string
	stateForeign string
	dotDraft     string
	stateDraft   string
	stateDelete  string
	dotStopped   string
	stateStopped string
	divider      string
}

var (
	rowStyleCacheMu sync.RWMutex
	rowStyleCache   sessionRowStyleCache
)

func getRowStyles() sessionRowStyleCache {
	p := lipgloss.ColorProfile()
	rowStyleCacheMu.RLock()
	if rowStyleCache.profile == p && rowStyleCache.marker != "" {
		c := rowStyleCache
		rowStyleCacheMu.RUnlock()
		return c
	}
	rowStyleCacheMu.RUnlock()

	rowStyleCacheMu.Lock()
	defer rowStyleCacheMu.Unlock()
	if rowStyleCache.profile == p && rowStyleCache.marker != "" {
		return rowStyleCache
	}
	c := sessionRowStyleCache{
		profile:      p,
		marker:       pkAccent.Render("▌ "),
		dotRunning:   pkStateRunning.Render("●"),
		stateRunning: pkRunning.Render("running"),
		dotForeign:   pkStateForeign.Render("◆"),
		stateForeign: pkForeign.Render("tmux only"),
		dotDraft:     pkForeign.Render("*"),
		stateDraft:   pkForeign.Render("draft"),
		stateDelete:  pkForeign.Render("delete"),
		dotStopped:   pkStopped.Render("○"),
		stateStopped: pkDim.Render("stopped"),
		divider:      " " + pkBorder.Render("│") + " ",
	}
	rowStyleCache = c
	return c
}

func sessionRow(name, state string, selected bool, width int) string {
	styles := getRowStyles()
	var dot, renderedState string
	switch state {
	case "running":
		dot, renderedState = styles.dotRunning, styles.stateRunning
	case "tmux only":
		dot, renderedState = styles.dotForeign, styles.stateForeign
	case "draft":
		dot, renderedState = styles.dotDraft, styles.stateDraft
	case "delete":
		dot, renderedState = styles.dotDraft, styles.stateDelete
	case "stopped":
		dot, renderedState = styles.dotStopped, styles.stateStopped
	case "":
		dot, renderedState = styles.dotStopped, ""
	default:
		dot = styles.dotStopped
		renderedState = pkDim.Render(state)
	}
	if width < 20 {
		state, renderedState = "", ""
	}
	marker := "  "
	if selected {
		marker = styles.marker
	}
	stateW := len(state)
	if !isPrintableASCII(state) {
		stateW = ansi.StringWidth(state)
	}
	nameWidth := max(1, width-4-stateW-3)
	name, nameW := clipTextWidth(name, nameWidth)
	if selected {
		name = pkSelected.Render(name)
		// Underlining inserts escapes between runes and can change grapheme widths.
		nameW = ansi.StringWidth(name)
	}
	edgeW := max(0, width-4)
	if edgeW <= 0 {
		return marker + dot + " "
	}
	if renderedState == "" || nameW+stateW+3 > edgeW {
		return marker + dot + " " + fitCellWithWidth(name, nameW, edgeW)
	}
	pad := edgeW - nameW - stateW
	return marker + dot + " " + name + padding(pad) + renderedState
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
