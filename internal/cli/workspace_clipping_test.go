package cli

import "testing"

func TestClipTextWidthNarrowASCII(t *testing.T) {
	for _, tt := range []struct {
		width int
		want  string
	}{
		{0, ""},
		{1, "a"},
		{2, "a…"},
		{3, "abc"},
	} {
		got, width := clipTextWidth("abc", tt.width)
		if got != tt.want || width != tt.width {
			t.Errorf("clipTextWidth(abc, %d) = %q, %d; want %q, %d", tt.width, got, width, tt.want, tt.width)
		}
	}
}
