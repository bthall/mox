package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
	"github.com/spf13/cobra"
)

const defaultRecentLimit = 10

func newRecentCommand() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:     "recent",
		Aliases: []string{"r"},
		GroupID: groupSession,
		Short:   "Show recently created or attached sessions",
		Long: `Show the sessions you most recently created or attached to, newest first.
History persists across session death, so a session shown here may no longer
be running (STATE "gone").`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRecent(cmd, limit)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", defaultRecentLimit, "maximum number of entries to show")
	return cmd
}

func runRecent(cmd *cobra.Command, limit int) error {
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	entries, err := history.Load()
	if err != nil {
		logger.Debug("load session history failed", "error", err)
	}

	// Determine which recorded sessions are still running, for the STATE
	// column. A missing/invalid config or tmux server is non-fatal here.
	running := map[string]bool{}
	if cfg, cfgErr := tryLoadConfig(opts.configPath); cfgErr == nil && cfg != nil {
		if mgr, mgrErr := session.NewManager(cfg, logger); mgrErr == nil {
			if infos, listErr := mgr.List(); listErr == nil {
				for _, s := range infos {
					if s.Running {
						running[s.Name] = true
					}
				}
			}
		}
	}

	renderRecent(os.Stdout, entries, running, time.Now(), limit)
	return nil
}

// renderRecent writes the recent-sessions table to out. now anchors relative
// time; limit caps the rows shown (<= 0 means no cap).
func renderRecent(out io.Writer, entries []history.Entry, running map[string]bool, now time.Time, limit int) {
	if len(entries) == 0 {
		fmt.Fprintln(out, "No recent sessions.")
		return
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	rows := [][]string{{"SESSION", "LAST ACTION", "WHEN", "STATE"}}
	for _, e := range entries {
		rows = append(rows, []string{
			e.Name,
			e.Action,
			relativeTime(now, e.Time),
			recentStateCell(out, running[e.Name]),
		})
	}
	renderTable(out, rows)
}

func recentStateCell(out io.Writer, running bool) string {
	if running {
		return colorize(out, ansiGreen, "running")
	}
	return colorize(out, ansiDim, "gone")
}
