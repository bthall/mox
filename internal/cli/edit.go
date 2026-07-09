package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bthall/mox/internal/config"
	"github.com/spf13/cobra"
)

func newEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "edit",
		GroupID: groupConfig,
		Short:   "Open the config in your editor, then validate it",
		Long: `Open the configuration file in $VISUAL (falling back to $EDITOR, then vi)
and validate it after the editor exits. Validation errors are reported with
line numbers but never block the save — the file is already written; fix it
and run 'mox edit' or 'mox validate' again.`,
		Example: `  mox edit
  mox edit -c ~/other/config.yml`,
		Args: cobra.NoArgs,
		RunE: runEdit,
	}
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
