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
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "OK: configuration is valid")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Sessions: %d\n", len(cfg.Sessions))
	fmt.Fprintf(out, "Layouts:  %d\n", len(cfg.Layouts))

	if len(cfg.Sessions) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Sessions:")
		for _, name := range cfg.ListSessionNames() {
			s := cfg.Sessions[name]
			if s.IsSimple() {
				fmt.Fprintf(out, "  - %s (simple: %d hosts)\n", name, len(s.Hosts))
			} else {
				fmt.Fprintf(out, "  - %s (complex: %d windows)\n", name, len(s.Windows))
			}
		}
	}

	if len(cfg.Layouts) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Layouts:")
		for _, name := range cfg.ListLayoutNames() {
			fmt.Fprintf(out, "  - %s (%d panes)\n", name, len(cfg.Layouts[name].Panes))
		}
	}
	return nil
}
