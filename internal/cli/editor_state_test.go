package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bthall/mox/internal/config"
)

// editorFixtureYAML has comments everywhere the editor must preserve them:
// file header, above an untouched session, and inside an untouched session.
const editorFixtureYAML = `# yaml-language-server: $schema=https://example.invalid/mox.json
# file header comment
sessions:
    # webfarm comment
    webfarm:
        hosts: [web1, web2]
        sync: true
    solo:
        # solo inner comment
        root: /tmp/solo
`

func testEditorState(t *testing.T, body string) *editorState {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := loadEditorState(p)
	if err != nil {
		t.Fatalf("loadEditorState: %v", err)
	}
	return st
}

func TestLoadEditorState(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	if len(st.cfg.Sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(st.cfg.Sessions))
	}
	if st.mtime.IsZero() {
		t.Fatal("mtime not recorded")
	}

	// invalid config refuses to load
	p := filepath.Join(t.TempDir(), "bad.yml")
	os.WriteFile(p, []byte("sessions:\n    x:\n        retry: -3\n"), 0o600)
	if _, err := loadEditorState(p); err == nil {
		t.Fatal("loadEditorState accepted an invalid config")
	}
}

func TestDraftDirty(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	d := newDraft(st.cfg, "webfarm")
	if d.dirty(st.cfg) {
		t.Fatal("fresh draft reports dirty")
	}
	d.sess.Sync = false
	if !d.dirty(st.cfg) {
		t.Fatal("edited draft reports clean")
	}
	// mutating the draft must not touch the loaded config
	if !st.cfg.Sessions["webfarm"].Sync {
		t.Fatal("draft mutation leaked into loaded config")
	}

	d2 := newDraft(st.cfg, "solo")
	d2.name = "solo2"
	if !d2.dirty(st.cfg) {
		t.Fatal("renamed draft reports clean")
	}
	d3 := newDraft(st.cfg, "solo")
	d3.deleted = true
	if !d3.dirty(st.cfg) {
		t.Fatal("deleted draft reports clean")
	}
}

func TestApplyDraftEdit(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	d := newDraft(st.cfg, "webfarm")
	d.sess.Sync = false
	d.sess.Hosts = []string{"web1", "web2", "web3"}

	if err := st.applyDraft(d); err != nil {
		t.Fatalf("applyDraft: %v", err)
	}

	data, _ := os.ReadFile(st.path)
	out := string(data)
	for _, want := range []string{
		"# file header comment",
		"# webfarm comment",
		"# solo inner comment",
		"root: /tmp/solo",
		"web3",
		"yaml-language-server",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("saved file missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sync: true") {
		t.Errorf("stale sync value survived:\n%s", out)
	}
	// typed view synced
	if st.cfg.Sessions["webfarm"].Sync {
		t.Fatal("st.cfg not updated after save")
	}
	// file still loads through the strict loader
	if _, err := config.Load(st.path); err != nil {
		t.Fatalf("saved config invalid: %v", err)
	}
}

func TestApplyDraftRenameDeleteAdd(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)

	// rename keeps position + key comment
	d := newDraft(st.cfg, "webfarm")
	d.name = "farm"
	if err := st.applyDraft(d); err != nil {
		t.Fatalf("rename: %v", err)
	}
	data, _ := os.ReadFile(st.path)
	if !strings.Contains(string(data), "farm:") || strings.Contains(string(data), "webfarm:") {
		t.Fatalf("rename not applied:\n%s", data)
	}
	if !strings.Contains(string(data), "# webfarm comment") {
		t.Fatalf("key comment lost on rename:\n%s", data)
	}

	// add
	add := &sessionDraft{name: "extra", added: true, sess: &config.Session{Hosts: []string{"h1"}}}
	if err := st.applyDraft(add); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, ok := st.cfg.Sessions["extra"]; !ok {
		t.Fatal("added session missing from typed config")
	}

	// delete
	del := newDraft(st.cfg, "solo")
	del.deleted = true
	if err := st.applyDraft(del); err != nil {
		t.Fatalf("delete: %v", err)
	}
	data, _ = os.ReadFile(st.path)
	if strings.Contains(string(data), "solo:") {
		t.Fatalf("delete not applied:\n%s", data)
	}
	if _, err := config.Load(st.path); err != nil {
		t.Fatalf("saved config invalid after delete: %v", err)
	}
}

func TestApplyDraftValidationBlocksWrite(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	before, _ := os.ReadFile(st.path)

	d := newDraft(st.cfg, "webfarm")
	d.sess.Retry = 99 // invalid: retry must be 0-10
	if err := st.applyDraft(d); err == nil {
		t.Fatal("applyDraft accepted invalid draft")
	}
	after, _ := os.ReadFile(st.path)
	if string(before) != string(after) {
		t.Fatal("file changed despite validation failure")
	}
}

func TestApplyDraftStaleBlocksWrite(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	// simulate an external write: bump mtime well past the recorded one
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(st.path, future, future); err != nil {
		t.Fatal(err)
	}
	d := newDraft(st.cfg, "webfarm")
	d.sess.Sync = false
	err := st.applyDraft(d)
	if err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("applyDraft = %v, want stale-config error", err)
	}
}
