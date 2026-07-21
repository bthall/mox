package cli

// The full-screen config editor: configured sessions on the left, the
// selected session's field form on the right. All changes buffer into a
// single active draft; 's' runs the save pipeline (validate → diff preview
// → staleness check → node-surgery write). Runs in the alt screen.

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

// Diff/error styles; everything else reuses the picker's pk* palette.
var (
	pkDiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) //nolint:unused // wired in by Task 10
	pkDiffDel = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) //nolint:unused // wired in by Task 10
	pkErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

type editorMode int

const (
	modeBrowse        editorMode = iota // navigating list/form
	modeFilter                          // typing in the list filter
	modeFieldEdit                       // inline text/number input on a field
	modeListEdit                        // list sub-editor owns the right pane
	modeInput                           // one-line prompt (rename / duplicate)
	modeConfirmDelete                   // 'y' confirms, esc cancels
	modeDiff                            // save preview modal
	modeGuard                           // unsaved-changes prompt before nav/quit
	modeWizard                          // embedded 'mox add' wizard
	modeStale                           // save refused: file changed on disk
)

type editorPane int

const (
	paneList editorPane = iota
	paneForm
)

type inputPurpose int

const (
	inputRename inputPurpose = iota
	inputDuplicate
)

// pendingAction is what a guard resolution continues with.
type pendingKind int //nolint:unused // wired in by Task 9

const (
	pendingNone   pendingKind = iota //nolint:unused // wired in by Task 9
	pendingSelect                    //nolint:unused // wired in by Task 9 (move list selection to target)
	pendingQuit                      //nolint:unused // wired in by Task 9
)

type pendingAction struct { //nolint:unused // wired in by Task 9
	kind   pendingKind
	target int
}

// listEditState is the list sub-editor (hosts, commands, pre, hooks).
type listEditState struct {
	field   int
	sel     int
	editing bool // inline input active
	adding  bool // editing a new entry vs replacing sel
	input   []rune
	errMsg  string
}

type editorModel struct {
	st       *editorState
	clusters map[string][]string
	running  map[string]session.SessionInfo

	filter  []rune
	visible []string // filtered display names
	sel     int      // index into visible
	offset  int      // list scroll offset

	draft    *sessionDraft
	fields   []fieldDef
	fieldSel int
	pane     editorPane

	mode         editorMode
	input        []rune // shared buffer for modeFieldEdit / modeInput
	inputPurpose inputPurpose
	inputErr     string

	listEd  listEditState
	diff    []diffLine    //nolint:unused // wired in by Task 10
	diffOff int           //nolint:unused // wired in by Task 10
	pending pendingAction //nolint:unused // wired in by Task 9
	wizard  *addModel

	status    string
	statusErr bool

	width, height int
}

func newEditorModel(st *editorState, clusters map[string][]string, running map[string]session.SessionInfo, initial string) editorModel {
	m := editorModel{
		st:       st,
		clusters: clusters,
		running:  running,
		width:    100,
		height:   30,
	}
	m.refilter()
	if initial != "" {
		for i, n := range m.visible {
			if n == initial {
				m.sel = i
				m.pane = paneForm
				break
			}
		}
	}
	m.resetDraft()
	return m
}

func (m editorModel) Init() tea.Cmd { return nil }

// displayNames is the list-pane order: sorted config names plus a pending
// added draft (wizard / duplicate) that exists only in the draft.
func (m editorModel) displayNames() []string {
	names := m.st.cfg.ListSessionNames()
	if m.draft != nil && m.draft.added {
		names = append(names, m.draft.name)
	}
	return names
}

