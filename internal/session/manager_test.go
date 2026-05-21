package session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux/tmuxtest"
)

func newTestConfig() *config.Config {
	return &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a", "b"}},
		},
	}
}

func TestManager_CreateOrAttach_NewSession(t *testing.T) {
	cfg := newTestConfig()
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)

	if err := m.CreateOrAttach(context.Background(), "dev", false); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	if tx.AttachCalled != "dev" {
		t.Errorf("expected AttachSession(dev), got %q", tx.AttachCalled)
	}
}

func TestManager_CreateOrAttach_ExistingSession(t *testing.T) {
	cfg := newTestConfig()
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	m := session.NewManagerWith(cfg, tx, nil)

	if err := m.CreateOrAttach(context.Background(), "dev", false); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "CreateSession ") {
			t.Errorf("expected to skip CreateSession when session exists, but saw %q", c)
		}
	}
	if tx.AttachCalled != "dev" {
		t.Errorf("expected AttachSession(dev)")
	}
}

func TestManager_CreateOrAttach_Force(t *testing.T) {
	cfg := newTestConfig()
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	m := session.NewManagerWith(cfg, tx, nil)

	if err := m.CreateOrAttach(context.Background(), "dev", true); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	killed := false
	created := false
	for _, c := range tx.Calls {
		if c == "KillSession dev" {
			killed = true
		}
		if strings.HasPrefix(c, "CreateSession dev") {
			created = true
		}
	}
	if !killed {
		t.Errorf("expected KillSession when --force, calls: %v", tx.Calls)
	}
	if !created {
		t.Errorf("expected CreateSession after kill, calls: %v", tx.Calls)
	}
}

func TestManager_CreateOrAttach_UnknownSession(t *testing.T) {
	cfg := newTestConfig()
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)
	err := m.CreateOrAttach(context.Background(), "ghost", false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestManager_BuildFailureCleansUpPartialSession(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a", "b", "c"}},
		},
	}
	tx := tmuxtest.NewFake()
	tx.SplitFailOn = "%2" // fail when splitting from second pane
	m := session.NewManagerWith(cfg, tx, nil)

	err := m.CreateOrAttach(context.Background(), "dev", false)
	if err == nil {
		t.Fatal("expected build failure")
	}
	killed := false
	for _, c := range tx.Calls {
		if c == "KillSession dev" {
			killed = true
		}
	}
	if !killed {
		t.Errorf("expected KillSession after build failure, got %v", tx.Calls)
	}
}

func TestManager_Kill(t *testing.T) {
	cfg := newTestConfig()
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	m := session.NewManagerWith(cfg, tx, nil)

	if err := m.Kill("dev"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if err := m.Kill("ghost"); err == nil {
		t.Error("expected error killing nonexistent session")
	}
}

func TestManager_List(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev":  {Hosts: []string{"a"}},
			"prod": {Hosts: []string{"b"}},
		},
	}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	m := session.NewManagerWith(cfg, tx, nil)

	got, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	for _, s := range got {
		if !s.Managed {
			t.Errorf("configured session %q should be managed", s.Name)
		}
		switch s.Name {
		case "dev":
			if !s.Running {
				t.Errorf("dev should be running")
			}
		case "prod":
			if s.Running {
				t.Errorf("prod should not be running")
			}
		}
	}
}

func TestManager_List_IncludesUnmanagedSessions(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a"}},
		},
	}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	tx.SetSession("manually-created")
	m := session.NewManagerWith(cfg, tx, nil)

	got, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d (%+v)", len(got), got)
	}
	var unmanagedFound bool
	for _, s := range got {
		if s.Name == "manually-created" {
			if s.Managed {
				t.Errorf("manually-created should be unmanaged")
			}
			if !s.Running {
				t.Errorf("manually-created should be running")
			}
			unmanagedFound = true
		}
	}
	if !unmanagedFound {
		t.Errorf("expected manually-created in list, got %+v", got)
	}
}

func TestManager_CreateAdHoc(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)

	sess := &config.Session{Hosts: []string{"a", "b"}}
	err := m.CreateAdHoc(context.Background(), "scratch", sess, session.AdHocOptions{})
	if err != nil {
		t.Fatalf("CreateAdHoc() error = %v", err)
	}
	if tx.AttachCalled != "scratch" {
		t.Errorf("expected AttachSession(scratch), got %q", tx.AttachCalled)
	}
}

func TestManager_CreateAdHoc_Temporary(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)

	sess := &config.Session{Hosts: []string{"a"}}
	err := m.CreateAdHoc(context.Background(), "tmp1", sess, session.AdHocOptions{Temporary: true})
	if err != nil {
		t.Fatalf("CreateAdHoc(temporary) error = %v", err)
	}
	want := `SetHook tmp1 client-attached = "set-option -t tmp1 destroy-unattached on"`
	found := false
	for _, c := range tx.Calls {
		if c == want {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hook %q, got calls: %v", want, tx.Calls)
	}
	// And ensure no eager SetSessionOption call snuck back in.
	for _, c := range tx.Calls {
		if strings.Contains(c, "SetSessionOption") && strings.Contains(c, "destroy-unattached") {
			t.Errorf("destroy-unattached should be set via hook, not directly; got %q", c)
		}
	}
}

func TestManager_CreateAdHoc_Detach(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)

	sess := &config.Session{Hosts: []string{"a"}}
	err := m.CreateAdHoc(context.Background(), "bg", sess, session.AdHocOptions{Detach: true})
	if err != nil {
		t.Fatalf("CreateAdHoc(detach) error = %v", err)
	}
	if tx.AttachCalled != "" {
		t.Errorf("expected no AttachSession with --detach, got %q", tx.AttachCalled)
	}
}

func TestManager_CreateAdHoc_RejectsExisting(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("scratch")
	m := session.NewManagerWith(cfg, tx, nil)

	err := m.CreateAdHoc(context.Background(), "scratch", &config.Session{Hosts: []string{"a"}}, session.AdHocOptions{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected already-exists error, got %v", err)
	}
}

func TestManager_CreateAdHoc_ForceRecreate(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("scratch")
	m := session.NewManagerWith(cfg, tx, nil)

	err := m.CreateAdHoc(context.Background(), "scratch", &config.Session{Hosts: []string{"a"}}, session.AdHocOptions{Force: true})
	if err != nil {
		t.Fatalf("CreateAdHoc(force) error = %v", err)
	}
	killed := false
	for _, c := range tx.Calls {
		if c == "KillSession scratch" {
			killed = true
		}
	}
	if !killed {
		t.Errorf("expected KillSession to precede recreate, got %v", tx.Calls)
	}
}

func TestManager_CreateAdHoc_ValidatesHost(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	m := session.NewManagerWith(cfg, tx, nil)

	// Hostname with shell metacharacter should be rejected by validation.
	err := m.CreateAdHoc(context.Background(), "evil",
		&config.Session{Hosts: []string{"host; rm -rf /"}},
		session.AdHocOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("expected unsafe-host error, got %v", err)
	}
}
