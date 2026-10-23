package cli

import "testing"

func TestSanitizeCaptureLineInvalidUTF8(t *testing.T) {
	for _, input := range []string{"a\xff\tb", "a\xc3\tb", "a\x80\tb"} {
		if got, want := sanitizeCaptureLine(input), "a\ufffd      b"; got != want {
			t.Errorf("sanitizeCaptureLine(%q) = %q, want %q", input, got, want)
		}
	}
}
