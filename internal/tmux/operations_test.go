//go:build integration

package tmux

import (
	"strings"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	const name = "mox-test-session"
	_ = client.KillSession(name)

	exists, err := client.SessionExists(name)
	if err != nil {
		t.Fatalf("SessionExists() error = %v", err)
	}
	if exists {
		t.Fatal("session exists before creation")
	}

	if err := client.CreateSession(name, "", ""); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	t.Cleanup(func() { _ = client.KillSession(name) })

	exists, err = client.SessionExists(name)
	if err != nil {
		t.Fatalf("SessionExists() error = %v", err)
	}
	if !exists {
		t.Fatal("session does not exist after creation")
	}

	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session %q not in list: %v", name, sessions)
	}
}

func TestWindowOperations(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	const name = "mox-test-windows"
	_ = client.KillSession(name)
	if err := client.CreateSession(name, "", "main"); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	t.Cleanup(func() { _ = client.KillSession(name) })

	winID, err := client.CreateWindow(name, "test-window", "")
	if err != nil {
		t.Fatalf("CreateWindow() error = %v", err)
	}
	if !strings.HasPrefix(winID, "@") {
		t.Errorf("window id should start with '@', got %q", winID)
	}
	if err := client.RenameWindow(winID, "renamed"); err != nil {
		t.Fatalf("RenameWindow() error = %v", err)
	}
}

func TestPaneOperations(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	const name = "mox-test-panes"
	_ = client.KillSession(name)
	if err := client.CreateSession(name, "", "main"); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	t.Cleanup(func() { _ = client.KillSession(name) })

	winID, err := client.FirstWindowID(name)
	if err != nil {
		t.Fatalf("FirstWindowID() error = %v", err)
	}
	paneID, err := client.FirstPaneID(winID)
	if err != nil {
		t.Fatalf("FirstPaneID() error = %v", err)
	}

	newID, err := client.SplitPane(paneID, SplitVertical, 50, "")
	if err != nil {
		t.Fatalf("SplitPane() error = %v", err)
	}
	if !strings.HasPrefix(newID, "%") {
		t.Errorf("pane id should start with '%%', got %q", newID)
	}
	if err := client.SendKeys(newID, []string{"echo test"}); err != nil {
		t.Fatalf("SendKeys() error = %v", err)
	}
}

func TestSessionExistsNoServer(t *testing.T) {
	// On a system with no tmux server (best-effort), SessionExists for a
	// non-existent session should return (false, nil) without error.
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	exists, err := client.SessionExists("mox-definitely-does-not-exist-xyz")
	if err != nil {
		t.Fatalf("SessionExists returned error for missing session: %v", err)
	}
	if exists {
		t.Errorf("SessionExists returned true for non-existent session")
	}
}
