package cli

// The full-screen config editor: configured sessions on the left, the
// selected session's field form on the right. All changes buffer into a
// single active draft; 's' runs the save pipeline (validate → diff preview
// → staleness check → node-surgery write). Runs in the alt screen.

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

// Diff/error styles; everything else reuses the picker's pk* palette.
var (
	pkDiffAdd = lipgloss.NewStyle().Bold(true)
	pkDiffDel = lipgloss.NewStyle().Bold(true)
	pkErr     = lipgloss.NewStyle().Bold(true)
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
type pendingKind int

const (
	pendingNone   pendingKind = iota
	pendingSelect             // move list selection to target
	pendingQuit
)

type pendingAction struct {
	kind   pendingKind
	target int
}

// editorReturnMsg arrives when the external $EDITOR process exits.
type editorReturnMsg struct{ err error }

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
	help         bool
	helpOff      int
	guardReturn  editorMode
	input        []rune // shared buffer for modeFieldEdit / modeInput
	inputPurpose inputPurpose
	inputErr     string

	listEd  listEditState
	diff    []diffLine
	diffOff int
	pending pendingAction
	wizard  *addModel

	status    string
	statusErr bool
	statusOK  bool // success feedback renders green

	// startSession starts a just-saved session detached, from the post-save
	// config. nil when tmux is unavailable (and in most tests).
	startSession func(cfg *config.Config, name string) error

	// leaving is set once the editor quits or suspends for $EDITOR, so a
	// batched rune replay stops there.
	leaving bool

	width, height int
}

func newEditorModel(st *editorState, clusters map[string][]string, running map[string]session.SessionInfo, initial string) editorModel {
	if running == nil {
		running = map[string]session.SessionInfo{}
	}
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
	keep := m.draft != nil &&
		((m.draft.added && m.draft.name == name) || (!m.draft.added && m.draft.orig == name))
	prev := ""
	if m.draft != nil {
		// The draft's list identity: renamed drafts stay listed under orig.
		prev = m.draft.orig
		if m.draft.added {
			prev = m.draft.name
		}
	}
	if !keep {
		m.draft = newDraft(m.st.cfg, name)
	}
	m.fields = sessionFields(m.draft.sess)
	// A stale cursor on a different session lands Enter on the wrong
	// field; keep the position only while it's the same session.
	if name != prev || m.fieldSel > len(m.fields)-1 {
		m.fieldSel = 0
	}
}

func (m *editorModel) isDirty() bool { return m.draft.dirty(m.st.cfg) }

func (m editorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.keepVisible()
		if m.mode == modeWizard && m.wizard != nil {
			nm, _ := m.wizard.Update(msg)
			if aw, ok := nm.(addModel); ok {
				m.wizard = &aw
			}
		}
		return m, nil
	case editorReturnMsg:
		m.leaving = false // back from $EDITOR
		if msg.err != nil {
			m.status = "editor: " + msg.err.Error()
			m.statusErr = true
			return m, nil
		}
		st, err := loadEditorState(m.st.path)
		if err != nil {
			// keep the last-good state; the user can fix the file and retry
			m.status = "config now invalid — fix it and press o again: " + err.Error()
			m.statusErr = true
			return m, nil
		}
		m.st = st
		m.draft = nil
		m.refilter()
		if m.sel > len(m.visible)-1 {
			m.sel = len(m.visible) - 1
		}
		m.resetDraft()
		m.status = "reloaded after external edit"
		m.statusErr = false
		return m, nil
	case tea.KeyMsg:
		if m.help {
			switch {
			case msg.Type == tea.KeyCtrlC:
				m.help = false
				return m.requestQuit()
			case msg.Type == tea.KeyEsc || string(msg.Runes) == "?":
				m.help = false
			case msg.Type == tea.KeyDown || string(msg.Runes) == "j":
				m.helpOff = min(m.helpOff+1, max(0, len(m.helpActions())-1))
			case msg.Type == tea.KeyUp || string(msg.Runes) == "k":
				m.helpOff = max(0, m.helpOff-1)
			}
			return m, nil
		}
		textInput := m.mode == modeFilter || m.mode == modeFieldEdit || m.mode == modeInput || (m.mode == modeListEdit && m.listEd.editing)
		if m.mode != modeWizard && !textInput && string(msg.Runes) == "?" {
			m.help, m.helpOff = true, 0
			return m, nil
		}
		if msg.Type == tea.KeyCtrlC && m.mode != modeWizard && m.mode != modeGuard {
			return m.requestQuit()
		}
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			return replayRunes(m, msg.Runes)
		}
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
		case modeDiff:
			return m.updateDiff(msg)
		case modeStale:
			return m.updateStale(msg)
		case modeGuard:
			return m.updateGuard(msg)
		case modeWizard:
			return m.updateWizard(msg)
		}
	}
	return m, nil
}

