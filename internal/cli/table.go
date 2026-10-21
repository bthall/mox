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

const spaces = "                                                                                                " // 96 spaces

func writePadding(b *strings.Builder, n int) {
	for n > len(spaces) {
		b.WriteString(spaces)
		n -= len(spaces)
	}
	if n > 0 {
		b.WriteString(spaces[:n])
	}
}

// visibleWidth returns the number of visible runes in s, ignoring any ANSI
// color escape sequences it contains.
func visibleWidth(s string) int {
	// Optimization: fast path when string contains no ANSI escape sequences,
	// avoiding scanning and heap allocation.
	if !strings.Contains(s, "\x1b") {
		return utf8.RuneCountInString(s)
	}

	// Optimization: fast single-pass scanner for ANSI SGR sequences (\x1b\[[0-9;]*m)
	// eliminating regexp execution and string heap allocations.
	count := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';') {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				i = j + 1
				continue
			}
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		count++
		i += size
	}
	return count
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
	// Optimization: precompute visible widths in a single contiguous slice to
	// avoid redundant visibleWidth calculation, regex executions, and per-row
	// slice allocations.
	cellWidths := make([]int, len(rows)*cols)
	for rIdx, r := range rows {
		rowOffset := rIdx * cols
		for i, c := range r {
			w := visibleWidth(c)
			cellWidths[rowOffset+i] = w
			if w > widths[i] {
				widths[i] = w
			}
		}
	}
	for rIdx, r := range rows {
		rowOffset := rIdx * cols
		var b strings.Builder
		for i, c := range r {
			b.WriteString(c)
			if i < len(r)-1 {
				pad := widths[i] - cellWidths[rowOffset+i] + 2
				// Optimization: slice a constant spaces string to avoid heap allocations from strings.Repeat.
				writePadding(&b, pad)
			}
		}
		fmt.Fprintln(out, strings.TrimRight(b.String(), " "))
	}
}

// truncate shortens s to at most max runes, replacing the tail with "…" when
// it would otherwise overflow. max < 1 returns s unchanged.
func truncate(s string, max int) string {
	if max < 1 {
		return s
	}

	// Optimization: single-pass rune index scan to eliminate []rune heap allocation
	// and avoid redundant full-string utf8.RuneCountInString traversal.
	count := 0
	cut := -1
	for i := range s {
		if count == max-1 {
			cut = i
		}
		if count == max {
			return s[:cut] + "…"
		}
		count++
	}
	return s
}
