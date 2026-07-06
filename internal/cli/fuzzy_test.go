package cli

import (
	"slices"
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		query, name string
		want        matchClass
	}{
		{"", "anything", matchPrefix},
		{"web", "web-cluster", matchPrefix},
		{"WEB", "web-cluster", matchPrefix}, // case-insensitive
		{"cluster", "web-cluster", matchSubstring},
		{"wcl", "web-cluster", matchSubsequence},
		{"xyz", "web-cluster", matchNone},
		{"webx", "web-cluster", matchNone},
	}
	for _, c := range cases {
		if got := fuzzyMatch(c.query, c.name); got != c.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", c.query, c.name, got, c.want)
		}
	}
}

func TestFuzzyFilter_RanksAndPreservesOrder(t *testing.T) {
	names := []string{"analytics", "web-prod", "prod-db", "scratch"}

	got := fuzzyFilter("prod", names)
	// prod-db is a prefix match, web-prod a substring match; analytics and
	// scratch don't match at all.
	want := []int{2, 1}
	if !slices.Equal(got, want) {
		t.Errorf("fuzzyFilter(prod) = %v, want %v", got, want)
	}

	if got := fuzzyFilter("", names); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Errorf("empty query should keep original order, got %v", got)
	}
	if got := fuzzyFilter("zzz", names); len(got) != 0 {
		t.Errorf("no-match query should return empty, got %v", got)
	}
}