func (m editorModel) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.quit()
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
			// Clearing the filter re-derives the visible list; put the
			// selection back on the active draft's session first so a
			// dirty draft is never silently replaced.
			draftName := ""
			if m.draft != nil {
				draftName = m.draft.orig
				if m.draft.added {
					draftName = m.draft.name
				}
			}
			m.filter = nil
			m.refilter()
			for i, n := range m.visible {
				if n == draftName {
					m.sel = i
					break
				}
			}
			m.keepVisible()
			m.resetDraft()
			return m, nil
		}
		return m.requestQuit()
	}
	switch string(msg.Runes) {
	case "q":
		return m.requestQuit()
	case "j":
		return m.moveCursor(1)
	case "k":
		return m.moveCursor(-1)
	case "/":
		if m.pane == paneList {
			if m.isDirty() {
				m.status = "unsaved changes — s save / u discard before filtering"
				m.statusErr = true
				return m, nil
			}
			m.mode = modeFilter
		}
		return m, nil
	case "a":
		if m.isDirty() {
			m.status = "unsaved changes — s save / u discard before adding"
			m.statusErr = true
			return m, nil
		}
		w := newAddModel(m.st.cfg, m.clusters, "")
		w.width = m.width // embedded: no initial WindowSizeMsg arrives
		w.height = m.height
		w.embedded = true
		m.wizard = &w
		m.mode = modeWizard
		m.pane = paneForm
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
			m.status = "unsaved changes — s save / u discard before duplicating"
			m.statusErr = true
			return m, nil
		}
		m.mode = modeInput
		m.inputPurpose = inputDuplicate
		m.input = nil
		m.inputErr = ""
		return m, nil
	case "s":
		return m.startSave()
	case "u":
		if m.isDirty() {
			m.guardReturn = m.mode
			m.mode = modeGuard
			m.pending = pendingAction{}
		}
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
			m.mode = modeConfirmDelete
			return m, nil
		}
		if len(m.st.cfg.Sessions) == 1 {
			m.status = "cannot delete the last session (a config needs at least one)"
			m.statusErr = true
			return m, nil
		}
		m.mode = modeConfirmDelete
		return m, nil
	case "o":
		if m.isDirty() {
			m.status = "unsaved changes — s save / u discard before opening $EDITOR"
			m.statusErr = true
			return m, nil
		}
		editor := editorCommand()
		if editor == "" {
			m.status = "no editor found: set $VISUAL or $EDITOR"
			m.statusErr = true
			return m, nil
		}
		parts := strings.Fields(editor)
		ed := exec.Command(parts[0], append(parts[1:], m.st.path)...) //nolint:gosec // the user's own $EDITOR choice
		m.leaving = true
		return m, tea.ExecProcess(ed, func(err error) tea.Msg { return editorReturnMsg{err: err} })
	}
	return m, nil
}

func (m editorModel) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.quit()
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
		return m.requestSelect(next)
	}
	if len(m.fields) > 0 {
		m.fieldSel = clampChoice(m.fieldSel+delta, len(m.fields))
	}
	return m, nil
}

// quit ends the editor, marking it so a batched rune replay stops here.
func (m editorModel) quit() (tea.Model, tea.Cmd) {
	m.leaving = true
	return m, tea.Quit
}

func (m editorModel) isLeaving() bool { return m.leaving }

// selectIndex moves the list selection and resets the draft. The guard
// interposition is done by requestSelect.
func (m editorModel) selectIndex(idx int) (tea.Model, tea.Cmd) {
	m.sel = idx
	m.keepVisible()
	m.resetDraft()
	m.status = ""
	m.statusErr = false
	m.statusOK = false
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
		return m.quit()
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
		return m.quit()
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
		return m.quit()
	}
	if msg.Type == tea.KeyEsc || string(msg.Runes) == "n" || string(msg.Runes) == "N" {
		m.mode = modeBrowse
		return m, nil
	}
	if string(msg.Runes) == "y" || string(msg.Runes) == "Y" || string(msg.Runes) == "d" || string(msg.Runes) == "D" || msg.Type == tea.KeyEnter {
		if m.draft.added {
			m.draft = nil
			m.refilter()
			m.resetDraft()
			m.mode = modeBrowse
			m.status = "discarded unsaved session"
			m.statusErr = false
			return m, nil
		}
		m.draft.deleted = true
		m.mode = modeBrowse
		m.status = "delete pending — press s to save, D to undo"
		m.statusErr = false
	}
	return m, nil
}

