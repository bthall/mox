package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestSessionRowNarrowKeepsStatusIndicator(t *testing.T) {
	previous := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	states := []struct{ state, dot string }{
		{"running", "●"},
		{"tmux only", "◆"},
		{"draft", "*"},
		{"delete", "*"},
		{"stopped", "○"},
	}
	for _, profile := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(profile)
		for _, entry := range states {
			for _, selected := range []bool{false, true} {
				for _, width := range []int{4, 12, 19} {
					row := sessionRow("dev", entry.state, selected, width)
					plain := ansi.Strip(row)
					marker := "  "
					if selected {
						marker = "▌ "
					}
					if !strings.HasPrefix(plain, marker+entry.dot+" ") {
						t.Errorf("state %q, selected=%t, width=%d lost its indicator: %q", entry.state, selected, width, plain)
					}
					if strings.Contains(plain, entry.state) {
						t.Errorf("state label %q was not hidden at width %d: %q", entry.state, width, plain)
					}
					if got := ansi.StringWidth(row); got > width {
						t.Errorf("state %q occupies %d cells in width %d", entry.state, got, width)
					}
				}
			}
		}
	}
}
