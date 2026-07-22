package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

// fakeHubManager records calls and serves canned listings. No live tmux.
type fakeHubManager struct {
	killed    []string
	created   []string
	createErr error
	killErr   error
	infos     []session.SessionInfo
}

func (f *fakeHubManager) Kill(name string) error {
	f.killed = append(f.killed, name)
	if f.killErr != nil {
		return f.killErr
	}
	for i := range f.infos {
		if f.infos[i].Name == name {
			f.infos[i].Running = false
		}
	}
	return nil
}

func (f *fakeHubManager) Create(_ context.Context, name string, _ bool) error {
	f.created = append(f.created, name)
	if f.createErr != nil {
		return f.createErr
	}
	for i := range f.infos {
		if f.infos[i].Name == name {
			f.infos[i].Running = true
			f.infos[i].Windows = 1
		}
	}
	return nil
}

func (f *fakeHubManager) List() ([]session.SessionInfo, error) {
	out := make([]session.SessionInfo, len(f.infos))
	copy(out, f.infos)
	return out, nil
}

func hubFixture() ([]session.SessionInfo, map[string]*config.Session) {
	candidates := []session.SessionInfo{
		{Name: "webfarm", Running: true, Managed: true, Windows: 3, LastActivity: time.Now().Add(-2 * time.Minute), Hosts: []string{"web1", "web2"}},
		{Name: "dbcluster", Managed: true, Hosts: []string{"db1", "db2"}},
		{Name: "scratch", Running: true, Managed: false, Windows: 1, LastActivity: time.Now().Add(-time.Hour)},
	}
	sessions := map[string]*config.Session{
		"webfarm":   {Hosts: []string{"web1", "web2"}, Sync: true},
		"dbcluster": {Hosts: []string{"db1", "db2"}, SSHUser: "admin"},
	}
	return candidates, sessions
}

func testHubModel(t *testing.T, mgr hubManager) hubModel {
	t.Helper()
	old := hubTickInterval
	hubTickInterval = time.Millisecond
	t.Cleanup(func() { hubTickInterval = old })
	candidates, sessions := hubFixture()
	if mgr == nil {
		mgr = &fakeHubManager{infos: candidates}
	}
	capture := func(name string) (string, error) {
		return "$ tail -f app.log\nline one\nline two\n", nil
	}
	windows := func(name string) (string, error) {
		return "1:servers* 2:logs", nil
	}
	m := newHubModel(context.Background(), mgr, nil, capture, windows, candidates, sessions, time.Now())
	m.width, m.height = 100, 26
	return m
}

func hubKey(t *testing.T, m hubModel, msg tea.KeyMsg) (hubModel, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(msg)
	out, ok := nm.(hubModel)
	if !ok {
		t.Fatalf("Update returned %T", nm)
	}
	return out, cmd
}

func hubRunes(t *testing.T, m hubModel, s string) (hubModel, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range s {
		m, cmd = hubKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m, cmd
}

// drain executes a tea.Cmd (possibly a batch) and feeds every produced
// message back into the model, returning the settled model.
func drain(t *testing.T, m hubModel, cmd tea.Cmd) hubModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			inner := c()
			if _, isTick := inner.(hubTickMsg); isTick {
				continue // don't recurse into the timer loop
			}
			nm, next := m.Update(inner)
			m = nm.(hubModel)
			m = drain(t, m, next)
		}
		return m
	}
	if _, isTick := msg.(hubTickMsg); isTick {
		return m
	}
	nm, next := m.Update(msg)
	m = nm.(hubModel)
	return drain(t, m, next)
}

