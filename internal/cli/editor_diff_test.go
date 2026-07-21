package cli

import (
	"strings"
	"testing"

	"github.com/bthall/mox/internal/config"
)

// renderDiffForTest flattens a diff into "±text" lines for compact assertions.
func renderDiffForTest(dls []diffLine) string {
	var b strings.Builder
	for _, dl := range dls {
		switch dl.kind {
		case diffDel:
			b.WriteString("-")
		case diffAdd:
			b.WriteString("+")
		default:
			b.WriteString(" ")
		}
		b.WriteString(dl.text + "\n")
	}
	return b.String()
}

// diffHasKind checks if a diff contains any line of the given kind.
func diffHasKind(dls []diffLine, k diffKind) bool {
	for _, dl := range dls {
		if dl.kind == k {
			return true
		}
	}
	return false
}

// diffHasKindWithText checks if a diff contains a line of the given kind that contains text.
func diffHasKindWithText(dls []diffLine, k diffKind, text string) bool {
	for _, dl := range dls {
		if dl.kind == k && strings.Contains(dl.text, text) {
			return true
		}
	}
	return false
}

func TestDiffLines(t *testing.T) {
	got := renderDiffForTest(diffLines(
		[]string{"a", "b", "c"},
		[]string{"a", "x", "c"},
	))
	want := " a\n-b\n+x\n c\n"
	if got != want {
		t.Fatalf("diff mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	// pure insert and pure delete
	if got := renderDiffForTest(diffLines(nil, []string{"n"})); got != "+n\n" {
		t.Fatalf("insert diff = %q", got)
	}
	if got := renderDiffForTest(diffLines([]string{"o"}, nil)); got != "-o\n" {
		t.Fatalf("delete diff = %q", got)
	}
}

func TestDraftDiffShapes(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)

	// edit: changed lines marked, unchanged lines context
	d := newDraft(st.cfg, "webfarm")
	d.sess.Sync = false
	dls := draftDiff(st.cfg, d)
	if !diffHasKindWithText(dls, diffDel, "sync") {
		t.Fatalf("edit diff missing removal of sync line:\n%s", renderDiffForTest(dls))
	}

	// delete: everything removed
	del := newDraft(st.cfg, "solo")
	del.deleted = true
	dls = draftDiff(st.cfg, del)
	if diffHasKind(dls, diffAdd) {
		t.Fatalf("delete diff has additions:\n%s", renderDiffForTest(dls))
	}
	if !diffHasKind(dls, diffDel) {
		t.Fatalf("delete diff has no removals:\n%s", renderDiffForTest(dls))
	}

	// add: everything added
	add := &sessionDraft{name: "extra", added: true, sess: &config.Session{Hosts: []string{"h1"}}}
	dls = draftDiff(st.cfg, add)
	if diffHasKind(dls, diffDel) {
		t.Fatalf("add diff has removals:\n%s", renderDiffForTest(dls))
	}
	if !diffHasKind(dls, diffAdd) || !diffHasKindWithText(dls, diffAdd, "extra") {
		t.Fatalf("add diff missing new block:\n%s", renderDiffForTest(dls))
	}
	// Assert the preview is truthful: YAML lists use block style with "-"
	if !diffHasKindWithText(dls, diffAdd, "- h1") {
		t.Fatalf("add diff should show block-style list marker (- h1):\n%s", renderDiffForTest(dls))
	}

	// rename: old key removed, new key added
	rn := newDraft(st.cfg, "webfarm")
	rn.name = "farm"
	dls = draftDiff(st.cfg, rn)
	if !diffHasKind(dls, diffDel) || !diffHasKind(dls, diffAdd) {
		t.Fatalf("rename diff missing both sides:\n%s", renderDiffForTest(dls))
	}
}