// startSave validates the draft and opens the diff preview.
func (m editorModel) startSave() (tea.Model, tea.Cmd) {
	if !m.isDirty() {
		m.status = "no changes to save"
		m.statusErr = false
		return m, nil
	}
	if err := m.st.nextConfig(m.draft).Validate(); err != nil {
		m.status = err.Error()
		m.statusErr = true
		m.jumpToErrorField(err)
		return m, nil
	}
	m.diff = draftDiff(m.st.cfg, m.draft)
	m.diffOff = 0
	m.mode = modeDiff
	return m, nil
}

// updateDiff is the save-preview modal: Enter, y, or s writes, Esc backs out.
func (m editorModel) updateDiff(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m.quit()
	case tea.KeyEsc:
		m.mode = modeBrowse
		return m, nil
	case tea.KeyEnter:
		m, _ = m.finishSave()
		return m, nil
	case tea.KeyUp:
		m.diffOff = clampChoice(m.diffOff-1, len(m.diff))
		return m, nil
	case tea.KeyDown:
		m.diffOff = clampChoice(m.diffOff+1, len(m.diff))
		return m, nil
	}
	switch string(msg.Runes) {
	case "y", "Y", "s", "S":
		m, _ = m.finishSave()
		return m, nil
	case "k":
		m.diffOff = clampChoice(m.diffOff-1, len(m.diff))
	case "j":
		m.diffOff = clampChoice(m.diffOff+1, len(m.diff))
	}
	return m, nil
}

// updateStale is the refused-save view: R reloads from disk.
func (m editorModel) updateStale(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || string(msg.Runes) == "n" || string(msg.Runes) == "N" {
		m.mode = modeBrowse
		return m, nil
	}
	if msg.Type == tea.KeyCtrlC {
		return m.quit()
	}
	if string(msg.Runes) == "R" || string(msg.Runes) == "r" {
		st, err := loadEditorState(m.st.path)
		if err != nil {
			m.mode = modeBrowse
			m.status = "reload failed: " + err.Error()
			m.statusErr = true
			return m, nil
		}
		m.st = st
		m.draft = nil
		m.pending = pendingAction{}
		m.refilter()
		if m.sel > len(m.visible)-1 {
			m.sel = len(m.visible) - 1
		}
		m.resetDraft()
		m.mode = modeBrowse
		m.status = "reloaded from disk"
		m.statusErr = false
	}
	return m, nil
}

// requestSelect moves the list selection, interposing the guard when the
// active draft has unsaved changes.
func (m editorModel) requestSelect(idx int) (tea.Model, tea.Cmd) {
	if m.isDirty() {
		m.guardReturn = m.mode
		m.mode = modeGuard
		m.pending = pendingAction{kind: pendingSelect, target: idx}
		return m, nil
	}
	return m.selectIndex(idx)
}

// requestQuit quits, interposing the guard when the draft is dirty.
func (m editorModel) requestQuit() (tea.Model, tea.Cmd) {
	if m.isDirty() || m.hasPendingInput() {
		m.guardReturn = m.mode
		m.mode = modeGuard
		m.pending = pendingAction{kind: pendingQuit}
		return m, nil
	}
	return m.quit()
}

func (m editorModel) hasPendingInput() bool {
	if m.draft == nil {
		return false
	}
	mode := m.mode
	if mode == modeGuard {
		mode = m.guardReturn
	}
	switch mode {
	case modeFieldEdit:
		return string(m.input) != m.fields[m.fieldSel].text(m.draft.sess)
	case modeInput:
		if m.inputPurpose == inputDuplicate {
			return len(m.input) > 0
		}
		return string(m.input) != m.draft.name
	case modeListEdit:
		if m.listEd.editing {
			if m.listEd.adding {
				return len(m.listEd.input) > 0
			}
			items := m.fields[m.listEd.field].list(m.draft.sess)
			return string(m.listEd.input) != (*items)[m.listEd.sel]
		}
	}
	return false
}

