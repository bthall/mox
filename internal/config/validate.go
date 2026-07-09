package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// reservedNameChars are characters that are special in tmux's target syntax
// (`session:window.pane`) or that produce surprising shell behavior. Names
// containing any of these are rejected at validation time.
const reservedNameChars = ":.$ \t\r\n"

// hostRe matches strings safe to interpolate into the default `ssh {{host}}`
// command without shell escaping. Allows letters, digits, dot, underscore,
// hyphen, '%' (IPv6 zone), '@' (user@host), and ':' is intentionally excluded
// because we use it as the tmux target separator. Users with more exotic
// hostnames can override the connect template, but the default must be safe.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9._%@-]+$`)

// sshUserRe matches characters allowed in a Unix username we'll interpolate
// into the default `ssh USER@host` template.
var sshUserRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validArrangeLayouts is the set of tmux built-in layout names accepted by
// the `arrange:` field.
var validArrangeLayouts = map[string]bool{
	"tiled":           true,
	"even-horizontal": true,
	"even-vertical":   true,
	"main-horizontal": true,
	"main-vertical":   true,
}

// Validate validates the entire configuration.
func (c *Config) Validate() error {
	if len(c.Sessions) == 0 {
		return errors.New("no sessions defined in configuration")
	}

	for name, session := range c.Sessions {
		if err := session.Validate(name); err != nil {
			return fmt.Errorf("session %q: %w", name, err)
		}
	}

	for name, layout := range c.Layouts {
		if err := layout.Validate(name); err != nil {
			return fmt.Errorf("layout %q: %w", name, err)
		}
	}

	// Validate cross-references from windows to layouts.
	for sessionName, session := range c.Sessions {
		for _, window := range session.Windows {
			if window.Layout == "" {
				continue
			}
			if _, ok := c.Layouts[window.Layout]; !ok {
				if len(c.Layouts) == 0 {
					return fmt.Errorf("session %q, window %q: layout %q not found (no layouts: section in config)",
						sessionName, window.Name, window.Layout)
				}
				return fmt.Errorf("session %q, window %q: layout %q not found",
					sessionName, window.Name, window.Layout)
			}
		}
	}

	return nil
}

// Validate validates a session configuration. The name argument is the map
// key from Config.Sessions and is checked for tmux-incompatible characters.
func (s *Session) Validate(name string) error {
	if name == "" {
		return errors.New("session name is required")
	}
	if i := strings.IndexAny(name, reservedNameChars); i >= 0 {
		return fmt.Errorf("session name contains reserved character %q (forbidden: %q)",
			name[i:i+1], reservedNameChars)
	}

	if err := validateConnectAndUser(s.Connect, s.SSHUser); err != nil {
		return err
	}
	if err := validateArrange(s.Arrange); err != nil {
		return err
	}
	if err := validateRetry(s.Retry); err != nil {
		return err
	}

	hasHosts := len(s.Hosts) > 0
	hasWindows := len(s.Windows) > 0

	// hosts vs windows are mutually exclusive. Defining neither is allowed:
	// the session opens a single local pane (with optional commands).
	if hasHosts && hasWindows {
		return errors.New("session cannot define both 'hosts' and 'windows'")
	}

	if hasHosts {
		if err := validateHosts(s.Hosts); err != nil {
			return err
		}
	}

	if hasWindows {
		if len(s.Commands) > 0 {
			return errors.New("session-level 'commands' has no effect in complex mode; move it to a window or pane")
		}
		// Duplicate window names are intentionally allowed — tmux addresses
		// windows by id, and users commonly have several windows with the
		// same name (e.g. multiple "claude" windows).
		for i, window := range s.Windows {
			if err := window.Validate(); err != nil {
				return fmt.Errorf("window %d (%q): %w", i, window.Name, err)
			}
		}
	}

	return nil
}

// validateRetry bounds the connect retry count: negative makes no sense and
// large values just hide a host that is actually down.
func validateRetry(n int) error {
	if n < 0 || n > 10 {
		return fmt.Errorf("retry must be between 0 and 10, got %d", n)
	}
	return nil
}

