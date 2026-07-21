package cli

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

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

// focusField moves the form cursor onto the field with the given key.
func focusField(t *testing.T, m editorModel, key string) editorModel {
	t.Helper()
	m.pane = paneForm
	for i, f := range m.fields {
		if f.key == key {
			m.fieldSel = i
			return m
		}
	}
	t.Fatalf("no field %q on %q", key, m.draft.name)
	return m
}

func TestEditorCycleField(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // select webfarm (sync: true)
	m = focusField(t, m, "sync")
	m = edType(t, m, tea.KeySpace)
	if m.draft.sess.Sync {
		t.Fatal("space did not toggle sync off")
	}
	if !m.isDirty() {
		t.Fatal("cycle edit did not mark draft dirty")
	}
	if !strings.Contains(m.statusLine(), "unsaved") {
		t.Fatal("status line missing unsaved marker")
	}
}

func TestEditorTextFieldEdit(t *testing.T) {
	m := testEditorModel(t)
	m = focusField(t, m, "connect")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeFieldEdit {
		t.Fatal("enter did not start field edit")
	}
	m = edRunes(t, m, "ssh -A {{host}}")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse {
		t.Fatal("commit did not return to browse")
	}
	if m.draft.sess.Connect != "ssh -A {{host}}" {
		t.Fatalf("connect = %q", m.draft.sess.Connect)
	}
}

func TestEditorTextFieldCancel(t *testing.T) {
	m := testEditorModel(t)
	m = focusField(t, m, "root")
	orig := m.draft.sess.Root
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "/changed")
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse || m.draft.sess.Root != orig {
		t.Fatalf("esc did not cancel edit: root = %q", m.draft.sess.Root)
	}
}

func TestEditorNumberFieldValidation(t *testing.T) {
	m := testEditorModel(t)
	m = focusField(t, m, "retry")
	m = edType(t, m, tea.KeyEnter)
	// seed is "0"; clear it, type garbage
	m = edType(t, m, tea.KeyBackspace)
	m = edRunes(t, m, "banana")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeFieldEdit {
		t.Fatal("invalid retry was accepted")
	}
	if m.inputErr == "" {
		t.Fatal("no inline error for invalid retry")
	}
	// fix it
	m.input = nil
	m = edRunes(t, m, "5")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse || m.draft.sess.Retry != 5 {
		t.Fatalf("retry = %d, mode = %v", m.draft.sess.Retry, m.mode)
	}
}

func TestEditorStructureFieldHint(t *testing.T) {
	yml := editorFixtureYAML + `    layered:
        windows:
            - name: main
              hosts: [a1]
`
	st := testEditorState(t, yml)
	m := newEditorModel(st, nil, nil, "layered")
	m = focusField(t, m, "windows")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse || m.status == "" {
		t.Fatal("structure row should stay in browse and hint at 'o'")
	}
}

func TestEditorListSubEditor(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm: hosts [web1 web2]
	m = focusField(t, m, "hosts")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeListEdit {
		t.Fatal("enter on hosts did not open the list sub-editor")
	}

	// add an entry
	m = edRunes(t, m, "a")
	if !m.listEd.editing || !m.listEd.adding {
		t.Fatal("a did not start an add input")
	}
	m = edRunes(t, m, "web3")
	m = edType(t, m, tea.KeyEnter)
	if got := m.draft.sess.Hosts; len(got) != 3 || got[2] != "web3" {
		t.Fatalf("hosts after add = %v", got)
	}

	// edit an entry (poke cursor/input directly; navigation noise isn't under test)
	m.listEd.sel = 0
	m = edRunes(t, m, "e")
	m.listEd.input = []rune("web1a")
	m = edType(t, m, tea.KeyEnter)
	if m.draft.sess.Hosts[0] != "web1a" {
		t.Fatalf("hosts after edit = %v", m.draft.sess.Hosts)
	}

	// reorder down, then delete
	m.listEd.sel = 0
	m = edRunes(t, m, "J")
	if m.draft.sess.Hosts[1] != "web1a" {
		t.Fatalf("hosts after J = %v", m.draft.sess.Hosts)
	}
	m = edRunes(t, m, "d")
	if len(m.draft.sess.Hosts) != 2 {
		t.Fatalf("hosts after d = %v", m.draft.sess.Hosts)
	}

	// esc returns to the form
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse {
		t.Fatal("esc did not leave the sub-editor")
	}
	if !m.isDirty() {
		t.Fatal("list edits did not dirty the draft")
	}
}

