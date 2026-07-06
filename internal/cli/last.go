package cli

import (
	"fmt"
	"os"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux"
	"github.com/spf13/cobra"
)

func newLastCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "last",
		GroupID: groupSession,
		Short:   "Attach to the session you used before this one",
		Long: `Attach to the most recently used session that isn't the one you're in —
the session equivalent of 'cd -'. Configured sessions are built first if
they aren't running. Bind it inside tmux to bounce between two sessions:

  bind-key L run-shell "mox last"`,
		Args: cobra.NoArgs,
		RunE: runLast,
	}
}

func runLast(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	entries, err := history.Load()
	if err != nil {
		logger.Debug("load session history failed", "error", err)
	}

	current := ""
	if os.Getenv("TMUX") != "" {
		if client, err := tmux.NewClient(); err == nil {
			current, _ = client.CurrentSession()
		}
	}

	name := pickLast(entries, current)
	if name == "" {
		return fmt.Errorf("no previous session in history (it fills in as you create and attach)")
	}

	// A config is optional — mox last also returns to unmanaged sessions.
	cfg, _ := tryLoadConfig(opts.configPath)
	if cfg == nil {
		cfg = &config.Config{Sessions: map[string]*config.Session{}}
	}
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return err
	}
	return mgr.CreateOrAttach(cmd.Context(), name, false)
}

// pickLast returns the most recent history entry that isn't the current
// session, or "" when there is none. entries are newest-first.
func pickLast(entries []history.Entry, current string) string {
	for _, e := range entries {
		if e.Name != current {
			return e.Name
		}
	}
	return ""
}
