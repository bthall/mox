package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_StrictRejectsUnknownKeys(t *testing.T) {
	p := writeTemp(t, `
sessions:
  dev:
    hots: [a, b]
`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for unknown key 'hots', got nil")
	}
	if !strings.Contains(err.Error(), "field hots") && !strings.Contains(err.Error(), "not found in type") {
		t.Errorf("error should mention unknown field, got: %v", err)
	}
}

func TestLoad_ValidSimple(t *testing.T) {
	p := writeTemp(t, `
sessions:
  dev:
    hosts: [a, b]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(cfg.Sessions))
	}
	dev := cfg.Sessions["dev"]
	if !dev.IsSimple() || len(dev.Hosts) != 2 {
		t.Errorf("expected simple session with 2 hosts, got %+v", dev)
	}
}

func TestLoad_RejectsBadHostname(t *testing.T) {
	p := writeTemp(t, `
sessions:
  dev:
    hosts: ["host; rm -rf /"]
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "unsafe to pass") {
		t.Errorf("expected unsafe-host error, got: %v", err)
	}
}

func TestLoad_RejectsReservedNameChar(t *testing.T) {
	p := writeTemp(t, `
sessions:
  "bad:name":
    hosts: [a]
`)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "reserved character") {
		t.Errorf("expected reserved-character error, got: %v", err)
	}
}

func TestDefaultConfigPath_RespectXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	got := DefaultConfigPath()
	if got != "/tmp/xdg-test/mox/config.yml" {
		t.Errorf("expected XDG path, got %q", got)
	}
}

func TestResolvePath_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := ResolvePath("~/foo"); got != filepath.Join(home, "foo") {
		t.Errorf("expected %s, got %s", filepath.Join(home, "foo"), got)
	}
	if got := ResolvePath("~"); got != home {
		t.Errorf("expected %s, got %s", home, got)
	}
	if got := ResolvePath("/abs/path"); got != "/abs/path" {
		t.Errorf("absolute path should pass through, got %s", got)
	}
}

func TestExists(t *testing.T) {
	p := writeTemp(t, `sessions:
  dev:
    hosts: [a]
`)
	if !Exists(p) {
		t.Errorf("Exists(%q) = false, want true", p)
	}
	if Exists("/nonexistent/path/to/nothing.yml") {
		t.Errorf("Exists(nonexistent) = true, want false")
	}
}
