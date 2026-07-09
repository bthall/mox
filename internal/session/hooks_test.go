package session_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux/tmuxtest"
)

type hookRecorder struct {
	ran  []string
	fail string // hook command that should fail
}

func (h *hookRecorder) run(cmd string) error {
	h.ran = append(h.ran, cmd)
	if cmd == h.fail {
		return errors.New("boom")
	}
	return nil
}

func TestHooks_OnStartRunsBeforeBuild(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"dev": {Hosts: []string{"a"}, OnStart: []string{"vpn up", "echo go"}},
	}}
	tx := tmuxtest.NewFake()
	rec := &hookRecorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	if err := m.Create(context.Background(), "dev", false); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(rec.ran) != 2 || rec.ran[0] != "vpn up" || rec.ran[1] != "echo go" {
		t.Errorf("on_start hooks = %v, want both in order", rec.ran)
	}
	if exists, _ := tx.SessionExists("dev"); !exists {
		t.Error("session should have been built after hooks")
	}
}

func TestHooks_OnStartFailureAbortsBeforeCreating(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"dev": {Hosts: []string{"a"}, OnStart: []string{"vpn up"}},
	}}
	tx := tmuxtest.NewFake()
	rec := &hookRecorder{fail: "vpn up"}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	err := m.Create(context.Background(), "dev", false)
	if err == nil || !strings.Contains(err.Error(), "on_start hook") {
		t.Fatalf("want on_start failure, got %v", err)
	}
	if exists, _ := tx.SessionExists("dev"); exists {
		t.Error("session must not exist after an aborted on_start")
	}
}

func TestHooks_OnStopRunsAfterKill(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"dev": {Hosts: []string{"a"}, OnStop: []string{"cleanup sockets"}},
	}}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	rec := &hookRecorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	if err := m.Kill("dev"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if len(rec.ran) != 1 || rec.ran[0] != "cleanup sockets" {
		t.Errorf("on_stop hooks = %v", rec.ran)
	}
}

func TestHooks_OnStopFailureDoesNotBlockKill(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"dev": {Hosts: []string{"a"}, OnStop: []string{"broken"}},
	}}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	rec := &hookRecorder{fail: "broken"}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	if err := m.Kill("dev"); err != nil {
		t.Fatalf("Kill() must succeed despite on_stop failure, got %v", err)
	}
}

func TestHooks_UnmanagedKillRunsNoHooks(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("manual")
	rec := &hookRecorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	if err := m.Kill("manual"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if len(rec.ran) != 0 {
		t.Errorf("unmanaged kill ran hooks: %v", rec.ran)
	}
}

func TestHooks_AttachToRunningSkipsOnStart(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{
		"dev": {Hosts: []string{"a"}, OnStart: []string{"vpn up"}},
	}}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev") // already running
	rec := &hookRecorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithHookRunner(rec.run))

	if err := m.CreateOrAttach(context.Background(), "dev", false); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	if len(rec.ran) != 0 {
		t.Errorf("attaching to a running session must not re-run on_start, ran %v", rec.ran)
	}
}
