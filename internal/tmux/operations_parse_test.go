package tmux

import (
	"testing"
	"time"
)

func TestParseSessionDetails(t *testing.T) {
	output := "dev\x1f3\x1f1\x1f1700000000\n" +
		"prod\x1f1\x1f0\x1f1700000060"

	got := parseSessionDetails(output)
	if len(got) != 2 {
		t.Fatalf("want 2 details, got %d (%+v)", len(got), got)
	}

	dev := got[0]
	if dev.Name != "dev" || dev.Windows != 3 || !dev.Attached {
		t.Errorf("dev parsed wrong: %+v", dev)
	}
	if !dev.Activity.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("dev activity = %v", dev.Activity)
	}

	prod := got[1]
	if prod.Name != "prod" || prod.Windows != 1 || prod.Attached {
		t.Errorf("prod parsed wrong: %+v", prod)
	}
}

func TestParseSessionDetails_EmptyAndMalformed(t *testing.T) {
	if got := parseSessionDetails(""); len(got) != 0 {
		t.Errorf("empty output should yield no details, got %+v", got)
	}
	if got := parseSessionDetails("\n"); len(got) != 0 {
		t.Errorf("blank line should yield no details, got %+v", got)
	}
	// A line missing fields is skipped rather than panicking.
	if got := parseSessionDetails("only-name"); len(got) != 0 {
		t.Errorf("short line should be skipped, got %+v", got)
	}
	// Non-numeric window/activity default to zero values, not an error.
	got := parseSessionDetails("dev\x1fNaN\x1f1\x1fNaN")
	if len(got) != 1 || got[0].Windows != 0 {
		t.Errorf("malformed numerics should default to zero, got %+v", got)
	}
}