// updateGuard resolves the save/discard/stay prompt, then continues the
// pending action. Guard-save skips the diff preview — choosing "save" is
// the confirmation.
func (m editorModel) updateGuard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, nil
	}
	if msg.Type == tea.KeyEsc || string(msg.Runes) == "n" || string(msg.Runes) == "N" {
		m.mode = m.guardReturn
		m.pending = pendingAction{}
		return m, nil
	}
	switch string(msg.Runes) {
	case "s", "S":
		// A quit from a text input must offer saving the text the user just
		// typed, not only the last committed draft. Validation keeps focus
		// in the input when it cannot be committed.
		switch m.guardReturn {
		case modeFieldEdit, modeInput:
			m.mode = m.guardReturn
			var nm tea.Model
			if m.mode == modeFieldEdit {
				nm, _ = m.updateFieldEdit(tea.KeyMsg{Type: tea.KeyEnter})
			} else {
				nm, _ = m.updateInput(tea.KeyMsg{Type: tea.KeyEnter})
			}
			m = nm.(editorModel)
			if m.mode != modeBrowse {
				m.pending = pendingAction{}
				return m, nil
			}
		case modeListEdit:
			if m.listEd.editing {
				m.commitListInput()
				if m.listEd.editing {
					m.mode = modeListEdit
					m.pending = pendingAction{}
					return m, nil
				}
			}
		}
		var ok bool
		m, ok = m.finishSave()
		if !ok {
			m.pending = pendingAction{}
			return m, nil // finishSave set mode/status
		}
		return m.continuePending()
	case "d", "D":
		m.draft = nil // drop an added draft entirely; resetDraft rebuilds others
		m.refilter()
		m.resetDraft()
		m.mode = modeBrowse
		return m.continuePending()
	}
	return m, nil
}

// continuePending performs the action the guard was protecting.
func (m editorModel) continuePending() (tea.Model, tea.Cmd) {
	p := m.pending
	m.pending = pendingAction{}
	m.mode = modeBrowse
	switch p.kind {
	case pendingQuit:
		return m.quit()
	case pendingSelect:
		idx := p.target
		if idx > len(m.visible)-1 {
			idx = len(m.visible) - 1
		}
		if idx < 0 {
			return m, nil
		}
		return m.selectIndex(idx)
	}
	return m, nil
}

// updateWizard forwards input to the embedded 'mox add' wizard and, when it
// finishes, converts its result into the active draft. The wizard's file
// write never runs here — the editor's save pipeline is the only writer.
// "save + start now" jumps straight into that pipeline (diff → write) with
// the draft marked to start detached once the save lands.
func (m editorModel) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyTab && !m.wizard.cancelConfirm && !m.wizard.help {
		if m.pane == paneForm {
			m.pane = paneList
		} else {
			m.pane = paneForm
		}
		return m, nil
	}
	if m.pane == paneList && !m.wizard.cancelConfirm && !m.wizard.help && msg.Type != tea.KeyCtrlC {
		if msg.Type == tea.KeyEsc {
			m.pane = paneForm
		}
		return m, nil
	}
	nm, _ := m.wizard.Update(msg) // swallow the wizard's tea.Quit
	aw, ok := nm.(addModel)
	if !ok {
		return m, nil
	}
	m.wizard = &aw
	if !aw.finished {
		return m, nil
	}
	res := aw.done
	m.wizard = nil
	m.mode = modeBrowse
	if res.action == addActionCancel {
		if aw.cancelQuit {
			return m.requestQuit()
		}
		return m, nil
	}
	d := &sessionDraft{name: res.name, sess: res.sess, startAfter: res.action == addActionSaveStart}
	if _, exists := m.st.cfg.Sessions[res.name]; exists {
		d.orig = res.name // wizard-confirmed overwrite of an existing session
	} else {
		d.added = true
	}
	m.draft = d
	m.refilter()
	for i, n := range m.visible {
		if n == res.name {
			m.sel = i
			break
		}
	}
	m.keepVisible()
	m.fields = sessionFields(d.sess)
	m.fieldSel = 0
	m.pane = paneForm
	if d.startAfter {
		return m.startSave()
	}
	m.status = "new session drafted — press s to save it"
	m.statusErr = false
	return m, nil
}

