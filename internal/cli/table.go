package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ansiRE matches ANSI SGR (color) escape sequences so column widths can be
// measured by visible width rather than byte length. text/tabwriter does not
// understand these sequences, so we lay out columns ourselves.
var ansiRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visibleWidth returns the number of visible runes in s, ignoring any ANSI
// color escape sequences it contains.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansiRE.ReplaceAllString(s, ""))
}

// renderTable left-aligns rows into columns separated by two spaces. Cells may
// already contain ANSI color codes; padding is computed from visible width so
// colored and uncolored cells align identically. Trailing whitespace on each
// line is trimmed.
func renderTable(out io.Writer, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	for _, r := range rows {
		for i, c := range r {
			if w := visibleWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, r := range rows {
		var b strings.Builder
		for i, c := range r {
			b.WriteString(c)
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-visibleWidth(c)+2))
			}
		}
		fmt.Fprintln(out, strings.TrimRight(b.String(), " "))
	}
}

// truncate shortens s to at most max runes, replacing the tail with "…" when
// it would otherwise overflow. max < 1 returns s unchanged.
func truncate(s string, max int) string {
	if max < 1 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}