func (m *editorModel) refilter() {
	names := m.displayNames()
	if len(m.filter) == 0 {
		m.visible = names
	} else {
		matches := fuzzy.Find(string(m.filter), names)
		m.visible = make([]string, len(matches))
		for i, match := range matches {
			m.visible[i] = names[match.Index]
		}
	}
	if m.sel > len(m.visible)-1 {
		m.sel = len(m.visible) - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.offset > m.sel {
		m.offset = m.sel
	}
	m.keepVisible()
}

func (m *editorModel) selectedName() string {
	if len(m.visible) == 0 {
		return ""
	}
	return m.visible[m.sel]
}

// resetDraft starts a clean draft for the selected session. An added draft
// (no config entry behind it) is kept as long as it stays selected.
func (m *editorModel) resetDraft() {
	name := m.selectedName()
	if name == "" {
		m.draft = nil
		m.fields = nil
		return
	}
	if m.draft == nil || !m.draft.added || m.draft.name != name {
		m.draft = newDraft(m.st.cfg, name)
	}
	m.fields = sessionFields(m.draft.sess)
	if m.fieldSel > len(m.fields)-1 {
		m.fieldSel = 0
	}
}

func (m *editorModel) isDirty() bool { return m.draft.dirty(m.st.cfg) }

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeBrowse:
			return m.updateBrowse(msg)
		case modeFilter:
			return m.updateFilter(msg)
		case modeFieldEdit:
			return m.updateFieldEdit(msg)
		case modeListEdit:
			return m.updateListEdit(msg)
		case modeInput:
			return m.updateInput(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		}
		// remaining modes are wired in by later tasks
	}
	return m, nil
}

func (m editorModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyTab:
		if m.pane == paneList {
			m.pane = paneForm
		} else {
			m.pane = paneList
		}
		return m, nil
	case tea.KeyUp, tea.KeyCtrlP:
		return m.moveCursor(-1)
	case tea.KeyDown, tea.KeyCtrlN:
		return m.moveCursor(1)
	case tea.KeyEnter:
		if m.pane == paneList {
			m.pane = paneForm
			return m, nil
		}
		return m.enterField()
	case tea.KeySpace:
		if m.pane == paneForm {
			return m.cycleField()
		}
		return m, nil
	case tea.KeyEsc:
		if len(m.filter) > 0 {
			m.filter = nil
			m.refilter()
			m.resetDraft()
			return m, nil
		}
		return m, tea.Quit // Task 9 routes this through the guard
	}
	switch string(msg.Runes) {
	case "q":
		return m, tea.Quit // Task 9 routes this through the guard
	case "j":
		return m.moveCursor(1)
	case "k":
		return m.moveCursor(-1)
	case "/":
		if m.pane == paneList {
			m.mode = modeFilter
		}
		return m, nil
	case "r":
		if m.draft != nil && !m.draft.deleted {
			m.mode = modeInput
			m.inputPurpose = inputRename
			m.input = []rune(m.draft.name)
			m.inputErr = ""
		}
		return m, nil
	case "y":
		if m.draft == nil {
			return m, nil
		}
		if m.isDirty() {
			m.status = "unsaved changes — save (s) or discard (D) before duplicating"
			m.statusErr = true
			return m, nil
		}
		m.mode = modeInput
		m.inputPurpose = inputDuplicate
		m.input = nil
		m.inputErr = ""
		return m, nil
	case "D":
		if m.draft == nil {
			return m, nil
		}
		if m.draft.deleted {
			m.draft.deleted = false // undo pending delete
			m.status = ""
			return m, nil
		}
		if m.draft.added {
			// a never-saved draft just disappears
			m.draft = nil
			m.refilter()
			m.resetDraft()
			m.status = "discarded unsaved session"
			m.statusErr = false
			return m, nil
		}
		if len(m.st.cfg.Sessions) == 1 {
			m.status = "cannot delete the last session (a config needs at least one)"
			m.statusErr = true
			return m, nil
		}
		m.mode = modeConfirmDelete
		return m, nil
	}
	return m, nil
}

func (m editorModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.filter = nil
		m.mode = modeBrowse
		m.refilter()
		m.resetDraft()
		return m, nil
	case tea.KeyEnter:
		m.mode = modeBrowse
		return m, nil
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.refilter()
			m.resetDraft()
		}
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				m.filter = append(m.filter, r)
			}
		}
		m.refilter()
		m.resetDraft()
		return m, nil
	}
	return m, nil
}

// moveCursor moves the active pane's cursor by delta.
func (m editorModel) moveCursor(delta int) (tea.Model, tea.Cmd) {
	if m.pane == paneList {
		if len(m.visible) == 0 {
			return m, nil
		}
		next := clampChoice(m.sel+delta, len(m.visible))
		if next == m.sel {
			return m, nil
		}
		return m.selectIndex(next)
	}
	if len(m.fields) > 0 {
		m.fieldSel = clampChoice(m.fieldSel+delta, len(m.fields))
	}
	return m, nil
}

// selectIndex moves the list selection and resets the draft. Task 9 adds
// the unsaved-changes guard in front of this.
func (m editorModel) selectIndex(idx int) (tea.Model, tea.Cmd) {
	m.sel = idx
	m.keepVisible()
	m.resetDraft()
	m.status = ""
	m.statusErr = false
	return m, nil
}

