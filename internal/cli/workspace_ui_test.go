package cli

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/bthall/mox/internal/config"
)

func assertTerminalBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		t.Fatalf("view has %d rows in a %d-row terminal", len(lines), height)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("row %d is %d cells wide in a %d-column terminal: %q", i, w, width, line)
		}
	}
}

func TestHintWidthAndResetSuffix(t *testing.T) {
	t.Run("hintWidth ASCII vs Unicode", func(t *testing.T) {
		gotASCII := hintWidth("Enter", "attach")
		wantASCII := ansi.StringWidth("Enter") + 1 + ansi.StringWidth("attach")
		if gotASCII != wantASCII {
			t.Errorf("hintWidth(ASCII) = %d, want %d", gotASCII, wantASCII)
		}

		gotUni := hintWidth("Enter", "attach 🚀")
		wantUni := ansi.StringWidth("Enter") + 1 + ansi.StringWidth("attach 🚀")
		if gotUni != wantUni {
			t.Errorf("hintWidth(Unicode) = %d, want %d", gotUni, wantUni)
		}
	})

	t.Run("clipTextWidth reset suffix handling", func(t *testing.T) {
		input := "\x1b[31mhello\x1b[0m"
		got, w := clipTextWidth(input, 10)
		if w != 5 {
			t.Errorf("clipTextWidth width = %d, want 5", w)
		}
		if strings.HasSuffix(got, "\x1b[0m\x1b[0m") {
			t.Errorf("clipTextWidth produced duplicate reset suffix: %q", got)
		}
	})
}

func TestWorkspaceViewsRespectTerminalBounds(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 35}, {18, 8}, {1, 1}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			w, h := size[0], size[1]
			hub := testHubModel(t, nil)
			hub.width, hub.height = w, h
			hub.previewName = hub.selectedName()
			hub.previewBody = []string{"\x1b[32m界界界界\x1b[0m " + strings.Repeat("long output ", 20)}
			assertTerminalBounds(t, hub.View(), w, h)
			editor := testEditorModel(t)
			editor.width, editor.height = w, h
			for _, mode := range []editorMode{modeBrowse, modeFieldEdit, modeListEdit, modeDiff, modeGuard, modeStale} {
				editor.mode = mode
				if mode == modeFieldEdit {
					for i, field := range editor.fields {
						if field.kind == fieldText || field.kind == fieldNumber {
							editor.fieldSel = i
							break
						}
					}
				}
				if mode == modeListEdit {
					for i, field := range editor.fields {
						if field.kind == fieldList {
							editor.listEd.field = i
							break
						}
					}
				}
				editor.input = []rune(strings.Repeat("界", 80))
				assertTerminalBounds(t, editor.View(), w, h)
			}
			add := newTestAddModel()
			add.width, add.height = w, h
			for _, step := range []addStep{stepName, stepHosts, stepCommands, stepConfirm} {
				add = add.goTo(step)
				add.input = []rune(strings.Repeat("界", 80))
				assertTerminalBounds(t, add.View(), w, h)
			}
		})
	}
}

func TestEmbeddedWizardKeepsEditorWorkspace(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "a")
	view := m.View()
	for _, want := range []string{"mox / edit", "sessions", "solo", "webfarm", "add session", "step 1/8"} {
		if !strings.Contains(view, want) {
			t.Errorf("embedded add lost %q: %s", want, view)
		}
	}
	m = edRunes(t, m, "new-draft")
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = nm.(editorModel)
	assertTerminalBounds(t, m.View(), 40, 12)
	if string(m.wizard.input) != "new-draft" {
		t.Fatal("resize lost wizard input")
	}
}

func TestHubShowsOnlyAvailableActions(t *testing.T) {
	m := testHubModel(t, nil)
	if strings.Contains(m.footer(), "S start") || strings.Contains(m.footer(), "i import") {
		t.Fatal("running managed session shows unavailable start/import actions")
	}
	m, _ = hubRunes(t, m, "j")
	if !strings.Contains(m.footer(), "S start") || strings.Contains(m.footer(), "K kill") {
		t.Fatal("stopped session action hints do not match its state")
	}
}

