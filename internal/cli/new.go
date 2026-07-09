package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/session"
	"github.com/bthall/mox/internal/tmux"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type newOpts struct {
	name      string
	connect   string
	user      string
	root      string
	from      string
	file      string
	arrange   string
	exclude   []string
	sync      bool
	sudo      bool
	temporary bool
	detach    bool
	force     bool
	window    bool
}

func newNewCommand() *cobra.Command {
	// cssh-style defaults: tiled layout, synchronize-panes on, and `sudo -i`
	// after connect. These are right for the daily multi-host admin workflow.
	// Override any of them with --sync=false, --arrange='', --sudo=false.
	o := &newOpts{
		sync:    true,
		arrange: "tiled",
		sudo:    true,
	}
	cmd := &cobra.Command{
		Use:     "new [hosts...]",
		Aliases: []string{"cssh"},
		GroupID: groupSession,
		Short:   "Create an ad-hoc session or window",
		Long: `Create a tmux session (or window, with -w) from the command line.

  Zero hosts   -> single local pane, plain shell.
  Hosts given  -> one pane per host with cssh-style defaults (tiled layout,
                  synchronize-panes on, 'sudo -i' after connect). Override
                  with --sync=false, --arrange='', --sudo=false.

Cluster expansion: any positional starting with '@' is expanded — first
against the mox config (session hosts), then the clusterssh 'clusters'
file. Nested clusters are flattened.

Use -f / --file to supply a full session body in YAML, or --from to clone
an existing configured session and override its hosts.`,
		Example: `  mox new                            quick local session
  mox new -n work -r ~/proj          named local session in ~/proj
  mox new -s                         local session that drops to 'sudo -i'
  mox new -t                         local session, destroyed on detach

  mox new @api-cluster                   cssh-style on a cluster
  mox new host1 host2 host3          cssh-style on literal hosts
  mox new -u root @monitoring    ssh as root
  mox new @api-cluster -x api2           the cluster minus one host
  mox new -S=false @api-cluster          no synchronize-panes
  mox new -w @api-cluster                open as a window in current tmux

  mox new --from api-cluster extra-host  clone configured session, add a host
  echo 'hosts: [a, b]' | mox new -n tmp -f -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd, args, o)
		},
	}

	cmd.Flags().StringVarP(&o.name, "name", "n", "", "session/window name (default: tmp-<timestamp>)")
	cmd.Flags().StringVarP(&o.connect, "connect", "C", "", "connect template (default: 'ssh {{host}}')")
	cmd.Flags().StringVarP(&o.user, "user", "u", "", "ssh as USER (prefixes default template; ignored if --connect is set)")
	cmd.Flags().StringVarP(&o.root, "root", "r", "", "working directory for panes")
	cmd.Flags().StringVar(&o.from, "from", "", "clone settings from an existing configured session")
	cmd.Flags().StringVarP(&o.file, "file", "f", "", "load session body from YAML file (- for stdin)")
	cmd.Flags().StringVarP(&o.arrange, "arrange", "a", o.arrange, "tmux layout (set to '' to disable)")
	cmd.Flags().BoolVarP(&o.sync, "sync", "S", o.sync, "synchronize-panes (broadcast typing)")
	cmd.Flags().BoolVarP(&o.sudo, "sudo", "s", o.sudo, "send 'sudo -i' after connect")
	cmd.Flags().BoolVarP(&o.temporary, "temporary", "t", false, "destroy session when the last client detaches")
	cmd.Flags().BoolVarP(&o.detach, "detach", "d", false, "create without attaching")
	cmd.Flags().BoolVarP(&o.force, "force", "F", false, "recreate if a session with the same name exists")
	cmd.Flags().BoolVarP(&o.window, "window", "w", false, "open as a new window in the current tmux session (requires $TMUX)")
	cmd.Flags().StringArrayVarP(&o.exclude, "exclude", "x", nil, "drop HOST (or @cluster) from the expanded host list; repeatable")

	cmd.ValidArgsFunction = completeHostsOrClusters
	_ = cmd.RegisterFlagCompletionFunc("arrange", completeArrange)
	_ = cmd.RegisterFlagCompletionFunc("from", completeConfiguredSession)
	_ = cmd.RegisterFlagCompletionFunc("exclude", completeHostsOrClusters)
	return cmd
}

func runNew(cmd *cobra.Command, args []string, o *newOpts) error {
	gopts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())

	if err := o.validate(); err != nil {
		return err
	}

	name := o.name
	if name == "" {
		name = inferAdHocName(args, gopts.configPath)
	}

	expanded, err := expandPositional(args, gopts.configPath)
	if err != nil {
		return err
	}
	args = expanded

	if len(o.exclude) > 0 {
		excluded, err := expandPositional(o.exclude, gopts.configPath)
		if err != nil {
			return err
		}
		args, err = excludeHosts(args, excluded)
		if err != nil {
			return err
		}
	}

	// When there are no hosts to broadcast to, the cssh-style defaults
	// don't make sense. Honor explicit overrides; clear unsignaled defaults.
	if len(args) == 0 && o.from == "" && o.file == "" {
		if !cmd.Flag("sync").Changed {
			o.sync = false
		}
		if !cmd.Flag("sudo").Changed {
			o.sudo = false
		}
		if !cmd.Flag("arrange").Changed {
			o.arrange = ""
		}
	}

	sess, err := buildAdHocSession(o, args, gopts.configPath)
	if err != nil {
		return err
	}

	cfg := &config.Config{Sessions: map[string]*config.Session{name: sess}}
	mgr, err := session.NewManager(cfg, logger)
	if err != nil {
		return err
	}

	if o.window {
		client, err := tmux.NewClient()
		if err != nil {
			return err
		}
		parent, err := client.CurrentSession()
		if err != nil {
			return fmt.Errorf("--window requires being inside a tmux session: %w", err)
		}
		return mgr.CreateAdHocWindow(cmd.Context(), parent, name, sess)
	}

	return mgr.CreateAdHoc(cmd.Context(), name, sess, session.AdHocOptions{
		Force:     o.force,
		Detach:    o.detach,
		Temporary: o.temporary,
	})
}

func (o *newOpts) validate() error {
	if o.from != "" && o.file != "" {
		return errors.New("--from and --file are mutually exclusive")
	}
	if o.temporary && o.detach {
		return errors.New("--temporary requires attaching: tmux's destroy-unattached would kill the session immediately (drop --detach)")
	}
	if o.window && (o.temporary || o.detach || o.force) {
		return errors.New("--window is incompatible with --temporary, --detach, and --force (those are session-lifecycle flags)")
	}
	if o.user != "" && o.connect != "" {
		return errors.New("--user is ignored when --connect is set; specify the user inside the connect template instead")
	}
	return nil
}

// buildAdHocSession assembles a *config.Session from --file/--from/flags/args.
// Precedence rules:
//   - --file provides the initial base session body.
//   - --from clones the named configured session as the initial base.
//   - Without --file or --from, the base is empty.
//   - Positional hosts override Hosts (and clear Windows if present).
//   - --connect / --user / --root / --arrange / --sync override the corresponding fields.
//   - --sudo appends `sudo -i` to commands.
func buildAdHocSession(o *newOpts, args []string, configPath string) (*config.Session, error) {
	var base *config.Session

	switch {
	case o.file != "":
		body, err := readFileOrStdin(o.file)
		if err != nil {
			return nil, fmt.Errorf("read --file: %w", err)
		}
		base = &config.Session{}
		if err := yaml.Unmarshal(body, base); err != nil {
			return nil, fmt.Errorf("parse --file: %w", err)
		}
	case o.from != "":
		cfg, err := loadConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("--from requires a valid config: %w", err)
		}
		src, ok := cfg.GetSession(o.from)
		if !ok {
			return nil, fmt.Errorf("--from session %q not found in config", o.from)
		}
		clone := *src
		base = &clone
	default:
		base = &config.Session{}
	}

	if len(args) > 0 {
		base.Hosts = args
		base.Windows = nil
	}
	if o.connect != "" {
		base.Connect = o.connect
		base.SSHUser = ""
	}
	if o.user != "" {
		base.SSHUser = o.user
		base.Connect = ""
	}
	if o.root != "" {
		base.Root = o.root
	}
	if o.arrange != "" {
		base.Arrange = o.arrange
	}
	if o.sync {
		base.Sync = true
	}
	if o.sudo {
		base.Commands = append(base.Commands, "sudo -i")
	}

	// Zero hosts + zero windows is allowed — it becomes a single local
	// pane in a new session (with --sudo / commands optional).
	return base, nil
}

func readFileOrStdin(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path) //nolint:gosec // user-supplied path is intentional
}

func defaultAdHocName() string {
	return "tmp-" + strings.ReplaceAll(time.Now().Format("20060102-150405"), ":", "")
}

// inferAdHocName returns the session name for an ad-hoc new command. If the
// sole positional arg is @<name> and <name> is a configured mox session, that
// name is used so that `mox new @foo` creates a session named "foo" rather
// than a generated tmp- name. Falls back to defaultAdHocName otherwise.
func inferAdHocName(args []string, configPath string) string {
	if len(args) == 1 && strings.HasPrefix(args[0], clusterPrefix) {
		candidate := strings.TrimPrefix(args[0], clusterPrefix)
		if cfg, _ := tryLoadConfig(configPath); cfg != nil {
			if _, ok := cfg.GetSession(candidate); ok {
				return candidate
			}
		}
	}
	return defaultAdHocName()
}

// excludeHosts removes every host in exclude from hosts. An exclusion that
// matches nothing is an error — it is almost always a typo, and silently
// keeping the host defeats the point of excluding it.
func excludeHosts(hosts, exclude []string) ([]string, error) {
	drop := make(map[string]bool, len(exclude))
	for _, e := range exclude {
		drop[e] = false // false = not yet matched
	}
	kept := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if _, excluded := drop[h]; excluded {
			drop[h] = true
			continue
		}
		kept = append(kept, h)
	}
	for e, matched := range drop {
		if !matched {
			return nil, fmt.Errorf("--exclude %s: host is not in the expanded host list", e)
		}
	}
	return kept, nil
}

// expandPositional applies @cluster expansion to positional args. Missing
// config or clusterssh files are not errors — those sources are simply
// unavailable for lookup. Unknown clusters do return an error.
func expandPositional(args []string, configPath string) ([]string, error) {
	if !hasClusterRef(args) {
		return args, nil
	}
	cfg, _ := tryLoadConfig(configPath) // missing config is fine for ad-hoc
	clusters, _ := loadClusterssh()     // missing file is fine
	return expandHosts(args, cfg, clusters)
}

func hasClusterRef(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, clusterPrefix) {
			return true
		}
	}
	return false
}

// tryLoadConfig is a non-erroring config loader: returns (nil, nil) when
// the config file is missing or invalid. Used for completion/expansion
// where the absence of a config should not break the command.
func tryLoadConfig(path string) (*config.Config, error) {
	resolved, _ := config.EffectivePath(path)
	cfg, err := loadConfigAt(resolved)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