// enterField acts on the focused form row: cycle fields advance, text and
// number fields open the inline input, list fields open the sub-editor
// (Task 7), the structure row points at 'o'.
func (m editorModel) enterField() (tea.Model, tea.Cmd) {
	if m.draft == nil || m.draft.deleted || len(m.fields) == 0 {
		return m, nil
	}
	f := m.fields[m.fieldSel]
	switch f.kind {
	case fieldCycle:
		return m.cycleField()
	case fieldText, fieldNumber:
		m.mode = modeFieldEdit
		m.input = []rune(f.text(m.draft.sess))
		m.inputErr = ""
		return m, nil
	case fieldStructure:
		m.status = "window/pane structure is YAML-only — press o to open $EDITOR"
		m.statusErr = false
		return m, nil
	case fieldList:
		m.mode = modeListEdit
		m.listEd = listEditState{field: m.fieldSel}
		return m, nil
	}
	return m, nil
}

// cycleField advances a cycle field (bool / tri-state / enum) in place.
func (m editorModel) cycleField() (tea.Model, tea.Cmd) {
	if m.draft == nil || m.draft.deleted || len(m.fields) == 0 {
		return m, nil
	}
	if f := m.fields[m.fieldSel]; f.kind == fieldCycle {
		f.cycle(m.draft.sess)
		m.status = ""
	}
	return m, nil
}

// updateFieldEdit is the inline input for text/number fields.
func (m editorModel) updateFieldEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.inputErr = ""
		return m, nil
	case tea.KeyEnter:
		f := m.fields[m.fieldSel]
		if err := f.set(m.draft.sess, strings.TrimSpace(string(m.input))); err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
		m.mode = modeBrowse
		m.inputErr = ""
		return m, nil
	case tea.KeyCtrlU:
		m.input = nil
		m.inputErr = ""
		return m, nil
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		m.inputErr = ""
		return m, nil
	case tea.KeySpace:
		m.input = append(m.input, ' ')
		m.inputErr = ""
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				m.input = append(m.input, r)
			}
		}
		m.inputErr = ""
		return m, nil
	}
	return m, nil
}

// validateSessionName applies the real config name rules plus uniqueness
// among configured sessions. allowSelf permits one existing name — the
// draft's own original — so a rename can keep (or restore) its name; a
// duplicate passes "" and collides with everything.
func (m *editorModel) validateSessionName(name, allowSelf string) error {
	if err := (&config.Session{}).Validate(name); err != nil {
		return err
	}
	if _, exists := m.st.cfg.Sessions[name]; exists && name != allowSelf {
		return fmt.Errorf("session %q already exists", name)
	}
	return nil
}

// updateInput is the one-line prompt for rename and duplicate.
func (m editorModel) updateInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		m.inputErr = ""
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(string(m.input))
		allowSelf := m.draft.orig
		if m.inputPurpose == inputDuplicate {
			allowSelf = "" // a copy may not reuse any existing name
		}
		if err := m.validateSessionName(name, allowSelf); err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
		switch m.inputPurpose {
		case inputRename:
			m.draft.name = name
			m.refilter()
			for i, n := range m.visible {
				if n == name {
					m.sel = i
					break
				}
			}
			m.keepVisible()
		case inputDuplicate:
			src := m.st.cfg.Sessions[m.selectedName()]
			m.draft = &sessionDraft{name: name, added: true, sess: cloneSession(src)}
			m.refilter()
			for i, n := range m.visible {
				if n == name {
					m.sel = i
					break
				}
			}
			m.keepVisible()
			m.fields = sessionFields(m.draft.sess)
			m.fieldSel = 0
			m.pane = paneForm
		}
		m.mode = modeBrowse
		m.inputErr = ""
		m.status = ""
		return m, nil
	case tea.KeyCtrlU:
		m.input, m.inputErr = nil, ""
		return m, nil
	case tea.KeyBackspace:
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		m.inputErr = ""
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if unicode.IsPrint(r) {
				m.input = append(m.input, r)
			}
		}
		m.inputErr = ""
		return m, nil
	}
	return m, nil
}