func TestHubViewBasics(t *testing.T) {
	m := testHubModel(t, nil)
	out := m.View()
	for _, want := range []string{"webfarm", "dbcluster", "scratch", "3w", "attach", "S start", "K kill"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	// first candidate (webfarm, running) is selected; title reflects it
	if !strings.Contains(m.previewTitle(), "webfarm · running") {
		t.Errorf("preview title = %q", m.previewTitle())
	}
}

func TestHubPreviewLifecycle(t *testing.T) {
	m := testHubModel(t, nil)
	m = drain(t, m, m.Init())
	out := m.View()
	for _, want := range []string{"1:servers* 2:logs", "tail -f app.log", "live ·"} {
		if !strings.Contains(out, want) {
			t.Errorf("live preview missing %q:\n%s", want, out)
		}
	}

	// stale preview for a session no longer highlighted is dropped
	m2, _ := hubRunes(t, m, "j") // now dbcluster (stopped)
	nm, _ := m2.Update(hubPreviewMsg{name: "webfarm", body: "junk"})
	m2 = nm.(hubModel)
	if strings.Contains(m2.View(), "junk") {
		t.Fatal("stale preview applied after highlight moved")
	}
	// stopped session shows the config summary + start hint
	out = m2.View()
	for _, want := range []string{"stopped", "admin@{{host}}", "S starts it detached"} {
		if !strings.Contains(out, want) {
			t.Errorf("stopped preview missing %q:\n%s", want, out)
		}
	}
}

func TestHubTickLifecycle(t *testing.T) {
	m := testHubModel(t, nil)
	// the constructor records the Init-started loop for a running highlight
	if !m.ticking {
		t.Fatal("initial running highlight did not record the tick loop")
	}
	// tick while a running session is highlighted → capture + next tick
	nm, cmd := m.Update(hubTickMsg{})
	m = nm.(hubModel)
	if cmd == nil {
		t.Fatal("tick on running highlight produced no follow-up")
	}
	// tick while a stopped session is highlighted → cycle stops
	m2, _ := hubRunes(t, m, "j")
	nm, cmd = m2.Update(hubTickMsg{})
	m2 = nm.(hubModel)
	if cmd != nil || m2.ticking {
		t.Fatal("tick did not stop on a stopped highlight")
	}
}

func TestHubCaptureErrorDegrades(t *testing.T) {
	m := testHubModel(t, nil)
	nm, _ := m.Update(hubPreviewMsg{name: "webfarm", err: errors.New("no server running")})
	m = nm.(hubModel)
	out := m.View()
	if !strings.Contains(out, "preview unavailable") || !strings.Contains(out, "state") {
		t.Fatalf("capture error did not degrade to summary:\n%s", out)
	}
}

func TestHubStartDetached(t *testing.T) {
	mgr := &fakeHubManager{}
	mgr.infos, _ = hubFixture()
	m := testHubModel(t, mgr)
	m, _ = hubRunes(t, m, "j") // dbcluster (stopped, managed)
	m, cmd := hubRunes(t, m, "S")
	if m.pending == "" {
		t.Fatal("S did not set a pending state")
	}
	// keys are ignored while pending
	m2, _ := hubRunes(t, m, "S")
	if len(mgr.created) != 0 {
		t.Fatal("second S dispatched while pending")
	}
	_ = m2
	m = drain(t, m, cmd)
	if len(mgr.created) != 1 || mgr.created[0] != "dbcluster" {
		t.Fatalf("created = %v", mgr.created)
	}
	if m.pending != "" || !strings.Contains(m.status, "started dbcluster ✓") {
		t.Fatalf("pending=%q status=%q", m.pending, m.status)
	}
	// list refreshed: dbcluster now running, still selected
	if c, _ := m.selected(); c.Name != "dbcluster" || !c.Running {
		t.Fatalf("selection after start = %+v", c)
	}
}

func TestHubStartGuards(t *testing.T) {
	m := testHubModel(t, nil)
	// S on a running session
	m, cmd := hubRunes(t, m, "S")
	if cmd != nil || !strings.Contains(m.status, "already running") {
		t.Fatalf("S on running: status=%q", m.status)
	}
	// S on an unmanaged session
	m, _ = hubRunes(t, m, "j")
	m, _ = hubRunes(t, m, "j") // scratch (running, unmanaged)... running guard hits first
	m, cmd = hubRunes(t, m, "S")
	if cmd != nil || !strings.Contains(m.status, "already running") {
		t.Fatalf("S on scratch: status=%q", m.status)
	}
}

func TestHubStartUnmanagedStopped(t *testing.T) {
	mgr := &fakeHubManager{infos: []session.SessionInfo{{Name: "ghost"}}}
	m := newHubModel(context.Background(), mgr, nil, func(string) (string, error) { return "", nil }, nil,
		mgr.infos, map[string]*config.Session{}, time.Now())
	m, cmd := hubRunes(t, m, "S")
	if cmd != nil || !strings.Contains(m.status, "not in the config") {
		t.Fatalf("S on unmanaged stopped: status=%q", m.status)
	}
}

func TestHubKillConfirmFlow(t *testing.T) {
	mgr := &fakeHubManager{}
	mgr.infos, _ = hubFixture()
	m := testHubModel(t, mgr)
	m, _ = hubRunes(t, m, "K") // webfarm is running
	if m.mode != hubConfirmKill {
		t.Fatal("K did not ask for confirmation")
	}
	if !strings.Contains(m.statusLine(), "kill webfarm?") {
		t.Fatalf("confirm prompt missing: %q", m.statusLine())
	}
	// esc cancels
	m2, _ := hubKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m2.mode != hubBrowse || len(mgr.killed) != 0 {
		t.Fatal("esc did not cancel the kill")
	}
	// y confirms
	m, cmd := hubRunes(t, m, "y")
	if m.pending == "" {
		t.Fatal("confirmed kill did not set pending")
	}
	m = drain(t, m, cmd)
	if len(mgr.killed) != 1 || mgr.killed[0] != "webfarm" {
		t.Fatalf("killed = %v", mgr.killed)
	}
	if !strings.Contains(m.status, "killed webfarm ✓") {
		t.Fatalf("status = %q", m.status)
	}
	if c, _ := m.selected(); c.Name != "webfarm" || c.Running {
		t.Fatalf("selection after kill = %+v", c)
	}
}

func TestHubKillOnStopped(t *testing.T) {
	m := testHubModel(t, nil)
	m, _ = hubRunes(t, m, "j") // dbcluster, stopped
	m, _ = hubRunes(t, m, "K")
	if m.mode == hubConfirmKill || !strings.Contains(m.status, "not running") {
		t.Fatalf("K on stopped: mode=%v status=%q", m.mode, m.status)
	}
}

func TestHubActionError(t *testing.T) {
	mgr := &fakeHubManager{createErr: errors.New("on_start: exit status 1")}
	mgr.infos, _ = hubFixture()
	m := testHubModel(t, mgr)
	m, _ = hubRunes(t, m, "j") // dbcluster
	m, cmd := hubRunes(t, m, "S")
	m = drain(t, m, cmd)
	if !m.statusErr || !strings.Contains(m.status, "on_start: exit status 1") {
		t.Fatalf("statusErr=%v status=%q", m.statusErr, m.status)
	}
}

func TestHubAttachAndEdit(t *testing.T) {
	m := testHubModel(t, nil)
	m2, cmd := hubKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !isQuit(cmd) || m2.action != hubAttach || m2.choice != "webfarm" {
		t.Fatalf("enter: action=%v choice=%q", m2.action, m2.choice)
	}
	m3, cmd := hubKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if !isQuit(cmd) || m3.action != hubEdit || m3.choice != "webfarm" {
		t.Fatalf("ctrl+e: action=%v choice=%q", m3.action, m3.choice)
	}
	// ctrl+e on an unmanaged session is a no-op
	m4, _ := hubRunes(t, m, "j")
	m4, _ = hubRunes(t, m4, "j") // scratch
	m5, cmd := hubKey(t, m4, tea.KeyMsg{Type: tea.KeyCtrlE})
	if cmd != nil || m5.action != hubQuit {
		t.Fatal("ctrl+e acted on an unmanaged session")
	}
}

func TestHubFilterAndBatchedKeys(t *testing.T) {
	m := testHubModel(t, nil)
	if !strings.Contains(m.View(), "/ filter") {
		t.Fatal("inactive filter hint missing")
	}
	m, _ = hubRunes(t, m, "/")
	m, _ = hubRunes(t, m, "db")
	if len(m.visible) != 1 || m.candidates[m.visible[0]].Name != "dbcluster" {
		t.Fatalf("filter db → %d matches", len(m.visible))
	}
	m, _ = hubKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.visible) != 3 {
		t.Fatal("esc did not clear the filter")
	}
	// batched runes navigate
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	m = nm.(hubModel)
	if m.selectedName() != "scratch" {
		t.Fatalf("batched jj → %q", m.selectedName())
	}
}

