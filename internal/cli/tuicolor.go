package cli

// The full-screen TUIs render through lipgloss, which by default asks
// termenv to sniff the environment for color support. That sniffing
// returns no-color for any TERM it doesn't recognize, silently stripping
// every style while `mox list` (which colors on its own TTY + NO_COLOR
// policy) stays colored on the same terminal. mox makes the color
// decision once, with the same policy, for both.

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// tuiColorProfile maps mox's color policy onto a lipgloss/termenv profile.
// Every style in the TUIs uses the base 16-color palette, so ANSI256 is
// never more than the terminal already proved it can render for mox list.
func tuiColorProfile(w io.Writer) termenv.Profile {
	if useColor(w) {
		return termenv.ANSI256
	}
	return termenv.Ascii
}

// applyTUIColorPolicy pins lipgloss's global profile before a TUI starts.
// A terminal that can host the alt-screen UI can render SGR colors.
func applyTUIColorPolicy() {
	lipgloss.SetColorProfile(tuiColorProfile(os.Stdout))
}