func (m editorModel) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if msg.Type == tea.KeyEsc {
		m.mode = modeBrowse
		return m, nil
	}
	if string(msg.Runes) == "y" || msg.Type == tea.KeyEnter {
		m.draft.deleted = true
		m.mode = modeBrowse
		m.status = "delete pending — press s to save, D to undo"
		m.statusErr = false
	}
	return m, nil
}

// inputLines renders the rename/duplicate prompt in the right pane.
func (m editorModel) inputLines(w int) []string {
	label := "rename to:"
	if m.inputPurpose == inputDuplicate {
		label = "duplicate " + m.selectedName() + " as:"
	}
	lines := []string{
		pkDim.Render(truncate(label, w)),
		"",
		pkAccent.Render("▸ ") + string(m.input) + pkAccent.Render("█"),
	}
	if m.inputErr != "" {
		lines = append(lines, "", pkErr.Render(truncate(m.inputErr, w)))
	}
	return lines
}

// confirmDeleteLines renders the delete confirmation in the right pane.
func (m editorModel) confirmDeleteLines(w int) []string {
	return []string{
		pkErr.Render(truncate(fmt.Sprintf("delete session %q from the config?", m.draft.name), w)),
		"",
		pkDim.Render("The change is buffered — the file is only touched on save (s)."),
	}
}

func (m *editorModel) keepVisible() {
	rows := m.effectiveListRows()
	if m.sel < m.offset {
		m.offset = m.sel
	}
	if m.sel >= m.offset+rows {
		m.offset = m.sel - rows + 1
	}
}

// effectiveListRows is listRows minus the line reserved for the overflow
// indicator when the list doesn't fit. It is the single source of truth
// for both scrolling (keepVisible) and rendering (listLines) — the two
// went out of sync once and hid the selection behind the reserved row.
func (m editorModel) effectiveListRows() int {
	rows := m.listRows()
	if len(m.visible) > rows {
		rows--
		if rows < 1 {
			rows = 1
		}
	}
	return rows
}

// --- view ---

func (m editorModel) panelHeight() int {
	h := m.height - 2 // status line + trailing newline
	if h < 8 {
		h = 8
	}
	return h
}

// listRows is how many session rows fit in the list pane.
func (m editorModel) listRows() int {
	h := m.panelHeight() - 4 // borders, filter line, blank line
	if h < 1 {
		h = 1
	}
	return h
}

func (m editorModel) View() string {
	if m.mode == modeWizard && m.wizard != nil {
		return m.wizard.View()
	}
	h := m.panelHeight()
	if m.width < 60 {
		// Narrow: one pane at a time, Tab flips between them.
		w := m.width
		if w < 24 {
			w = 24
		}
		if (m.pane == paneList && m.mode == modeBrowse) || m.mode == modeFilter {
			return panel(m.listTitle(), m.footer(), m.listLines(w-4, h-2), w, h) + "\n" + m.statusLine()
		}
		return panel(m.formTitle(), m.footer(), m.rightLines(w-4, h-2), w, h) + "\n" + m.statusLine()
	}
	leftW := m.width / 4
	if leftW < 20 {
		leftW = 20
	}
	if leftW > 36 {
		leftW = 36
	}
	rightW := m.width - leftW - 1
	left := panel(m.listTitle(), "", m.listLines(leftW-4, h-2), leftW, h)
	right := panel(m.formTitle(), m.footer(), m.rightLines(rightW-4, h-2), rightW, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right) + "\n" + m.statusLine()
}

func (m editorModel) listTitle() string {
	if len(m.filter) > 0 || m.mode == modeFilter {
		return fmt.Sprintf("sessions · %d/%d", len(m.visible), len(m.displayNames()))
	}
	return "sessions"
}

func (m editorModel) formTitle() string {
	if m.draft == nil {
		return "mox edit"
	}
	badge := "local"
	switch {
	case m.draft.deleted:
		badge = "delete pending"
	case len(m.draft.sess.Windows) > 0:
		badge = pluralize(len(m.draft.sess.Windows), "window")
	case len(m.draft.sess.Hosts) > 0:
		badge = pluralize(len(m.draft.sess.Hosts), "host")
	}
	return m.draft.name + " · " + badge
}

