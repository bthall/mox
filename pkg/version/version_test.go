package version

import (
	"strings"
	"testing"
)

func TestString_DevBuild(t *testing.T) {
	saveV, saveC, saveD := Version, GitCommit, BuildDate
	t.Cleanup(func() { Version, GitCommit, BuildDate = saveV, saveC, saveD })

	Version = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"

	got := String()
	if !strings.Contains(got, "dev") {
		t.Errorf("expected dev string, got %q", got)
	}
	if strings.HasPrefix(got, "mox ") {
		t.Errorf("String() should not include binary name (cobra adds it), got %q", got)
	}
}

func TestString_Tagged(t *testing.T) {
	saveV, saveC, saveD := Version, GitCommit, BuildDate
	t.Cleanup(func() { Version, GitCommit, BuildDate = saveV, saveC, saveD })

	Version = "1.2.3"
	GitCommit = "abcdef1234567890"
	BuildDate = "2026-05-06T00:00:00Z"

	got := String()
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("expected version, got %q", got)
	}
	if !strings.Contains(got, "abcdef1") {
		t.Errorf("expected short SHA in %q", got)
	}
	if strings.Contains(got, "abcdef1234567890") {
		t.Errorf("expected SHA truncated, got %q", got)
	}
}

func TestInfo(t *testing.T) {
	bi := Info()
	if bi.Version != Version || bi.GitCommit != GitCommit || bi.BuildDate != BuildDate {
		t.Errorf("Info() did not match package vars: %+v", bi)
	}
}
