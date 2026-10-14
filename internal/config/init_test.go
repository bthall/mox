package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitWritesSchemaModeline(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	path, err := Init(false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffolded config: %v", err)
	}

	first := strings.SplitN(string(data), "\n", 2)[0]
	want := "# yaml-language-server: $schema=" + SchemaURL
	if first != want {
		t.Errorf("first line = %q, want %q", first, want)
	}

	// The modeline must not break the config for mox itself.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load scaffolded config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("scaffolded config invalid: %v", err)
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, err := Init(false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if _, err := Init(false); err == nil {
		t.Error("second Init without force should fail")
	}
	if _, err := Init(true); err != nil {
		t.Errorf("Init with force: %v", err)
	}
}

func TestInitForceReplacesTargetWithoutTruncatingOpenReaders(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	configDir := filepath.Join(base, "mox")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "original.yml")
	if err := os.WriteFile(target, []byte("# previous config\n"), 0o644); err != nil { //nolint:gosec // verifies that force replaces the old mode
		t.Fatal(err)
	}
	path := filepath.Join(configDir, "config.yml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Close() }()
	if _, err := Init(true); err != nil {
		t.Fatal(err)
	}
	oldData, err := io.ReadAll(old)
	if err != nil || string(oldData) != "# previous config\n" {
		t.Fatalf("open reader saw %q, %v", oldData, err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink was replaced: %v, %v", info, err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("replacement config is invalid: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("replacement mode = %v, %v; want 0600", info, err)
	}
}
