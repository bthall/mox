package tmux

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/bthall/mox/internal/config"
)

// DefaultConnectTemplate is used when neither a session nor a window
// specifies a custom connect template.
const DefaultConnectTemplate = "ssh {{host}}"

// Builder builds tmux sessions from configuration.
type Builder struct {
	tx     Tmux
	config *config.Config
	log    *slog.Logger
}

// NewBuilder creates a new builder that drives the given Tmux implementation.
// If logger is nil, slog.Default() is used.
func NewBuilder(tx Tmux, cfg *config.Config, logger *slog.Logger) *Builder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Builder{tx: tx, config: cfg, log: logger}
}

// hostPaneOpts collects the per-host-pane settings derived from a session
// and/or window. Computed once per buildHostPanes call.
type hostPaneOpts struct {
	connect string
	arrange string
	sync    bool
	hold    bool
	retry   int
}

// BuildSession creates a tmux session from configuration. The context is
// honored at command boundaries — if the context is canceled mid-build the
// caller should call KillSession to clean up the partial session.
func (b *Builder) BuildSession(ctx context.Context, name string, session *config.Session) error {
	root, err := resolveDir(session.Root)
	if err != nil {
		return fmt.Errorf("resolve session root: %w", err)
	}

	switch {
	case session.IsSimple():
		return b.buildSimpleSession(ctx, name, session, root)
	case len(session.Windows) > 0:
		return b.buildComplexSession(ctx, name, session, root)
	default:
		return b.buildLocalSession(ctx, name, session, root)
	}
}

// prependCmds returns pre followed by cmds in a fresh slice; nil when both
// are empty.
func prependCmds(pre, cmds []string) []string {
	if len(pre) == 0 {
		return cmds
	}
	out := make([]string, 0, len(pre)+len(cmds))
	out = append(out, pre...)
	return append(out, cmds...)
}

// buildLocalSession creates a session with a single local pane — no ssh,
// no broadcast — and optionally runs the session.Commands in that pane.
// Used when the user wants a quick named tmux session without any host list.
func (b *Builder) buildLocalSession(ctx context.Context, name string, session *config.Session, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.tx.CreateSession(name, root, "main"); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	cmds := prependCmds(session.Pre, session.Commands)
	if len(cmds) == 0 {
		return nil
	}
	winID, err := b.tx.FirstWindowID(name)
	if err != nil {
		return fmt.Errorf("locate first window: %w", err)
	}
	paneID, err := b.tx.FirstPaneID(winID)
	if err != nil {
		return fmt.Errorf("locate first pane: %w", err)
	}
	if err := b.tx.SendKeys(paneID, cmds); err != nil {
		return fmt.Errorf("send commands: %w", err)
	}
	return nil
}

// BuildAdHocWindow creates a new window inside an existing tmux session.
// With hosts, the window gets one pane per host like a simple-mode session.
// Without hosts, the window opens with a single local pane (commands, if
// any, are sent to it). Used by `mox new --window`.
func (b *Builder) BuildAdHocWindow(ctx context.Context, parentSession, windowName string, session *config.Session) (string, error) {
	if len(session.Windows) > 0 {
		return "", fmt.Errorf("--window cannot create a multi-window layout; drop --window or provide hosts")
	}
	root, err := resolveDir(session.Root)
	if err != nil {
		return "", fmt.Errorf("resolve session root: %w", err)
	}
	winID, err := b.tx.CreateWindow(parentSession, windowName, root)
	if err != nil {
		return "", fmt.Errorf("create window: %w", err)
	}

	if !session.IsSimple() {
		// Local single-pane window. Just send commands if any.
		if cmds := prependCmds(session.Pre, session.Commands); len(cmds) > 0 {
			paneID, err := b.tx.FirstPaneID(winID)
			if err != nil {
				return winID, fmt.Errorf("locate first pane: %w", err)
			}
			if err := b.tx.SendKeys(paneID, cmds); err != nil {
				return winID, fmt.Errorf("send commands: %w", err)
			}
		}
		return winID, nil
	}

	opts := hostPaneOptsFor(session, nil)
	if err := b.buildHostPanes(ctx, winID, session.Hosts, opts, prependCmds(session.Pre, session.Commands), root); err != nil {
		return winID, err
	}
	b.applyWindowPostBuild(winID, opts)
	return winID, nil
}

// buildSimpleSession creates a session with one window and a pane per host.
func (b *Builder) buildSimpleSession(ctx context.Context, name string, session *config.Session, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := b.tx.CreateSession(name, root, "main"); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	winID, err := b.tx.FirstWindowID(name)
	if err != nil {
		return fmt.Errorf("locate first window: %w", err)
	}

	opts := hostPaneOptsFor(session, nil)
	if err := b.buildHostPanes(ctx, winID, session.Hosts, opts, prependCmds(session.Pre, session.Commands), root); err != nil {
		return err
	}
	b.applyWindowPostBuild(winID, opts)
	return nil
}

