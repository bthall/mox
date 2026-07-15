package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAdd_NonTTYErrors(t *testing.T) {
	cmd := newAddCommand()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())

	err := runAdd(cmd, nil)
	if err == nil {
		t.Fatal("add without a terminal should error")
	}
	if !strings.Contains(err.Error(), "--save") {
		t.Errorf("error should point at the non-interactive alternative, got: %v", err)
	}
}

func TestRunAdd_InvalidConfigFailsFast(t *testing.T) {
	// A loadable-but-invalid config must abort the wizard up front — not
	// silently disable collision checks and fail after all input is typed.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  bad:\n    hosts: [a]\n    retry: 99\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newAddCommand()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.WithValue(context.Background(), ctxKeyOpts, &globalOpts{configPath: cfgPath}))

	err := runAdd(cmd, nil)
	if err == nil {
		t.Fatal("invalid config should abort mox add")
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error should surface the underlying validation failure, got: %v", err)
	}
}

func TestRunAdd_DevNullStdinErrors(t *testing.T) {
	// /dev/null is a character device but not a terminal; the friendly
	// error must fire instead of bubbletea's TTY failure.
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()

	cmd := newAddCommand()
	cmd.SetIn(devnull)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetContext(context.Background())

	err = runAdd(cmd, nil)
	if err == nil {
		t.Fatal("add with /dev/null stdin should error")
	}
	if !strings.Contains(err.Error(), "--save") {
		t.Errorf("error should point at the non-interactive alternative, got: %v", err)
	}
}
