package config

import (
	"os"
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
