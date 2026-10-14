package cli

// Screenshot frame generator, not a test: `make screenshots` runs this with
// MOX_SCREENSHOT_DIR set, writing the README screenshots' terminal frames as
// raw ANSI to that directory. Everything is staged demo data — never real
// session names. The frames are rendered to PNG by charm's freeze (see the
// Makefile target). Skipped entirely during normal test runs.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
)

const screenshotConfigYAML = `sessions:
    webfarm:
        hosts: [web1, web2, web3]
        sync: true
        arrange: tiled
        pre:
            - export TERM=xterm-256color
    dev:
        root: /home/demo/dev
    dbcluster:
        hosts: [db1, db2]
        ssh_user: admin
`

func TestGenerateScreenshots(t *testing.T) {
	dir := os.Getenv("MOX_SCREENSHOT_DIR")
	if dir == "" {
		t.Skip("set MOX_SCREENSHOT_DIR to generate screenshot frames")
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	t.Setenv("CLICOLOR_FORCE", "1") // colors mox list's raw-ANSI path into files

	now := time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC)
	write := func(name, frame string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(frame), 0o600); err != nil { //nolint:gosec // dev-only generator; dir is the developer's own env var
			t.Fatal(err)
		}
	}

	write("hub.ans", hubFrame(t, now))
	write("list.ans", listFrame(t, now))
	write("editor.ans", editorFrame(t))
	write("add.ans", wizardFrame(t, 110, 30, false))
	write("review.ans", wizardFrame(t, 80, 24, true))
	help := screenshotHubModel(t, now)
	help.width, help.height, help.help = 80, 24, true
	write("help.ans", help.View())
	cancel := screenshotWizardModel(t, 80, 24, false)
	cancel = edType(t, cancel, tea.KeyCtrlC)
	write("cancel.ans", cancel.View())
	st := testEditorState(t, screenshotConfigYAML)
	errors := newEditorModel(st, nil, nil, "webfarm")
	errors.width, errors.height = 80, 24
	for i, field := range errors.fields {
		if field.key == "retry" {
			errors.fieldSel = i
		}
	}
	errors = edType(t, errors, tea.KeyEnter)
	errors = edType(t, errors, tea.KeyCtrlU)
	errors = edRunes(t, errors, "-3")
	errors = edType(t, errors, tea.KeyEnter)
	write("validation.ans", errors.View())
	errors.mode = modeStale
	errors.draft.sess.Root = "/srv/ops"
	write("stale.ans", errors.View())
	discard := screenshotWizardModel(t, 80, 24, true)
	discard = edType(t, discard, tea.KeyEnter)
	discard = edRunes(t, discard, "D")
	write("draft-discard.ans", discard.View())
	broadcast := screenshotWizardModel(t, 80, 24, false)
	broadcast = edType(t, broadcast, tea.KeyEnter)
	broadcast = edType(t, broadcast, tea.KeyEnter)
	broadcast = edRunes(t, broadcast, "n")
	broadcast = edType(t, broadcast, tea.KeyEsc)
	broadcast = edType(t, broadcast, tea.KeyEnter)
	write("broadcast-back.ans", broadcast.View())
	recovery := newEditorModel(testEditorState(t, screenshotConfigYAML), nil, nil, "webfarm")
	recovery.width, recovery.height = 80, 24
	recovery = edRunes(t, recovery, "r")
	recovery = edType(t, recovery, tea.KeyCtrlU)
	recovery = edRunes(t, recovery, "webfarm-renamed")
	recovery = edType(t, recovery, tea.KeyEnter)
	recovery = edRunes(t, recovery, "a")
	write("draft-recovery.ans", recovery.View())
	recovery = edRunes(t, recovery, "u")
	write("discard-edits.ans", recovery.View())
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 35}} {
		w, h := size[0], size[1]
		m := screenshotHubModel(t, now)
		m.width, m.height = w, h
		write(fmt.Sprintf("hub-%dx%d.ans", w, h), m.View())
		if w == 40 {
			m.focusDetail = true
			write("preview-40x12.ans", m.View())
		}
		st := testEditorState(t, screenshotConfigYAML)
		ed := newEditorModel(st, nil, nil, "webfarm")
		ed.width, ed.height = w, h
		if w == 40 {
			ed.fieldSel = len(ed.fields) - 1
		}
		write(fmt.Sprintf("editor-%dx%d.ans", w, h), ed.View())
		write(fmt.Sprintf("add-%dx%d.ans", w, h), wizardFrame(t, w, h, false))
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	m := screenshotHubModel(t, now)
	m.width, m.height = 80, 24
	write("hub-no-color.ans", m.View())
	ed := newEditorModel(testEditorState(t, screenshotConfigYAML), nil, nil, "webfarm")
	ed.width, ed.height = 80, 24
	write("editor-no-color.ans", ed.View())
	write("add-no-color.ans", wizardFrame(t, 80, 24, true))
}

