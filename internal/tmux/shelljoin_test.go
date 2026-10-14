package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

func TestShellJoinRoundTripsThroughShell(t *testing.T) {
	args := []string{"new-session", "-s", "plain-name", "", "path\\with", "a[b]", "line\nbreak", "semi;colon", "quote's", "$HOME", "hello world"}
	cmd := exec.Command("sh", "-c", "set -- "+shellJoin(args)+"; printf '%s\\000' \"$@\"")
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("run rendered command: %v", err)
	}
	want := strings.Join(args, "\x00") + "\x00"
	if string(got) != want {
		t.Fatalf("shellJoin round-trip = %q, want %q", got, want)
	}
}
