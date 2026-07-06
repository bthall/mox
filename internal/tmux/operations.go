package tmux

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// SessionExists reports whether a tmux session with the given name exists.
//
// `tmux has-session` exits 1 when the session is missing OR when no tmux
// server is running. Both cases mean "session does not exist" from the
// caller's perspective, so we map them to (false, nil).
func (c *Client) SessionExists(name string) (bool, error) {
	_, err := c.Run("has-session", "-t", "="+name)
	if err == nil {
		return true, nil
	}
	var tErr *Error
	if errors.As(err, &tErr) && tErr.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

// ListSessions returns all active session names. Returns an empty slice
// (not an error) when no tmux server is running.
func (c *Client) ListSessions() ([]string, error) {
	output, err := c.Run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		var tErr *Error
		if errors.As(err, &tErr) && tErr.ExitCode == 1 {
			return []string{}, nil
		}
		return nil, err
	}
	if output == "" {
		return []string{}, nil
	}
	return strings.Split(output, "\n"), nil
}

// SessionDetail carries the per-session facts `mox list` displays. It is
// populated from a single `tmux list-sessions` call.
type SessionDetail struct {
	Name     string
	Windows  int
	Attached bool
	Activity time.Time
}

// detailFormat is the -F format string for ListSessionsDetailed. Fields are
// joined by the ASCII unit separator (\x1f), which cannot appear in a session
// name, so splitting is unambiguous.
const detailFormat = "#{session_name}\x1f#{session_windows}\x1f#{session_attached}\x1f#{session_activity}"

// ListSessionsDetailed returns per-session detail for every active session.
// Like ListSessions, it returns an empty slice (not an error) when no tmux
// server is running.
func (c *Client) ListSessionsDetailed() ([]SessionDetail, error) {
	output, err := c.Run("list-sessions", "-F", detailFormat)
	if err != nil {
		var tErr *Error
		if errors.As(err, &tErr) && tErr.ExitCode == 1 {
			return []SessionDetail{}, nil
		}
		return nil, err
	}
	return parseSessionDetails(output), nil
}

// parseSessionDetails parses the newline-delimited output of a list-sessions
// call formatted with detailFormat. Malformed numeric fields default to zero
// values rather than failing the whole listing.
func parseSessionDetails(output string) []SessionDetail {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return []SessionDetail{}
	}
	lines := strings.Split(output, "\n")
	details := make([]SessionDetail, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(line, "\x1f")
		if len(fields) < 4 {
			continue
		}
		windows, _ := strconv.Atoi(fields[1])
		activitySecs, _ := strconv.ParseInt(fields[3], 10, 64)
		details = append(details, SessionDetail{
			Name:     fields[0],
			Windows:  windows,
			Attached: fields[2] == "1",
			Activity: time.Unix(activitySecs, 0),
		})
	}
	return details
}

// CreateSession creates a new detached session with an optional starting
// directory and an optional first-window name.
func (c *Client) CreateSession(name, startDir, firstWindow string) error {
	args := []string{"new-session", "-d", "-s", name}
	if firstWindow != "" {
		args = append(args, "-n", firstWindow)
	}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	_, err := c.Run(args...)
	return err
}

// KillSession destroys a session.
func (c *Client) KillSession(name string) error {
	_, err := c.Run("kill-session", "-t", "="+name)
	return err
}

// AttachSession attaches to a session. If invoked from inside an existing
// tmux client (TMUX env set), it uses switch-client instead of attach-session
// — attach-session refuses to nest by default.
func (c *Client) AttachSession(name string) error {
	if os.Getenv("TMUX") != "" {
		return c.RunInteractive("switch-client", "-t", "="+name)
	}
	return c.RunInteractive("attach-session", "-t", "="+name)
}

