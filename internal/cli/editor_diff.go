package cli

// The save-preview diff. Session blocks are tiny (tens of lines), so a
// plain O(n*m) LCS line diff is plenty.

import (
	"strings"

	"github.com/bthall/mox/internal/config"
)

type diffKind int

const (
	diffSame diffKind = iota
	diffDel
	diffAdd
)

type diffLine struct {
	kind diffKind
	text string
}

// sessionBlock renders one session as the YAML lines a save would produce,
// including the "name:" key line (the `sessions:` wrapper is stripped).
// A nil session yields no lines (the deleted/absent side of a diff).
func sessionBlock(name string, sess *config.Session) []string {
	if sess == nil {
		return nil
	}
	var b strings.Builder
	if err := printSessionYAML(&b, name, sess); err != nil {
		return []string{"(preview unavailable: " + err.Error() + ")"}
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "sessions:") {
		lines = lines[1:]
	}
	// Collapse block-style lists to flow style to avoid "-" markers in diffs.
	lines = collapseListsToFlow(lines)
	return lines
}

// collapseListsToFlow converts YAML block-style lists to flow style.
// Converts:
//
//	key:
//	    - item1
//	    - item2
//
// To:
//
//	key: [item1, item2]
func collapseListsToFlow(lines []string) []string {
	var out []string
	i := 0
	for i < len(lines) {
		line := lines[i]
		// Check if this line ends with ":" (potential list key)
		if strings.HasSuffix(strings.TrimRight(line, " "), ":") {
			// Check if next line is a list item (starts with indent and "- ")
			if i+1 < len(lines) && isListItem(lines[i+1]) {
				// Collect all list items
				keyLine := line
				keyIndent := getIndent(line)
				var items []string
				j := i + 1
				for j < len(lines) && isListItem(lines[j]) && getIndent(lines[j]) > keyIndent {
					item := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[j]), "- "))
					items = append(items, item)
					j++
				}
				// Add the key with flow-style list
				if len(items) > 0 {
					out = append(out, keyLine[:len(keyLine)-1]+" ["+strings.Join(items, ", ")+"]")
					i = j
					continue
				}
			}
		}
		out = append(out, line)
		i++
	}
	return out
}

func isListItem(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "- ")
}

func getIndent(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

// diffLines computes a line diff of a → b via longest common subsequence.
func diffLines(a, b []string) []diffLine {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, diffLine{diffSame, a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{diffDel, a[i]})
			i++
		default:
			out = append(out, diffLine{diffAdd, b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{diffDel, a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{diffAdd, b[j]})
	}
	return out
}

// draftDiff builds the save preview: the draft's session block against what
// the loaded config holds for it now.
func draftDiff(cfg *config.Config, d *sessionDraft) []diffLine {
	var before, after []string
	if d.orig != "" {
		before = sessionBlock(d.orig, cfg.Sessions[d.orig])
	}
	if !d.deleted {
		after = sessionBlock(d.name, d.sess)
	}
	return diffLines(before, after)
}
