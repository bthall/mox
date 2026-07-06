package cli

import (
	"reflect"
	"testing"

	"github.com/bthall/mox/internal/config"
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

func TestBuildWindow_UniformSSHFanout(t *testing.T) {
	// The reported bug: a window of plain ssh panes must import as simple-mode
	// hosts, not anonymous panes.
	w := buildWindow("main", []capturedPane{
		{path: "/home/bthall", argv: []string{"ssh", "apisix-1.example.com"}},
		{path: "/home/bthall", argv: []string{"ssh", "apisix-2.example.com"}},
		{path: "/home/bthall", argv: []string{"ssh", "apisix-3.example.com"}},
	})
	if w.Panes != nil {
		t.Fatalf("uniform ssh fan-out should not produce panes, got %+v", w.Panes)
	}
	want := []string{"apisix-1.example.com", "apisix-2.example.com", "apisix-3.example.com"}
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
	w := buildWindow("main", []capturedPane{
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
	w := buildWindow("ssh", []capturedPane{
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
	w := buildWindow("main", []capturedPane{
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
	w := buildWindow("dev", []capturedPane{
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

func TestBuildWindow_PartialSSHRecoversCommand(t *testing.T) {
	// One ssh pane + one local pane: stays complex, ssh pane keeps its command.
	w := buildWindow("main", []capturedPane{
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