// CreateWindow creates a new window in a session and returns the new window's id.
func (c *Client) CreateWindow(session, name, startDir string) (string, error) {
	args := []string{"new-window", "-t", session + ":", "-P", "-F", "#{window_id}", "-n", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	out, err := c.Run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SplitDirection identifies how a pane is split. Horizontal stacks panes
// top-and-bottom (tmux's default split). Vertical places them side-by-side
// (tmux's `-h`).
type SplitDirection int

const (
	SplitHorizontal SplitDirection = iota // top/bottom
	SplitVertical                         // side-by-side
)

// SplitPane splits the target pane and returns the new pane's id (`%N`).
// size is interpreted as a percentage of the parent pane (0 means default).
// The `-l N%` form is used, which is preferred since tmux 3.1.
func (c *Client) SplitPane(target string, dir SplitDirection, sizePercent int, startDir string) (string, error) {
	args := []string{"split-window", "-t", target, "-P", "-F", "#{pane_id}"}
	if dir == SplitVertical {
		args = append(args, "-h")
	}
	if sizePercent > 0 {
		args = append(args, "-l", fmt.Sprintf("%d%%", sizePercent))
	}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	out, err := c.Run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// SendKeys sends each command as a literal string followed by Enter (C-m).
// Each command is a separate keystroke event, so multi-line commands should
// be passed as a single string with newlines expressed via & or shell escapes
// — tmux send-keys does not interpret backslash sequences specially.
func (c *Client) SendKeys(target string, commands []string) error {
	for _, cmd := range commands {
		if _, err := c.Run("send-keys", "-t", target, cmd, "C-m"); err != nil {
			return fmt.Errorf("send-keys to %s: %w", target, err)
		}
	}
	return nil
}

// RenameWindow renames a window.
func (c *Client) RenameWindow(target, newName string) error {
	_, err := c.Run("rename-window", "-t", target, newName)
	return err
}

// SelectWindowByID activates a window by its tmux window id (`@N`).
func (c *Client) SelectWindowByID(windowID string) error {
	_, err := c.Run("select-window", "-t", windowID)
	return err
}

// BaseIndex returns the global base-index option (default 0).
func (c *Client) BaseIndex() (int, error) {
	out, err := c.Run("show-options", "-gv", "base-index")
	if err != nil {
		return 0, nil
	}
	idx, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, nil
	}
	return idx, nil
}

// SetPaneTitle sets the title of the target pane (visible in tmux's
// pane-border-format if configured to show #{pane_title}).
func (c *Client) SetPaneTitle(target, title string) error {
	_, err := c.Run("select-pane", "-t", target, "-T", title)
	return err
}

// SetSessionOption sets a tmux session-scoped option (set-option -t SESSION).
// Used e.g. to set destroy-unattached on for ephemeral sessions.
//
// Note: tmux's set-option command rejects the `=NAME` exact-match prefix
// that most other commands accept, so we pass the raw name here.
func (c *Client) SetSessionOption(session, option, value string) error {
	_, err := c.Run("set-option", "-t", session, option, value)
	return err
}

// SetHook registers a tmux hook on the named session. When `event` fires
// (e.g. "client-attached"), tmux runs `command` in that session's context.
// Used to defer destroy-unattached until after first attach so the session
// isn't reaped before the user reaches it.
func (c *Client) SetHook(session, event, command string) error {
	_, err := c.Run("set-hook", "-t", session, event, command)
	return err
}

// SetWindowOption sets a tmux window-scoped option (set-window-option).
// Used to enable synchronize-panes etc.
func (c *Client) SetWindowOption(windowTarget, option, value string) error {
	_, err := c.Run("set-window-option", "-t", windowTarget, option, value)
	return err
}

// SelectLayout applies one of tmux's built-in pane layouts (tiled,
// even-horizontal, even-vertical, main-horizontal, main-vertical) to the
// target window.
func (c *Client) SelectLayout(windowTarget, layoutName string) error {
	_, err := c.Run("select-layout", "-t", windowTarget, layoutName)
	return err
}

// NewWindowInSession creates a new window in the named session and returns
// the new window's id. The session must already exist. Unlike CreateWindow,
// the new window is selected (active) immediately.
func (c *Client) NewWindowInSession(session, name, startDir string) (string, error) {
	args := []string{"new-window", "-t", "=" + session, "-P", "-F", "#{window_id}", "-n", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	out, err := c.Run(args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// WindowInfo describes a single window for inspection / import.
type WindowInfo struct {
	ID     string // tmux window id (@N)
	Name   string
	Layout string // raw tmux layout string
}

// ListWindowsForSession returns the windows of the named session in display
// order. Used by `mox import` to capture an existing tmux session.
func (c *Client) ListWindowsForSession(session string) ([]WindowInfo, error) {
	out, err := c.Run("list-windows", "-t", "="+session, "-F", "#{window_id}\t#{window_name}\t#{window_layout}")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var wins []WindowInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		w := WindowInfo{ID: parts[0], Name: parts[1]}
		if len(parts) == 3 {
			w.Layout = parts[2]
		}
		wins = append(wins, w)
	}
	return wins, nil
}

// PaneInfo describes a single pane for inspection / import.
type PaneInfo struct {
	ID             string // tmux pane id (%N)
	CurrentPath    string // pane_current_path
	CurrentCommand string // pane_current_command (best-effort label)
}

// ListPanesForWindow returns the panes in the given window in display order.
func (c *Client) ListPanesForWindow(windowID string) ([]PaneInfo, error) {
	out, err := c.Run("list-panes", "-t", windowID, "-F", "#{pane_id}\t#{pane_current_path}\t#{pane_current_command}")
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var panes []PaneInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 1 {
			continue
		}
		p := PaneInfo{ID: parts[0]}
		if len(parts) >= 2 {
			p.CurrentPath = parts[1]
		}
		if len(parts) >= 3 {
			p.CurrentCommand = parts[2]
		}
		panes = append(panes, p)
	}
	return panes, nil
}

// CurrentSession returns the name of the tmux session the caller is inside,
// determined by querying the running server (TMUX env var must be set).
// Returns an error if not inside tmux.
func (c *Client) CurrentSession() (string, error) {
	if os.Getenv("TMUX") == "" {
		return "", fmt.Errorf("not inside a tmux session (TMUX env not set)")
	}
	out, err := c.Run("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// FirstWindowID returns the id (`@N`) of the first window in the named session.
// Used after CreateSession to obtain the window we just created without
// relying on base-index arithmetic.
func (c *Client) FirstWindowID(session string) (string, error) {
	out, err := c.Run("list-windows", "-t", "="+session, "-F", "#{window_id}")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no windows in session %q", session)
	}
	return lines[0], nil
}

// FirstPaneID returns the id (`%N`) of the first pane in the given window id.
func (c *Client) FirstPaneID(windowID string) (string, error) {
	out, err := c.Run("list-panes", "-t", windowID, "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no panes in window %q", windowID)
	}
	return lines[0], nil
}
