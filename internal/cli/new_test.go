package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
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

func TestNewValidate_SaveRequiresName(t *testing.T) {
	o := &newOpts{save: true}
	if err := o.validate(); err == nil {
		t.Error("--save without --name should error")
	}
	o.name = "web"
	if err := o.validate(); err != nil {
		t.Errorf("--save with --name should be valid, got: %v", err)
	}
}

func TestNewValidate_SaveConflicts(t *testing.T) {
	cases := []struct {
		name string
		o    newOpts
	}{
		{"save+temporary", newOpts{save: true, name: "x", temporary: true}},
		{"save+print", newOpts{save: true, name: "x", print: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.o.validate(); err == nil {
				t.Errorf("%s should error", tc.name)
			}
		})
	}
}

func TestSaveNewSession_WritesAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  existing:\n    hosts: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sess := &config.Session{
		Hosts:    []string{"db1", "db2"},
		Sync:     true,
		Arrange:  "tiled",
		Commands: []string{"sudo -i"},
	}
	if err := saveNewSession(cfgPath, "dbfarm", sess); err != nil {
		t.Fatalf("saveNewSession: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("saved config invalid: %v", err)
	}
	got, ok := cfg.GetSession("dbfarm")
	if !ok {
		t.Fatal("saved session not found in config")
	}
	if len(got.Hosts) != 2 || got.Hosts[0] != "db1" || !got.Sync || got.Arrange != "tiled" {
		t.Errorf("saved session did not round-trip: %+v", got)
	}
	if _, ok := cfg.GetSession("existing"); !ok {
		t.Error("existing session lost on save")
	}
}

func TestSaveNewSession_CollisionErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  web:\n    hosts: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := saveNewSession(cfgPath, "web", &config.Session{Hosts: []string{"b"}})
	if err == nil {
		t.Fatal("saving over existing session should error")
	}

	// The original definition must be untouched.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	got, _ := cfg.GetSession("web")
	if len(got.Hosts) != 1 || got.Hosts[0] != "a" {
		t.Errorf("collision overwrote existing session: %+v", got)
	}
}

func TestSaveNewSession_InvalidSessionRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	// Invalid: bad hostname characters for the default ssh template.
	err := saveNewSession(cfgPath, "bad", &config.Session{Hosts: []string{"host;rm -rf"}})
	if err == nil {
		t.Fatal("invalid session should not be saved")
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		t.Error("config file should not have been created for invalid session")
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