func TestHubNarrowTabShowsPreviewAndKeepsSelection(t *testing.T) {
	m := testHubModel(t, nil)
	m.width, m.height = 40, 12
	m = drain(t, m, m.Init())
	name := m.selectedName()
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(hubModel)
	if !strings.Contains(m.View(), "tail -f app.log") {
		t.Fatal("Tab did not expose the narrow-terminal preview")
	}
	if m.selectedName() != name {
		t.Fatal("focus change changed selection")
	}
}

func TestEditorCtrlCProtectsUncommittedInput(t *testing.T) {
	m := testEditorModel(t)
	m.pane = paneForm
	for i, f := range m.fields {
		if f.key == "root" {
			m.fieldSel = i
		}
	}
	m = edType(t, m, tea.KeyEnter)
	m = edType(t, m, tea.KeyCtrlU)
	m = edRunes(t, m, "/tmp/redesign")
	m = edType(t, m, tea.KeyCtrlC)
	if m.mode != modeGuard || m.leaving {
		t.Fatal("Ctrl+C abandoned uncommitted input")
	}
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeFieldEdit || string(m.input) != "/tmp/redesign" {
		t.Fatal("staying did not restore the active input")
	}
	m = edType(t, m, tea.KeyCtrlC)
	m = edRunes(t, m, "s")
	if !m.leaving {
		t.Fatal("guard save did not continue the requested quit")
	}
	cfg, err := config.Load(m.st.path)
	if err != nil || cfg.Sessions["solo"].Root != "/tmp/redesign" {
		t.Fatalf("guard did not save the typed value: %v", err)
	}
}

func TestAddBackPreservesCommandsAndInput(t *testing.T) {
	m := newTestAddModel()
	m = through(t, m, "new-work")
	m = through(t, m, "")
	m = through(t, m, "/tmp")
	m = through(t, m, "echo ready")
	m = sendAdd(t, m, runes("echo unfinished")...)
	m = sendAdd(t, m, keyEsc)
	m = sendAdd(t, m, keyEnter)
	if m.step != stepCommands || len(m.commands) != 1 || m.commands[0] != "echo ready" || string(m.input) != "echo unfinished" {
		t.Fatalf("back/forward lost the command draft: %+v", m)
	}
	m = sendAdd(t, m, keyCtrlC)
	if !m.cancelConfirm || m.finished {
		t.Fatal("Ctrl+C discarded a wizard draft")
	}
	m = sendAdd(t, m, keyEsc)
	if string(m.input) != "echo unfinished" {
		t.Fatal("cancel prompt lost input")
	}
}

func TestEditorNewDraftRequiresDiscardConfirmation(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "new-draft")
	for range 5 {
		m = edType(t, m, tea.KeyEnter)
	}
	if m.draft == nil || !m.draft.added {
		t.Fatal("wizard did not return a new draft")
	}
	m = edRunes(t, m, "D")
	if m.mode != modeConfirmDelete || m.draft == nil || !strings.Contains(m.View(), "discard unsaved session") {
		t.Fatal("D discarded a new draft without confirmation")
	}
	hintsStr := renderHints(m.actionHints(), 100)
	if !strings.Contains(hintsStr, "d/y") {
		t.Errorf("modeConfirmDelete for added draft action hints = %q, want 'd/y'", hintsStr)
	}
	m = edType(t, m, tea.KeyEsc)
	if m.mode != modeBrowse || m.draft.name != "new-draft" || !m.isDirty() {
		t.Fatal("Esc did not preserve the new draft")
	}
	m = edRunes(t, m, "D")
	m = edRunes(t, m, "y")
	if m.mode != modeBrowse || m.isDirty() || strings.Contains(m.View(), "new-draft") {
		t.Fatal("confirmed discard did not remove the new draft")
	}
	if _, exists := m.st.cfg.Sessions["solo"]; !exists {
		t.Fatal("discard changed the saved sessions")
	}
}

