package tmux

import (
	"strings"
	"testing"
)

func TestWrapConnect_HoldOnly(t *testing.T) {
	got := wrapConnect("ssh web1", "web1", true, 0)
	if !strings.HasPrefix(got, "ssh web1;") {
		t.Errorf("connect must stay the leading command: %q", got)
	}
	for _, want := range []string{"connection ended", "read -r", "exit"} {
		if !strings.Contains(got, want) {
			t.Errorf("hold wrap missing %q: %q", want, got)
		}
	}
}

func TestWrapConnect_RetryAndHold(t *testing.T) {
	got := wrapConnect("ssh web1", "web1", true, 2)
	for _, want := range []string{
		"for _mox_try in 1 2 3", // retry: 2 = three total attempts
		"ssh web1 && break",
		"sleep 3",
		"connection ended",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("retry+hold wrap missing %q: %q", want, got)
		}
	}
}

func TestWrapConnect_Disabled(t *testing.T) {
	if got := wrapConnect("ssh web1", "web1", false, 0); got != "ssh web1" {
		t.Errorf("hold:false retry:0 must leave the command untouched, got %q", got)
	}
}

func TestPrependCmds(t *testing.T) {
	got := prependCmds([]string{"a", "b"}, []string{"c"})
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("prependCmds = %v", got)
	}
	if got := prependCmds(nil, []string{"c"}); len(got) != 1 {
		t.Errorf("nil pre should pass through, got %v", got)
	}
	if got := prependCmds([]string{"a"}, nil); len(got) != 1 || got[0] != "a" {
		t.Errorf("pre alone should still run, got %v", got)
	}
}