func TestHubNarrowView(t *testing.T) {
	m := testHubModel(t, nil)
	m.width = 40
	out := m.View()
	if !strings.Contains(out, "webfarm") || strings.Contains(out, "live · 1s") {
		t.Fatalf("narrow view should be list-only:\n%s", out)
	}
}

func TestHubManyOverflow(t *testing.T) {
	var infos []session.SessionInfo
	for i := 0; i < 40; i++ {
		infos = append(infos, session.SessionInfo{Name: fmt.Sprintf("sess%02d", i), Managed: true})
	}
	m := newHubModel(context.Background(), &fakeHubManager{infos: infos}, nil,
		func(string) (string, error) { return "", nil }, nil, infos, map[string]*config.Session{}, time.Now())
	m.width, m.height = 100, 20
	for i := 0; i < 39; i++ {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = nm.(hubModel)
		if !strings.Contains(m.View(), "▌ "+statusDot(session.SessionInfo{})+" "+m.selectedName()) &&
			!strings.Contains(m.View(), m.selectedName()) {
			t.Fatalf("step %d: selection %q hidden", i, m.selectedName())
		}
	}
	// scroll back up one: overflow below again → indicator must render
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = nm.(hubModel)
	for i := 0; i < 20; i++ {
		nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		m = nm.(hubModel)
	}
	if !strings.Contains(m.View(), "more") {
		t.Fatal("overflow indicator missing after scrolling back up")
	}
}