// finishSave applies the active draft: validate → staleness → write. It
// refreshes the model on success and reports ok=false when the save did
// not happen (validation error or stale file — mode/status already set).
// The diff preview's confirm (updateDiff) calls this too.
func (m editorModel) finishSave() (editorModel, bool) {
	d := m.draft
	err := m.st.applyDraft(d)
	switch {
	case errors.Is(err, errStaleConfig):
		m.mode = modeStale
		return m, false
	case err != nil:
		m.mode = modeBrowse
		m.status = err.Error()
		m.statusErr = true
		m.statusOK = false
		m.jumpToErrorField(err)
		return m, false
	}
	m.draft = nil
	m.refilter()
	if !d.deleted {
		for i, n := range m.visible {
			if n == d.name {
				m.sel = i
				break
			}
		}
	}
	if m.sel > len(m.visible)-1 {
		m.sel = len(m.visible) - 1
	}
	m.keepVisible()
	m.resetDraft()
	m.mode = modeBrowse
	verb := "saved "
	if d.deleted {
		verb = "deleted "
	}
	m.status = verb + d.name + " ✓"
	if info, ok := m.running[d.name]; ok && info.Running {
		m.status += " — session is running; changes apply on next build"
	}
	m.statusErr = false
	m.statusOK = true
	if d.startAfter && !d.deleted {
		m.startSaved(d.name)
	}
	return m, true
}

// startSaved honors a draft's "save + start now" intent: start the session
// detached from the freshly saved config. Synchronous — session builds are
// tmux calls, quick enough to run on the UI thread, and this way the start
// also happens when a guard-save quits the editor right after.
func (m *editorModel) startSaved(name string) {
	if m.startSession == nil {
		m.status = "saved " + name + " ✓ — start skipped: tmux unavailable"
		m.statusErr = true
		m.statusOK = false
		return
	}
	if err := m.startSession(m.st.cfg, name); err != nil {
		m.status = "saved " + name + " ✓ — start failed: " + err.Error()
		m.statusErr = true
		m.statusOK = false
		return
	}
	m.status = "saved + started " + name + " ✓ (detached)"
	m.running[name] = session.SessionInfo{Name: name, Running: true, Managed: true}
}

// jumpToErrorField moves the form cursor to the field a validation error
// names, when one matches (best-effort substring match on field keys).
func (m *editorModel) jumpToErrorField(err error) {
	msg := err.Error()
	// Pane/window errors belong to the windows structure row, and their
	// text often contains "split: root" — which would false-match the
	// session-level root (working directory) field below.
	if strings.Contains(msg, "window") || strings.Contains(msg, "pane") {
		// A named simple-mode window's editable hosts row beats the
		// read-only structure row when the error is about its hosts.
		if strings.Contains(msg, "hosts") {
			for i, f := range m.fields {
				if name, ok := strings.CutSuffix(f.key, " hosts"); ok && strings.Contains(msg, "\""+name+"\"") {
					m.fieldSel = i
					m.pane = paneForm
					return
				}
			}
		}
		for i, f := range m.fields {
			if f.key == "windows" {
				m.fieldSel = i
				m.pane = paneForm
				return
			}
		}
		return // structural error on a session with no windows row: no jump
	}
	bestIdx := -1
	bestPos := len(msg)
	for i, f := range m.fields {
		if pos := strings.Index(msg, f.key); pos >= 0 && pos < bestPos {
			bestIdx = i
			bestPos = pos
		}
	}
	if bestIdx >= 0 {
		m.fieldSel = bestIdx
		m.pane = paneForm
	}
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
		pkAccent.Render("▸ ") + inputTail(string(m.input), w-2),
	}
	if m.inputErr != "" {
		lines = append(lines, "", pkErr.Render(truncate(m.inputErr, w)))
	}
	return lines
}

// confirmDeleteLines renders the delete confirmation in the right pane.
func (m editorModel) confirmDeleteLines(w int) []string {
	if m.draft.added {
		return []string{
			pkErr.Render(truncate(fmt.Sprintf("discard unsaved session %q?", m.draft.name), w)),
			"",
			"This draft has not been saved to the config.",
		}
	}
	return []string{
		pkErr.Render(truncate(fmt.Sprintf("delete session %q from the config?", m.draft.name), w)),
		"",
		pkDim.Render("The change is buffered — the file is only touched on save (s)."),
	}
}

// guardLines renders the unsaved-changes prompt in the right pane.
func (m editorModel) guardLines(w int) []string {
	return []string{
		pkForeign.Render(truncate(fmt.Sprintf("%q has unsaved changes", m.draft.name), w)),
		"",
		"  " + pkSelected.Render("s") + pkDim.Render("  save them, then continue"),
		"  " + pkSelected.Render("d") + pkDim.Render("  discard them, then continue"),
		"  " + pkSelected.Render("esc") + pkDim.Render("  stay here"),
	}
}

