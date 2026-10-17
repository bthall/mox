package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRootCommand_PositionalAttach(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  work:\n    hosts: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--print", "-c", cfgPath, "work"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("mox work --print failed: %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Error("expected dry-run output for positional attachment, got empty")
	}
}

func TestAttachSubcommand(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  work:\n    hosts: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{"attach", "a"} {
		t.Run(sub, func(t *testing.T) {
			cmd := NewRootCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"--print", "-c", cfgPath, sub, "work"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("mox %s work --print failed: %v", sub, err)
			}

			out := buf.String()
			if out == "" {
				t.Errorf("expected dry-run output for mox %s work --print, got empty", sub)
			}
		})
	}
}

func TestSubcommandAliases(t *testing.T) {
	root := NewRootCommand()

	aliases := map[string][]string{
		"attach": {"a"},
		"edit":   {"e"},
		"kill":   {"k", "rm"},
		"list":   {"ls", "l"},
		"import": {"i", "imp"},
		"new":    {"n", "cssh"},
	}

	for cmdName, expectedAliases := range aliases {
		cmd, _, err := root.Find([]string{cmdName})
		if err != nil || cmd == nil {
			t.Fatalf("command %q not found", cmdName)
		}
		for _, want := range expectedAliases {
			found := false
			for _, got := range cmd.Aliases {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("command %q missing alias %q (got %v)", cmdName, want, cmd.Aliases)
			}
		}
	}
}
