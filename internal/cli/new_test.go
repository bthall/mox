package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInferAdHocName_ConfigSession(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  foo:\n    hosts: [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := inferAdHocName([]string{"@foo"}, cfgPath)
	if got != "foo" {
		t.Errorf("inferAdHocName(@foo) = %q, want %q", got, "foo")
	}
}

func TestInferAdHocName_UnknownRefFallsBack(t *testing.T) {
	// @unknown is not in any config — should fall back to tmp-<timestamp>
	got := inferAdHocName([]string{"@unknown"}, "")
	if !strings.HasPrefix(got, "tmp-") {
		t.Errorf("inferAdHocName(@unknown) = %q, want tmp- prefix", got)
	}
}

func TestInferAdHocName_MultipleArgsFallBack(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  foo:\n    hosts: [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Multiple args: even if one is a config ref, don't infer a name.
	got := inferAdHocName([]string{"@foo", "extra"}, cfgPath)
	if !strings.HasPrefix(got, "tmp-") {
		t.Errorf("inferAdHocName(@foo extra) = %q, want tmp- prefix", got)
	}
}

func TestInferAdHocName_LiteralHostFallsBack(t *testing.T) {
	got := inferAdHocName([]string{"host1", "host2"}, "")
	if !strings.HasPrefix(got, "tmp-") {
		t.Errorf("inferAdHocName(host1 host2) = %q, want tmp- prefix", got)
	}
}

func TestInferAdHocName_ClustersshOnlyFallsBack(t *testing.T) {
	// @name in clusterssh but NOT in mox config — should still use tmp- name.
	// (We only infer the name for mox-configured sessions, not clusterssh ones.)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  other:\n    hosts: [x]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := inferAdHocName([]string{"@notinconfig"}, cfgPath)
	if !strings.HasPrefix(got, "tmp-") {
		t.Errorf("inferAdHocName(@notinconfig) = %q, want tmp- prefix", got)
	}
}

func TestExcludeHosts(t *testing.T) {
	got, err := excludeHosts([]string{"a", "b", "c"}, []string{"b"})
	if err != nil {
		t.Fatalf("excludeHosts() error = %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("got %v, want [a c]", got)
	}
}

func TestExcludeHosts_UnmatchedIsError(t *testing.T) {
	_, err := excludeHosts([]string{"a", "b"}, []string{"nope"})
	if err == nil {
		t.Fatal("want error for exclusion that matches nothing")
	}
}

func TestExcludeHosts_DuplicateHostAllDropped(t *testing.T) {
	got, err := excludeHosts([]string{"a", "b", "a"}, []string{"a"})
	if err != nil {
		t.Fatalf("excludeHosts() error = %v", err)
	}
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("got %v, want [b]", got)
	}
}
