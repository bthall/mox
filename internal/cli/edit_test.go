package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEditAndValidate_ValidConfig(t *testing.T) {
	path := writeTempConfig(t, "sessions:\n  dev:\n    hosts: [a, b]\n")

	var out bytes.Buffer
	// `true` exits 0 without touching the file — a no-op editor.
	if err := editAndValidate(path, "true", &out); err != nil {
		t.Fatalf("editAndValidate() error = %v", err)
	}
	if !strings.Contains(out.String(), "is valid (1 sessions") {
		t.Errorf("unexpected output: %q", out.String())
	}
}

func TestEditAndValidate_EditorFails(t *testing.T) {
	path := writeTempConfig(t, "sessions: {}\n")

	var out bytes.Buffer
	err := editAndValidate(path, "false", &out)
	if err == nil || !strings.Contains(err.Error(), "editor") {
		t.Errorf("want editor error, got %v", err)
	}
}

func TestEditAndValidate_InvalidConfigReported(t *testing.T) {
	path := writeTempConfig(t, "sessions:\n  dev:\n    hots: [a]\n") // typo key

	var out bytes.Buffer
	err := editAndValidate(path, "true", &out)
	if err == nil || !strings.Contains(err.Error(), "saved, but") {
		t.Errorf("want validation error mentioning the save, got %v", err)
	}
}

func TestEditorCommand_EnvPrecedence(t *testing.T) {
	t.Setenv("VISUAL", "visual-editor")
	t.Setenv("EDITOR", "plain-editor")
	if got := editorCommand(); got != "visual-editor" {
		t.Errorf("VISUAL should win, got %q", got)
	}
	t.Setenv("VISUAL", "")
	if got := editorCommand(); got != "plain-editor" {
		t.Errorf("EDITOR should be next, got %q", got)
	}
}
