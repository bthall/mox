package tmux

import (
	"bytes"
	"strings"
	"testing"
)

func TestDryRunOperationsOptionTerminator(t *testing.T) {
	var buf bytes.Buffer
	client := NewDryRun(&buf)

	if err := client.SendKeys("%1", []string{"-t", "-R", "--help"}); err != nil {
		t.Fatalf("SendKeys error = %v", err)
	}
	if err := client.RenameWindow("@1", "-mywin"); err != nil {
		t.Fatalf("RenameWindow error = %v", err)
	}
	if err := client.SelectLayout("@1", "tiled"); err != nil {
		t.Fatalf("SelectLayout error = %v", err)
	}

	got := buf.String()
	wantLines := []string{
		"tmux send-keys -t %1 -- -t C-m",
		"tmux send-keys -t %1 -- -R C-m",
		"tmux send-keys -t %1 -- --help C-m",
		"tmux rename-window -t @1 -- -mywin",
		"tmux select-layout -t @1 -- tiled",
	}

	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q, got:\n%s", want, got)
		}
	}
}