// hubFrame stages the session hub: a running unmanaged session (◆), the
// highlighted running webfarm with a live preview, and two stopped sessions.
func hubFrame(t *testing.T, now time.Time) string {
	t.Helper()
	return screenshotHubModel(t, now).View()
}

func screenshotHubModel(t *testing.T, now time.Time) hubModel {
	t.Helper()
	candidates := []session.SessionInfo{
		{Name: "main", Running: true, Managed: false, Windows: 1, LastActivity: now},
		{Name: "webfarm", Running: true, Managed: true, Windows: 1, LastActivity: now, Hosts: []string{"web1", "web2", "web3"}},
		{Name: "dev", Managed: true},
		{Name: "dbcluster", Managed: true, Hosts: []string{"db1", "db2"}},
	}
	sessions := demoConfigSessions(t)
	m := newHubModel(context.Background(), nil, nil, nil, nil, candidates, sessions, now)
	m.width, m.height = 110, 30
	m.sel = 1 // webfarm
	m.previewName = "webfarm"
	m.previewWin = "1:sh*"
	m.previewBody = []string{
		"\x1b[32m✔ deploy ok\x1b[0m",
		"\x1b[31m✘ web3 unreachable\x1b[0m",
		"\x1b[33m⚠ retrying\x1b[0m",
		"ok: [\x1b[36mweb1\x1b[0m] task 0",
		"ok: [\x1b[36mweb2\x1b[0m] task 1",
		"ok: [\x1b[36mweb3\x1b[0m] task 2",
		"ok: [\x1b[36mweb1\x1b[0m] task 3",
		"ok: [\x1b[36mweb2\x1b[0m] task 4",
	}
	return m
}

func wizardFrame(t *testing.T, width, height int, review bool) string {
	t.Helper()
	return screenshotWizardModel(t, width, height, review).View()
}

func screenshotWizardModel(t *testing.T, width, height int, review bool) editorModel {
	t.Helper()
	st := testEditorState(t, screenshotConfigYAML)
	m := newEditorModel(st, nil, nil, "webfarm")
	m.width, m.height = width, height
	m = edRunes(t, m, "a")
	m = edRunes(t, m, "ops")
	m = edType(t, m, tea.KeyEnter)
	m = edRunes(t, m, "@webfarm")
	if review {
		m = edType(t, m, tea.KeyEnter)
		m = edType(t, m, tea.KeyEnter)
		m = edType(t, m, tea.KeyEnter)
		m = edType(t, m, tea.KeyEnter)
		m = edRunes(t, m, "/srv/ops")
		m = edType(t, m, tea.KeyEnter)
		m = edRunes(t, m, "htop")
		m = edType(t, m, tea.KeyEnter)
		m = edType(t, m, tea.KeyEnter)
	}
	return m
}

// listFrame stages mox list: configured stopped/running sessions plus one
// unmanaged (◆) tmux-only session, with the recents footer and summary.
func listFrame(t *testing.T, now time.Time) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "list-*.ans")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	infos := []session.SessionInfo{
		{Name: "db-primary", Managed: true, Hosts: []string{"db1", "db2"}},
		{Name: "monitoring", Managed: true, Hosts: []string{"mon1"}},
		{Name: "web-cluster", Running: true, Managed: true, Windows: 1, LastActivity: now, Hosts: []string{"web1", "web2", "web3"}},
		{Name: "scratch", Running: true, Managed: false, Windows: 1, LastActivity: now},
	}
	recent := []history.Entry{
		{Name: "scratch", Action: history.ActionCreated, Time: now},
		{Name: "web-cluster", Action: history.ActionCreated, Time: now},
	}
	renderList(f, infos, recent, now)
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// editorFrame stages the session editor on webfarm's field form.
func editorFrame(t *testing.T) string {
	t.Helper()
	st := testEditorState(t, screenshotConfigYAML)
	m := newEditorModel(st, nil, nil, "webfarm")
	m.width, m.height = 110, 30
	return m.View()
}

// demoConfigSessions parses the demo config for the hub's managed lookup.
func demoConfigSessions(t *testing.T) map[string]*config.Session {
	t.Helper()
	st := testEditorState(t, screenshotConfigYAML)
	return st.cfg.Sessions
}