// buildComplexSession creates a session with multiple windows.
func (b *Builder) buildComplexSession(ctx context.Context, name string, session *config.Session, sessionRoot string) error {
	if len(session.Windows) == 0 {
		return fmt.Errorf("no windows defined")
	}

	first := session.Windows[0]
	firstRoot, err := resolveOrFallback(first.Root, sessionRoot)
	if err != nil {
		return err
	}

	if err := b.tx.CreateSession(name, firstRoot, first.Name); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	firstWinID, err := b.tx.FirstWindowID(name)
	if err != nil {
		return fmt.Errorf("locate first window: %w", err)
	}
	if err := b.buildWindow(ctx, firstWinID, session, first, firstRoot); err != nil {
		return fmt.Errorf("window %q: %w", first.Name, err)
	}

	for _, window := range session.Windows[1:] {
		if err := ctx.Err(); err != nil {
			return err
		}
		winRoot, err := resolveOrFallback(window.Root, sessionRoot)
		if err != nil {
			return err
		}
		winID, err := b.tx.CreateWindow(name, window.Name, winRoot)
		if err != nil {
			return fmt.Errorf("create window %q: %w", window.Name, err)
		}
		if err := b.buildWindow(ctx, winID, session, window, winRoot); err != nil {
			return fmt.Errorf("window %q: %w", window.Name, err)
		}
	}

	if err := b.tx.SelectWindowByID(firstWinID); err != nil {
		b.log.Warn("select first window failed", "session", name, "error", err)
	}
	return nil
}

func (b *Builder) buildWindow(ctx context.Context, winID string, session *config.Session, window *config.Window, root string) error {
	pre := prependCmds(session.Pre, window.Pre)
	if window.IsSimple() {
		opts := hostPaneOptsFor(session, window)
		if err := b.buildHostPanes(ctx, winID, window.Hosts, opts, prependCmds(pre, window.Commands), root); err != nil {
			return err
		}
		b.applyWindowPostBuild(winID, opts)
		return nil
	}

	var panes []*config.Pane
	if window.Layout != "" {
		layout, ok := b.config.GetLayout(window.Layout)
		if !ok {
			return fmt.Errorf("layout %q not found", window.Layout)
		}
		panes = layout.Panes
	} else {
		panes = window.Panes
	}
	return b.buildPanes(ctx, winID, panes, pre, root)
}

// buildHostPanes splits a window into one pane per host, runs the connect
// template (e.g. "ssh {{host}}") in each pane, then sends any extra commands.
//
// When opts.arrange is set, the layout is re-applied after each split so
// tmux progressively redistributes space — this is how the cssh-style
// workflow handles >5 hosts without "no space for new pane" errors. When
// arrange is empty, we fall back to applying `tiled` between splits if the
// host count crosses a threshold, since vertical strips alone run out of
// horizontal room around 6-7 panes on a typical terminal.
func (b *Builder) buildHostPanes(ctx context.Context, winID string, hosts []string, opts hostPaneOpts, commands []string, root string) error {
	firstPane, err := b.tx.FirstPaneID(winID)
	if err != nil {
		return fmt.Errorf("locate first pane: %w", err)
	}

	// Pick the per-split rebalance layout: prefer the explicit arrange, else
	// auto-tile when there are enough hosts to run out of space.
	rebalance := opts.arrange
	if rebalance == "" && len(hosts) > 4 {
		rebalance = "tiled"
	}

	prevPane := firstPane
	for i, host := range hosts {
		if err := ctx.Err(); err != nil {
			return err
		}

		paneID := prevPane
		if i > 0 {
			paneID, err = b.tx.SplitPane(prevPane, SplitVertical, 0, root)
			if err != nil {
				return fmt.Errorf("split pane for host %q: %w", host, err)
			}
			if rebalance != "" {
				if rbErr := b.tx.SelectLayout(winID, rebalance); rbErr != nil {
					b.log.Warn("rebalance layout failed", "window", winID, "error", rbErr)
				}
			}
		}
		prevPane = paneID

		if err := b.tx.SetPaneTitle(paneID, host); err != nil {
			b.log.Warn("set pane title failed", "host", host, "error", err)
		}

		connectCmd := wrapConnect(strings.ReplaceAll(opts.connect, "{{host}}", host), host, opts.hold, opts.retry)
		if err := b.tx.SendKeys(paneID, []string{connectCmd}); err != nil {
			return fmt.Errorf("connect to host %q: %w", host, err)
		}

		if len(commands) > 0 {
			if err := b.tx.SendKeys(paneID, commands); err != nil {
				return fmt.Errorf("send commands to host %q: %w", host, err)
			}
		}
	}
	return nil
}

