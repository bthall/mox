package cli

// Shared keyboard helpers for bubbletea model tests.

import tea "github.com/charmbracelet/bubbletea"

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
	keyBksp  = tea.KeyMsg{Type: tea.KeyBackspace}
)

// runes turns a string into one KeyRunes message per character.
func runes(s string) []tea.KeyMsg {
	msgs := make([]tea.KeyMsg, 0, len(s))
	for _, r := range s {
		msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return msgs
}