func TestEditorHostsClusterExpansion(t *testing.T) {
	st := testEditorState(t, editorFixtureYAML)
	clusters := map[string][]string{"db": {"db1", "db2"}}
	m := newEditorModel(st, clusters, nil, "webfarm")

	m = focusField(t, m, "hosts")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "@db")
	m = edType(t, m, tea.KeyEnter)
	got := m.draft.sess.Hosts
	if len(got) != 4 || got[2] != "db1" || got[3] != "db2" {
		t.Fatalf("hosts after @db = %v, want expansion", got)
	}

	// unknown cluster: inline error, list unchanged
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "@nope")
	m = edType(t, m, tea.KeyEnter)
	if m.listEd.errMsg == "" {
		t.Fatal("no error for unknown cluster")
	}
	if len(m.draft.sess.Hosts) != 4 {
		t.Fatalf("hosts changed on failed expansion: %v", m.draft.sess.Hosts)
	}
}

func TestEditorCommandsList(t *testing.T) {
	m := testEditorModel(t)
	m = focusField(t, m, "commands")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "sudo -i")
	m = edType(t, m, tea.KeyEnter)
	if got := m.draft.sess.Commands; len(got) != 1 || got[0] != "sudo -i" {
		t.Fatalf("commands = %v", got)
	}
	// commands are NOT host-expanded even with an @
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "echo @db")
	m = edType(t, m, tea.KeyEnter)
	if got := m.draft.sess.Commands; len(got) != 2 || got[1] != "echo @db" {
		t.Fatalf("commands = %v", got)
	}
}

func TestEditorRename(t *testing.T) {
	m := testEditorModel(t) // solo selected
	m = edRunes(t, m, "r")
	if m.mode != modeInput || m.inputPurpose != inputRename {
		t.Fatal("r did not open the rename prompt")
	}
	// seeded with the current name
	if string(m.input) != "solo" {
		t.Fatalf("rename seed = %q", m.input)
	}
	m.input = nil
	m = edRunes(t, m, "lonely")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse || m.draft.name != "lonely" || m.draft.orig != "solo" {
		t.Fatalf("rename draft = %+v", m.draft)
	}
	if !m.isDirty() {
		t.Fatal("rename did not dirty the draft")
	}
}

func TestEditorRenameRejectsCollisionAndBadNames(t *testing.T) {
	m := testEditorModel(t) // solo selected
	m = edRunes(t, m, "r")
	m.input = nil
	m = edRunes(t, m, "webfarm") // exists
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeInput || m.inputErr == "" {
		t.Fatal("rename accepted a colliding name")
	}
	m.input = []rune("bad:name") // reserved character
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeInput || m.inputErr == "" {
		t.Fatal("rename accepted a reserved character")
	}
	// rename back to its own current name is allowed (no-op rename)
	m.input = []rune("solo")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse {
		t.Fatalf("rename to own name rejected: %q", m.inputErr)
	}
}

