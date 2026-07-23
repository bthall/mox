package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/muesli/termenv"
)

// TestTUIColorProfile pins that the TUIs follow mox's own color policy
// (useColor: TTY + NO_COLOR) instead of termenv's TERM sniffing, which
// strips every style on TERM values it doesn't recognize.
func TestTUIColorProfile(t *testing.T) {
	if got := tuiColorProfile(&bytes.Buffer{}); got != termenv.Ascii {
		t.Errorf("non-file writer: profile = %v, want Ascii", got)
	}

	tty, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0) // char device: useColor says yes
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tty.Close() }()
	if got := tuiColorProfile(tty); got != termenv.ANSI256 {
		t.Errorf("char device: profile = %v, want ANSI256", got)
	}

	t.Setenv("CLICOLOR_FORCE", "1")
	if got := tuiColorProfile(&bytes.Buffer{}); got != termenv.ANSI256 {
		t.Errorf("CLICOLOR_FORCE set: profile = %v, want ANSI256 even for a non-file writer", got)
	}

	t.Setenv("NO_COLOR", "1") // NO_COLOR beats CLICOLOR_FORCE
	if got := tuiColorProfile(tty); got != termenv.Ascii {
		t.Errorf("NO_COLOR set: profile = %v, want Ascii", got)
	}
}
