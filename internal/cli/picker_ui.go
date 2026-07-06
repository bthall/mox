package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/bthall/mox/internal/session"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// pickerItem adapts a SessionInfo to the list component: the session name is
// the title, and a state/size/activity/hosts summary is the description.
type pickerItem struct {
	info session.SessionInfo
	now  time.Time
}

func (i pickerItem) Title() string       { return i.info.Name }
func (i pickerItem) FilterValue() string { return i.info.Name }

func (i pickerItem) Description() string {
	var parts []string
	if i.info.Running {
		state := "running"
		if i.info.Attached {
			state = "running · attached"
		}
		parts = append(parts, state)
		switch i.info.Windows {
		case 0: // unknown; don't claim a count
		case 1:
			parts = append(parts, "1 window")
		default:
			parts = append(parts, fmt.Sprintf("%d windows", i.info.Windows))
		}
		parts = append(parts, relativeTime(i.now, i.info.LastActivity))
	} else {
		parts = append(parts, "stopped")
	}
	if hosts := hostsCell(i.info); hosts != "-" {
		parts = append(parts, hosts)
	}
	return strings.Join(parts, " · ")
}

// pickerModel is the Bubble Tea model around the list: typing filters
// immediately, Enter attaches the highlighted session, Esc backs out of the
// filter and then out of the picker entirely.
type pickerModel struct {
	list   list.Model
	choice string
}

func newPickerModel(candidates []session.SessionInfo, now time.Time) pickerModel {
	items := make([]list.Item, len(candidates))
	for i, c := range candidates {
		items[i] = pickerItem{info: c, now: now}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, listHeight(len(items)))
	l.Title = "Pick a session"
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings() // Esc/q are handled below, q must stay typable
	return pickerModel{list: l}
}

// listHeight sizes the inline list: three terminal rows per two-line item,
// plus chrome, capped so tall session lists page instead of flooding.
func listHeight(items int) int {
	h := items*3 + 5
	if h > 24 {
		h = 24
	}
	return h
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h := listHeight(len(m.list.Items()))
		if msg.Height > 0 && h > msg.Height-1 {
			h = msg.Height - 1
		}
		m.list.SetSize(msg.Width, h)
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyEnter:
			if item, ok := m.list.SelectedItem().(pickerItem); ok {
				m.choice = item.info.Name
			}
			return m, tea.Quit

		case tea.KeyEsc:
			// Esc first clears an active filter (the list handles that);
			// on an unfiltered list it cancels the picker.
			if m.list.FilterState() == list.Unfiltered {
				return m, tea.Quit
			}

		case tea.KeyRunes:
			// Type-to-filter: printable input outside filter mode opens the
			// filter first, so the picker feels like a search box rather
			// than requiring "/".
			if m.list.FilterState() == list.Unfiltered {
				var cmd tea.Cmd
				m.list, cmd = m.list.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
				cmds := []tea.Cmd{cmd}
				m.list, cmd = m.list.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string { return m.list.View() }

// runFuzzyPicker runs the interactive picker inline (no alt screen). ran is
// false when the terminal can't host it and the caller should fall back to
// the numbered prompt.
func runFuzzyPicker(candidates []session.SessionInfo) (name string, ran bool) {
	final, err := tea.NewProgram(newPickerModel(candidates, time.Now())).Run()
	if err != nil {
		return "", false
	}
	m, ok := final.(pickerModel)
	if !ok {
		return "", false
	}
	return m.choice, true
}