func TestEditorDuplicate(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm
	m = edRunes(t, m, "y")
	if m.mode != modeInput || m.inputPurpose != inputDuplicate {
		t.Fatal("y did not open the duplicate prompt")
	}
	m = edRunes(t, m, "webfarm2")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse {
		t.Fatalf("duplicate did not finish: err=%q", m.inputErr)
	}
	if m.draft == nil || !m.draft.added || m.draft.name != "webfarm2" {
		t.Fatalf("duplicate draft = %+v", m.draft)
	}
	if got := m.draft.sess.Hosts; len(got) != 2 {
		t.Fatalf("duplicate did not clone hosts: %v", got)
	}
	// the new name shows up in the list and is selected
	if m.selectedName() != "webfarm2" {
		t.Fatalf("selected %q, want webfarm2", m.selectedName())
	}
	// mutating the copy must not touch the source session
	m.draft.sess.Hosts[0] = "changed"
	if m.st.cfg.Sessions["webfarm"].Hosts[0] != "web1" {
		t.Fatal("duplicate shares memory with the source session")
	}
}

func TestEditorDuplicateRejectsExistingNames(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm
	m = edRunes(t, m, "y")
	m = edRunes(t, m, "webfarm") // its own name — still a collision for a copy
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeInput || m.inputErr == "" {
		t.Fatal("duplicate accepted an existing name")
	}
}

func TestEditorDuplicateRequiresCleanDraft(t *testing.T) {
	m := testEditorModel(t)
	m = focusField(t, m, "sync")
	m = edType(t, m, tea.KeySpace) // dirty now
	m = edRunes(t, m, "y")
	if m.mode == modeInput {
		t.Fatal("duplicate opened despite a dirty draft")
	}
	if !m.statusErr {
		t.Fatal("no error status about the dirty draft")
	}
}

func TestEditorDelete(t *testing.T) {
	m := testEditorModel(t) // solo selected; two sessions exist
	m = edRunes(t, m, "D")
	if m.mode != modeConfirmDelete {
		t.Fatal("D did not ask for confirmation")
	}
	m = edRunes(t, m, "y")
	if m.mode != modeBrowse || !m.draft.deleted {
		t.Fatalf("delete not marked: %+v", m.draft)
	}
	// D again undoes the pending delete
	m = edRunes(t, m, "D")
	if m.draft.deleted {
		t.Fatal("second D did not undo the pending delete")
	}
}

func TestEditorDeleteLastSessionBlocked(t *testing.T) {
	st := testEditorState(t, "sessions:\n    only:\n        root: /tmp\n")
	m := newEditorModel(st, nil, nil, "")
	m = edRunes(t, m, "D")
	if m.mode == modeConfirmDelete {
		t.Fatal("delete offered on the last remaining session")
	}
	if !m.statusErr {
		t.Fatal("no error status for last-session delete")
	}
}

func TestEditorDuplicateThenRename(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm
	m = edRunes(t, m, "y")
	m = edRunes(t, m, "webfarm2")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "r")
	m.input = nil
	m = edRunes(t, m, "webfarm3")
	m = edType(t, m, tea.KeyEnter)
	if m.selectedName() != "webfarm3" {
		t.Fatalf("selected %q, want webfarm3", m.selectedName())
	}
	if m.draft == nil || !m.draft.added || m.draft.name != "webfarm3" {
		t.Fatalf("draft = %+v", m.draft)
	}
	// navigating away and back must not destroy the renamed duplicate
	m = edRunes(t, m, "k")
	m = edRunes(t, m, "j")
	// After navigation, the added draft is gone (guard behavior arrives in Task 9),
	// but we must not panic and the draft must be properly initialized
	if m.draft == nil {
		t.Fatal("draft became nil after navigation")
	}
}

func dirtyModel(t *testing.T) editorModel {
	t.Helper()
	m := testEditorModel(t) // solo selected
	m = focusField(t, m, "root")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "x")
	m = edType(t, m, tea.KeyEnter) // root = "/tmp/solox" — dirty
	return m
}

func TestEditorGuardOnSwitch(t *testing.T) {
	m := dirtyModel(t)
	m.pane = paneList
	m = edRunes(t, m, "j")
	if m.mode != modeGuard {
		t.Fatal("switching away from a dirty draft did not guard")
	}
	if m.selectedName() != "solo" {
		t.Fatal("selection moved before the guard resolved")
	}
	// esc: stay put, draft intact
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse || m.selectedName() != "solo" || !m.isDirty() {
		t.Fatal("esc did not cancel the guard cleanly")
	}
}

