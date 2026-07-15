package cli

import (
	"context"
	"os"
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
