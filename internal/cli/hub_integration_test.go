//go:build integration

package cli

import (
	"strings"
	"testing"

	"github.com/bthall/mox/internal/tmux"
)

// TestHubTmuxFuncsAgainstRealTmux pins the tmux target syntax the hub's
// preview uses. The v0.4.0 hub shipped with "=NAME" as a capture-pane
// target, which every tmux rejects ("can't find pane") — only a live
// round-trip catches that class of bug. Uses a dedicated session name on
// the default server, matching the other integration tests.
func TestHubTmuxFuncsAgainstRealTmux(t *testing.T) {
	client, err := tmux.NewClient()
	if err != nil {
		t.Skipf("tmux unavailable: %v", err)
	}
	const name = "mox-hub-capture-test"
	_ = client.KillSession(name)
	if err := client.CreateSession(name, "", ""); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { _ = client.KillSession(name) })

	capture, windows := hubTmuxFuncs(client, nil)
	if _, err := capture(name); err != nil {
		t.Fatalf("capture(%q): %v", name, err)
	}
	out, err := windows(name)
	if err != nil {
		t.Fatalf("windows(%q): %v", name, err)
	}
	if !strings.Contains(out, "*") {
		t.Fatalf("window summary missing active marker: %q", out)
	}
}
