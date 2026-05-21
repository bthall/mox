package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/bthall/mox/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		GroupID: groupConfig,
		Short:   "Show config path or print the file",
		Long:    `View and inspect the mox configuration. Subcommands: path, view.`,
	}
	cmd.AddCommand(newConfigPathCommand())
	cmd.AddCommand(newConfigViewCommand())
	return cmd
}

func newConfigPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Show config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := optsFromContext(cmd.Context())
			path := opts.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			fmt.Println(config.ResolvePath(path))
			return nil
		},
	}
}

func newConfigViewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "View current configuration (raw file contents)",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := optsFromContext(cmd.Context())
			path := opts.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			path = config.ResolvePath(path)
			f, err := os.Open(path) //nolint:gosec // user-supplied path is intentional
			if err != nil {
				return fmt.Errorf("open config: %w", err)
			}
			defer f.Close()
			if _, err := io.Copy(os.Stdout, f); err != nil {
				return fmt.Errorf("read config: %w", err)
			}
			return nil
		},
	}
}
