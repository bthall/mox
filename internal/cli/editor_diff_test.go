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
	out := renderDiffForTest(draftDiff(st.cfg, d))
	if !strings.Contains(out, "-") || !strings.Contains(out, "sync") {
		t.Fatalf("edit diff missing removal of sync line:\n%s", out)
	}

	// delete: everything removed
	del := newDraft(st.cfg, "solo")
	del.deleted = true
	out = renderDiffForTest(draftDiff(st.cfg, del))
	if strings.Contains(out, "+") {
		t.Fatalf("delete diff has additions:\n%s", out)
	}
	if !strings.Contains(out, "-") {
		t.Fatalf("delete diff has no removals:\n%s", out)
	}

	// add: everything added
	add := &sessionDraft{name: "extra", added: true, sess: &config.Session{Hosts: []string{"h1"}}}
	out = renderDiffForTest(draftDiff(st.cfg, add))
	if strings.Contains(out, "-") {
		t.Fatalf("add diff has removals:\n%s", out)
	}
	if !strings.Contains(out, "+") || !strings.Contains(out, "extra") {
		t.Fatalf("add diff missing new block:\n%s", out)
	}

	// rename: old key removed, new key added
	rn := newDraft(st.cfg, "webfarm")
	rn.name = "farm"
	out = renderDiffForTest(draftDiff(st.cfg, rn))
	if !strings.Contains(out, "-") || !strings.Contains(out, "+") {
		t.Fatalf("rename diff missing both sides:\n%s", out)
	}
}