func TestHubFilterBackspaceAndQuit(t *testing.T) {
	m := testHubModel(t, nil)
	m, _ = hubRunes(t, m, "/")
	m, _ = hubRunes(t, m, "dbx")
	if len(m.visible) != 0 {
		t.Fatalf("filter dbx matched %d", len(m.visible))
	}
	m, _ = hubKey(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.visible) != 1 {
		t.Fatal("backspace did not refilter")
	}
	m, _ = hubKey(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // keep filter, back to browse
	// esc clears the kept filter first, second esc quits
	m, cmd := hubKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if isQuit(cmd) || len(m.filter) != 0 {
		t.Fatal("first esc should only clear the filter")
	}
	_, cmd = hubKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !isQuit(cmd) {
		t.Fatal("second esc did not quit")
	}
	// q quits too
	_, cmd = hubRunes(t, m, "q")
	if !isQuit(cmd) {
		t.Fatal("q did not quit")
	}
}

// TestHubRefreshKeepsOrdering pins that the post-action listing refresh
// goes through the same ordering as the initial candidates.
func TestHubRefreshKeepsOrdering(t *testing.T) {
	mgr := &fakeHubManager{}
	mgr.infos, _ = hubFixture()
	candidates, sessions := hubFixture()
	ordered := 0
	order := func(infos []session.SessionInfo) []session.SessionInfo {
		ordered++
		return orderPickerCandidates(infos, nil)
	}
	old := hubTickInterval
	hubTickInterval = time.Millisecond
	t.Cleanup(func() { hubTickInterval = old })
	m := newHubModel(context.Background(), mgr, order,
		func(string) (string, error) { return "", nil }, nil, candidates, sessions, time.Now())
	m.width, m.height = 100, 26
	m, _ = hubRunes(t, m, "j") // dbcluster
	m, cmd := hubRunes(t, m, "S")
	m = drain(t, m, cmd)
	if ordered == 0 {
		t.Fatal("refresh did not go through the ordering seam")
	}
	// running sessions sort before stopped ones after the refresh
	first := m.candidates[0]
	if !first.Running {
		t.Fatalf("first candidate after refresh = %+v, want a running one", first)
	}
}

// lipglossWidth is a test-local alias to keep assertions readable.
func lipglossWidth(s string) int { return lipgloss.Width(s) }

// TestHubColoredPreviewNoBleed pins the ANSI safety properties: colored
// capture lines are width-clipped without breaking sequences, every body
// line is reset-terminated, and panel rows stay aligned.
func TestHubColoredPreviewNoBleed(t *testing.T) {
	longRed := "\x1b[31m" + strings.Repeat("x", 200) // long colored line, no reset
	capture := func(string) (string, error) {
		return "\x1b[32mgreen ok\x1b[0m\n" + longRed + "\n", nil
	}
	candidates, sessions := hubFixture()
	old := hubTickInterval
	hubTickInterval = time.Millisecond
	t.Cleanup(func() { hubTickInterval = old })
	m := newHubModel(context.Background(), &fakeHubManager{infos: candidates}, nil,
		capture, func(string) (string, error) { return "1:sh*", nil }, candidates, sessions, time.Now())
	m.width, m.height = 80, 20
	m = drain(t, m, m.Init())

	out := m.View()
	if !strings.Contains(out, "\x1b[32mgreen ok") {
		t.Fatal("session colors stripped from the preview")
	}
	// every rendered line must be equally wide (borders aligned) despite
	// embedded SGR sequences, and no line may exceed the terminal width
	for i, line := range strings.Split(out, "\n") {
		if lw := lipglossWidth(line); lw > m.width {
			t.Fatalf("line %d overflows: width %d > %d:\n%q", i, lw, m.width, line)
		}
	}
	// the clipped red line is reset before the border glyph that follows
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "\x1b[31m") && !strings.Contains(line, "\x1b[0m") {
			t.Fatalf("colored line not reset-terminated:\n%q", line)
		}
	}
}

