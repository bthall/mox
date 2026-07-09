package config

// Config represents the entire configuration file.
type Config struct {
	Layouts  map[string]*Layout  `yaml:"layouts,omitempty"`
	Sessions map[string]*Session `yaml:"sessions"`
}

// Session represents one tmux session. A session is in "simple" mode when it
// has Hosts (one window with a pane per host) and "complex" mode when it has
// Windows. The two modes are mutually exclusive.
type Session struct {
	Root string `yaml:"root,omitempty"` // working directory for the session

	// Connect is the template used to connect to each host in simple mode.
	// "{{host}}" is substituted with the host string. Defaults to "ssh {{host}}".
	// Window-level Connect overrides session-level.
	Connect string `yaml:"connect,omitempty"`

	// SSHUser, when set and Connect is empty, prefixes each host with
	// "USER@" in the default ssh template. Ignored if Connect is set.
	SSHUser string `yaml:"ssh_user,omitempty"`

	// Sync enables synchronize-panes on each simple-mode window in this
	// session — typed input is broadcast to every pane. Defaults to false.
	Sync bool `yaml:"sync,omitempty"`

	// Arrange applies a built-in tmux layout to each simple-mode window
	// after panes are created. Valid: tiled, even-horizontal, even-vertical,
	// main-horizontal, main-vertical. Empty = no rearrangement.
	Arrange string `yaml:"arrange,omitempty"`

	// Hold controls what happens to a host pane when its connection ends
	// (failure or clean exit). When on (the default), the pane shows a
	// notice and waits for Enter before closing, instead of dropping to a
	// local shell — which in a sync window would silently receive the
	// broadcast keystrokes. Set hold: false to restore the local shell.
	Hold *bool `yaml:"hold,omitempty"`

	// Retry re-runs a failed connect command up to N extra times (3s apart)
	// before giving up. 0 (the default) disables retrying. A connection
	// that exits cleanly is never retried.
	Retry int `yaml:"retry,omitempty"`

	// OnStart commands run locally (sh -c, in order) before the session is
	// built; a non-zero exit aborts creation. Use for prerequisites like
	// bringing up a VPN. They do not run when attaching to an already
	// running session.
	OnStart []string `yaml:"on_start,omitempty"`

	// OnStop commands run locally after 'mox kill' destroys this session.
	// Failures are logged but never block the kill.
	OnStop []string `yaml:"on_stop,omitempty"`

	// Pre commands are prepended to every pane's command list in this
	// session — handy for environment setup that every pane needs.
	Pre []string `yaml:"pre,omitempty"`

	// Simple mode (mutually exclusive with Windows).
	Hosts    []string `yaml:"hosts,omitempty"`
	Commands []string `yaml:"commands,omitempty"` // sent to each host pane after connect

	// Complex mode (mutually exclusive with Hosts).
	Windows []*Window `yaml:"windows,omitempty"`
}

// Window represents a tmux window. Like Session, a window can be "simple"
// (Hosts) or "complex" (Panes/Layout); the modes are mutually exclusive.
type Window struct {
	Name string `yaml:"name"`
	Root string `yaml:"root,omitempty"`

	// Connect overrides the session-level connect template for this window.
	Connect string `yaml:"connect,omitempty"`

	// SSHUser overrides the session-level ssh_user for this window.
	SSHUser string `yaml:"ssh_user,omitempty"`

	// Sync overrides session-level synchronize-panes for this window.
	// Tri-state via *bool: nil = inherit, false = off, true = on.
	Sync *bool `yaml:"sync,omitempty"`

	// Arrange overrides session-level arrange for this window.
	Arrange string `yaml:"arrange,omitempty"`

	// Hold overrides session-level hold for this window (nil = inherit).
	Hold *bool `yaml:"hold,omitempty"`

	// Retry overrides session-level retry for this window (nil = inherit).
	Retry *int `yaml:"retry,omitempty"`

	// Pre commands are prepended to every pane's command list in this
	// window, after any session-level pre commands.
	Pre []string `yaml:"pre,omitempty"`

	// Simple mode (mutually exclusive with Panes/Layout).
	Hosts    []string `yaml:"hosts,omitempty"`
	Commands []string `yaml:"commands,omitempty"`

	// Complex mode (mutually exclusive with Hosts).
	Layout string  `yaml:"layout,omitempty"` // reference to a top-level layout
	Panes  []*Pane `yaml:"panes,omitempty"`  // inline panes
}

// Pane represents one tmux pane.
//
// The first pane in a window/layout must use SplitRoot. SplitRoot is a marker:
// it identifies the pane that already exists when the window was created (no
// split is performed). Subsequent panes use SplitHorizontal (top/bottom split,
// new pane stacked under the previous) or SplitVertical (side-by-side split).
type Pane struct {
	Split    SplitType `yaml:"split"`
	Size     int       `yaml:"size,omitempty"` // percent of parent pane (1-99); 0 = default
	Commands []string  `yaml:"commands,omitempty"`
}

// Layout is a reusable named pane configuration.
type Layout struct {
	Name  string  `yaml:"name"`
	Panes []*Pane `yaml:"panes"`
}

// SplitType defines how a pane is split.
type SplitType string

const (
	// SplitRoot marks the first pane of a window — the one that exists
	// implicitly when the window is created. No split is performed.
	SplitRoot SplitType = "root"
	// SplitHorizontal stacks the new pane under the previous (tmux's default
	// split direction).
	SplitHorizontal SplitType = "horizontal"
	// SplitVertical places the new pane to the right of the previous (tmux -h).
	SplitVertical SplitType = "vertical"
)

// IsSimple reports whether the session is in simple mode.
func (s *Session) IsSimple() bool { return len(s.Hosts) > 0 }

// IsSimple reports whether the window is in simple mode.
func (w *Window) IsSimple() bool { return len(w.Hosts) > 0 }

// HostSummary returns the de-duplicated, order-stable list of hosts this
// session connects to: its simple-mode hosts plus the hosts of any simple-mode
// windows. Complex-mode panes carry no host, so they contribute nothing. The
// result is used for at-a-glance display (e.g. `mox list`); it is empty for
// purely complex sessions.
func (s *Session) HostSummary() []string {
	seen := make(map[string]bool)
	var hosts []string
	add := func(hs []string) {
		for _, h := range hs {
			if h == "" || seen[h] {
				continue
			}
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	add(s.Hosts)
	for _, w := range s.Windows {
		add(w.Hosts)
	}
	return hosts
}
