package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Client wraps tmux command execution. In dry-run mode it prints each tmux
// invocation instead of executing it, fabricating just enough output
// (window/pane ids, existence checks) for session building to proceed.
type Client struct {
	executable string

	dryOut   io.Writer // non-nil enables dry-run
	nextWin  int
	nextPane int
}

// NewClient locates tmux on PATH and returns a Client.
func NewClient() (*Client, error) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux not found in PATH: %w", err)
	}
	return &Client{executable: path}, nil
}

// NewDryRun returns a Client that prints every tmux command to out instead
// of executing it. tmux does not need to be installed.
func NewDryRun(out io.Writer) *Client {
	return &Client{executable: "tmux", dryOut: out}
}

// dryRun prints the command and fabricates the minimal responses the
// builder needs to keep going: fresh window/pane ids for the -P prints,
// "does not exist" for session checks, and empty output otherwise.
func (c *Client) dryRun(args []string) (string, error) {
	fmt.Fprintln(c.dryOut, "tmux "+shellJoin(args))
	switch args[0] {
	case "has-session":
		return "", &Error{ExitCode: 1, Stderr: "can't find session (dry-run)"}
	case "list-sessions":
		return "", &Error{ExitCode: 1, Stderr: "no server running (dry-run)"}
	case "new-window", "split-window":
		for _, a := range args {
			if a == "-P" {
				if args[0] == "new-window" {
					c.nextWin++
					return fmt.Sprintf("@%d", c.nextWin), nil
				}
				c.nextPane++
				return fmt.Sprintf("%%%d", c.nextPane), nil
			}
		}
		return "", nil
	case "new-session":
		c.nextWin++
		return "", nil
	case "list-windows":
		return fmt.Sprintf("@%d", max(c.nextWin, 1)), nil
	case "list-panes":
		c.nextPane++
		return fmt.Sprintf("%%%d", c.nextPane), nil
	case "show-options":
		return "0", nil
	default:
		return "", nil
	}
}

// shellJoin renders an argv as a copy-pasteable shell line, quoting any
// argument that needs it.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'$&|;<>(){}*?#~") {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}

// Error is returned by Run when tmux exits non-zero.
// It captures both the exit code and tmux's stderr text so callers can match
// on either programmatically (via errors.As) or with the original message.
type Error struct {
	ExitCode int
	Stderr   string
	Err      error
}

func (e *Error) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("tmux: %s", e.Stderr)
	}
	if e.Err != nil {
		return fmt.Sprintf("tmux: %s", e.Err.Error())
	}
	return fmt.Sprintf("tmux: exit code %d", e.ExitCode)
}

func (e *Error) Unwrap() error { return e.Err }

// IsExitCode reports whether err is a tmux *Error with the given exit code.
func IsExitCode(err error, code int) bool {
	var tErr *Error
	if errors.As(err, &tErr) {
		return tErr.ExitCode == code
	}
	return false
}

// Run executes a tmux command and returns its stdout (trimmed).
// On non-zero exit it returns an *Error capturing the exit code and stderr.
func (c *Client) Run(args ...string) (string, error) {
	return c.RunContext(context.Background(), args...)
}

// RunContext is Run with cancellation support.
func (c *Client) RunContext(ctx context.Context, args ...string) (string, error) {
	if c.dryOut != nil {
		return c.dryRun(args)
	}
	cmd := exec.CommandContext(ctx, c.executable, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &Error{
				ExitCode: exitErr.ExitCode(),
				Stderr:   strings.TrimSpace(stderr.String()),
				Err:      err,
			}
		}
		return "", &Error{ExitCode: -1, Stderr: strings.TrimSpace(stderr.String()), Err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// RunInteractive executes a tmux command with stdio attached. Used for
// attach-session and switch-client which take over the terminal.
func (c *Client) RunInteractive(args ...string) error {
	if c.dryOut != nil {
		_, err := c.dryRun(args)
		return err
	}
	cmd := exec.Command(c.executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Version returns the tmux version banner (`tmux 3.4` etc).
func (c *Client) Version() (string, error) {
	return c.Run("-V")
}