// TestHubStateColoredNames pins the mox-list color vocabulary in the list.
func TestHubStateColoredNames(t *testing.T) {
	m := testHubModel(t, nil)
	out := m.View()
	if !strings.Contains(out, pkRunning.Render("webfarm")) && !strings.Contains(out, pkSelected.Render(pkRunning.Render("webfarm"))) {
		t.Fatal("running managed session not green")
	}
	if !strings.Contains(out, pkForeign.Render("scratch")) {
		t.Fatal("running unmanaged session not yellow")
	}
	// footer keys are accent-styled
	if !strings.Contains(out, pkAccent.Render("S")) {
		t.Fatal("footer keys not accent-styled")
	}
}

// TestSanitizeCaptureLine pins tab expansion (to real 8-column stops,
// including after wide prefixes), SGR passthrough, and control-char
// scrubbing.
func TestSanitizeCaptureLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\tb", "a       b"},                                      // tab from col 1 → pad to col 8
		{"12345678\tx", "12345678        x"},                       // tab at a stop → full 8
		{"\x1b[32mok\x1b[0m\tdone", "\x1b[32mok\x1b[0m      done"}, // SGR kept, width 2 before tab
		{"bad\rcr\x07bell", "badcrbell"},                           // CR and BEL dropped
	}
	for _, c := range cases {
		if got := sanitizeCaptureLine(c.in); got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHubTabbyPreviewKeepsFooter is the regression for the disappearing
// shortcuts bar: a buffer full of tab-indented lines must not widen any
// rendered line past the terminal (which would wrap and push the footer
// and status line off-screen).
func TestHubTabbyPreviewKeepsFooter(t *testing.T) {
	capture := func(string) (string, error) {
		var b strings.Builder
		for i := 0; i < 30; i++ {
			b.WriteString("func\tname\targs\treturn\tvery\tlong\ttabbed\tline\n")
		}
		return b.String(), nil
	}
	candidates, sessions := hubFixture()
	old := hubTickInterval
	hubTickInterval = time.Millisecond
	t.Cleanup(func() { hubTickInterval = old })
	m := newHubModel(context.Background(), &fakeHubManager{infos: candidates}, nil,
		capture, func(string) (string, error) { return "1:sh*", nil }, candidates, sessions, time.Now())
	m.width, m.height = 90, 20
	m = drain(t, m, m.Init())

	out := m.View()
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "\t") {
			t.Fatalf("line %d contains a raw tab", i)
		}
		if lw := lipglossWidth(line); lw > m.width {
			t.Fatalf("line %d wraps: width %d > %d", i, lw, m.width)
		}
	}
	if !strings.Contains(out, "attach") {
		t.Fatal("footer (shortcuts bar) missing from the rendered view")
	}
}