// Validate validates a window configuration.
func (w *Window) Validate() error {
	if w.Name == "" {
		return errors.New("window name is required")
	}
	if i := strings.IndexAny(w.Name, reservedNameChars); i >= 0 {
		return fmt.Errorf("window name %q contains reserved character %q (forbidden: %q)",
			w.Name, w.Name[i:i+1], reservedNameChars)
	}
	if err := validateConnectAndUser(w.Connect, w.SSHUser); err != nil {
		return err
	}
	if err := validateArrange(w.Arrange); err != nil {
		return err
	}
	if w.Retry != nil {
		if err := validateRetry(*w.Retry); err != nil {
			return err
		}
	}

	hasHosts := len(w.Hosts) > 0
	hasPanes := len(w.Panes) > 0
	hasLayout := w.Layout != ""

	switch {
	case hasHosts && (hasPanes || hasLayout):
		return errors.New("window cannot combine 'hosts' with 'panes' or 'layout'")
	case hasPanes && hasLayout:
		return errors.New("window cannot have both 'panes' and 'layout'")
	case !hasHosts && !hasPanes && !hasLayout:
		return errors.New("window must define one of 'hosts', 'panes', or 'layout'")
	}

	if hasHosts {
		if err := validateHosts(w.Hosts); err != nil {
			return err
		}
	}

	if hasPanes {
		if w.Panes[0].Split != SplitRoot {
			return errors.New("first pane must have split: root")
		}
		for i, pane := range w.Panes {
			if err := pane.Validate(i == 0); err != nil {
				return fmt.Errorf("pane %d: %w", i, err)
			}
		}
	}

	return nil
}

// Validate validates a pane. isFirst indicates whether the pane is the root
// pane of its window/layout.
func (p *Pane) Validate(isFirst bool) error {
	switch p.Split {
	case SplitRoot:
		if !isFirst {
			return errors.New("split: root is only valid for the first pane of a window")
		}
		if p.Size != 0 {
			return errors.New("size has no effect on the root pane")
		}
	case SplitHorizontal, SplitVertical:
		if isFirst {
			return errors.New("first pane must have split: root, not " + string(p.Split))
		}
	default:
		return fmt.Errorf("invalid split %q (must be one of: root, horizontal, vertical)", p.Split)
	}

	if p.Size < 0 || p.Size > 99 {
		return fmt.Errorf("size must be between 1 and 99 (percent), got %d", p.Size)
	}

	return nil
}

// Validate validates a layout.
func (l *Layout) Validate(name string) error {
	if name == "" {
		return errors.New("layout name is required")
	}
	if len(l.Panes) == 0 {
		return errors.New("layout must define at least one pane")
	}
	if l.Panes[0].Split != SplitRoot {
		return errors.New("first pane must have split: root")
	}
	for i, pane := range l.Panes {
		if err := pane.Validate(i == 0); err != nil {
			return fmt.Errorf("pane %d: %w", i, err)
		}
	}
	return nil
}

func validateHosts(hosts []string) error {
	for i, h := range hosts {
		if h == "" {
			return fmt.Errorf("hosts[%d] is empty", i)
		}
		if !hostRe.MatchString(h) {
			return fmt.Errorf("hosts[%d] = %q: hostname contains characters that are unsafe to pass to the default ssh command (override 'connect:' if you need them)", i, h)
		}
	}
	return nil
}

func validateConnectAndUser(connect, user string) error {
	if user == "" {
		return nil
	}
	if connect != "" {
		return errors.New("'ssh_user' is ignored when 'connect:' is set; specify the user inside the connect template instead")
	}
	if !sshUserRe.MatchString(user) {
		return fmt.Errorf("ssh_user %q contains characters that are unsafe to pass to the default ssh command", user)
	}
	return nil
}

func validateArrange(arrange string) error {
	if arrange == "" {
		return nil
	}
	if !validArrangeLayouts[arrange] {
		valid := []string{"tiled", "even-horizontal", "even-vertical", "main-horizontal", "main-vertical"}
		return fmt.Errorf("invalid arrange %q (must be one of: %s)", arrange, strings.Join(valid, ", "))
	}
	return nil
}
