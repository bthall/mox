package cli

import (
	"fmt"

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
	client, err := tmux.NewClient()
	if err != nil {
		return err
	}
	exists, err := client.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %q does not exist", name)
	}
	if err := client.KillSession(name); err != nil {
		return fmt.Errorf("kill session: %w", err)
	}
	fmt.Printf("Session %q killed successfully.\n", name)
	return nil
}