func TestEditorGuardDiscard(t *testing.T) {
	m := dirtyModel(t)
	m.pane = paneList
	m = edRunes(t, m, "j")
	m = edRunes(t, m, "d")
	if m.mode != modeBrowse || m.selectedName() != "webfarm" {
		t.Fatalf("guard discard: mode=%v sel=%q", m.mode, m.selectedName())
	}
	if m.isDirty() {
		t.Fatal("draft still dirty after discard")
	}
	// the config was never written
	if m.st.cfg.Sessions["solo"].Root != "/tmp/solo" {
		t.Fatal("discard leaked into the config")
	}
}

func TestEditorGuardSave(t *testing.T) {
	m := dirtyModel(t)
	m.pane = paneList
	m = edRunes(t, m, "j")
	m = edRunes(t, m, "s")
	if m.mode != modeBrowse || m.selectedName() != "webfarm" {
		t.Fatalf("guard save: mode=%v sel=%q", m.mode, m.selectedName())
	}
	data, _ := os.ReadFile(m.st.path)
	if !strings.Contains(string(data), "/tmp/solox") {
		t.Fatalf("guard save did not write the file:\n%s", data)
	}
}

func TestEditorGuardOnQuit(t *testing.T) {
	m := dirtyModel(t)
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = nm.(editorModel)
	if cmd != nil || m.mode != modeGuard {
		t.Fatal("q with a dirty draft did not guard")
	}
	nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !isQuit(cmd) {
		t.Fatal("guard discard on quit did not quit")
	}
	_ = nm
}

func TestEditorFilterBlockedWhenDirty(t *testing.T) {
	m := dirtyModel(t)
	m.pane = paneList
	m = edRunes(t, m, "/")
	if m.mode == modeFilter {
		t.Fatal("filter opened despite a dirty draft")
	}
	if !m.statusErr {
		t.Fatal("no error status about the dirty draft")
	}
}

func TestEditorGuardProtectsAddedDraft(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm
	m = edRunes(t, m, "y")
	m = edRunes(t, m, "copy1")
	m = edType(t, m, tea.KeyEnter) // added draft, inherently dirty
	m.pane = paneList
	m = edRunes(t, m, "k") // navigate away → must guard, not silently discard
	if m.mode != modeGuard {
		t.Fatal("navigating away from an added draft did not guard")
	}
	m = edRunes(t, m, "s") // save it
	if _, ok := m.st.cfg.Sessions["copy1"]; !ok {
		t.Fatal("guard save did not persist the added draft")
	}
	data, _ := os.ReadFile(m.st.path)
	if !strings.Contains(string(data), "copy1") {
		t.Fatal("added draft not in the file after guard save")
	}
}

func TestEditorSaveNoChanges(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "s")
	if m.mode != modeBrowse || !strings.Contains(m.status, "no changes") {
		t.Fatalf("mode=%v status=%q", m.mode, m.status)
	}
}

func TestEditorSaveDiffAndWrite(t *testing.T) {
	m := dirtyModel(t) // solo root → /tmp/solox
	m = edRunes(t, m, "s")
	if m.mode != modeDiff {
		t.Fatalf("s did not open the diff: mode=%v status=%q", m.mode, m.status)
	}
	out := m.View()
	if !strings.Contains(out, "/tmp/solox") {
		t.Fatalf("diff view missing new value:\n%s", out)
	}

	// esc backs out, draft intact
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse || !m.isDirty() {
		t.Fatal("esc from diff lost the draft")
	}

	// confirm writes
	m = edRunes(t, m, "s")
	m = edType(t, m, tea.KeyEnter)
	if m.mode != modeBrowse || !strings.Contains(m.status, "saved solo") {
		t.Fatalf("after confirm: mode=%v status=%q", m.mode, m.status)
	}
	data, _ := os.ReadFile(m.st.path)
	if !strings.Contains(string(data), "/tmp/solox") {
		t.Fatalf("file not written:\n%s", data)
	}
	if m.isDirty() {
		t.Fatal("draft still dirty after save")
	}
}

