package cli

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --- key helpers shared by all editor UI tests ---

func edKey(t *testing.T, m editorModel, msg tea.KeyMsg) editorModel {
	t.Helper()
	nm, _ := m.Update(msg)
	out, ok := nm.(editorModel)
	if !ok {
		t.Fatalf("Update returned %T, want editorModel", nm)
	}
	return out
}

func edRunes(t *testing.T, m editorModel, s string) editorModel {
	t.Helper()
	for _, r := range s {
		m = edKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func edType(t *testing.T, m editorModel, kt tea.KeyType) editorModel {
	t.Helper()
	return edKey(t, m, tea.KeyMsg{Type: kt})
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func testEditorModel(t *testing.T) editorModel {
	t.Helper()
	st := testEditorState(t, editorFixtureYAML)
	m := newEditorModel(st, nil, nil, "")
	m.width, m.height = 100, 30
	return m
}

// --- tests ---

func TestEditorSkeletonView(t *testing.T) {
	m := testEditorModel(t)
	out := m.View()
	for _, want := range []string{"solo", "webfarm", "hosts", "connect"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	// sorted names: solo before webfarm → solo selected first
	if !strings.Contains(m.formTitle(), "solo") {
		t.Errorf("form title = %q, want solo selected", m.formTitle())
	}
}

func TestEditorListNavigation(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j")
	if got := m.selectedName(); got != "webfarm" {
		t.Fatalf("after j: selected %q, want webfarm", got)
	}
	// draft follows the selection
	if m.draft == nil || m.draft.name != "webfarm" {
		t.Fatalf("draft = %+v, want webfarm", m.draft)
	}
	m = edType(t, m, tea.KeyUp)
	if got := m.selectedName(); got != "solo" {
		t.Fatalf("after up: selected %q, want solo", got)
	}
}

func TestEditorInitialSession(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	m := newEditorModel(st, nil, nil, "webfarm")
	if m.selectedName() != "webfarm" {
		t.Fatalf("initial selection %q, want webfarm", m.selectedName())
	}
	if m.pane != paneForm {
		t.Fatal("initial session should focus the form pane")
	}
}

func TestEditorPaneSwitchAndFormNav(t *testing.T) {
	m := testEditorModel(t)
	if m.pane != paneList {
		t.Fatal("start pane should be the list")
	}
	m = edType(t, m, tea.KeyTab)
	if m.pane != paneForm {
		t.Fatal("tab did not switch to form")
	}
	before := m.fieldSel
	m = edRunes(t, m, "j")
	if m.fieldSel != before+1 {
		t.Fatalf("fieldSel = %d, want %d", m.fieldSel, before+1)
	}
	// help text of the focused field is rendered
	if !strings.Contains(m.View(), m.fields[m.fieldSel].help[:20]) {
		t.Error("focused field help not rendered")
	}
}

func TestEditorFilter(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "/")
	if m.mode != modeFilter {
		t.Fatal("/ did not enter filter mode")
	}
	m = edRunes(t, m, "web")
	if len(m.visible) != 1 || m.visible[0] != "webfarm" {
		t.Fatalf("filter web → %v, want [webfarm]", m.visible)
	}
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse || len(m.visible) != 2 {
		t.Fatal("esc did not clear the filter")
	}
}

func TestEditorQuit(t *testing.T) {
	m := testEditorModel(t)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	_ = nm
	if !isQuit(cmd) {
		t.Fatal("q did not quit")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !isQuit(cmd) {
		t.Fatal("ctrl+c did not quit")
	}
}

func TestEditorNarrowView(t *testing.T) {
	m := testEditorModel(t)
	m.width = 40
	out := m.View()
	if !strings.Contains(out, "sessions") {
		t.Fatalf("narrow view missing list pane:\n%s", out)
	}
	m = edType(t, m, tea.KeyTab)
	out = m.View()
	if !strings.Contains(out, "connect") {
		t.Fatalf("narrow view after tab missing form:\n%s", out)
	}
}

func TestEditorEmptyFilterAndNavGuards(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "/")
	m = edRunes(t, m, "zzz")
	if len(m.visible) != 0 {
		t.Fatalf("visible = %v, want none", m.visible)
	}
	m = edType(t, m, tea.KeyEnter) // back to browse with empty filter result
	m = edRunes(t, m, "j")         // must not panic
	m = edRunes(t, m, "k")
	if !strings.Contains(m.View(), "no match") {
		t.Fatal("empty-match placeholder missing")
	}
}

func TestEditorScrolledFilterShowsSelection(t *testing.T) {
	var b strings.Builder
	b.WriteString("sessions:\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "    sess%02d:\n        root: /tmp\n", i)
	}
	st := testEditorState(t, b.String())
	m := newEditorModel(st, nil, nil, "")
	m.width, m.height = 100, 20
	for i := 0; i < 39; i++ {
		m = edRunes(t, m, "j") // scroll to the bottom
	}
	m = edRunes(t, m, "/")
	m = edRunes(t, m, "sess03")
	if len(m.visible) != 1 {
		t.Fatalf("visible = %v", m.visible)
	}
	if !strings.Contains(m.View(), "sess03") {
		t.Fatal("filtered selection not visible after scrolling (stale offset)")
	}
}

func TestEditorOverflowIndicator(t *testing.T) {
	var b strings.Builder
	b.WriteString("sessions:\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "    sess%02d:\n        root: /tmp\n", i)
	}
	st := testEditorState(t, b.String())
	m := newEditorModel(st, nil, nil, "")
	m.width, m.height = 100, 20
	if !strings.Contains(m.View(), "more") {
		t.Fatal("overflow indicator not rendered with 40 sessions in a 20-row terminal")
	}
}

// TestEditorSelectionAlwaysVisible walks the cursor through a list that
// overflows the pane and asserts the selected row is rendered at every
// position — pinning the scroll math shared by keepVisible and listLines
// (the reserved overflow-indicator row once hid the selection).
func TestEditorSelectionAlwaysVisible(t *testing.T) {
	var b strings.Builder
	b.WriteString("sessions:\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "    sess%02d:\n        root: /tmp/x\n", i)
	}
	st := testEditorState(t, b.String())
	m := newEditorModel(st, nil, nil, "")
	m.width, m.height = 100, 20
	for i := 0; i < 39; i++ {
		m = edRunes(t, m, "j")
		if !strings.Contains(m.View(), "▌ ○ "+m.selectedName()) {
			t.Fatalf("step %d: selected %q hidden (sel=%d offset=%d)", i, m.selectedName(), m.sel, m.offset)
		}
	}
	// and back up
	for i := 0; i < 39; i++ {
		m = edRunes(t, m, "k")
		if !strings.Contains(m.View(), "▌ ○ "+m.selectedName()) {
			t.Fatalf("up step %d: selected %q hidden (sel=%d offset=%d)", i, m.selectedName(), m.sel, m.offset)
		}
	}
}
