package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type importOpts struct {
	as    string // rename target on import
	print bool   // print YAML to stdout instead of saving
	force bool   // overwrite existing config entry
}

func newImportCommand() *cobra.Command {
	o := &importOpts{}
	cmd := &cobra.Command{
		Use:     "import <tmux-session>",
		GroupID: groupSession,
		Short:   "Capture a running tmux session into the config",
		Long: `Inspect a running tmux session and add it to your mox config so it
can be recreated later with 'mox -a <name>'. Window/pane structure and
each pane's current working directory are captured.

Note: per-pane shell commands cannot be recovered from a running tmux
session (send-keys is one-way), so the imported session is structure-only.
Add 'commands:' entries to make the imported session fully reproducible.`,
		Example: `  mox import work               under its tmux name
  mox import work -n my-work    rename on import
  mox import work -p            preview on stdout, don't save
  mox import work -F            overwrite an existing config entry`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeRunningTmuxSessions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd, args, o)
		},
	}

	cmd.Flags().StringVarP(&o.as, "as", "n", "", "save under this name instead of the tmux session's name")
	cmd.Flags().BoolVarP(&o.print, "print", "p", false, "print YAML to stdout instead of saving to config")
	cmd.Flags().BoolVarP(&o.force, "force", "F", false, "overwrite an existing config entry with the same name")
	return cmd
}

func runImport(cmd *cobra.Command, args []string, o *importOpts) error {
	src := args[0]
	dst := o.as
	if dst == "" {
		dst = src
	}

	client, err := tmux.NewClient()
	if err != nil {
		return err
	}

	exists, err := client.SessionExists(src)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("tmux session %q does not exist", src)
	}

	imported, err := inspectSession(client, src)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", src, err)
	}

	if err := imported.Validate(dst); err != nil {
		return fmt.Errorf("imported session is invalid: %w", err)
	}

	if o.print {
		return printSessionYAML(cmd.OutOrStdout(), dst, imported)
	}

	gopts := optsFromContext(cmd.Context())
	path := gopts.configPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	path = config.ResolvePath(path)
	if err := appendSessionToConfig(path, dst, imported, o.force); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported tmux session %q -> %s as %q\n", src, path, dst)
	fmt.Fprintln(cmd.OutOrStdout(), "(Add 'commands:' entries to the new session to make it reproducible.)")
	return nil
}

// inspectSession queries tmux and builds a config.Session reflecting the
// window/pane structure of the running session.
func inspectSession(c *tmux.Client, name string) (*config.Session, error) {
	wins, err := c.ListWindowsForSession(name)
	if err != nil {
		return nil, fmt.Errorf("list windows: %w", err)
	}
	if len(wins) == 0 {
		return nil, fmt.Errorf("session %q has no windows (unexpected)", name)
	}

	sess := &config.Session{}
	for _, w := range wins {
		panes, err := c.ListPanesForWindow(w.ID)
		if err != nil {
			return nil, fmt.Errorf("list panes for window %s: %w", w.Name, err)
		}
		if len(panes) == 0 {
			return nil, fmt.Errorf("window %s has no panes (unexpected)", w.Name)
		}

		winRoot := panes[0].CurrentPath
		// If every pane shares the same cwd, set window root; otherwise
		// leave it empty (per-pane cwd isn't representable in mox's schema).
		for _, p := range panes[1:] {
			if p.CurrentPath != winRoot {
				winRoot = ""
				break
			}
		}

		// Default to horizontal stacks for non-root panes. Users can adjust
		// after import — we can't recover the exact split direction from
		// tmux's binary layout string without a full parser.
		configPanes := make([]*config.Pane, len(panes))
		for i := range panes {
			split := config.SplitHorizontal
			if i == 0 {
				split = config.SplitRoot
			}
			configPanes[i] = &config.Pane{Split: split}
		}

		sess.Windows = append(sess.Windows, &config.Window{
			Name:  w.Name,
			Root:  winRoot,
			Panes: configPanes,
		})
	}
	return sess, nil
}

// printSessionYAML writes a YAML snippet of the form `sessions: { name: {...} }`
// suitable for copy-pasting into a config file.
func printSessionYAML(w io.Writer, name string, sess *config.Session) error {
	wrapped := map[string]map[string]*config.Session{"sessions": {name: sess}}
	enc := yaml.NewEncoder(w)
	defer enc.Close()
	enc.SetIndent(4)
	return enc.Encode(wrapped)
}

// appendSessionToConfig adds the given session to the YAML config file under
// the existing `sessions:` mapping. Uses the yaml.v3 Node API so existing
// comments and ordering survive.
func appendSessionToConfig(path, name string, sess *config.Session, force bool) error {
	var root yaml.Node
	data, err := os.ReadFile(path) //nolint:gosec // user-supplied path is intentional
	switch {
	case err == nil:
		if len(data) > 0 {
			if err := yaml.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("parse existing config %s: %w", path, err)
			}
		}
	case os.IsNotExist(err):
		// New file — we'll create it below.
	default:
		return fmt.Errorf("read %s: %w", path, err)
	}

	if root.Kind == 0 {
		root = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}},
		}
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("unexpected config structure (root is not a mapping)")
	}
	topMap := root.Content[0]

	sessionsMap := findOrCreateMapKey(topMap, "sessions")

	// Encode the new session into a Node.
	var sessNode yaml.Node
	if err := sessNode.Encode(sess); err != nil {
		return fmt.Errorf("encode session: %w", err)
	}

	for i := 0; i < len(sessionsMap.Content); i += 2 {
		if sessionsMap.Content[i].Value == name {
			if !force {
				return fmt.Errorf("config already has session %q (use --force to overwrite)", name)
			}
			sessionsMap.Content[i+1] = &sessNode
			return writeYAMLNode(path, &root)
		}
	}
	sessionsMap.Content = append(sessionsMap.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: name},
		&sessNode,
	)
	return writeYAMLNode(path, &root)
}

// findOrCreateMapKey returns the value Node for a given key in a MappingNode,
// creating an empty MappingNode if the key doesn't exist yet.
func findOrCreateMapKey(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		v,
	)
	return v
}

func writeYAMLNode(path string, node *yaml.Node) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// completeRunningTmuxSessions powers tab completion for `mox import <TAB>`.
// It returns the running tmux sessions only (no config involvement).
func completeRunningTmuxSessions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	client, err := tmux.NewClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	sessions, err := client.ListSessions()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return sessions, cobra.ShellCompDirectiveNoFileComp
}
