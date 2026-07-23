package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
	"github.com/spf13/cobra"
)

// hostsColWidth caps the rendered width of the HOSTS column so each session
// stays on a single row.
const hostsColWidth = 40

// recentFooterLimit is how many history entries the inline Recent: footer
// shows under `mox list`.
const recentFooterLimit = 5

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		GroupID: groupSession,
		Short:   "List configured and running sessions",
		Long: `List all sessions in a single table: those defined in the mox config
and any tmux sessions running that are not in the config (origin "tmux").
Running sessions also show their window count, attached state, and last
activity. A Recent: footer lists sessions you recently created or attached to.`,
		RunE: runList,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}

	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return err
	}
	sessions, err := mgr.List()
	if err != nil {
		return err
	}

	recent, err := history.Load()
	if err != nil {
		logger.Debug("load session history failed", "error", err)
	}

	renderList(os.Stdout, sessions, recent, time.Now())
	return nil
}

// renderList writes the session table, the Recent: footer, and the summary
// line to out. now anchors relative-time formatting (injected for tests).
func renderList(out io.Writer, infos []session.SessionInfo, recent []history.Entry, now time.Time) {
	managed, unmanaged := splitByManaged(infos)
	slices.SortFunc(managed, func(a, b session.SessionInfo) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(unmanaged, func(a, b session.SessionInfo) int { return strings.Compare(a.Name, b.Name) })
	ordered := slices.Concat(managed, unmanaged)

	if len(ordered) == 0 {
		fmt.Fprintln(out, "No sessions configured or running.")
	} else {
		rows := [][]string{{"NAME", "ORIGIN", "STATE", "WIN", "ACTIVITY", "HOSTS"}}
		for _, s := range ordered {
			rows = append(rows, []string{
				nameCell(out, s),
				originCell(s),
				stateCell(out, s),
				winCell(s),
				relativeTime(now, s.LastActivity),
				hostsCell(s),
			})
		}
		renderTable(out, rows)
	}

	fmt.Fprintln(out)
	renderRecentFooter(out, recent, now)
	fmt.Fprintln(out)
	renderSummary(out, managed, unmanaged)
}

func splitByManaged(infos []session.SessionInfo) (managed, unmanaged []session.SessionInfo) {
	for _, s := range infos {
		if s.Managed {
			managed = append(managed, s)
		} else {
			unmanaged = append(unmanaged, s)
		}
	}
	return managed, unmanaged
}

// nameCell prefixes the session name with a colored status glyph when color is
// enabled (● running, ◆ running tmux-only, ○ stopped) — the same vocabulary
// as the hub's status dots. Under NO_COLOR or a non-TTY the glyph is omitted
// entirely; the STATE and ORIGIN columns carry the meaning.
func nameCell(out io.Writer, s session.SessionInfo) string {
	if !useColor(out) {
		return s.Name
	}
	glyph, code := "○", ansiDim
	if s.Running {
		glyph, code = "●", ansiGreen
		if !s.Managed {
			glyph, code = "◆", ansiYellow
		}
	}
	return colorize(out, code, glyph) + " " + s.Name
}

func originCell(s session.SessionInfo) string {
	if s.Managed {
		return "mox"
	}
	return "tmux"
}

func stateCell(out io.Writer, s session.SessionInfo) string {
	if !s.Running {
		return colorize(out, ansiDim, "stopped")
	}
	state := colorize(out, ansiGreen, "running")
	if s.Attached {
		state += " " + colorize(out, ansiBold, "attached")
	}
	return state
}

func winCell(s session.SessionInfo) string {
	if !s.Running {
		return "-"
	}
	return strconv.Itoa(s.Windows)
}

func hostsCell(s session.SessionInfo) string {
	if !s.Managed || len(s.Hosts) == 0 {
		return "-"
	}
	return truncate(strings.Join(s.Hosts, ", "), hostsColWidth)
}

// renderRecentFooter prints the inline "Recent:" line summarizing the newest
// history entries, or "(none)" when history is empty.
func renderRecentFooter(out io.Writer, recent []history.Entry, now time.Time) {
	label := colorize(out, ansiBold, "Recent:")
	if len(recent) == 0 {
		fmt.Fprintf(out, "%s (none)\n", label)
		return
	}
	limit := min(len(recent), recentFooterLimit)
	parts := make([]string, 0, limit)
	for _, e := range recent[:limit] {
		parts = append(parts, fmt.Sprintf("%s (%s %s)", e.Name, e.Action, relativeShort(now, e.Time)))
	}
	fmt.Fprintf(out, "%s %s\n", label, strings.Join(parts, " · "))
}

func renderSummary(out io.Writer, managed, unmanaged []session.SessionInfo) {
	running := 0
	for _, s := range managed {
		if s.Running {
			running++
		}
	}
	fmt.Fprintf(out, "%d configured · %d running", len(managed), running)
	if len(unmanaged) > 0 {
		fmt.Fprintf(out, " · %d unmanaged", len(unmanaged))
	}
	fmt.Fprintln(out)
}
