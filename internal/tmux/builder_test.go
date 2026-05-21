package tmux_test

import (
	"context"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
	"github.com/bthall/mox/internal/tmux/tmuxtest"
)

func TestBuilder_SimpleSession(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a", "b", "c"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)

	if err := b.BuildSession(context.Background(), "dev", cfg.Sessions["dev"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}

	splits, creates := 0, 0
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "SplitPane ") {
			splits++
		}
		if strings.HasPrefix(c, "CreateSession ") {
			creates++
		}
	}
	if creates != 1 {
		t.Errorf("expected 1 CreateSession, got %d (calls: %v)", creates, tx.Calls)
	}
	if splits != 2 {
		t.Errorf("expected 2 SplitPane, got %d", splits)
	}

	for _, host := range []string{"a", "b", "c"} {
		want := "ssh " + host
		if !sentKey(tx, want) {
			t.Errorf("expected SendKeys with %q, not found", want)
		}
	}
}

func TestBuilder_CustomConnectTemplate(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"prod": {Connect: "mosh --port 60000 {{host}}", Hosts: []string{"jump1"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "prod", cfg.Sessions["prod"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "mosh --port 60000 jump1") {
		t.Errorf("expected mosh substitution, got %v", tx.KeysByPane)
	}
}

func TestBuilder_WindowConnectOverridesSession(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {
				Connect: "ssh -i sess {{host}}",
				Windows: []*config.Window{
					{Name: "w1", Connect: "ssh -i window {{host}}", Hosts: []string{"h1"}},
				},
			},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "ssh -i window h1") {
		t.Errorf("expected window-level connect to win, got %v", tx.KeysByPane)
	}
	if sentKey(tx, "ssh -i sess h1") {
		t.Errorf("session-level connect should not be used when window-level set")
	}
}

func TestBuilder_ComplexSessionWithLayout(t *testing.T) {
	cfg := &config.Config{
		Layouts: map[string]*config.Layout{
			"two-pane": {
				Name: "two-pane",
				Panes: []*config.Pane{
					{Split: config.SplitRoot, Commands: []string{"htop"}},
					{Split: config.SplitVertical, Size: 30, Commands: []string{"df -h"}},
				},
			},
		},
		Sessions: map[string]*config.Session{
			"mon": {Windows: []*config.Window{{Name: "system", Layout: "two-pane"}}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "mon", cfg.Sessions["mon"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	splits := 0
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "SplitPane ") {
			splits++
		}
	}
	if splits != 1 {
		t.Errorf("expected 1 SplitPane (root + 1 vertical), got %d (calls: %v)", splits, tx.Calls)
	}
	if !sentKey(tx, "htop") {
		t.Errorf("expected root pane to receive htop, got %v", tx.KeysByPane)
	}
	if !sentKey(tx, "df -h") {
		t.Errorf("expected new pane to receive df -h, got %v", tx.KeysByPane)
	}
}

func TestBuilder_ContextCancellation(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"dev": {Hosts: []string{"a", "b", "c", "d"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := b.BuildSession(ctx, "dev", cfg.Sessions["dev"]); err == nil {
		t.Fatal("expected context.Canceled, got nil")
	}
}

func TestBuilder_LiteralSubstitution(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {Hosts: []string{"host-1"}, Connect: "ssh user@{{host}}"},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "ssh user@host-1") {
		t.Errorf("expected literal substitution, got %v", tx.KeysByPane)
	}
}

func TestBuilder_SSHUserShortcut(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {SSHUser: "root", Hosts: []string{"a"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "ssh root@a") {
		t.Errorf("expected ssh root@a, got %v", tx.KeysByPane)
	}
}

func TestBuilder_ConnectBeatsSSHUser(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {SSHUser: "root", Connect: "ssh ops@{{host}}", Hosts: []string{"a"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "ssh ops@a") {
		t.Errorf("expected connect: to win over ssh_user:, got %v", tx.KeysByPane)
	}
}

func TestBuilder_ArrangeAndSync(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {Sync: true, Arrange: "tiled", Hosts: []string{"a", "b"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	saw := func(want string) bool {
		for _, c := range tx.Calls {
			if c == want {
				return true
			}
		}
		return false
	}
	if !saw("SelectLayout @1 tiled") {
		t.Errorf("expected SelectLayout @1 tiled, calls: %v", tx.Calls)
	}
	if !saw("SetWindowOption @1 synchronize-panes=on") {
		t.Errorf("expected synchronize-panes set, calls: %v", tx.Calls)
	}
}

func TestBuilder_WindowSyncOverridesSession(t *testing.T) {
	off := false
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"x": {
				Sync: true,
				Windows: []*config.Window{
					{Name: "w1", Hosts: []string{"a"}, Sync: &off},
				},
			},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "x", cfg.Sessions["x"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	for _, c := range tx.Calls {
		if strings.Contains(c, "synchronize-panes=on") {
			t.Errorf("window-level sync=false should override session sync=true, but saw: %v", tx.Calls)
		}
	}
}

func TestBuilder_BuildAdHocWindow(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("orph") // pretend parent session already exists
	// Also set up a window so FirstPaneID has something to find when split happens.
	// CreateWindow on fake auto-creates a first pane.
	b := tmux.NewBuilder(tx, cfg, nil)

	winID, err := b.BuildAdHocWindow(context.Background(), "orph", "scratch",
		&config.Session{Hosts: []string{"a", "b"}, Sync: true, Arrange: "tiled"})
	if err != nil {
		t.Fatalf("BuildAdHocWindow() error = %v", err)
	}
	if winID == "" {
		t.Error("expected non-empty window id")
	}
	saw := func(want string) bool {
		for _, c := range tx.Calls {
			if c == want {
				return true
			}
		}
		return false
	}
	if !saw("CreateWindow orph name=\"scratch\" dir=\"\"") {
		t.Errorf("expected CreateWindow call, got %v", tx.Calls)
	}
	if !saw("SetWindowOption " + winID + " synchronize-panes=on") {
		t.Errorf("expected sync on the new window, got %v", tx.Calls)
	}
	if !saw("SelectLayout " + winID + " tiled") {
		t.Errorf("expected tiled layout, got %v", tx.Calls)
	}
}

func TestBuilder_LocalSession_NoHosts(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"work": {Root: "/tmp"},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "work", cfg.Sessions["work"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	creates, splits := 0, 0
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "CreateSession ") {
			creates++
		}
		if strings.HasPrefix(c, "SplitPane ") {
			splits++
		}
	}
	if creates != 1 || splits != 0 {
		t.Errorf("local session should have 1 CreateSession + 0 splits, got creates=%d splits=%d (%v)",
			creates, splits, tx.Calls)
	}
	// No connect template should be sent — no ssh keystrokes
	for _, batches := range tx.KeysByPane {
		for _, batch := range batches {
			for _, c := range batch {
				if strings.HasPrefix(c, "ssh ") {
					t.Errorf("local session should not send ssh; got %q", c)
				}
			}
		}
	}
}

func TestBuilder_LocalSession_WithCommands(t *testing.T) {
	cfg := &config.Config{
		Sessions: map[string]*config.Session{
			"build": {Commands: []string{"make watch"}},
		},
	}
	tx := tmuxtest.NewFake()
	b := tmux.NewBuilder(tx, cfg, nil)
	if err := b.BuildSession(context.Background(), "build", cfg.Sessions["build"]); err != nil {
		t.Fatalf("BuildSession() error = %v", err)
	}
	if !sentKey(tx, "make watch") {
		t.Errorf("expected 'make watch' sent to default pane, got %v", tx.KeysByPane)
	}
}

func TestBuilder_AdHocWindow_NoHosts(t *testing.T) {
	cfg := &config.Config{Sessions: map[string]*config.Session{}}
	tx := tmuxtest.NewFake()
	tx.SetSession("parent")
	b := tmux.NewBuilder(tx, cfg, nil)
	winID, err := b.BuildAdHocWindow(context.Background(), "parent", "scratch", &config.Session{})
	if err != nil {
		t.Fatalf("BuildAdHocWindow() error = %v", err)
	}
	if winID == "" {
		t.Error("expected non-empty window id")
	}
	// No splits and no ssh keystrokes
	for _, c := range tx.Calls {
		if strings.HasPrefix(c, "SplitPane ") {
			t.Errorf("local ad-hoc window should not split; got %q", c)
		}
	}
}

func sentKey(f *tmuxtest.Fake, want string) bool {
	for _, batches := range f.KeysByPane {
		for _, batch := range batches {
			for _, c := range batch {
				if c == want {
					return true
				}
			}
		}
	}
	return false
}
