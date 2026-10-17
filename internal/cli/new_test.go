package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
)

func TestNewCommand_ExplicitNamePreservesHosts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("directory-host", 0o750); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  configured-host:\n    hosts: [expanded-host]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		host string
	}{
		{"root flag", []string{"-r", dir, "root-host"}, "root-host"},
		{"directory collision", []string{"directory-host"}, "directory-host"},
		{"config collision", []string{"configured-host"}, "configured-host"},
		{"explicit cluster", []string{"@configured-host"}, "expanded-host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"-n", "scratch"}, tc.args...)
			out := newCommandDryRun(t, cfgPath, args...)
			if !strings.Contains(out, "new-session -d -s scratch ") || !strings.Contains(out, "ssh "+tc.host+";") {
				t.Fatalf("expected SSH to %s in session scratch, got:\n%s", tc.host, out)
			}
			if tc.host != "expanded-host" && strings.Contains(out, "ssh expanded-host;") {
				t.Fatalf("literal host was expanded as a cluster:\n%s", out)
			}
		})
	}
}

func TestNewCommand_LocalNameInference(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "my-project")
	if err := os.Mkdir(project, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	cfgPath := filepath.Join(dir, "missing.yml")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"current directory", []string{"."}, "my-project"},
		{"parent directory", []string{".."}, filepath.Base(dir)},
		{"absolute directory", []string{project}, "my-project"},
		{"name with root", []string{"custom-name", "-r", project}, "custom-name"},
		{"README named local session", []string{"-n", "my-project"}, "my-project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := newCommandDryRun(t, cfgPath, tc.args...)
			if !strings.Contains(out, "new-session -d -s "+tc.want+" ") || strings.Contains(out, "ssh ") {
				t.Fatalf("expected local session %s, got:\n%s", tc.want, out)
			}
		})
	}
}

func TestNewCommand_LiteralHost(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	out := newCommandDryRun(t, filepath.Join(dir, "missing.yml"), "server.example.test")
	if !strings.Contains(out, "new-session -d -s tmp-") || !strings.Contains(out, "ssh server.example.test;") {
		t.Fatalf("expected a literal SSH host with a generated name, got:\n%s", out)
	}
}

func newCommandDryRun(t *testing.T, configPath string, args ...string) string {
	t.Helper()
	cmd := NewRootCommand()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"-c", configPath, "new", "--print"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mox new %v --print: %v; output:\n%s", args, err, buf)
	}
	return buf.String()
}

func TestInferAdHocName_ConfigSession(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  foo:\n    hosts: [a, b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got := inferAdHocName([]string{"@foo"}, nil, cfgPath)
	if got != "foo" {
		t.Errorf("inferAdHocName(@foo) = %q, want %q", got, "foo")
	}

	gotUnprefixed := inferAdHocName([]string{"foo"}, nil, cfgPath)
	if gotUnprefixed != "foo" {
		t.Errorf("inferAdHocName(foo) = %q, want %q", gotUnprefixed, "foo")
	}
}

func TestInferAdHocName_Directory(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, "my-project")
	if err := os.Mkdir(projDir, 0o750); err != nil {
		t.Fatal(err)
	}

	got := inferAdHocName([]string{projDir}, nil, "")
	if got != "my-project" {
		t.Errorf("inferAdHocName(%s) = %q, want %q", projDir, got, "my-project")
	}
}

func TestInferAdHocName_WithRootFlag(t *testing.T) {
	o := &newOpts{root: "~/my-root"}
	got := inferAdHocName([]string{"custom-name"}, o, "")
	if got != "custom-name" {
		t.Errorf("inferAdHocName(custom-name with root) = %q, want %q", got, "custom-name")
	}
}

func TestInferAdHocName_UnknownRefFallsBack(t *testing.T) {
	// @unknown is not in any config — should fall back to tmp-<timestamp>
	got := inferAdHocName([]string{"@unknown"}, nil, "")
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
	got := inferAdHocName([]string{"@foo", "extra"}, nil, cfgPath)
	if !strings.HasPrefix(got, "tmp-") {
		t.Errorf("inferAdHocName(@foo extra) = %q, want tmp- prefix", got)
	}
}

func TestInferAdHocName_LiteralHostFallsBack(t *testing.T) {
	got := inferAdHocName([]string{"host1", "host2"}, nil, "")
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

	got := inferAdHocName([]string{"@notinconfig"}, nil, cfgPath)
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

func TestSaveNewSession_IdenticalResaveIsIdempotent(t *testing.T) {
	// A failed `mox new --save` retry must not be blocked by the entry the
	// first attempt already wrote.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")

	sess := &config.Session{Hosts: []string{"db1"}, Sync: true}
	if err := saveNewSession(cfgPath, "web", sess); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := saveNewSession(cfgPath, "web", &config.Session{Hosts: []string{"db1"}, Sync: true}); err != nil {
		t.Errorf("re-saving an identical definition should succeed, got: %v", err)
	}
}

func TestSaveNewSession_DifferingCollisionErrorIsActionable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(cfgPath, []byte("sessions:\n  web:\n    hosts: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := saveNewSession(cfgPath, "web", &config.Session{Hosts: []string{"b"}})
	if err == nil {
		t.Fatal("differing definition must not overwrite")
	}
	// `mox new` has no flag that can force this; the advice must not name one.
	if strings.Contains(err.Error(), "--force") {
		t.Errorf("error suggests --force, which cannot work here: %v", err)
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
