package cli

import "strings"

// matchClass ranks how well a name matches a query; lower is better.
type matchClass int

const (
	matchPrefix matchClass = iota
	matchSubstring
	matchSubsequence
	matchNone
)

// fuzzyMatch classifies a case-insensitive match of query against name:
// prefix beats substring beats subsequence (query runes appearing in order,
// not necessarily adjacent). An empty query matches everything as a prefix.
func fuzzyMatch(query, name string) matchClass {
	q := strings.ToLower(query)
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, q):
		return matchPrefix
	case strings.Contains(n, q):
		return matchSubstring
	}
	i := 0
	for _, r := range n {
		if i < len(q) && rune(q[i]) == r {
			i++
		}
	}
	if i == len(q) {
		return matchSubsequence
	}
	return matchNone
}

// fuzzyFilter returns the indices of candidates whose name matches query,
// best matches first; within the same match class the original order (which
// encodes running-state and recency) is preserved.
func fuzzyFilter(query string, names []string) []int {
	var byClass [3][]int
	for i, n := range names {
		if c := fuzzyMatch(query, n); c != matchNone {
			byClass[c] = append(byClass[c], i)
		}
	}
	out := make([]int, 0, len(names))
	for _, class := range byClass {
		out = append(out, class...)
	}
	return out
}
