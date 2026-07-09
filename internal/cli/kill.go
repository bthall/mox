package cli

import (
	"fmt"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux"
	"github.com/spf13/cobra"
)

func newKillCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "kill <session>",
		GroupID: groupSession,
		Short:   "Kill a running tmux session",
		Long: `Destroy a tmux session by name. Works on any running tmux session,
not just ones in the mox config.`,
		Example: `  mox kill work
  mox kill <TAB>    completes from running tmux sessions`,
		Args: cobra.ExactArgs(1),
		RunE: runKill,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			client, err := tmux.NewClient()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			sessions, err := client.ListSessions()
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return sessions, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func runKill(cmd *cobra.Command, args []string) error {
	name := args[0]
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	// Config is optional (unmanaged sessions can be killed too), but when
	// present it supplies on_stop hooks for managed sessions.
	cfg, _ := tryLoadConfig(opts.configPath)
	if cfg == nil {
		cfg = &config.Config{Sessions: map[string]*config.Session{}}
	}
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return err
	}
	if err := mgr.Kill(name); err != nil {
		return err
	}
	fmt.Printf("Session %q killed successfully.\n", name)
	return nil
}
