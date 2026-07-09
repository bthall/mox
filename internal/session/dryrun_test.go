package session_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux"
)

func TestDryRun_PrintsCommandsWithoutExecuting(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {
				Hosts:   []string{"web1", "web2"},
				Sync:    true,
				Arrange: "tiled",
			},
		},
	}
	var out bytes.Buffer
	m := session.NewManagerWith(cfg, tmux.NewDryRun(&out), nil)

	if err := m.Create(context.Background(), "dev", false); err != nil {
		t.Fatalf("dry-run Create() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"tmux new-session -d -s dev",
		"tmux split-window",
		"ssh web1",
		"ssh web2",
		"tmux select-layout",
		"synchronize-panes on",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
	// Every line is a tmux invocation — nothing else may leak into stdout.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if !strings.HasPrefix(line, "tmux ") {
			t.Errorf("non-command line in dry-run output: %q", line)
		}
	}
}

func TestDryRun_AttachPathPrintsAttach(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{"dev": {Hosts: []string{"a"}}},
	}
	var out bytes.Buffer
	m := session.NewManagerWith(cfg, tmux.NewDryRun(&out), nil)

	if err := m.CreateOrAttach(context.Background(), "dev", false); err != nil {
		t.Fatalf("dry-run CreateOrAttach() error = %v", err)
	}
	if !strings.Contains(out.String(), "attach-session") && !strings.Contains(out.String(), "switch-client") {
		t.Errorf("dry-run should print the attach step:\n%s", out.String())
	}
}
