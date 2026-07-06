package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(t *testing.T) pickerModel {
	t.Helper()
	m := newPickerModel(orderPickerCandidates(pickerFixtures(), nil), time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC))
	// Simulate the size message Bubble Tea sends on startup.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	return next.(pickerModel)
}

// send runs a sequence of key messages through the model, executing any
// commands each update returns (as the Bubble Tea runtime would) so that
// asynchronous behavior like filter application actually happens.
func send(t *testing.T, m pickerModel, keys ...tea.KeyMsg) pickerModel {
	t.Helper()
	for _, k := range keys {
		m = drain(t, m, k)
	}
	return m
}

func drain(t *testing.T, m pickerModel, msg tea.Msg) pickerModel {
	t.Helper()
	return drainDepth(t, m, msg, 0)
}

// drainDepth executes returned commands like the runtime would, but skips
// cursor-blink messages (they schedule each other forever) and refuses to
// recurse without bound.
func drainDepth(t *testing.T, m pickerModel, msg tea.Msg, depth int) pickerModel {
	t.Helper()
	if depth > 16 {
		return m
	}
	next, cmd := m.Update(msg)
	m = next.(pickerModel)
	if cmd == nil {
		return m
	}
	out := cmd()
	switch v := out.(type) {
	case nil, tea.QuitMsg, cursor.BlinkMsg:
		return m
	case tea.BatchMsg:
		for _, c := range v {
			if c == nil {
				continue
			}
			inner := c()
			if inner == nil {
				continue
			}
			switch inner.(type) {
			case tea.QuitMsg, cursor.BlinkMsg:
				continue
			}
			m = drainDepth(t, m, inner, depth+1)
		}
		return m
	default:
		return drainDepth(t, m, out, depth+1)
	}
}

func runes(s string) []tea.KeyMsg {
	msgs := make([]tea.KeyMsg, 0, len(s))
	for _, r := range s {
		msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return msgs
}

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
)

func TestPickerModel_EnterAcceptsTopCandidate(t *testing.T) {
	// Candidate order: dev, web, batch, old-thing (running by activity first).
	m := send(t, newTestModel(t), keyEnter)
	if m.choice != "dev" {
		t.Errorf("bare Enter should pick the top candidate, got %q", m.choice)
	}
}

func TestPickerModel_TypeToFilterThenEnter(t *testing.T) {
	// Typing opens the filter without needing "/" first.
	m := send(t, newTestModel(t), append(runes("web"), keyEnter)...)
	if m.choice != "web" {
		t.Errorf("typed filter should select web, got %q", m.choice)
	}
}

func TestPickerModel_DownMovesSelection(t *testing.T) {
	m := send(t, newTestModel(t), keyDown, keyEnter)
	if m.choice != "web" {
		t.Errorf("down+enter should pick the second candidate, got %q", m.choice)
	}
}

func TestPickerModel_EscOnUnfilteredCancels(t *testing.T) {
	m := send(t, newTestModel(t), keyEsc)
	if m.choice != "" {
		t.Errorf("esc should cancel with empty choice, got %q", m.choice)
	}
}

func TestPickerModel_EscClearsFilterFirst(t *testing.T) {
	// Esc while filtering backs out of the filter, not the picker; a second
	// Enter then accepts the top of the full list again.
	m := send(t, newTestModel(t), append(runes("web"), keyEsc, keyEnter)...)
	if m.choice != "dev" {
		t.Errorf("esc should only clear the filter; enter should then pick dev, got %q", m.choice)
	}
}

func TestPickerModel_CtrlCCancels(t *testing.T) {
	m := send(t, newTestModel(t), append(runes("we"), keyCtrlC)...)
	if m.choice != "" {
		t.Errorf("ctrl-c should cancel, got %q", m.choice)
	}
}

func TestPickerModel_ViewShowsSessions(t *testing.T) {
	view := newTestModel(t).View()
	for _, want := range []string{"dev", "web", "running", "Pick a session"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestPickerItem_Description(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	items := orderPickerCandidates(pickerFixtures(), nil)
	top := pickerItem{info: items[0], now: now} // dev: running, 1m ago
	desc := top.Description()
	for _, want := range []string{"running", "1m ago"} {
		if !strings.Contains(desc, want) {
			t.Errorf("description missing %q: %q", want, desc)
		}
	}
	stopped := pickerItem{info: items[3], now: now} // batch: stopped, hosts unset
	if got := stopped.Description(); got != "stopped" {
		t.Errorf("stopped description = %q, want %q", got, "stopped")
	}
}
