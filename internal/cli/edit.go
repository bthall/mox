package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/spf13/cobra"
)

func newEditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edit [session]",
		GroupID: groupConfig,
		Short:   "Edit the config in the full-screen editor (or $EDITOR)",
		Long: `Open the full-screen session editor: navigate sessions and their fields,
edit hosts and hooks, rename/duplicate/delete — and save through a
validated diff preview. With a session name, that session is selected on
launch. The same editor is available from the bare 'mox' picker via ctrl+e.

The raw file is never far away: press 'o' inside the editor to open the
config in $VISUAL (falling back to $EDITOR, then vi). Invalid configs and
non-terminal invocations skip the editor and go straight to $EDITOR, which
validates the file after the editor exits.`,
		Example: `  mox edit               open the session editor
  mox edit webfarm       open it with a session selected
  mox edit -c ~/other/config.yml`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeConfiguredSession,
		RunE:              runEdit,
	}
	return cmd
}

func runEdit(cmd *cobra.Command, args []string) error {
	opts := optsFromContext(cmd.Context())

	path, local := config.EffectivePath(opts.configPath)
	if local {
		fmt.Fprintf(os.Stderr, "mox: using ./%s\n", config.LocalConfigName)
	}
	if !config.Exists(path) {
		return fmt.Errorf("config file not found at %s\n\nRun 'mox init' to create a default configuration", path)
	}

	if len(args) == 1 {
		st, err := loadEditorState(path)
		if err != nil {
			return err
		}
		if _, ok := st.cfg.Sessions[args[0]]; !ok {
			return fmt.Errorf("no configured session %q\n\nConfigured sessions: %s",
				args[0], strings.Join(st.cfg.ListSessionNames(), ", "))
		}
		stdin, ok := cmd.InOrStdin().(*os.File)
		if !ok || !isTerminal(stdin) {
			return errors.New("the session editor is interactive and needs a terminal; run 'mox edit' (no argument) to use $EDITOR")
		}
		return runEditorTUI(cmd, st, args[0])
	}

	// Bare 'mox edit' on a terminal opens the full-screen editor too. A
	// config the editor can't load (parse/validation errors) falls back to
	// $EDITOR — fixing a broken config by hand is this command's oldest job.
	if stdin, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(stdin) {
		st, err := loadEditorState(path)
		if err == nil {
			return runEditorTUI(cmd, st, "")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "mox: config has errors, opening $EDITOR instead:\n  %v\n", err)
	}

	editor := editorCommand()
	if editor == "" {
		return fmt.Errorf("no editor found: set $VISUAL or $EDITOR")
	}

	return editAndValidate(path, editor, cmd.OutOrStdout())
}

// editorCommand picks the user's editor: $VISUAL, then $EDITOR, then vi if
// present on PATH. The returned string may contain arguments ("code --wait").
func editorCommand() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	if _, err := exec.LookPath("vi"); err == nil {
		return "vi"
	}
	return ""
}

// editAndValidate runs the editor on path, waits for it to exit, then
// validates the file. The editor inherits the terminal. A validation failure
// is returned as an error (non-zero exit) but the edit itself has already
// been saved.
func editAndValidate(path, editor string, out io.Writer) error {
	parts := strings.Fields(editor)
	ed := exec.Command(parts[0], append(parts[1:], path)...) //nolint:gosec // the user's own $EDITOR choice
	ed.Stdin = os.Stdin
	ed.Stdout = os.Stdout
	ed.Stderr = os.Stderr
	if err := ed.Run(); err != nil {
		return fmt.Errorf("editor %q: %w", parts[0], err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("saved, but the config has errors:\n\n%w", err)
	}
	fmt.Fprintf(out, "OK: %s is valid (%d sessions, %d layouts)\n", path, len(cfg.Sessions), len(cfg.Layouts))
	return nil
}

// runEditorTUI starts the full-screen session editor on an already-loaded
// state (callers validate the initial session before getting here).
func runEditorTUI(cmd *cobra.Command, st *editorState, initial string) error {
	clusters, _ := loadClusterssh() // missing file is fine

	// Running-state dots are best-effort: no tmux, no dots.
	running := map[string]session.SessionInfo{}
	if mgr, err := session.NewManager(st.cfg, loggerFromContext(cmd.Context())); err == nil {
		if infos, err := mgr.List(); err == nil {
			for _, info := range infos {
				running[info.Name] = info
			}
		}
	}

	_, err := tea.NewProgram(newEditorModel(st, clusters, running, initial), tea.WithAltScreen()).Run()
	return err
}
