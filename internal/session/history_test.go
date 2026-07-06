package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux/tmuxtest"
)

func TestManager_List_Enriched(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a", "b"}},
		},
	}
	tx := tmuxtest.NewFake()
	tx.SetSession("dev")
	activity := time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC)
	tx.AttachedSessions = map[string]bool{"dev": true}
	tx.ActivityBySess = map[string]time.Time{"dev": activity}

	m := session.NewManagerWith(cfg, tx, nil)
	got, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var dev session.SessionInfo
	for _, s := range got {
		if s.Name == "dev" {
			dev = s
		}
	}
	if !dev.Attached {
		t.Errorf("dev should report Attached")
	}
	if !dev.LastActivity.Equal(activity) {
		t.Errorf("dev LastActivity = %v, want %v", dev.LastActivity, activity)
	}
	if dev.Windows < 1 {
		t.Errorf("running dev should have at least 1 window, got %d", dev.Windows)
	}
	if len(dev.Hosts) != 2 || dev.Hosts[0] != "a" || dev.Hosts[1] != "b" {
		t.Errorf("dev Hosts = %v, want [a b]", dev.Hosts)
	}
}

// recorder collects history calls for assertions.
type recorder struct{ calls [][2]string }

func (r *recorder) record(name, action string) error {
	r.calls = append(r.calls, [2]string{name, action})
	return nil
}

func TestManager_RecordsCreatedOnAdHoc(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	rec := &recorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithRecorder(rec.record))

	sess := &config.Session{Hosts: []string{"a"}}
	if err := m.CreateAdHoc(context.Background(), "scratch", sess, session.AdHocOptions{Detach: true}); err != nil {
		t.Fatalf("CreateAdHoc() error = %v", err)
	}
	assertRecorded(t, rec, "scratch", "created")
}

func TestManager_RecordsAttachedOnUnmanaged(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("manual")
	rec := &recorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithRecorder(rec.record))

	if err := m.CreateOrAttach(context.Background(), "manual", false); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	assertRecorded(t, rec, "manual", "attached")
}

func TestManager_RecordsCreatedOnConfiguredBuild(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{"dev": {Hosts: []string{"a"}}},
	}
	tx := tmuxtest.NewFake() // dev does not yet exist -> will be built
	rec := &recorder{}
	m := session.NewManagerWith(cfg, tx, nil, session.WithRecorder(rec.record))

	if err := m.CreateOrAttach(context.Background(), "dev", false); err != nil {
		t.Fatalf("CreateOrAttach() error = %v", err)
	}
	assertRecorded(t, rec, "dev", "created")
}

func assertRecorded(t *testing.T, rec *recorder, name, action string) {
	t.Helper()
	for _, c := range rec.calls {
		if c[0] == name && c[1] == action {
			return
		}
	}
	t.Errorf("expected history record %s/%s, got %v", name, action, rec.calls)
}
