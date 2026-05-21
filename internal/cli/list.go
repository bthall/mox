package cli

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/bthall/mox/internal/session"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		GroupID: groupSession,
		Short:   "List configured and running sessions",
		Long: `List sessions in two sections: those defined in the mox config
(with their state — running or stopped), and any tmux sessions running
that are not in the config (unmanaged).`,
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

	managed, unmanaged := splitByManaged(sessions)
	slices.SortFunc(managed, func(a, b session.SessionInfo) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(unmanaged, func(a, b session.SessionInfo) int { return strings.Compare(a.Name, b.Name) })

	out := os.Stdout
	runningCount := renderManaged(out, managed)
	renderUnmanaged(out, unmanaged)
	renderTotals(out, managed, unmanaged, runningCount)
	return nil
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

func renderManaged(out *os.File, items []session.SessionInfo) int {
	fmt.Fprintln(out, colorize(out, ansiBold, "Configured:"))
	if len(items) == 0 {
		fmt.Fprintln(out, "  (none)")
		fmt.Fprintln(out)
		return 0
	}
	running := 0
	for _, s := range items {
		status := colorize(out, ansiDim, "stopped")
		if s.Running {
			status = colorize(out, ansiGreen, "running")
			running++
		}
		fmt.Fprintf(out, "  %-22s %s\n", s.Name, status)
	}
	fmt.Fprintln(out)
	return running
}

func renderUnmanaged(out *os.File, items []session.SessionInfo) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(out, colorize(out, ansiBold, "Unmanaged (tmux only):"))
	for _, s := range items {
		// All unmanaged sessions are running by definition (they came from
		// tmux's session list), so we render them in a single color.
		fmt.Fprintf(out, "  %-22s %s\n", s.Name, colorize(out, ansiYellow, "running"))
	}
	fmt.Fprintln(out)
}

func renderTotals(out *os.File, managed, unmanaged []session.SessionInfo, running int) {
	totalConfigured := len(managed)
	fmt.Fprintf(out, "Total: %d configured (%d running)", totalConfigured, running)
	if len(unmanaged) > 0 {
		fmt.Fprintf(out, ", %d unmanaged", len(unmanaged))
	}
	fmt.Fprintln(out)
}