// diffModalLines renders the save preview. The diff shows both
// sides in canonical (re-encoded) form — a semantic preview, not a byte
// diff of the file; unchanged context lines may differ in formatting from
// what is on disk.
func (m editorModel) diffModalLines(w, h int) []string {
	title := fmt.Sprintf("save %q — changes shown in canonical form:", m.draft.name)
	if m.draft.deleted {
		title = fmt.Sprintf("delete %q — review the change:", m.draft.name)
	}
	lines := []string{pkDim.Render(truncate(title, w)), ""}
	rows := h - 3
	if rows < 1 {
		rows = 1
	}
	end := m.diffOff + rows
	if end > len(m.diff) {
		end = len(m.diff)
	}
	for _, dl := range m.diff[m.diffOff:end] {
		switch dl.kind {
		case diffAdd:
			lines = append(lines, pkDiffAdd.Render(truncate("+ "+dl.text, w)))
		case diffDel:
			lines = append(lines, pkDiffDel.Render(truncate("- "+dl.text, w)))
		default:
			lines = append(lines, pkDim.Render(truncate("  "+dl.text, w)))
		}
	}
	if end < len(m.diff) {
		lines = append(lines, pkDim.Render(fmt.Sprintf("  … %d more (j/k to scroll)", len(m.diff)-end)))
	}
	return lines
}

