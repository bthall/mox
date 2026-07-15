package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/proc"
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
		Use:     "import [tmux-session]",
		GroupID: groupSession,
		Short:   "Capture a running tmux session into the config",
		Long: `Inspect a running tmux session and add it to your mox config so it
can be recreated later with 'mox -a <name>'. Window/pane structure —
including split directions and sizes — and each pane's current working
directory are captured. With no argument, the session you are currently
inside is imported.

SSH connections are recovered from the OS process table: a window whose
panes are all plain 'ssh <host>' connections is imported as a simple-mode
'hosts:' list, and any other pane running ssh keeps its connection as a
'commands:' entry.

Note: other per-pane shell commands cannot be recovered (an editor or REPL
you started by typing is not reproducible), so those panes are
structure-only. Add 'commands:' entries to make them fully reproducible.`,
		Example: `  mox import work               under its tmux name
  mox import                    the session you're inside
  mox import work -n my-work    rename on import
  mox import work -p            preview on stdout, don't save
  mox import work -F            overwrite an existing config entry`,
		Args:              cobra.MaximumNArgs(1),
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
	client, err := tmux.NewClient()
	if err != nil {
		return err
	}

	src, err := resolveImportSource(client, args)
	if err != nil {
		return err
	}
	dst := o.as
	if dst == "" {
		dst = src
	}

	exists, err := client.SessionExists(src)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("tmux session %q does not exist", src)
	}

	imported, warnings, err := inspectSession(client, src)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", src, err)
	}
	for _, w := range warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "mox: %s\n", w)
	}

	if err := imported.Validate(dst); err != nil {
		return fmt.Errorf("imported session is invalid: %w", err)
	}

	if o.print {
		return printSessionYAML(cmd.OutOrStdout(), dst, imported)
	}

	gopts := optsFromContext(cmd.Context())
	path, local := config.EffectivePath(gopts.configPath)
	if local {
		fmt.Fprintf(cmd.ErrOrStderr(), "mox: importing into ./%s\n", config.LocalConfigName)
	}
	if err := appendSessionToConfig(path, dst, imported, o.force); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Imported tmux session %q -> %s as %q\n", src, path, dst)
	fmt.Fprintln(cmd.OutOrStdout(), "(Add 'commands:' entries to the new session to make it reproducible.)")
	return nil
}

// capturedPane is the per-pane state import recovers from tmux and the process
// table: the pane's id (for matching against the window layout), its working
// directory, and the argv of its foreground process (nil when none could be
// recovered or the pane is just a shell).
type capturedPane struct {
	id   string
	path string
	argv []string
}

// inspectSession queries tmux and builds a config.Session reflecting the
// window/pane structure of the running session. For each pane it also tries to
// recover the foreground ssh connection from the OS process table so that
// SSH fan-outs round-trip as simple-mode `hosts:` rather than losing the host
// (tmux only reports the command basename, not its arguments). warnings lists
// windows whose pane geometry could not be captured faithfully.
func inspectSession(c *tmux.Client, name string) (*config.Session, []string, error) {
	wins, err := c.ListWindowsForSession(name)
	if err != nil {
		return nil, nil, fmt.Errorf("list windows: %w", err)
	}
	if len(wins) == 0 {
		return nil, nil, fmt.Errorf("session %q has no windows (unexpected)", name)
	}

	// Best-effort process snapshot. On failure we degrade to structure-only
	// import rather than failing the whole command.
	procs, _ := proc.Capture(context.Background())

	sess := &config.Session{}
	var warnings []string
	for _, w := range wins {
		panes, err := c.ListPanesForWindow(w.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list panes for window %s: %w", w.Name, err)
		}
		if len(panes) == 0 {
			return nil, nil, fmt.Errorf("window %s has no panes (unexpected)", w.Name)
		}

		captured := make([]capturedPane, len(panes))
		for i, p := range panes {
			cp := capturedPane{id: p.ID, path: p.CurrentPath}
			if p.PID > 0 && len(procs) > 0 {
				cp.argv = proc.ForegroundCommand(procs, p.PID, isSSHCommand)
			}
			captured[i] = cp
		}

		win, degraded := buildWindow(w.Name, w.Layout, captured)
		if degraded {
			warnings = append(warnings, fmt.Sprintf("window %q: pane geometry simplified (layout is not expressible as sequential splits)", w.Name))
		}
		sess.Windows = append(sess.Windows, win)
	}
	return sess, warnings, nil
}