// wrapConnect decorates the substituted connect command with retry and hold
// behavior. With retry, a failing connection is re-attempted (a clean exit
// never retries); with hold, an ended connection prints a notice and waits
// for Enter before the pane closes — the pane never drops back to a local
// shell, which in a sync window would silently receive broadcast keystrokes.
// host is already validated against the safe-hostname pattern.
func wrapConnect(connectCmd, host string, hold bool, retry int) string {
	cmd := connectCmd
	if retry > 0 {
		attempts := make([]string, 0, retry+1)
		for i := 1; i <= retry+1; i++ {
			attempts = append(attempts, fmt.Sprintf("%d", i))
		}
		cmd = fmt.Sprintf(
			"for _mox_try in %s; do %s && break; printf '[mox] %s: connection failed (attempt %%s)\\n' \"$_mox_try\"; sleep 3; done",
			strings.Join(attempts, " "), connectCmd, host)
	}
	if hold {
		cmd += fmt.Sprintf(
			"; printf '\\n[mox] %s: connection ended. Press Enter to close this pane.\\n'; read -r _mox_ack; exit",
			host)
	}
	return cmd
}

// applyWindowPostBuild applies arrange and sync to a window after its panes
// are built. Errors are logged but not fatal — these are presentation tweaks.
func (b *Builder) applyWindowPostBuild(winID string, opts hostPaneOpts) {
	if opts.arrange != "" {
		if err := b.tx.SelectLayout(winID, opts.arrange); err != nil {
			b.log.Warn("select-layout failed", "window", winID, "layout", opts.arrange, "error", err)
		}
	}
	if opts.sync {
		if err := b.tx.SetWindowOption(winID, "synchronize-panes", "on"); err != nil {
			b.log.Warn("set synchronize-panes failed", "window", winID, "error", err)
		}
	}
}

func (b *Builder) buildPanes(ctx context.Context, winID string, panes []*config.Pane, pre []string, root string) error {
	if len(panes) == 0 {
		return fmt.Errorf("no panes defined")
	}

	firstPane, err := b.tx.FirstPaneID(winID)
	if err != nil {
		return fmt.Errorf("locate first pane: %w", err)
	}

	if cmds := prependCmds(pre, panes[0].Commands); len(cmds) > 0 {
		if err := b.tx.SendKeys(firstPane, cmds); err != nil {
			return fmt.Errorf("send commands to root pane: %w", err)
		}
	}

	prevPane := firstPane
	for i := 1; i < len(panes); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		pane := panes[i]
		dir := SplitHorizontal
		if pane.Split == config.SplitVertical {
			dir = SplitVertical
		}
		paneID, err := b.tx.SplitPane(prevPane, dir, pane.Size, root)
		if err != nil {
			return fmt.Errorf("split pane %d: %w", i, err)
		}
		prevPane = paneID

		if cmds := prependCmds(pre, pane.Commands); len(cmds) > 0 {
			if err := b.tx.SendKeys(paneID, cmds); err != nil {
				return fmt.Errorf("send commands to pane %d: %w", i, err)
			}
		}
	}
	return nil
}

// hostPaneOptsFor combines session-level and window-level settings; window
// overrides session. window may be nil (session-level simple mode).
func hostPaneOptsFor(session *config.Session, window *config.Window) hostPaneOpts {
	sUser, wUser := session.SSHUser, ""
	sConnect, wConnect := session.Connect, ""
	sArrange, wArrange := session.Arrange, ""
	sSync := session.Sync
	var wSync *bool
	if window != nil {
		wUser = window.SSHUser
		wConnect = window.Connect
		wArrange = window.Arrange
		wSync = window.Sync
	}
	hold := true
	if window != nil && window.Hold != nil {
		hold = *window.Hold
	} else if session.Hold != nil {
		hold = *session.Hold
	}
	retry := session.Retry
	if window != nil && window.Retry != nil {
		retry = *window.Retry
	}
	return hostPaneOpts{
		connect: pickConnect(sConnect, sUser, wConnect, wUser),
		arrange: pickString(wArrange, sArrange),
		sync:    pickBool(wSync, sSync),
		hold:    hold,
		retry:   retry,
	}
}

func pickConnect(sConnect, sUser, wConnect, wUser string) string {
	// Connect templates always win over ssh_user (window-level winning over
	// session-level), since users specifying connect: are intentional.
	if wConnect != "" {
		return wConnect
	}
	if sConnect != "" {
		return sConnect
	}
	user := wUser
	if user == "" {
		user = sUser
	}
	if user != "" {
		return "ssh " + user + "@{{host}}"
	}
	return DefaultConnectTemplate
}

func pickString(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func pickBool(window *bool, session bool) bool {
	if window != nil {
		return *window
	}
	return session
}

// resolveDir expands ~ and converts a relative path to an absolute one.
// An empty input returns "" (meaning "no -c flag").
func resolveDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand ~: %w", err)
		}
		if dir == "~" {
			dir = home
		} else {
			dir = filepath.Join(home, dir[2:])
		}
	}
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", fmt.Errorf("absolute path: %w", err)
		}
		dir = abs
	}
	return dir, nil
}

// resolveOrFallback resolves dir if non-empty; otherwise returns fallback as-is.
func resolveOrFallback(dir, fallback string) (string, error) {
	if dir == "" {
		return fallback, nil
	}
	return resolveDir(dir)
}
