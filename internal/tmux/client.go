package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Client wraps tmux command execution.
type Client struct {
	executable string
}

// NewClient locates tmux on PATH and returns a Client.
func NewClient() (*Client, error) {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return nil, fmt.Errorf("tmux not found in PATH: %w", err)
	}
	return &Client{executable: path}, nil
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
