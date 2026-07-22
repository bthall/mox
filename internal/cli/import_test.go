package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
)

func TestParseSSHDest(t *testing.T) {
	tests := []struct {
		name      string
		argv      []string
		wantUser  string
		wantHost  string
		wantPlain bool
	}{
		{"bare host", []string{"ssh", "web-1.example.com"}, "", "web-1.example.com", true},
		{"user@host", []string{"ssh", "deploy@web-1"}, "deploy", "web-1", true},
		{"absolute ssh path", []string{"/usr/bin/ssh", "web-1"}, "", "web-1", true},
		{"with flag is not plain", []string{"ssh", "-p", "2222", "web-1"}, "", "", false},
		{"remote command is not plain", []string{"ssh", "web-1", "uptime"}, "", "", false},
		{"not ssh", []string{"vim", "file"}, "", "", false},
		{"no destination", []string{"ssh"}, "", "", false},
		{"flag-only dest rejected", []string{"ssh", "-v"}, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, h, plain := parseSSHDest(tt.argv)
			if u != tt.wantUser || h != tt.wantHost || plain != tt.wantPlain {
				t.Errorf("parseSSHDest(%v) = (%q,%q,%v), want (%q,%q,%v)",
					tt.argv, u, h, plain, tt.wantUser, tt.wantHost, tt.wantPlain)
			}
		})
	}
}

func TestResolveImportSource_ExplicitArg(t *testing.T) {
	got, err := resolveImportSource(nil, []string{"work"})
	if err != nil {
		t.Fatalf("resolveImportSource: %v", err)
	}
	if got != "work" {
		t.Errorf("source = %q, want work", got)
	}
}

func TestResolveImportSource_NoArgOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	client, err := tmux.NewClient()
	if err != nil {
		t.Skipf("tmux not installed: %v", err)
	}
	if _, err := resolveImportSource(client, nil); err == nil {
		t.Error("no-arg import outside tmux should error")
	}
}

func TestAppendSessionToConfig_FreshFileGetsModeline(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	err := appendSessionToConfig(cfgPath, "web", &config.Session{Hosts: []string{"a"}}, false)
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(data), "\n", 2)[0]
	if first != "# yaml-language-server: $schema="+config.SchemaURL {
		t.Errorf("fresh config should start with the schema modeline, got %q", first)
	}
	if _, err := config.Load(cfgPath); err != nil {
		t.Errorf("modeline must not break loading: %v", err)
	}
}

func TestAppendSessionToConfig_ExistingFileHeaderUntouched(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("# my config\nsessions:\n  a:\n    hosts: [x]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := appendSessionToConfig(cfgPath, "web", &config.Session{Hosts: []string{"b"}}, false); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# my config") {
		t.Errorf("existing header comment must survive, got: %q", strings.SplitN(string(data), "\n", 2)[0])
	}
	if strings.Contains(string(data), "yaml-language-server") {
		t.Error("modeline must not be injected into an existing config")
	}
}

func TestBuildWindow_UniformSSHFanout(t *testing.T) {
	// The reported bug: a window of plain ssh panes must import as simple-mode
	// hosts, not anonymous panes.
	w, _ := buildWindow("main", "", []capturedPane{
		{path: "/home/bthall", argv: []string{"ssh", "gateway-1.example.com"}},
		{path: "/home/bthall", argv: []string{"ssh", "gateway-2.example.com"}},
		{path: "/home/bthall", argv: []string{"ssh", "gateway-3.example.com"}},
	})
	if w.Panes != nil {
		t.Fatalf("uniform ssh fan-out should not produce panes, got %+v", w.Panes)
	}
	want := []string{"gateway-1.example.com", "gateway-2.example.com", "gateway-3.example.com"}
	if !reflect.DeepEqual(w.Hosts, want) {
		t.Errorf("hosts = %v, want %v", w.Hosts, want)
	}
	if w.Root != "/home/bthall" {
		t.Errorf("root = %q, want /home/bthall", w.Root)
	}
	if w.SSHUser != "" {
		t.Errorf("ssh_user = %q, want empty", w.SSHUser)
	}
}

func TestBuildWindow_UniformUser(t *testing.T) {
	w, _ := buildWindow("main", "", []capturedPane{
		{path: "/root", argv: []string{"ssh", "deploy@a"}},
		{path: "/root", argv: []string{"ssh", "deploy@b"}},
	})
	if w.SSHUser != "deploy" {
		t.Errorf("ssh_user = %q, want deploy", w.SSHUser)
	}
	if !reflect.DeepEqual(w.Hosts, []string{"a", "b"}) {
		t.Errorf("hosts = %v", w.Hosts)
	}
}