func (m editorModel) footer() string {
	switch m.mode {
	case modeFilter:
		return "type to filter · ↵ keep · esc clear"
	case modeFieldEdit:
		return "↵ commit · esc cancel"
	case modeListEdit:
		if m.listEd.editing {
			return "↵ commit · esc cancel"
		}
		return "a add · e edit · d delete · J/K move · esc back"
	case modeInput:
		return "↵ confirm · esc cancel"
	case modeConfirmDelete:
		return "y delete · esc cancel"
	case modeDiff:
		return "↵ write config · esc back"
	case modeGuard:
		return "s save · d discard · esc stay"
	case modeStale:
		return "R reload (discards changes) · esc back"
	default:
		if m.pane == paneList {
			return "↵ fields · a add · r rename · y dup · D del · s save · q quit"
		}
		return "↵ edit · space cycle · s save · tab sessions · q quit"
	}
}

// listLines renders the filter line plus the visible session rows.
func (m editorModel) listLines(w, h int) []string {
	lines := []string{
		pkAccent.Render("▸ ") + string(m.filter) + pkAccent.Render("█"),
		"",
	}
	if len(m.visible) == 0 {
		return append(lines, pkDim.Render("  (no match)"))
	}
	rows := m.effectiveListRows()
	end := m.offset + rows
	if end > len(m.visible) {
		end = len(m.visible)
	}
	for i := m.offset; i < end; i++ {
		name := m.visible[i]
		dot := pkStopped.Render("○")
		if info, ok := m.running[name]; ok && info.Running {
			dot = pkRunning.Render("●")
		}
		display := truncate(name, w-4)
		if m.draft != nil && m.draft.orig == name && m.draft.name != name {
			display = truncate(m.draft.name+"*", w-4) // pending rename
		}
		if i == m.sel {
			lines = append(lines, pkAccent.Render("▌")+" "+dot+" "+pkSelected.Render(display))
		} else {
			lines = append(lines, "  "+dot+" "+display)
		}
	}
	if end < len(m.visible) {
		lines = append(lines, pkDim.Render(fmt.Sprintf("  … %d more", len(m.visible)-end)))
	}
	return lines
}

// rightLines picks the right pane's content by mode. Later tasks add cases.
func (m editorModel) rightLines(w, h int) []string {
	switch m.mode {
	case modeListEdit:
		return m.listEditLines(w, h)
	case modeInput:
		return m.inputLines(w)
	case modeConfirmDelete:
		return m.confirmDeleteLines(w)
	}
	return m.formLines(w, h)
}

// formLines renders the field rows plus the focused field's help text.
func (m editorModel) formLines(w, h int) []string {
	if m.draft == nil {
		return []string{pkDim.Render("no sessions — press a to add one")}
	}
	var lines []string
	if m.draft.deleted {
		lines = append(lines, pkErr.Render("delete pending — press s to save, D to undo"), "")
	}
	for i, f := range m.fields {
		label := fmt.Sprintf("%-9s", f.key)
		val := truncate(f.display(m.draft.sess), w-13)
		switch {
		case i == m.fieldSel && m.mode == modeFieldEdit:
			in := truncate(string(m.input), w-14)
			lines = append(lines, pkAccent.Render("▸ ")+pkSelected.Render(label)+" "+in+pkAccent.Render("█"))
		case i == m.fieldSel && m.pane == paneForm && m.mode != modeListEdit:
			lines = append(lines, pkAccent.Render("▸ ")+pkSelected.Render(label)+" "+val)
		default:
			lines = append(lines, "  "+pkDim.Render(label)+" "+val)
		}
	}
	if m.mode == modeFieldEdit && m.inputErr != "" {
		lines = append(lines, "", pkErr.Render(truncate(m.inputErr, w)))
	}
	lines = append(lines, "")
	if m.pane == paneForm && len(m.fields) > 0 && m.fieldSel < len(m.fields) {
		for _, hl := range wrapWords(strings.Fields(m.fields[m.fieldSel].help), w) {
			lines = append(lines, pkDim.Render(hl))
		}
	}
	return lines
}

func (m editorModel) statusLine() string {
	var parts []string
	if m.isDirty() {
		parts = append(parts, pkForeign.Render("unsaved: "+m.draft.name))
	}
	if m.status != "" {
		style := pkDim
		if m.statusErr {
			style = pkErr
		}
		parts = append(parts, style.Render(m.status))
	}
	return " " + truncate(strings.Join(parts, pkDim.Render(" · ")), m.width-2)
}

// listItems returns the live backing slice for the sub-editor's field.
func (m *editorModel) listItems() *[]string {
	return m.fields[m.listEd.field].list(m.draft.sess)
}