func TestEditorSaveValidationJumpsToField(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm (has hosts)
	// connect + ssh_user together is a cross-field validation error
	m = focusField(t, m, "connect")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "ssh -J jump {{host}}")
	m = edType(t, m, tea.KeyEnter)
	m = focusField(t, m, "ssh_user")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "root")
	m = edType(t, m, tea.KeyEnter)

	m = edRunes(t, m, "s")
	if m.mode == modeDiff {
		t.Fatal("invalid draft reached the diff preview")
	}
	if !m.statusErr {
		t.Fatal("no validation error surfaced")
	}
	if m.fields[m.fieldSel].key != "ssh_user" {
		t.Fatalf("cursor on %q, want ssh_user", m.fields[m.fieldSel].key)
	}
}

func TestEditorGuardSaveValidationFailure(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "j") // webfarm
	m = focusField(t, m, "connect")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "ssh -J jump {{host}}")
	m = edType(t, m, tea.KeyEnter)
	m = focusField(t, m, "ssh_user")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "root")
	m = edType(t, m, tea.KeyEnter) // draft dirty AND invalid
	m.pane = paneList
	m = edRunes(t, m, "k") // guard
	if m.mode != modeGuard {
		t.Fatal("no guard on dirty switch")
	}
	m = edRunes(t, m, "s") // guard-save must fail validation cleanly
	if m.mode != modeBrowse || !m.statusErr {
		t.Fatalf("guard-save failure: mode=%v statusErr=%v", m.mode, m.statusErr)
	}
	if m.selectedName() != "webfarm" {
		t.Fatal("selection moved despite failed save")
	}
	if !m.isDirty() {
		t.Fatal("draft lost on failed guard-save")
	}
}

func TestEditorSaveStaleThenReload(t *testing.T) {
	m := dirtyModel(t)
	// external change after load
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(m.st.path, future, future); err != nil {
		t.Fatal(err)
	}
	m = edRunes(t, m, "s")
	m = edType(t, m, tea.KeyEnter) // confirm diff → hits staleness
	if m.mode != modeStale {
		t.Fatalf("mode=%v, want modeStale", m.mode)
	}
	// stale view renders an explanation
	if !strings.Contains(m.View(), "changed on disk") {
		t.Fatal("stale view missing explanation")
	}
	// R reloads from disk, dropping the draft
	m = edRunes(t, m, "R")
	if m.mode != modeBrowse || m.isDirty() {
		t.Fatal("reload did not produce a clean editor")
	}
	if m.st.cfg.Sessions["solo"].Root != "/tmp/solo" {
		t.Fatal("reload did not restore disk state")
	}
}

func TestEditorSaveRenameAndDelete(t *testing.T) {
	m := testEditorModel(t) // solo selected
	m = edRunes(t, m, "r")
	m.input = []rune("lonely")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "s")
	m = edType(t, m, tea.KeyEnter)
	if m.selectedName() != "lonely" {
		t.Fatalf("selection after rename-save = %q", m.selectedName())
	}
	data, _ := os.ReadFile(m.st.path)
	if !strings.Contains(string(data), "lonely:") || strings.Contains(string(data), "solo:") {
		t.Fatalf("rename not written:\n%s", data)
	}

	// now delete it
	m = edRunes(t, m, "D")
	m = edRunes(t, m, "y")
	m = edRunes(t, m, "s")
	m = edType(t, m, tea.KeyEnter)
	data, _ = os.ReadFile(m.st.path)
	if strings.Contains(string(data), "lonely:") {
		t.Fatalf("delete not written:\n%s", data)
	}
	if m.selectedName() != "webfarm" {
		t.Fatalf("selection after delete-save = %q", m.selectedName())
	}
	if !strings.Contains(m.status, "deleted lonely") {
		t.Fatalf("delete save status = %q, want deleted wording", m.status)
	}
}
