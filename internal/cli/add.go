package cli

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
)

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add [name]",
		GroupID: groupConfig,
		Short:   "Interactively add a session to the config",
		Long: `Walk through a short form — name, hosts, connection details — preview
the YAML, and save the session to your config. Only simple-mode sessions
(hosts, or a plain local session) are built here: for custom pane
layouts, build the window for real and capture it with 'mox import'.

Non-interactive alternative: 'mox new ... --save'.`,
		Example: `  mox add            start the wizard
  mox add dbfarm     start with the name filled in`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(cmd, args)
		},
	}
	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	stdin, ok := cmd.InOrStdin().(*os.File)
	if !ok || !term.IsTerminal(int(stdin.Fd())) {
		return errors.New("mox add is interactive and needs a terminal; use 'mox new ... --save' or 'mox edit' instead")
	}

	gopts := optsFromContext(cmd.Context())
	cfg, _ := tryLoadConfig(gopts.configPath)
	if cfg == nil {
		cfg = &config.Config{Sessions: map[string]*config.Session{}}
	}
	clusters, _ := loadClusterssh() // missing file is fine

	prefill := ""
	if len(args) == 1 {
		prefill = args[0]
	}

	final, err := tea.NewProgram(newAddModel(cfg, clusters, prefill)).Run()
	if err != nil {
		return err
	}
	m, ok := final.(addModel)
	if !ok || m.done.action == addActionCancel {
		return nil
	}
	res := m.done

	if err := res.sess.Validate(res.name); err != nil {
		return fmt.Errorf("built session is invalid: %w", err)
	}

	path, local := config.EffectivePath(gopts.configPath)
	if local {
		fmt.Fprintf(cmd.ErrOrStderr(), "mox: saving into ./%s\n", config.LocalConfigName)
	}
	if err := appendSessionToConfig(path, res.name, res.sess, res.overwrite); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Saved session %q -> %s\n", res.name, path)

	if res.action != addActionSaveStart {
		return nil
	}

	saved, err := loadConfigAt(path)
	if err != nil {
		return fmt.Errorf("reload config after save: %w", err)
	}
	mgr, err := session.NewManager(saved, loggerFromContext(cmd.Context()))
	if err != nil {
		return err
	}
	return mgr.CreateOrAttach(cmd.Context(), res.name, false)
}