func TestBuildWindow_SingleSSHPane(t *testing.T) {
	// e.g. samba / pythia: a one-pane ssh window -> single host.
	w, _ := buildWindow("ssh", "", []capturedPane{
		{path: "/home/bthall", argv: []string{"ssh", "samba"}},
	})
	if !reflect.DeepEqual(w.Hosts, []string{"samba"}) {
		t.Errorf("hosts = %v, want [samba]", w.Hosts)
	}
	if w.Panes != nil {
		t.Errorf("should be simple-mode, got panes %+v", w.Panes)
	}
}

func TestBuildWindow_MixedUsersStaysComplex(t *testing.T) {
	// Differing users can't be one ssh_user, so fall back to panes, but the ssh
	// connections are recovered as commands so the import stays reproducible.
	w, _ := buildWindow("main", "", []capturedPane{
		{path: "/x", argv: []string{"ssh", "alice@a"}},
		{path: "/y", argv: []string{"ssh", "bob@b"}},
	})
	if w.Hosts != nil {
		t.Fatalf("mixed users should not become hosts, got %v", w.Hosts)
	}
	if len(w.Panes) != 2 {
		t.Fatalf("want 2 panes, got %d", len(w.Panes))
	}
	if w.Panes[0].Split != config.SplitRoot || w.Panes[1].Split != config.SplitHorizontal {
		t.Errorf("split types wrong: %+v", w.Panes)
	}
	if !reflect.DeepEqual(w.Panes[0].Commands, []string{"ssh alice@a"}) {
		t.Errorf("pane 0 commands = %v", w.Panes[0].Commands)
	}
	if w.Root != "" {
		t.Errorf("differing paths should leave root empty, got %q", w.Root)
	}
}

func TestBuildWindow_NonSSHStaysStructureOnly(t *testing.T) {
	// A local working window: no ssh anywhere -> anonymous panes, no commands
	// (we don't try to reproduce editors/REPLs).
	w, _ := buildWindow("dev", "", []capturedPane{
		{path: "/home/bthall/proj", argv: []string{"vim"}},
		{path: "/home/bthall/proj", argv: nil},
	})
	if w.Hosts != nil {
		t.Fatalf("non-ssh window should not become hosts")
	}
	if len(w.Panes) != 2 {
		t.Fatalf("want 2 panes, got %d", len(w.Panes))
	}
	for i, p := range w.Panes {
		if p.Commands != nil {
			t.Errorf("pane %d should have no recovered commands, got %v", i, p.Commands)
		}
	}
	if w.Root != "/home/bthall/proj" {
		t.Errorf("root = %q", w.Root)
	}
}

func TestBuildWindow_GeometryFromLayout(t *testing.T) {
	// Real-world layout: left pane full height, right side stacked.
	// Non-uniform panes -> complex mode with faithful splits and sizes.
	layout := "d67e,80x24,0,0{40x24,0,0,0,39x24,41,0[39x12,41,0,1,39x11,41,13,2]}"
	w, degraded := buildWindow("dev", layout, []capturedPane{
		{id: "%0", path: "/p", argv: []string{"vim"}},
		{id: "%1", path: "/p"},
		{id: "%2", path: "/p", argv: []string{"ssh", "-p", "2222", "web"}},
	})
	if degraded {
		t.Fatal("linearizable layout should not be degraded")
	}
	if len(w.Panes) != 3 {
		t.Fatalf("want 3 panes, got %d", len(w.Panes))
	}
	if w.Panes[0].Split != config.SplitRoot || w.Panes[0].Size != 0 {
		t.Errorf("pane 0 = %+v, want root without size", w.Panes[0])
	}
	if w.Panes[1].Split != config.SplitVertical || w.Panes[1].Size != 49 {
		t.Errorf("pane 1 = %+v, want vertical size 49", w.Panes[1])
	}
	if w.Panes[2].Split != config.SplitHorizontal || w.Panes[2].Size != 46 {
		t.Errorf("pane 2 = %+v, want horizontal size 46", w.Panes[2])
	}
	if !reflect.DeepEqual(w.Panes[2].Commands, []string{"ssh -p 2222 web"}) {
		t.Errorf("pane 2 commands = %v, want the ssh connection", w.Panes[2].Commands)
	}
}