// staleLines renders the refused-save explanation.
func (m editorModel) staleLines(w int) []string {
	return []string{
		pkErr.Render("save refused: the config file changed on disk"),
		"",
		pkDim.Render(truncate("Something else wrote "+m.st.path+" after the editor loaded it.", w)),
		"",
		"  " + pkSelected.Render("R") + pkDim.Render("    reload from disk (discards your unsaved changes)"),
		"  " + pkSelected.Render("esc") + pkDim.Render("  go back (your draft is kept, but saving stays blocked)"),
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
	return terminalLayout(m.width, m.height).contentHeight + 2
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
	l := terminalLayout(m.width, m.height)
	detail, title := m.rightLines(l.detailWidth, l.contentHeight), m.formTitle()
	focus := m.pane == paneForm || (m.mode != modeBrowse && m.mode != modeFilter && m.mode != modeWizard)
	if m.mode == modeWizard && m.wizard != nil {
		title = m.wizard.title()
		focus = m.pane == paneForm || m.wizard.cancelConfirm || m.wizard.help
	}
	if m.help {
		detail = helpLines(m.helpActions(), l.detailWidth)
		detail = detail[min(m.helpOff, len(detail)):]
		title, focus = "help / edit", true
	}
	return renderWorkspace(workspaceView{width: m.width, height: m.height, view: "edit",
		context:   fmt.Sprintf("%s · %d sessions", filepath.Base(m.st.path), len(m.st.cfg.Sessions)),
		listTitle: m.listTitle(), detailTitle: title, list: m.listLines(l.listWidth, l.contentHeight),
		detail: detail, focusDetail: focus, actions: m.actionHints(), status: m.statusLine()})
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

func (m editorModel) actionHints() []keyHint {
	if m.help {
		return []keyHint{{"Esc", "back"}, {"j/k", "scroll"}}
	}
	switch m.mode {
	case modeFilter:
		return []keyHint{{"Enter", "keep filter"}, {"Esc", "clear"}}
	case modeFieldEdit:
		return []keyHint{{"Enter", "commit"}, {"Esc", "cancel"}}
	case modeListEdit:
		if m.listEd.editing {
			return []keyHint{{"Enter", "commit"}, {"Esc", "cancel"}}
		}
		actions := []keyHint{{"a", "add"}, {"Esc", "back"}}
		items := m.fields[m.listEd.field].list(m.draft.sess)
		if len(*items) > 0 {
			actions = append(actions, keyHint{"e", "edit"}, keyHint{"d", "delete"})
			if m.listEd.sel > 0 {
				actions = append(actions, keyHint{"K", "move up"})
			}
			if m.listEd.sel < len(*items)-1 {
				actions = append(actions, keyHint{"J", "move down"})
			}
		}
		return append(actions, keyHint{"?", "help"})
	case modeInput:
		return []keyHint{{"Enter", "confirm"}, {"Esc", "cancel"}}
	case modeConfirmDelete:
		if m.draft.added {
			return []keyHint{{"y", "discard"}, {"Esc", "stay"}, {"?", "help"}}
		}
		return []keyHint{{"y", "delete"}, {"Esc", "cancel"}, {"?", "help"}}
	case modeDiff:
		return []keyHint{{"Enter", "write config"}, {"Esc", "back"}, {"j/k", "scroll"}, {"?", "help"}}
	case modeGuard:
		return []keyHint{{"s", "save"}, {"d", "discard"}, {"Esc", "stay"}, {"?", "help"}}
	case modeStale:
		return []keyHint{{"Esc", "keep draft"}, {"R", "reload / discard"}, {"?", "help"}}
	case modeWizard:
		if m.wizard != nil {
			if m.wizard.cancelConfirm || m.wizard.help {
				return m.wizard.actionHints()
			}
			if m.pane == paneList {
				return []keyHint{{"Tab", "add session"}, {"Esc", "add session"}}
			}
			actions := m.wizard.actionHints()
			return append(actions, keyHint{"Tab", "sessions"})
		}
	}
	actions := []keyHint{{"Enter", "edit"}, {"Tab", "sessions"}}
	if m.pane == paneList {
		actions = []keyHint{{"Enter", "fields"}}
		if !m.isDirty() {
			actions = append(actions, keyHint{"a", "add"}, keyHint{"/", "filter"})
		}
	} else if m.draft != nil && len(m.fields) > m.fieldSel && m.fields[m.fieldSel].kind == fieldCycle {
		actions = append(actions, keyHint{"Space", "cycle"})
	}
	if m.isDirty() {
		actions = append(actions, keyHint{"s", "save"}, keyHint{"u", "discard edits"})
	}
	if m.draft == nil {
		filtered := actions[:0]
		for _, action := range actions {
			if action.key != "Enter" {
				filtered = append(filtered, action)
			}
		}
		actions = filtered
	}
	if !m.isDirty() {
		actions = append(actions, keyHint{"o", "YAML"})
	}
	return append(actions, keyHint{"q", "quit"}, keyHint{"?", "help"})
}

func (m editorModel) helpActions() []keyHint {
	m.help = false
	actions := append([]keyHint{{"j/k, ↑/↓", "navigate the focused area"}}, m.actionHints()...)
	if m.mode == modeBrowse {
		if !m.isDirty() {
			actions = append(actions, keyHint{"a", "add session"})
			if m.draft != nil {
				actions = append(actions, keyHint{"y", "duplicate session"})
			}
		}
		if m.draft != nil {
			if !m.draft.deleted {
				actions = append(actions, keyHint{"r", "rename session"})
			}
			if len(m.st.cfg.Sessions) > 1 || m.draft.added || m.draft.deleted {
				if m.draft.added {
					actions = append(actions, keyHint{"D", "discard unsaved session"})
				} else {
					actions = append(actions, keyHint{"D", "delete / undo deletion"})
				}
			}
		}
	}
	return actions
}

// listLines renders the filter line plus the visible session rows.
func (m editorModel) listLines(w, h int) []string {
	header := pkDim.Render("/ filter")
	if m.mode == modeFilter || len(m.filter) > 0 {
		header = pkAccent.Render("▸ ") + string(m.filter) + pkAccent.Render("█")
	}
	lines := []string{
		header,
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
		state := "stopped"
		if info, ok := m.running[name]; ok && info.Running {
			state = "running"
		}
		display := name
		if m.draft != nil && m.draft.orig == name && m.draft.name != name {
			display = m.draft.name + "*"
		}
		if m.draft != nil && (m.draft.orig == name || m.draft.name == name) && m.isDirty() {
			state = "draft"
			if m.draft.deleted {
				state = "delete"
			}
		}
		lines = append(lines, sessionRow(display, state, i == m.sel, w))
	}
	if end < len(m.visible) {
		lines = append(lines, pkDim.Render(fmt.Sprintf("  … %d more", len(m.visible)-end)))
	}
	return lines
}

// rightLines picks the right pane's content by mode.
func (m editorModel) rightLines(w, h int) []string {
	switch m.mode {
	case modeWizard:
		if m.wizard != nil {
			return m.wizard.detailLines(w, h)
		}
	case modeListEdit:
		return m.listEditLines(w, h)
	case modeInput:
		return m.inputLines(w)
	case modeConfirmDelete:
		return m.confirmDeleteLines(w)
	case modeGuard:
		return m.guardLines(w)
	case modeDiff:
		return m.diffModalLines(w, h)
	case modeStale:
		return m.staleLines(w)
	}
	return m.formLines(w, h)
}

// formLines renders the field rows plus the focused field's help text.
func (m editorModel) formLines(w, h int) []string {
	if m.draft == nil {
		if len(m.filter) > 0 || m.mode == modeFilter {
			return []string{pkDim.Render("No session matches the filter."), pkDim.Render("Press Esc to clear the search.")}
		}
		return []string{pkDim.Render("no sessions — press a to add one")}
	}
	var lines []string
	if m.draft.deleted {
		lines = append(lines, pkErr.Render("delete pending — press s to save, D to undo"), "")
	}
	labelW := 9
	for _, f := range m.fields {
		labelW = max(labelW, ansi.StringWidth(f.key))
	}
	labelW = min(labelW, max(1, w/3))
	fieldRows := max(1, h-len(lines)-3)
	start := max(0, m.fieldSel-fieldRows+1)
	end := min(len(m.fields), start+fieldRows)
	for i := start; i < end; i++ {
		f := m.fields[i]
		label := fitCell(f.key, labelW)
		val := clipText(f.display(m.draft.sess), w-labelW-4)
		switch {
		case i == m.fieldSel && m.mode == modeFieldEdit:
			in := inputTail(string(m.input), w-labelW-3)
			lines = append(lines, pkAccent.Render("▸ ")+pkSelected.Render(label)+" "+in)
		case i == m.fieldSel && m.pane == paneForm && m.mode != modeListEdit:
			lines = append(lines, pkAccent.Render("▸ ")+pkSelected.Render(label)+" "+val)
		default:
			lines = append(lines, "  "+pkDim.Render(label)+" "+val)
		}
	}
	if m.mode == modeFieldEdit && m.inputErr != "" {
		lines = append(lines, "", pkErr.Render(clipText("Error: "+m.inputErr, w)))
		return lines
	}
	lines = append(lines, "")
	if m.pane == paneForm && len(m.fields) > 0 && m.fieldSel < len(m.fields) {
		helps := wrapWords(strings.Fields(m.fields[m.fieldSel].help), w)
		for _, hl := range helps[:min(len(helps), max(0, h-len(lines)))] {
			lines = append(lines, pkDim.Render(hl))
		}
	}
	return lines
}

func (m editorModel) statusLine() string {
	var parts []string
	if m.mode == modeWizard && m.wizard != nil {
		return pkForeign.Render("Draft not saved · " + m.wizard.stepTitle())
	}
	if m.isDirty() {
		parts = append(parts, pkForeign.Render("unsaved: "+m.draft.name))
	} else if m.hasPendingInput() {
		parts = append(parts, pkForeign.Render("Editing · not saved: "+m.draft.name))
	}
	if m.status != "" {
		style := pkDim
		switch {
		case m.statusErr:
			style = pkErr
		case m.statusOK:
			style = pkOK
		}
		parts = append(parts, style.Render(m.status))
	}
	if len(parts) == 0 {
		return "Saved · changes apply on next build"
	}
	return strings.Join(parts, pkDim.Render(" · "))
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
	if m.fields[m.listEd.field].expand {
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
			return m.quit()
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
		return m.quit()
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
	if m.listEd.editing {
		lines = append(lines, pkAccent.Render("▸ ")+inputTail(string(m.listEd.input), w-2))
		if m.listEd.errMsg != "" {
			lines = append(lines, pkErr.Render(clipText("Error: "+m.listEd.errMsg, w)))
		}
		lines = append(lines, "")
	}
	rows := max(0, h-len(lines)-2)
	if !m.listEd.editing {
		rows = max(1, rows)
	}
	start := max(0, m.listEd.sel-rows+1)
	end := min(len(*items), start+rows)
	for i := start; i < end; i++ {
		row := clipText((*items)[i], w-4)
		if i == m.listEd.sel && !m.listEd.editing {
			lines = append(lines, pkAccent.Render("▌ ")+pkSelected.Render(row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if m.listEd.errMsg != "" && !m.listEd.editing {
		lines = append(lines, "", pkErr.Render(truncate(m.listEd.errMsg, w)))
	}
	if f.expand {
		lines = append(lines, "", pkDim.Render(truncate("@cluster expands on commit (config + clusterssh)", w)))
	}
	return lines
}
