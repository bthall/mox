// Package tmuxtest provides a Fake implementation of tmux.Tmux for use in tests.
package tmuxtest

import (
	"fmt"
	"sync"
	"time"

	"github.com/bthall/mox/internal/tmux"
)

// Fake records every call made to it. Tests construct it with NewFake() and
// inspect the resulting Calls slice and helper accessors.
type Fake struct {
	mu sync.Mutex

	sessions      map[string]bool
	windowsBySess map[string][]string
	panesByWindow map[string][]string

	TitlesByPane map[string]string
	KeysByPane   map[string][][]string

	// Optional per-session detail returned by ListSessionsDetailed.
	AttachedSessions map[string]bool
	ActivityBySess   map[string]time.Time

	// Failure injection
	CreateFails   bool
	SplitFailOn   string
	ConnectFailOn string

	AttachCalled string

	nextWinID  int
	nextPaneID int

	// Calls is an ordered call log for assertions.
	Calls []string
}

// NewFake creates an empty Fake.
func NewFake() *Fake {
	return &Fake{
		sessions:      map[string]bool{},
		windowsBySess: map[string][]string{},
		panesByWindow: map[string][]string{},
		TitlesByPane:  map[string]string{},
		KeysByPane:    map[string][][]string{},
	}
}

// Compile-time check.
var _ tmux.Tmux = (*Fake)(nil)

func (f *Fake) record(s string) { f.Calls = append(f.Calls, s) }

func (f *Fake) makeWinID() string  { f.nextWinID++; return fmt.Sprintf("@%d", f.nextWinID) }
func (f *Fake) makePaneID() string { f.nextPaneID++; return fmt.Sprintf("%%%d", f.nextPaneID) }

// SetSession marks a session as existing without going through CreateSession.
// Useful for arranging a "session already exists" precondition.
func (f *Fake) SetSession(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[name] = true
}

// SessionExists implements tmux.Tmux.
func (f *Fake) SessionExists(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("SessionExists " + name)
	return f.sessions[name], nil
}

// ListSessions implements tmux.Tmux.
func (f *Fake) ListSessions() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ListSessions")
	out := make([]string, 0, len(f.sessions))
	for n := range f.sessions {
		out = append(out, n)
	}
	return out, nil
}

// ListSessionsDetailed implements tmux.Tmux. Window count comes from the
// fake's recorded windows; Attached and Activity are taken from the optional
// AttachedSessions / ActivityBySess maps (zero values otherwise).
func (f *Fake) ListSessionsDetailed() ([]tmux.SessionDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ListSessionsDetailed")
	out := make([]tmux.SessionDetail, 0, len(f.sessions))
	for n := range f.sessions {
		windows := len(f.windowsBySess[n])
		if windows == 0 {
			windows = 1
		}
		out = append(out, tmux.SessionDetail{
			Name:     n,
			Windows:  windows,
			Attached: f.AttachedSessions[n],
			Activity: f.ActivityBySess[n],
		})
	}
	return out, nil
}

// CreateSession implements tmux.Tmux.
func (f *Fake) CreateSession(name, startDir, firstWindow string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("CreateSession %s dir=%q win=%q", name, startDir, firstWindow))
	if f.CreateFails {
		return fmt.Errorf("fake: CreateSession fails")
	}
	f.sessions[name] = true
	winID := f.makeWinID()
	f.windowsBySess[name] = []string{winID}
	paneID := f.makePaneID()
	f.panesByWindow[winID] = []string{paneID}
	return nil
}

// KillSession implements tmux.Tmux.
func (f *Fake) KillSession(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("KillSession " + name)
	delete(f.sessions, name)
	return nil
}

// AttachSession implements tmux.Tmux.
func (f *Fake) AttachSession(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("AttachSession " + name)
	f.AttachCalled = name
	return nil
}

// CreateWindow implements tmux.Tmux.
func (f *Fake) CreateWindow(session, name, startDir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("CreateWindow %s name=%q dir=%q", session, name, startDir))
	winID := f.makeWinID()
	f.windowsBySess[session] = append(f.windowsBySess[session], winID)
	paneID := f.makePaneID()
	f.panesByWindow[winID] = []string{paneID}
	return winID, nil
}

// RenameWindow implements tmux.Tmux.
func (f *Fake) RenameWindow(target, newName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("RenameWindow %s -> %s", target, newName))
	return nil
}

// SelectWindowByID implements tmux.Tmux.
func (f *Fake) SelectWindowByID(windowID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("SelectWindowByID " + windowID)
	return nil
}

// FirstWindowID implements tmux.Tmux.
func (f *Fake) FirstWindowID(session string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	wins := f.windowsBySess[session]
	if len(wins) == 0 {
		return "", fmt.Errorf("fake: no windows in %s", session)
	}
	return wins[0], nil
}

// FirstPaneID implements tmux.Tmux.
func (f *Fake) FirstPaneID(windowID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	panes := f.panesByWindow[windowID]
	if len(panes) == 0 {
		return "", fmt.Errorf("fake: no panes in %s", windowID)
	}
	return panes[0], nil
}

// SplitPane implements tmux.Tmux.
func (f *Fake) SplitPane(target string, dir tmux.SplitDirection, sizePercent int, startDir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dirStr := "h"
	if dir == tmux.SplitVertical {
		dirStr = "v"
	}
	f.record(fmt.Sprintf("SplitPane %s %s size=%d", target, dirStr, sizePercent))
	if target == f.SplitFailOn {
		return "", fmt.Errorf("fake: split fails on %s", target)
	}
	for winID, panes := range f.panesByWindow {
		for _, p := range panes {
			if p == target {
				newID := f.makePaneID()
				f.panesByWindow[winID] = append(f.panesByWindow[winID], newID)
				return newID, nil
			}
		}
	}
	return "", fmt.Errorf("fake: target %q not found", target)
}

// SendKeys implements tmux.Tmux.
func (f *Fake) SendKeys(target string, commands []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SendKeys %s %v", target, commands))
	if target == f.ConnectFailOn {
		return fmt.Errorf("fake: SendKeys fails on %s", target)
	}
	f.KeysByPane[target] = append(f.KeysByPane[target], commands)
	return nil
}

// SetPaneTitle implements tmux.Tmux.
func (f *Fake) SetPaneTitle(target, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SetPaneTitle %s = %q", target, title))
	f.TitlesByPane[target] = title
	return nil
}

// SetSessionOption implements tmux.Tmux.
func (f *Fake) SetSessionOption(session, option, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SetSessionOption %s %s=%s", session, option, value))
	return nil
}

// SetWindowOption implements tmux.Tmux.
func (f *Fake) SetWindowOption(windowTarget, option, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SetWindowOption %s %s=%s", windowTarget, option, value))
	return nil
}

// SelectLayout implements tmux.Tmux.
func (f *Fake) SelectLayout(windowTarget, layoutName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SelectLayout %s %s", windowTarget, layoutName))
	return nil
}

// SetHook implements tmux.Tmux.
func (f *Fake) SetHook(session, event, command string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(fmt.Sprintf("SetHook %s %s = %q", session, event, command))
	return nil
}
