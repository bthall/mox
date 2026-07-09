package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux"
	"github.com/bthall/mox/pkg/version"
	"github.com/spf13/cobra"
)

// globalOpts holds flags bound to the root command. Subcommands read these
// via the *cobra.Command they receive — we expose them through helpers
// rather than as package-level variables.
type globalOpts struct {
	configPath string
	force      bool
	verbose    bool
	quiet      bool
	attach     string // -a / --attach: session to attach to
	print      bool   // --print: emit tmux commands instead of executing
}

type ctxKey int

const (
	ctxKeyOpts ctxKey = iota
	ctxKeyLogger
)

// Command groups for the root --help listing. Group IDs are referenced from
// each subcommand's GroupID. Commands without a GroupID land in cobra's
// "Additional Commands:" section (completion, help).
const (
	groupSession = "session"
	groupConfig  = "config"
)

// NewRootCommand builds the root cobra command. Tests may call this directly
// to avoid invoking os.Exit via Execute.
func NewRootCommand() *cobra.Command {
	opts := &globalOpts{}

	rootCmd := &cobra.Command{
		Use:   "mox",
		Short: "Declarative tmux session manager",
		Long: `mox builds tmux sessions from a YAML config and attaches to them.
A session can be a simple host list (one pane per host, optionally with
synchronized typing and tiled layout) or a fully-specified set of windows
and panes.`,
		Example: `  mox                           pick a session from a list
  mox -a api-cluster                attach to a configured session
  mox new                       quick local session
  mox new @api-cluster              cssh-style multi-host session
  mox new -u root host1 host2   ssh as a specific user
  mox new -w @api-cluster           open as a window in the current tmux
  mox list                      what's configured + running
  mox import work               capture a hand-rolled tmux session into config
  mox kill scratch              destroy a running session`,
		Version:       version.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return fmt.Errorf("unexpected argument %q\n\nDid you mean: mox -a %s\n(Attaching to a configured session now requires the -a / --attach flag.)",
				args[0], args[0])
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx = context.WithValue(ctx, ctxKeyOpts, opts)
			ctx = context.WithValue(ctx, ctxKeyLogger, newLogger(opts))
			cmd.SetContext(ctx)
			return nil
		},
		RunE: runSession,
	}

	rootCmd.Flags().StringVarP(&opts.attach, "attach", "a", "", "attach to the named configured session")
	rootCmd.Flags().BoolVar(&opts.print, "print", false, "print the tmux commands instead of executing them")
	rootCmd.PersistentFlags().StringVarP(&opts.configPath, "config", "c", "", "config file path (default: $XDG_CONFIG_HOME/mox/config.yml)")
	rootCmd.PersistentFlags().BoolVar(&opts.force, "force", false, "force recreate existing sessions")
	rootCmd.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "verbose (debug) logging to stderr")
	rootCmd.PersistentFlags().BoolVarP(&opts.quiet, "quiet", "q", false, "quiet logging (warnings and errors only)")

	// Tab completion for -a: just session names (compact column layout).
	if err := rootCmd.RegisterFlagCompletionFunc("attach", completeSessionNamesOnly); err != nil {
		panic(fmt.Sprintf("register --attach completion: %v", err))
	}

	// Command groups bring structure to the otherwise-flat --help listing.
	rootCmd.AddGroup(
		&cobra.Group{ID: groupSession, Title: "Session lifecycle:"},
		&cobra.Group{ID: groupConfig, Title: "Configuration:"},
	)

	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newListCommand())
	rootCmd.AddCommand(newRecentCommand())
	rootCmd.AddCommand(newLastCommand())
	rootCmd.AddCommand(newKillCommand())
	rootCmd.AddCommand(newValidateCommand())
	rootCmd.AddCommand(newEditCommand())
	rootCmd.AddCommand(newConfigCommand())
	rootCmd.AddCommand(newNewCommand())
	rootCmd.AddCommand(newImportCommand())

	return rootCmd
}

func runSession(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	if opts.attach == "" {
		// On a terminal, bare `mox` offers a session picker; anywhere else
		// (pipes, scripts) it prints help like any other CLI.
		if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
			return runPicker(cmd)
		}
		return cmd.Help()
	}

	// Allow attach-to-unmanaged: a config is optional. If absent or invalid,
	// the manager will still attach to a running tmux session by that name.
	cfg, _ := tryLoadConfig(opts.configPath)
	if cfg == nil {
		cfg = &config.Config{Sessions: map[string]*config.Session{}}
	}

	if opts.print {
		mgr := session.NewManagerWith(cfg, tmux.NewDryRun(cmd.OutOrStdout()), logger)
		return mgr.CreateOrAttach(cmd.Context(), opts.attach, opts.force)
	}
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return err
	}
	return mgr.CreateOrAttach(cmd.Context(), opts.attach, opts.force)
}

// loadConfig is the shared loader used by subcommands. When ./.mox.yml is
// in play it says so on stderr, so there is never a mystery about which
// config file is live.
func loadConfig(path string) (*config.Config, error) {
	resolved, local := config.EffectivePath(path)
	if local {
		fmt.Fprintf(os.Stderr, "mox: using ./%s\n", config.LocalConfigName)
	}
	return loadConfigAt(resolved)
}

// loadConfigAt loads a fully resolved path with no notice — shared by the
// noisy and silent loaders.
func loadConfigAt(path string) (*config.Config, error) {
	if !config.Exists(path) {
		return nil, fmt.Errorf("config file not found at %s\n\nRun 'mox init' to create a default configuration", path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func newLogger(opts *globalOpts) *slog.Logger {
	level := slog.LevelInfo
	switch {
	case opts.verbose:
		level = slog.LevelDebug
	case opts.quiet:
		level = slog.LevelWarn
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	return slog.New(handler)
}

func optsFromContext(ctx context.Context) *globalOpts {
	if ctx == nil {
		return &globalOpts{}
	}
	if opts, ok := ctx.Value(ctxKeyOpts).(*globalOpts); ok && opts != nil {
		return opts
	}
	return &globalOpts{}
}

func loggerFromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if l, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// Execute runs the root command with a context canceled by SIGINT/SIGTERM.
func Execute(ctx context.Context) int {
	rootCmd := NewRootCommand()
	rootCmd.SetContext(ctx)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "interrupted")
			return 130
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}
