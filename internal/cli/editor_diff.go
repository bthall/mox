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
	return lines
}

// diffLines computes a line diff of a → b via longest common subsequence.
func diffLines(a, b []string) []diffLine {
	n, m := len(a), len(b)
	stride := m + 1
	// Optimization: preallocate a single contiguous slice for LCS matrix
	// and preallocate output slice capacity to minimize heap allocations.
	lcs := make([]int, (n+1)*stride)
	for i := n - 1; i >= 0; i-- {
		row := i * stride
		nextRow := (i + 1) * stride
		ai := a[i]
		for j := m - 1; j >= 0; j-- {
			if ai == b[j] {
				lcs[row+j] = lcs[nextRow+j+1] + 1
			} else if lcs[nextRow+j] >= lcs[row+j+1] {
				lcs[row+j] = lcs[nextRow+j]
			} else {
				lcs[row+j] = lcs[row+j+1]
			}
		}
	}
	capHint := n
	if m > capHint {
		capHint = m
	}
	out := make([]diffLine, 0, capHint)
	i, j := 0, 0
	for i < n && j < m {
		row := i * stride
		nextRow := (i + 1) * stride
		switch {
		case a[i] == b[j]:
			out = append(out, diffLine{diffSame, a[i]})
			i++
			j++
		case lcs[nextRow+j] >= lcs[row+j+1]:
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