func TestAddBroadcastChoiceSurvivesUserBacktracking(t *testing.T) {
	m := through(t, newTestAddModel(), "broadcast")
	m = through(t, m, "web1 web2")
	m = through(t, m, "")
	if m.step != stepSync || !m.sync {
		t.Fatal("multi-host default should enable broadcast")
	}
	m = sendAdd(t, m, runes("n")...)
	m = sendAdd(t, m, keyEsc)
	m = sendAdd(t, m, keyEnter)
	if m.step != stepSync || m.sync {
		t.Fatal("backtracking through SSH user reset explicit broadcast choice")
	}
	m = sendAdd(t, m, keyEsc, keyEsc)
	m = sendAdd(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	m = through(t, m, "web1 web2 web3")
	m = sendAdd(t, m, keyEnter)
	if m.step != stepSync || !m.sync {
		t.Fatal("changed hosts did not reseed the broadcast default")
	}
}

func TestEditorDiscardEditsKeepsSavedSession(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "r")
	m = edType(t, m, tea.KeyCtrlU)
	m = edRunes(t, m, "renamed-draft")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "a")
	if !strings.Contains(m.status, "u discard") || strings.Contains(m.status, "discard (D)") {
		t.Fatalf("blocked action offered the wrong recovery: %s", m.status)
	}
	m = edRunes(t, m, "u")
	if m.mode != modeGuard {
		t.Fatal("u did not open save/discard/stay")
	}
	m = edType(t, m, tea.KeyEsc)
	if !m.isDirty() || m.draft.name != "renamed-draft" {
		t.Fatal("staying lost unsaved edits")
	}
	m = edRunes(t, m, "u")
	m = edRunes(t, m, "d")
	if m.mode != modeBrowse || m.isDirty() || m.leaving {
		t.Fatal("discard did not return to the unchanged editor")
	}
	cfg, err := config.Load(m.st.path)
	if err != nil || cfg.Sessions["solo"] == nil || cfg.Sessions["renamed-draft"] != nil {
		t.Fatalf("discard altered the saved config: %v", err)
	}
}

func TestWorkspaceHelpAndTextInputKeys(t *testing.T) {
	hub := testHubModel(t, nil)
	name := hub.selectedName()
	hub, _ = hubRunes(t, hub, "?")
	if !strings.Contains(hub.View(), "Keyboard help") {
		t.Fatal("help key did not show the key map")
	}
	nm, _ := hub.Update(tea.KeyMsg{Type: tea.KeyEsc})
	hub = nm.(hubModel)
	if hub.help || hub.selectedName() != name {
		t.Fatal("closing help changed selection")
	}
	add := newTestAddModel()
	add = sendAdd(t, add, runes("literal/?")...)
	if string(add.input) != "literal/?" || add.help {
		t.Fatal("wizard hijacked printable input for help/filter")
	}
}

func TestWorkspaceNoColorStripsCapturedANSI(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := testHubModel(t, nil)
	m.previewName = m.selectedName()
	m.previewBody = []string{"\x1b[32mplain output\x1b[0m"}
	view := m.View()
	if strings.Contains(view, "\x1b") || !strings.Contains(view, "plain output") {
		t.Fatal("no-color view retained capture color or lost its content")
	}
}

func TestEditorFocusedFieldSurvivesSmallResize(t *testing.T) {
	m := testEditorModel(t)
	m.pane = paneForm
	m.fieldSel = len(m.fields) - 1
	key := m.fields[m.fieldSel].key
	name := m.selectedName()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m = nm.(editorModel)
	if !strings.Contains(m.View(), key) || m.selectedName() != name || m.fields[m.fieldSel].key != key {
		t.Fatal("resize hid the focused field or changed selection")
	}
}

