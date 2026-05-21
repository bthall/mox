//go:build integration

package tmux

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
	if client.executable == "" {
		t.Error("client.executable is empty")
	}
}

func TestVersion(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	v, err := client.Version()
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if v == "" {
		t.Error("Version() returned empty string")
	}
	t.Logf("tmux version: %s", v)
}