// buildWindow turns the captured panes of one tmux window into a config.Window.
//
// When every pane is a plain `ssh [user@]host` connection sharing one user, the
// window collapses to simple mode (a `hosts:` list) — this is how an SSH
// fan-out is meant to be expressed in mox and what makes the import
// reproducible. Otherwise the explicit pane structure is preserved with its
// real geometry when the window layout can be replayed as sequential splits;
// degraded reports the fallback to a plain horizontal stack. Any pane that
// *is* an ssh connection still records its command so that connection is not
// lost. Non-ssh panes stay structure-only — mox does not try to relaunch
// editors or REPLs.
func buildWindow(name, layout string, panes []capturedPane) (win *config.Window, degraded bool) {
	win = &config.Window{Name: name}

	// Shared working directory across panes becomes the window root; differing
	// cwds aren't representable per-pane in mox's schema, so leave it empty.
	if len(panes) > 0 {
		root := panes[0].path
		for _, p := range panes[1:] {
			if p.path != root {
				root = ""
				break
			}
		}
		win.Root = root
	}

	// Can the whole window be expressed as a uniform host fan-out?
	hosts := make([]string, 0, len(panes))
	user := ""
	uniform := len(panes) > 0
	for i, p := range panes {
		u, h, plain := parseSSHDest(p.argv)
		if !plain {
			uniform = false
			break
		}
		if i == 0 {
			user = u
		} else if u != user {
			uniform = false
			break
		}
		hosts = append(hosts, h)
	}
	if uniform {
		win.Hosts = hosts
		win.SSHUser = user
		return win, false
	}

	// Explicit panes, with real geometry when the layout linearizes into
	// mox's split-the-previous-pane model. Recover ssh connections as
	// commands either way.
	if chain := geometryChain(layout, panes); chain != nil {
		byID := make(map[string]capturedPane, len(panes))
		for _, p := range panes {
			byID[p.id] = p
		}
		win.Panes = make([]*config.Pane, len(chain))
		for i, g := range chain {
			pane := &config.Pane{Split: g.split, Size: g.size}
			if p := byID[g.paneID]; isSSHCommand(p.argv) {
				pane.Commands = []string{strings.Join(p.argv, " ")}
			}
			win.Panes[i] = pane
		}
		return win, false
	}

	// Fallback: a plain horizontal stack. Degraded only when there was
	// geometry to lose.
	win.Panes = make([]*config.Pane, len(panes))
	for i, p := range panes {
		split := config.SplitHorizontal
		if i == 0 {
			split = config.SplitRoot
		}
		pane := &config.Pane{Split: split}
		if isSSHCommand(p.argv) {
			pane.Commands = []string{strings.Join(p.argv, " ")}
		}
		win.Panes[i] = pane
	}
	return win, len(panes) > 1
}

// geometryChain parses and linearizes a window layout, returning nil when the
// geometry cannot be applied: unparseable layout, a tree that doesn't reduce
// to sequential splits, or panes that don't match the captured set.
func geometryChain(layout string, panes []capturedPane) []paneGeom {
	if layout == "" {
		return nil
	}
	root, err := tmux.ParseLayout(layout)
	if err != nil {
		return nil
	}
	chain, ok := chainFromLayout(root)
	if !ok || len(chain) != len(panes) {
		return nil
	}
	ids := make(map[string]bool, len(panes))
	for _, p := range panes {
		ids[p.id] = true
	}
	for _, g := range chain {
		if !ids[g.paneID] {
			return nil
		}
	}
	return chain
}

// resolveImportSource picks the tmux session to import: the explicit
// argument when given, otherwise the session the caller is inside — so a
// bare `mox import` from within tmux captures the current session.
func resolveImportSource(client *tmux.Client, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	name, err := client.CurrentSession()
	if err != nil {
		return "", fmt.Errorf("no session named and not inside tmux (run 'mox import <session>' or run from within the session): %w", err)
	}
	return name, nil
}

// isSSHCommand reports whether argv invokes the ssh client.
func isSSHCommand(argv []string) bool {
	return len(argv) > 0 && filepath.Base(argv[0]) == "ssh"
}

// parseSSHDest parses a plain `ssh [user@]host` invocation. plain is true only
// when argv is exactly the ssh executable followed by a single destination —
// no options and no remote command — which is the form representable as a mox
// host entry. Anything fancier returns plain=false (the caller keeps it as an
// explicit pane command instead).
func parseSSHDest(argv []string) (user, host string, plain bool) {
	if !isSSHCommand(argv) || len(argv) != 2 {
		return "", "", false
	}
	dest := argv[1]
	if strings.HasPrefix(dest, "-") {
		return "", "", false
	}
	if at := strings.Index(dest, "@"); at >= 0 {
		return dest[:at], dest[at+1:], true
	}
	return "", dest, true
}

// printSessionYAML writes a YAML snippet of the form `sessions: { name: {...} }`
// suitable for copy-pasting into a config file.
func printSessionYAML(w io.Writer, name string, sess *config.Session) error {
	wrapped := map[string]map[string]*config.Session{"sessions": {name: sess}}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(4)
	if err := enc.Encode(wrapped); err != nil {
		return err
	}
	return enc.Close() // Close flushes the encoder's buffer
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
