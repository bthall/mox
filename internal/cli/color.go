package cli

import (
	"io"
	"os"
)

// ANSI color escape sequences.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

// colorize wraps s in the given ANSI sequence if w is a terminal and color
// is not disabled. The de facto NO_COLOR standard (https://no-color.org)
// suppresses color when the env var is set to any non-empty value.
func colorize(w io.Writer, code, s string) string {
	if !useColor(w) {
		return s
	}
	return code + s + ansiReset
}

func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// The de facto CLICOLOR_FORCE standard: force color even when piped
	// (`mox list | less -R`, screenshot generation). NO_COLOR still wins.
	if os.Getenv("CLICOLOR_FORCE") == "1" {
		return true
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
