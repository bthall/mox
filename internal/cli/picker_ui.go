package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/bthall/mox/internal/session"
)

// pickerMaxRows caps how many candidate rows the interactive picker draws.
const pickerMaxRows = 10

// pickerUI is the interactive fuzzy picker: an inline prompt plus a filtered
// candidate list drawn under the cursor and redrawn on every keystroke. It
// never takes over the whole screen.
//
// Cursor invariant: between renders the cursor rests on the prompt line,
// just after the query — render() starts from there and returns there.
type pickerUI struct {
	names    []string // candidate names, selection order
	rows     []string // pre-rendered plain-text row bodies, same order
	total    int
	width    int // terminal width; rows are truncated to fit on one line
	query    []rune
	selected int // index into the current filtered view
	filtered []int
	drawn    int // total lines (prompt included) of the previous render
}

// newPickerUI pre-renders one plain-text row per candidate. Rows carry no
// color codes so truncation to the terminal width is safe.
func newPickerUI(candidates []session.SessionInfo, width int, now time.Time) *pickerUI {
	if width <= 0 {
		width = 80
	}
	ui := &pickerUI{total: len(candidates), width: width}
	nameW := 0
	for _, c := range candidates {
		if w := len(c.Name); w > nameW {
			nameW = w
		}
	}
	for _, c := range candidates {
		state := "stopped"
		if c.Running {
			state = "running"
			if c.Attached {
				state = "running attached"
			}
		}
		ui.names = append(ui.names, c.Name)
		row := fmt.Sprintf("%-*s  %-16s  %-7s  %s",
			nameW, c.Name, state, relativeShort(now, c.LastActivity), hostsCell(c))
		ui.rows = append(ui.rows, strings.TrimRight(row, " "))
	}
	ui.refilter()
	return ui
}

// refilter recomputes the visible set for the current query and resets the
// selection when it fell outside it.
func (ui *pickerUI) refilter() {
	ui.filtered = fuzzyFilter(string(ui.query), ui.names)
	if ui.selected >= len(ui.filtered) {
		ui.selected = 0
	}
}

// render redraws the whole block. Starting on the prompt line, it repaints
// the prompt, the visible rows, and a match counter, blanks any leftover
// lines from a taller previous render, and puts the cursor back on the
// prompt line after the query.
func (ui *pickerUI) render(out io.Writer) {
	var b strings.Builder

	// Prompt line (cursor starts here per the invariant).
	b.WriteString("\r\x1b[K> " + string(ui.query) + "\r\n")

	lines := 0 // lines below the prompt
	visible := ui.filtered
	if len(visible) > pickerMaxRows {
		visible = visible[:pickerMaxRows]
	}
	for i, idx := range visible {
		pointer, style, reset := "  ", "", ""
		if i == ui.selected {
			pointer = "> "
			style, reset = ansiBold, ansiReset
		}
		b.WriteString("\x1b[K" + style + truncate(pointer+ui.rows[idx], ui.width-1) + reset + "\r\n")
		lines++
	}
	if len(ui.filtered) == 0 {
		b.WriteString("\x1b[K  (no match)\r\n")
		lines++
	}
	fmt.Fprintf(&b, "\x1b[K  %d/%d\r\n", len(ui.filtered), ui.total)
	lines++

	// Blank leftovers when the previous render was taller.
	extra := ui.drawn - (lines + 1)
	if extra < 0 {
		extra = 0
	}
	for i := 0; i < extra; i++ {
		b.WriteString("\x1b[K\r\n")
	}

	// Cursor is now at the line below everything drawn; hop back up to the
	// prompt line and park after the query.
	fmt.Fprintf(&b, "\x1b[%dA\r\x1b[%dC", lines+1+extra, 2+len(ui.query))

	fmt.Fprint(out, b.String())
	ui.drawn = lines + 1
}

// clear erases the rendered block and leaves the cursor at column 0 of the
// (former) prompt line.
func (ui *pickerUI) clear(out io.Writer) {
	fmt.Fprint(out, "\r\x1b[K")
	for i := 1; i < ui.drawn; i++ {
		fmt.Fprint(out, "\x1b[B\x1b[K")
	}
	if ui.drawn > 1 {
		fmt.Fprintf(out, "\x1b[%dA", ui.drawn-1)
	}
	ui.drawn = 0
}

// run is the event loop: render, consume keystrokes from in, return the
// chosen session name ("" means canceled). The caller is responsible for
// raw mode.
func (ui *pickerUI) run(in io.Reader, out io.Writer) string {
	reader := bufio.NewReader(in)
	ui.render(out)
	for {
		r, _, err := reader.ReadRune()
		if err != nil { // EOF cancels
			ui.clear(out)
			return ""
		}
		switch {
		case r == '\r': // Enter (raw mode sends CR; LF is Ctrl-J below)
			if len(ui.filtered) == 0 {
				continue
			}
			ui.clear(out)
			return ui.names[ui.filtered[ui.selected]]
		case r == 3 || r == 4: // Ctrl-C, Ctrl-D
			ui.clear(out)
			return ""
		case r == 27: // ESC alone cancels; ESC [ A/B are arrows
			if next, _ := reader.Peek(1); len(next) == 1 && next[0] == '[' {
				_, _ = reader.Discard(1)
				dir, _, _ := reader.ReadRune()
				switch dir {
				case 'A':
					ui.moveSelection(-1)
				case 'B':
					ui.moveSelection(1)
				}
				ui.render(out)
				continue
			}
			ui.clear(out)
			return ""
		case r == 16 || r == 11: // Ctrl-P, Ctrl-K: up
			ui.moveSelection(-1)
			ui.render(out)
		case r == 14 || r == 10: // Ctrl-N, Ctrl-J: down
			ui.moveSelection(1)
			ui.render(out)
		case r == 21: // Ctrl-U clears the query
			ui.query = nil
			ui.selected = 0
			ui.refilter()
			ui.render(out)
		case r == 127 || r == 8: // backspace
			if len(ui.query) > 0 {
				ui.query = ui.query[:len(ui.query)-1]
				ui.selected = 0
				ui.refilter()
				ui.render(out)
			}
		case unicode.IsPrint(r):
			ui.query = append(ui.query, r)
			ui.selected = 0
			ui.refilter()
			ui.render(out)
		}
	}
}

// moveSelection moves the highlight by delta within the visible rows,
// wrapping at the ends.
func (ui *pickerUI) moveSelection(delta int) {
	visible := len(ui.filtered)
	if visible > pickerMaxRows {
		visible = pickerMaxRows
	}
	if visible == 0 {
		return
	}
	ui.selected = (ui.selected + delta + visible) % visible
}
