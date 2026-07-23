package cli

// Screenshot frame generator, not a test: `make screenshots` runs this with
// MOX_SCREENSHOT_DIR set, writing the README screenshots' terminal frames as
// raw ANSI to that directory. Everything is staged demo data — never real
// session names. The frames are rendered to PNG by charm's freeze (see the
// Makefile target). Skipped entirely during normal test runs.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	now := time.Now()
	write := func(name, frame string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(frame), 0o600); err != nil { //nolint:gosec // dev-only generator; dir is the developer's own env var
			t.Fatal(err)
		}
	}

	write("hub.ans", hubFrame(t, now))
	write("list.ans", listFrame(t, now))
	write("editor.ans", editorFrame(t))
}

// hubFrame stages the session hub: a running unmanaged session (◆), the
// highlighted running webfarm with a live preview, and two stopped sessions.
func hubFrame(t *testing.T, now time.Time) string {
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
	return m.View()
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
