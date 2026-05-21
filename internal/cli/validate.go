package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "validate",
		GroupID: groupConfig,
		Short:   "Check the config for syntax and schema errors",
		Long: `Parse the configuration file and report any errors. Useful after
hand-editing the YAML, or in CI to verify a checked-in config.`,
		RunE: runValidate,
	}
}

func runValidate(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())
	cfg, err := loadConfig(opts.configPath)
	if err != nil {
		return err
	}

	fmt.Println("OK: configuration is valid")
	fmt.Println()
	fmt.Printf("Sessions: %d\n", len(cfg.Sessions))
	fmt.Printf("Layouts:  %d\n", len(cfg.Layouts))

	if len(cfg.Sessions) > 0 {
		fmt.Println()
		fmt.Println("Sessions:")
		for _, name := range cfg.ListSessionNames() {
			s := cfg.Sessions[name]
			if s.IsSimple() {
				fmt.Printf("  - %s (simple: %d hosts)\n", name, len(s.Hosts))
			} else {
				fmt.Printf("  - %s (complex: %d windows)\n", name, len(s.Windows))
			}
		}
	}

	if len(cfg.Layouts) > 0 {
		fmt.Println()
		fmt.Println("Layouts:")
		for name, layout := range cfg.Layouts {
			fmt.Printf("  - %s (%d panes)\n", name, len(layout.Panes))
		}
	}
	return nil
}
