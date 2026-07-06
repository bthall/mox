// Package history records a small, best-effort log of the sessions a user
// recently created or attached to, so `mox list` and `mox recent` can answer
// "what was I just working in?". It is the only persistent state mox keeps.
//
// Every operation is best-effort: a missing, unwritable, or corrupt history
// file is treated as an empty history and never surfaces an error that could
// block a real session operation. Callers may log returned errors at debug
// level, but should not fail on them.
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Action values recorded for an entry.
const (
	ActionCreated  = "created"
	ActionAttached = "attached"
)

// maxEntries caps how many distinct sessions the history retains. Older
// entries beyond this count are dropped on write.
const maxEntries = 50

// Entry is one recorded interaction with a session.
type Entry struct {
	Name   string    `json:"name"`
	Action string    `json:"action"`
	Time   time.Time `json:"time"`
}

// store is the testable core: it reads and writes a specific file and uses an
// injectable clock. The package-level Record/Load wrap a store bound to the
// real default path and real clock.
type store struct {
	path string
	now  func() time.Time
}

// defaultStore returns a store at $XDG_STATE_HOME/mox/recent.json (falling
// back to ~/.local/state/mox/recent.json). path is empty when neither
// $XDG_STATE_HOME nor $HOME can be resolved, which makes Record/Load no-ops.
func defaultStore() *store {
	return &store{path: defaultPath(), now: time.Now}
}

func defaultPath() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "mox", "recent.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "state", "mox", "recent.json")
}

// Record notes that the named session was just created or attached to.
// Best-effort: see the package docs.
func Record(name, action string) error { return defaultStore().record(name, action) }

// Load returns the recorded entries, newest first. Best-effort: a missing or
// corrupt file yields an empty slice and a nil error.
func Load() ([]Entry, error) { return defaultStore().load() }

func (s *store) record(name, action string) error {
	if s.path == "" || name == "" {
		return nil
	}
	entries, _ := s.load() // corrupt/missing history is treated as empty

	// Drop any existing entry for this name; the newest action wins.
	filtered := entries[:0]
	for _, e := range entries {
		if e.Name != name {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, Entry{Name: name, Action: action, Time: s.now()})

	// Keep newest-first, then cap.
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Time.After(filtered[j].Time)
	})
	if len(filtered) > maxEntries {
		filtered = filtered[:maxEntries]
	}

	return s.write(filtered)
}

func (s *store) load() ([]Entry, error) {
	if s.path == "" {
		return []Entry{}, nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		// Missing file (or any read error) means "no history yet".
		return []Entry{}, nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Corrupt file: start fresh rather than erroring.
		return []Entry{}, nil
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Time.After(entries[j].Time)
	})
	return entries, nil
}

func (s *store) write(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
