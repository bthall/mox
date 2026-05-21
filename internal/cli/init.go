package cli

import (
	"fmt"

	"github.com/bthall/mox/internal/config"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "init",
		GroupID: groupConfig,
		Short:   "Scaffold a default config file",
		Long: `Create a default configuration file at $XDG_CONFIG_HOME/mox/config.yml
(falling back to ~/.config/mox/config.yml). The file is mode 0600 and
contains example sessions to get you started.

Use --force to overwrite an existing config.`,
		RunE: runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())
	path, err := config.Init(opts.force)
	if err != nil {
		return err
	}
	fmt.Printf("Configuration initialized at: %s\n", path)
	fmt.Println()
	fmt.Println("Example sessions created:")
	fmt.Println("  - example: simple session with localhost")
	fmt.Println("  - dev:     multi-window development session")
	fmt.Println()
	fmt.Println("Edit the config to customize, then run 'mox -a <session>' to start.")
	return nil
}
