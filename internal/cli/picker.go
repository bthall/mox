package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bthall/mox/internal/config"
	"github.com/bthall/mox/internal/history"
	"github.com/bthall/mox/internal/session"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// runPicker is what bare `mox` does on a terminal: show every session you
// could be in — running, configured, recent — and attach to the one you pick.
// Empty input cancels quietly.
func runPicker(cmd *cobra.Command) error {
	opts := optsFromContext(cmd.Context())
	logger := loggerFromContext(cmd.Context())
	out := cmd.OutOrStdout()

	for {
		cfg, _ := tryLoadConfig(opts.configPath)
		if cfg == nil {
			cfg = &config.Config{Sessions: map[string]*config.Session{}}
		}
		mgr, err := session.NewManager(cfg, logger)
		if err != nil {
			return err
		}
		infos, err := mgr.List()
		if err != nil {
			return err
		}
		recent, err := history.Load()
		if err != nil {
			logger.Debug("load session history failed", "error", err)
		}

		candidates := orderPickerCandidates(infos, recent)
		if len(candidates) == 0 {
			fmt.Fprintln(out, "No sessions configured or running.")
			fmt.Fprintln(out, "Try 'mox init' to create a config, or 'mox new' for an ad-hoc session.")
			return nil
		}

		// Interactive fuzzy picker when the terminal supports it; numbered
		// prompt as the fallback.
		if stdin, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(stdin) {
			if name, edit, ran := runFuzzyPicker(candidates, cfg.Sessions); ran {
				if name == "" {
					return nil // canceled
				}
				if edit {
					path, local := config.EffectivePath(opts.configPath)
					if local {
						fmt.Fprintf(os.Stderr, "mox: using ./%s\n", config.LocalConfigName)
					}
					st, err := loadEditorState(path)
					if err != nil {
						return err
					}
					if err := runEditorTUI(cmd, st, name); err != nil {
						return err
					}
					continue // fresh picker over the (possibly changed) config
				}
				return mgr.CreateOrAttach(cmd.Context(), name, false)
			}
		}

		renderPicker(out, candidates, time.Now())
		fmt.Fprint(out, "\nAttach to (number or name, empty cancels): ")

		line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(out)
			return nil // EOF (Ctrl-D) cancels like empty input
		}
		choice := strings.TrimSpace(line)
		if choice == "" {
			return nil
		}
		name, err := resolvePickerChoice(choice, candidates)
		if err != nil {
			return err
		}
		return mgr.CreateOrAttach(cmd.Context(), name, false)
	}
}

// orderPickerCandidates sorts sessions the way you reach for them: running
// first (most recent activity first), then stopped configured sessions with
// recently-used ones ahead, alphabetical as the tiebreak.
func orderPickerCandidates(infos []session.SessionInfo, recent []history.Entry) []session.SessionInfo {
	lastUsed := make(map[string]time.Time, len(recent))
	for _, e := range recent {
		if _, ok := lastUsed[e.Name]; !ok {
			lastUsed[e.Name] = e.Time
		}
	}
	slices.SortFunc(infos, func(a, b session.SessionInfo) int {
		if a.Running != b.Running {
			if a.Running {
				return -1
			}
			return 1
		}
		if a.Running {
			if c := b.LastActivity.Compare(a.LastActivity); c != 0 {
				return c
			}
		} else {
			if c := lastUsed[b.Name].Compare(lastUsed[a.Name]); c != 0 {
				return c
			}
		}
		return strings.Compare(a.Name, b.Name)
	})
	return infos
}

// renderPicker prints the numbered candidate table.
func renderPicker(out io.Writer, candidates []session.SessionInfo, now time.Time) {
	rows := make([][]string, 0, len(candidates))
	for i, s := range candidates {
		rows = append(rows, []string{
			fmt.Sprintf("%d.", i+1),
			s.Name,
			stateCell(out, s),
			relativeTime(now, s.LastActivity),
			hostsCell(s),
		})
	}
	renderTable(out, rows)
}

// resolvePickerChoice maps user input to a candidate name: a 1-based number,
// an exact name, or a unique name prefix.
func resolvePickerChoice(input string, candidates []session.SessionInfo) (string, error) {
	if n, err := strconv.Atoi(input); err == nil {
		if n < 1 || n > len(candidates) {
			return "", fmt.Errorf("no session numbered %d (1-%d)", n, len(candidates))
		}
		return candidates[n-1].Name, nil
	}
	var prefixMatches []string
	for _, c := range candidates {
		if c.Name == input {
			return c.Name, nil
		}
		if strings.HasPrefix(c.Name, input) {
			prefixMatches = append(prefixMatches, c.Name)
		}
	}
	switch len(prefixMatches) {
	case 1:
		return prefixMatches[0], nil
	case 0:
		return "", fmt.Errorf("no session matches %q", input)
	default:
		return "", fmt.Errorf("%q is ambiguous: %s", input, strings.Join(prefixMatches, ", "))
	}
}

// isTerminal reports whether f is attached to a terminal. The termios probe
// (rather than a char-device mode check) matters: /dev/null is a character
// device but can't host an interactive UI.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
