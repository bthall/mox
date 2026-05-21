package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/tmux"
)

// Manager orchestrates session lifecycle on top of a Tmux implementation.
type Manager struct {
	tx      tmux.Tmux
	config  *config.Config
	builder *tmux.Builder
	log     *slog.Logger
}

// NewManager wires a Manager backed by a real *tmux.Client.
// Returns an error if tmux is not on PATH.
func NewManager(cfg *config.Config, logger *slog.Logger) (*Manager, error) {
	client, err := tmux.NewClient()
	if err != nil {
		return nil, err
	}
	return NewManagerWith(cfg, client, logger), nil
}

// NewManagerWith builds a Manager backed by the given Tmux. Used by tests.
func NewManagerWith(cfg *config.Config, tx tmux.Tmux, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		tx:      tx,
		config:  cfg,
		builder: tmux.NewBuilder(tx, cfg, logger),
		log:     logger,
	}
}

// CreateOrAttach attaches to an existing tmux session, or builds one from
// the configuration and attaches. If the session name is not in the config,
// the caller can still attach to it as long as it already exists in tmux —
// this lets `mox -a foo` work for hand-rolled tmux sessions too.
//
// On context cancellation during build the partial session is killed.
func (m *Manager) CreateOrAttach(ctx context.Context, name string, force bool) error {
	exists, err := m.tx.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}

	session, configured := m.config.GetSession(name)

	if !configured {
		// Not in config: only valid if a tmux session by this name exists.
		// We can't "create" it because we have no spec to build from.
		if !exists {
			return fmt.Errorf("session %q not found in configuration and not running in tmux", name)
		}
		if force {
			return fmt.Errorf("cannot --force an unmanaged session (%q is not in the config); use 'mox kill' first if you want to recreate it manually", name)
		}
		if err := m.tx.AttachSession(name); err != nil {
			return fmt.Errorf("attach session: %w", err)
		}
		return nil
	}

	// Configured session.
	if force && exists {
		if err := m.tx.KillSession(name); err != nil {
			return fmt.Errorf("kill existing session: %w", err)
		}
		exists = false
	}

	if !exists {
		if err := m.builder.BuildSession(ctx, name, session); err != nil {
			// Best-effort cleanup of the partial session.
			if killErr := m.tx.KillSession(name); killErr != nil {
				m.log.Warn("cleanup of partial session failed", "session", name, "error", killErr)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return fmt.Errorf("build session: %w", err)
		}
	}

	if err := m.tx.AttachSession(name); err != nil {
		return fmt.Errorf("attach session: %w", err)
	}
	return nil
}

// Create creates a new session without attaching.
func (m *Manager) Create(ctx context.Context, name string, force bool) error {
	session, ok := m.config.GetSession(name)
	if !ok {
		return fmt.Errorf("session %q not found in configuration", name)
	}

	exists, err := m.tx.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if exists {
		if !force {
			return fmt.Errorf("session %q already exists (use --force to recreate)", name)
		}
		if err := m.tx.KillSession(name); err != nil {
			return fmt.Errorf("kill existing session: %w", err)
		}
	}

	if err := m.builder.BuildSession(ctx, name, session); err != nil {
		if killErr := m.tx.KillSession(name); killErr != nil {
			m.log.Warn("cleanup of partial session failed", "session", name, "error", killErr)
		}
		return fmt.Errorf("build session: %w", err)
	}
	return nil
}

// Kill destroys a session.
func (m *Manager) Kill(name string) error {
	exists, err := m.tx.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %q does not exist", name)
	}
	if err := m.tx.KillSession(name); err != nil {
		return fmt.Errorf("kill session: %w", err)
	}
	return nil
}

// SessionInfo describes a session and whether it is currently running.
// Managed is true if the name appears in the mox config; false if the
// session exists in tmux but has no config entry.
type SessionInfo struct {
	Name    string
	Running bool
	Managed bool
}

// List returns information about all configured sessions plus any running
// tmux sessions that are not in the config.
func (m *Manager) List() ([]SessionInfo, error) {
	running, err := m.tx.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	runningSet := make(map[string]bool, len(running))
	for _, n := range running {
		runningSet[n] = true
	}

	infos := make([]SessionInfo, 0, len(m.config.Sessions)+len(running))
	for name := range m.config.Sessions {
		infos = append(infos, SessionInfo{Name: name, Running: runningSet[name], Managed: true})
	}
	for _, name := range running {
		if _, configured := m.config.Sessions[name]; !configured {
			infos = append(infos, SessionInfo{Name: name, Running: true, Managed: false})
		}
	}
	return infos, nil
}

// CreateAdHoc creates a session from an in-memory Session definition (i.e.
// not loaded from the config file). The Session is validated with the same
// rules as configured sessions. If temporary is true, the session is set to
// destroy itself when the last client detaches.
func (m *Manager) CreateAdHoc(ctx context.Context, name string, session *config.Session, opts AdHocOptions) error {
	if err := session.Validate(name); err != nil {
		return fmt.Errorf("invalid ad-hoc session: %w", err)
	}

	exists, err := m.tx.SessionExists(name)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if exists {
		if !opts.Force {
			return fmt.Errorf("session %q already exists (use --force to recreate)", name)
		}
		if err := m.tx.KillSession(name); err != nil {
			return fmt.Errorf("kill existing session: %w", err)
		}
	}

	if err := m.builder.BuildSession(ctx, name, session); err != nil {
		if killErr := m.tx.KillSession(name); killErr != nil {
			m.log.Warn("cleanup of partial session failed", "session", name, "error", killErr)
		}
		return fmt.Errorf("build session: %w", err)
	}

	if opts.Temporary {
		// Defer destroy-unattached until after the first attach. If we set
		// it now, tmux reaps the freshly-built (still client-less) session
		// before we can attach to it, and AttachSession fails.
		hookCmd := fmt.Sprintf("set-option -t %s destroy-unattached on", name)
		if err := m.tx.SetHook(name, "client-attached", hookCmd); err != nil {
			m.log.Warn("set destroy-unattached hook failed", "session", name, "error", err)
		}
	}

	if !opts.Detach {
		if err := m.tx.AttachSession(name); err != nil {
			return fmt.Errorf("attach session: %w", err)
		}
	}
	return nil
}

// AdHocOptions tunes CreateAdHoc.
type AdHocOptions struct {
	Force     bool // destroy any existing session with the same name first
	Detach    bool // create without attaching
	Temporary bool // set destroy-unattached on the session
}

// CreateAdHocWindow creates a new window inside the named parent session
// (typically the current $TMUX session) and fills it with one pane per host.
// It does not change session focus — the caller is presumed to be inside
// the same tmux client and will see the new window appear.
func (m *Manager) CreateAdHocWindow(ctx context.Context, parentSession, windowName string, sess *config.Session) error {
	if err := sess.Validate(windowName); err != nil {
		return fmt.Errorf("invalid ad-hoc window: %w", err)
	}
	parentExists, err := m.tx.SessionExists(parentSession)
	if err != nil {
		return fmt.Errorf("check parent session: %w", err)
	}
	if !parentExists {
		return fmt.Errorf("parent session %q not found", parentSession)
	}
	if _, err := m.builder.BuildAdHocWindow(ctx, parentSession, windowName, sess); err != nil {
		return fmt.Errorf("build window: %w", err)
	}
	return nil
}