func TestEmbeddedWizardCancelFromSessionRail(t *testing.T) {
	m := testEditorModel(t)
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "new-work")
	m = edType(t, m, tea.KeyTab)
	m = edType(t, m, tea.KeyCtrlC)
	if !m.wizard.cancelConfirm || !strings.Contains(m.View(), "discard") {
		t.Fatal("cancel decision was not visible from the session rail")
	}
	m = edRunes(t, m, "d")
	if !m.leaving {
		t.Fatal("explicit discard did not finish the requested quit")
	}
}

func BenchmarkRenderWorkspace(b *testing.B) {
	v := workspaceView{
		width: 100, height: 30, view: "sessions",
		context:   "2 running · 5 sessions",
		listTitle: "sessions · 5", detailTitle: "dev · running · 2 windows",
		list: []string{
			sessionRow("dev", "running", true, 30),
			sessionRow("prod", "stopped", false, 30),
			sessionRow("staging", "running", false, 30),
			sessionRow("test", "stopped", false, 30),
			sessionRow("demo", "stopped", false, 30),
		},
		detail: []string{
			"win: 0:zsh* 1:vim",
			"live buffer line 1",
			"live buffer line 2",
			"live buffer line 3",
			"live buffer line 4",
		},
		focusDetail: false,
		actions:     []keyHint{{"Enter", "attach"}, {"K", "kill"}, {"q", "quit"}},
		status:      "Ready",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderWorkspace(v)
	}
}

func BenchmarkFitCell(b *testing.B) {
	s := "test session name"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fitCell(s, 50)
	}
}

func TestSessionRowSelectedComplexGraphemeFitsWidth(t *testing.T) {
	previous := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
	for _, name := range []string{"👩‍💻 team", "👨‍👩‍👧‍👦 home", "école"} {
		for _, state := range []string{"running", "stopped", "tmux only", "draft"} {
			for _, width := range []int{20, 24, 32} {
				row := sessionRow(name, state, true, width)
				if got := ansi.StringWidth(row); got > width {
					t.Errorf("selected %q (%s) uses %d cells in a %d-cell row: %q", name, state, got, width, row)
				}
			}
		}
	}
}

func TestFitCellControlBytesKeepDisplayWidth(t *testing.T) {
	for _, input := range []string{"a\tb", "a\rb", "a\bb", "a\x00b", "a\x7fb"} {
		got := fitCell(input, 8)
		if width := ansi.StringWidth(got); width != 8 {
			t.Errorf("fitCell(%q, 8) occupies %d cells: %q", input, width, got)
		}
	}
}

func TestEdgeTextNormalizesNewlinesBeforeCachedPadding(t *testing.T) {
	for _, right := range []string{"", "too long"} {
		if got := edgeText("a\nb", right, 6); got != "a b   " {
			t.Errorf("edgeText with right %q = %q, want normalized six-cell output", right, got)
		}
	}
}

func TestClipTextWidthAndFitCellWithWidth(t *testing.T) {
	s := "hello world"
	clipped, w := clipTextWidth(s, 5)
	if w != 5 || clipped != "hell…" {
		t.Errorf("clipTextWidth(%q, 5) = (%q, %d), want (%q, 5)", s, clipped, w, "hell…")
	}

	styled := pkTitle.Render("title")
	styledW := ansi.StringWidth(styled)
	fitted := fitCellWithWidth(styled, styledW, 10)
	if ansi.StringWidth(fitted) != 10 {
		t.Errorf("fitCellWithWidth(%q, %d, 10) visible width = %d, want 10", styled, styledW, ansi.StringWidth(fitted))
	}
}

func BenchmarkSessionRow(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sessionRow("dev-session-environment", "running", true, 30)
	}
}

func BenchmarkEdgeText(b *testing.B) {
	left := pkTitle.Render("dev-session-environment")
	right := pkRunning.Render("running")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = edgeText(left, right, 30)
	}
}