func TestBuildWindow_GeometryFollowsChainOrder(t *testing.T) {
	// Captured pane order and layout chain order can disagree; commands must
	// stay attached to the right pane.
	layout := "aaaa,208x62,0,0{104x62,0,0,1,103x62,105,0,2}"
	w, degraded := buildWindow("main", layout, []capturedPane{
		{id: "%2", path: "/y", argv: []string{"ssh", "bob@b"}},
		{id: "%1", path: "/x", argv: []string{"ssh", "alice@a"}},
	})
	if degraded {
		t.Fatal("should not be degraded")
	}
	if !reflect.DeepEqual(w.Panes[0].Commands, []string{"ssh alice@a"}) {
		t.Errorf("pane 0 commands = %v, want alice's (chain starts at %%1)", w.Panes[0].Commands)
	}
	if !reflect.DeepEqual(w.Panes[1].Commands, []string{"ssh bob@b"}) {
		t.Errorf("pane 1 commands = %v, want bob's", w.Panes[1].Commands)
	}
}

func TestBuildWindow_UnparseableLayoutFallsBack(t *testing.T) {
	w, degraded := buildWindow("dev", "not-a-layout", []capturedPane{
		{id: "%0", path: "/p", argv: []string{"vim"}},
		{id: "%1", path: "/p"},
	})
	if !degraded {
		t.Error("unparseable layout with multiple panes should report degraded geometry")
	}
	if len(w.Panes) != 2 || w.Panes[0].Split != config.SplitRoot || w.Panes[1].Split != config.SplitHorizontal {
		t.Errorf("fallback structure wrong: %+v", w.Panes)
	}
}

func TestBuildWindow_NonLinearizableFallsBack(t *testing.T) {
	// {[a,b],c}: container in non-last position can't be replayed as
	// sequential splits of the previous pane.
	layout := "aaaa,80x24,0,0{40x24,0,0[40x12,0,0,0,40x11,0,13,1],39x24,41,0,2}"
	w, degraded := buildWindow("dev", layout, []capturedPane{
		{id: "%0", path: "/p"},
		{id: "%1", path: "/p"},
		{id: "%2", path: "/p"},
	})
	if !degraded {
		t.Error("non-linearizable layout should report degraded geometry")
	}
	if len(w.Panes) != 3 {
		t.Errorf("fallback should keep all panes, got %d", len(w.Panes))
	}
}

func TestBuildWindow_PaneIDMismatchFallsBack(t *testing.T) {
	// Layout referencing panes we didn't capture must not panic or misassign.
	layout := "aaaa,208x62,0,0{104x62,0,0,7,103x62,105,0,8}"
	w, degraded := buildWindow("dev", layout, []capturedPane{
		{id: "%0", path: "/p"},
		{id: "%1", path: "/p"},
	})
	if !degraded {
		t.Error("id mismatch should report degraded geometry")
	}
	if len(w.Panes) != 2 {
		t.Errorf("fallback should keep all panes, got %d", len(w.Panes))
	}
}

func TestBuildWindow_HostsModeIgnoresLayout(t *testing.T) {
	// A uniform ssh fan-out imports as hosts regardless of layout geometry.
	w, degraded := buildWindow("main", "not-a-layout", []capturedPane{
		{id: "%0", path: "/h", argv: []string{"ssh", "a"}},
		{id: "%1", path: "/h", argv: []string{"ssh", "b"}},
	})
	if degraded {
		t.Error("hosts-mode import has no geometry to lose")
	}
	if !reflect.DeepEqual(w.Hosts, []string{"a", "b"}) {
		t.Errorf("hosts = %v", w.Hosts)
	}
}

func TestBuildWindow_SingleLocalPaneNotDegraded(t *testing.T) {
	w, degraded := buildWindow("solo", "", []capturedPane{
		{id: "%0", path: "/p", argv: []string{"vim"}},
	})
	if degraded {
		t.Error("a single pane has no geometry to lose")
	}
	if len(w.Panes) != 1 {
		t.Errorf("want 1 pane, got %d", len(w.Panes))
	}
}

func TestBuildWindow_PartialSSHRecoversCommand(t *testing.T) {
	// One ssh pane + one local pane: stays complex, ssh pane keeps its command.
	w, _ := buildWindow("main", "", []capturedPane{
		{path: "/a", argv: []string{"ssh", "web-1"}},
		{path: "/a", argv: []string{"htop"}},
	})
	if w.Hosts != nil {
		t.Fatalf("should stay complex")
	}
	if !reflect.DeepEqual(w.Panes[0].Commands, []string{"ssh web-1"}) {
		t.Errorf("pane 0 commands = %v, want [ssh web-1]", w.Panes[0].Commands)
	}
	if w.Panes[1].Commands != nil {
		t.Errorf("pane 1 (htop) should have no command, got %v", w.Panes[1].Commands)
	}
}