// commitListInput commits the sub-editor's inline input. Hosts entries go
// through expandHosts so @cluster references become their members; other
// list fields take the line verbatim.
func (m *editorModel) commitListInput() {
	line := strings.TrimSpace(string(m.listEd.input))
	if line == "" {
		m.listEd.errMsg = "empty entry"
		return
	}
	entries := []string{line}
	if m.fields[m.listEd.field].key == "hosts" {
		expanded, err := expandHosts(strings.Fields(line), m.st.cfg, m.clusters)
		if err != nil {
			m.listEd.errMsg = err.Error()
			return
		}
		entries = expanded
	}
	items := m.listItems()
	if m.listEd.adding {
		*items = append(*items, entries...)
		m.listEd.sel = len(*items) - 1
	} else {
		// Replace items[sel] with entries
		newItems := make([]string, 0, len(*items)-1+len(entries))
		newItems = append(newItems, (*items)[:m.listEd.sel]...)
		newItems = append(newItems, entries...)
		newItems = append(newItems, (*items)[m.listEd.sel+1:]...)
		*items = newItems
	}
	m.listEd.editing = false
	m.listEd.input = nil
	m.listEd.errMsg = ""
}

func (m editorModel) updateListEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	le := &m.listEd
	if le.editing {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			le.editing, le.input, le.errMsg = false, nil, ""
			return m, nil
		case tea.KeyEnter:
			m.commitListInput()
			return m, nil
		case tea.KeyCtrlU:
			le.input, le.errMsg = nil, ""
			return m, nil
		case tea.KeyBackspace:
			if len(le.input) > 0 {
				le.input = le.input[:len(le.input)-1]
			}
			return m, nil
		case tea.KeySpace:
			le.input = append(le.input, ' ')
			le.errMsg = ""
			return m, nil
		case tea.KeyRunes:
			for _, r := range msg.Runes {
				if unicode.IsPrint(r) {
					le.input = append(le.input, r)
				}
			}
			le.errMsg = ""
			return m, nil
		}
		return m, nil
	}

	items := m.listItems()
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeBrowse
		return m, nil
	case tea.KeyUp:
		le.sel = clampChoice(le.sel-1, len(*items))
		return m, nil
	case tea.KeyDown:
		le.sel = clampChoice(le.sel+1, len(*items))
		return m, nil
	}
	switch string(msg.Runes) {
	case "k":
		le.sel = clampChoice(le.sel-1, len(*items))
	case "j":
		le.sel = clampChoice(le.sel+1, len(*items))
	case "a":
		le.editing, le.adding, le.input, le.errMsg = true, true, nil, ""
	case "e":
		if len(*items) > 0 {
			le.editing, le.adding, le.errMsg = true, false, ""
			le.input = []rune((*items)[le.sel])
		}
	case "d":
		if len(*items) > 0 {
			*items = append((*items)[:le.sel], (*items)[le.sel+1:]...)
			le.sel = clampChoice(le.sel, len(*items))
		}
	case "J":
		if le.sel < len(*items)-1 {
			(*items)[le.sel], (*items)[le.sel+1] = (*items)[le.sel+1], (*items)[le.sel]
			le.sel++
		}
	case "K":
		if le.sel > 0 {
			(*items)[le.sel], (*items)[le.sel-1] = (*items)[le.sel-1], (*items)[le.sel]
			le.sel--
		}
	}
	return m, nil
}

// listEditLines renders the sub-editor into the right pane.
func (m editorModel) listEditLines(w, h int) []string {
	f := m.fields[m.listEd.field]
	lines := []string{pkTitle.Render(f.key), ""}
	items := f.list(m.draft.sess)
	if len(*items) == 0 && !m.listEd.editing {
		lines = append(lines, pkDim.Render("  (empty — a to add)"))
	}
	for i, it := range *items {
		row := truncate(it, w-4)
		if i == m.listEd.sel && !m.listEd.editing {
			lines = append(lines, pkAccent.Render("▌ ")+pkSelected.Render(row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if m.listEd.editing {
		lines = append(lines, "", pkAccent.Render("▸ ")+string(m.listEd.input)+pkAccent.Render("█"))
	}
	if m.listEd.errMsg != "" {
		lines = append(lines, "", pkErr.Render(truncate(m.listEd.errMsg, w)))
	}
	if f.key == "hosts" {
		lines = append(lines, "", pkDim.Render(truncate("@cluster expands on commit (config + clusterssh)", w)))
	}
	return lines
}
