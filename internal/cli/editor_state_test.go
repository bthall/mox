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
	if err := os.WriteFile(p, []byte("sessions:\n    x:\n        retry: -3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
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

// Regression test for C1: rollback on failed writes.
func TestApplyDraftRollbackOnWriteFailure(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	d := newDraft(st.cfg, "webfarm")
	d.name = "renamed"
	d.sess.Sync = false

	// Make the directory read-only to force the write to fail.
	dir := filepath.Dir(st.path)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // deliberately unwritable to force the save to fail
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // restore the temp dir so cleanup can remove it
			t.Errorf("cleanup chmod: %v", err)
		}
	})

	// Write should fail; the failed rename should not be in the node tree.
	if err := st.applyDraft(d); err == nil {
		t.Fatal("expected write to fail with read-only dir")
	}

	// Restore permissions to verify the file is unchanged.
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // restore the temp dir for the assertions below
		t.Fatal(err)
	}

	// Save a clean draft; the failed rename should not appear.
	d2 := newDraft(st.cfg, "webfarm")
	if err := st.applyDraft(d2); err != nil {
		t.Fatalf("clean save failed: %v", err)
	}
	data, _ := os.ReadFile(st.path)
	if strings.Contains(string(data), "renamed:") {
		t.Fatalf("failed rename leaked into file:\n%s", data)
	}

	// Verify the file loads cleanly (no duplicate keys).
	if _, err := config.Load(st.path); err != nil {
		t.Fatalf("file not valid after rollback: %v", err)
	}

	// Retry the original rename draft; it should succeed now.
	d3 := newDraft(st.cfg, "webfarm")
	d3.name = "renamed"
	d3.sess.Sync = false
	if err := st.applyDraft(d3); err != nil {
		t.Fatalf("retry rename: %v", err)
	}
	data, _ = os.ReadFile(st.path)
	if !strings.Contains(string(data), "renamed:") {
		t.Fatalf("retry rename not applied:\n%s", data)
	}
}

// Regression test for I1: collision guard.
func TestApplyDraftRejectsDuplicateNames(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)

	// Rename webfarm to solo (collides with existing).
	d := newDraft(st.cfg, "webfarm")
	d.name = "solo"
	err := st.applyDraft(d)
	if err == nil {
		t.Fatal("should reject rename that collides with existing session")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("wrong error: %v", err)
	}

	// File should be unchanged.
	data, _ := os.ReadFile(st.path)
	if !strings.Contains(string(data), "webfarm:") {
		t.Fatalf("file was modified despite collision error:\n%s", data)
	}

	// Added draft with duplicate name should also be rejected.
	add := &sessionDraft{name: "solo", added: true, sess: &config.Session{Hosts: []string{"h1"}}}
	if err := st.applyDraft(add); err == nil {
		t.Fatal("should reject added session with existing name")
	}

	// File should still be unchanged.
	data, _ = os.ReadFile(st.path)
	if !strings.Contains(string(data), "webfarm:") {
		t.Fatalf("file was modified despite added collision error:\n%s", data)
	}
}

// Regression test for I3: draft mutations don't leak into st.cfg.
func TestApplyDraftNoAliasAfterSave(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	d := newDraft(st.cfg, "webfarm")
	d.sess.Sync = false

	if err := st.applyDraft(d); err != nil {
		t.Fatalf("applyDraft: %v", err)
	}

	// After save, st.cfg.Sessions[d.name] should be a separate object.
	if st.cfg.Sessions["webfarm"] == d.sess {
		t.Fatal("post-save aliasing: st.cfg.Sessions[d.name] is d.sess")
	}

	// Mutating d.sess should not affect st.cfg.
	d.sess.Hosts = append(d.sess.Hosts, "extra-host")
	if sameSessionYAML(d.sess, st.cfg.Sessions["webfarm"]) {
		t.Fatal("draft mutation leaked into st.cfg after save")
	}

	// dirty() should now report true since d has diverged from st.cfg.
	if !d.dirty(st.cfg) {
		t.Fatal("d.dirty() should be true after mutating d.sess post-save")
	}
}

// Regression test for M1: pure rename preserves flow-style and comments.
func TestApplyDraftPureRenamePreservesFormat(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)

	// Pure rename (no content changes).
	d := newDraft(st.cfg, "webfarm")
	d.name = "farm"

	if err := st.applyDraft(d); err != nil {
		t.Fatalf("rename: %v", err)
	}

	data, _ := os.ReadFile(st.path)
	out := string(data)

	// Key name should be updated.
	if !strings.Contains(out, "farm:") || strings.Contains(out, "webfarm:") {
		t.Fatalf("rename not applied:\n%s", out)
	}

	// Flow-style hosts should be preserved.
	if !strings.Contains(out, "[web1, web2]") {
		t.Errorf("flow-style hosts not preserved:\n%s", out)
	}

	// Key comment should survive.
	if !strings.Contains(out, "# webfarm comment") {
		t.Errorf("key comment lost:\n%s", out)
	}

	// File should still load.
	if _, err := config.Load(st.path); err != nil {
		t.Fatalf("saved config invalid: %v", err)
	}
}
