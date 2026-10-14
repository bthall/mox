---
name: Mox
description: "A terminal workspace for tmux sessions and buffered configuration."
spacing:
  cell-gap: "1 terminal cell"
  marker-width: "2 terminal cells"
  divider-width: "3 terminal cells"
  section-gap: "1 terminal row"
components:
  session-row:
    height: "{spacing.section-gap}"
  selection-marker:
    width: "{spacing.marker-width}"
  action-row:
    height: "{spacing.section-gap}"
  feedback-row:
    height: "{spacing.section-gap}"
---

# Design System: Mox

## Overview

**Creative North Star: "Compact keyboard workspace"**

Mox keeps a stable session rail beside the current task. The hub, editor, and
add flow share the same frame, so an operator can inspect a session, change its
configuration, and review a draft without rebuilding their sense of place.

The material is terminal text: native foreground, bold and underline, thin
rules, and small state marks. Density follows the available terminal cells.
The user's font, size, background, and palette remain native.

**Key Characteristics:**

- One shared frame for navigation, work, actions, and feedback.
- Keyboard focus and session state remain visible without color.
- Add, review, and draft decisions occupy the current work area.
- Terminal width controls composition while selection and input persist.

This system is extracted from `internal/cli/picker_ui.go`,
`workspace_ui.go`, `hub_ui.go`, `editor_ui.go`, and `add_ui.go`.
The approved direction is in `docs/ui-ux-plan.md`. Frontmatter owns the
cell/row spacing and component dimensions; `.impeccable/design.json` owns
terminal attributes, palette indices, layout formulas, and behavior extensions.
These are terminal units, not CSS lengths. Color and typography primitives
are omitted because Mox does not define CSS colors or a font scale.

## Colors

Essential text uses the terminal's default foreground against its native
background. Secondary text is unstyled foreground, rather than dimmed text.
Action keys, state words, errors, successes, and diff text also keep that
foreground.

The sidecar records the actual ANSI palette roles: running glyph, tmux-only
glyph, focus/input marker, and decorative rule. Their displayed colors depend
on the user's palette. A selected session has a marker and emphasized name;
running, stopped, tmux-only, draft, and pending deletion have distinct words
and marks.

Captured tmux output retains its own SGR attributes inside the preview. Capture
colors are session content, not Mox palette tokens. The no-color profile strips
all ANSI sequences from the completed frame, including captured output and
text attributes; shape and wording carry the remaining hierarchy.

**The Terminal Inheritance Rule.** Keep essential text on the terminal
foreground. Color supplements a mark or state word and never owns a decision.

## Typography

Every role uses the user's terminal font and cell metrics. Mox sets no font
family, point size, numeric weight, or display/body size ratio.

- **Title:** bold terminal attribute for the header, area titles, help keys,
  and action keys.
- **Selection:** bold and underline for the focused area title, selected
  session name, selected field label, and selected choice.
- **Body and secondary text:** default terminal attributes for values,
  explanations, metadata, and action labels.
- **State and feedback emphasis:** bold for running/tmux-only state words,
  draft warnings, errors, successes, and changed diff lines.

The sidecar records these as ANSI attributes, including the unchanged text
roles. Bold is a terminal request; its visual treatment belongs to the terminal.
No-color mode removes these attributes.

## Layout

The shared frame uses a header, top rule, area titles, bounded content, bottom
rule, action row, and feedback row. Content rows and footer text use the
frontmatter cell gap at each outer edge. Wide terminals place the session rail
beside a larger work area with one divider; the divider uses the frontmatter
divider width. There are no enclosing panel boxes.

The terminal extension records the narrow breakpoint, rail-width limits,
content-height formula, and small-height fallback. Narrow terminals show one
focused area. Tab switches areas without replacing the model's selected session,
draft, or wizard buffers. Help and confirmations bring the relevant work area
into view.

Session lists reserve space for a continuation count and keep selection visible.
The editor keeps the focused field visible after resize. Preview output shows
the latest available rows; YAML review, diff review, and help have their own
scroll offsets. The frame measures ANSI-aware display width, clips each final
line, pads columns in cells, and preserves the action/feedback rows when height
is constrained.

**The Stable Frame Rule.** Change the work area's content for review, help,
errors, and confirmations while preserving the frame and session context.

## Elevation & Depth

The interface is flat. Rules, area titles, whitespace, and text emphasis express
boundaries and focus. Confirmations replace work-area content; they do not float
over it. Mox adds no shadows or background layers.

## Shapes

Horizontal and vertical terminal rules separate regions. A bar marks selected
rows; an arrow marks the active field, choice, or input. Running, stopped, and
tmux-only sessions use different glyph shapes; drafts and pending deletion use
an asterisk. The sidecar holds the exact glyph vocabulary.

## Components

### Workspace frame

A compact header names the current view and right-aligns context when it fits.
The active area's title is emphasized. The frame owns bounds, shared rules,
footer positions, and no-color stripping; surfaces supply their content and
context-sensitive actions.

### Session rail

Each row combines a selection marker, state mark, session name, and state word.
The selected name uses the selection treatment. At very small row widths the
state word yields to the name while the mark remains. The list has a filter
line, a blank separation row, and a continuation count when more sessions remain.

### Work area and previews

The hub shows a running pane capture or stopped-session summary. A failed
capture explains that attachment remains available. The editor shows labeled
fields and help for the focused field. Reviews, validation errors, changed-file
warnings, and destructive confirmations reuse this area.

### Text fields

An arrow precedes input. The visible tail and block caret stay in view as input
grows, including wide text. Field edits, rename/duplicate prompts, list edits,
filters, and wizard text steps accept literal `/` and `?`. Help is invoked
with `?` in command/choice states; text input owns its printable characters.

### Action and feedback rows

The action row presents bold keys with plain labels and middle-dot separators.
It displays only relevant actions and retains complete key/action pairs.
Width filtering reserves help when that action is available. The feedback row
reports pending operations, saved state, unsaved work, validation failures, or
the next useful decision in the same place.

### Embedded add and review

The eight-step add wizard runs inside the editor's work area and keeps its
session rail. Tab visits the rail and returns to the wizard. Going back retains
input and committed choices. Embedded completion either keeps an editable draft
or opens the editor's review/save pipeline, optionally starting after save.
Standalone add uses the same frame and shows its draft above configured sessions.

### Draft and destructive decisions

In editor browse state, `u` on a dirty draft opens the existing save/discard/stay
guard. It does not immediately discard edits. `D` opens a confirmation to
discard a newly added unsaved session or buffer deletion of a saved session;
pending saved-session deletion can be undone with `D` and only reaches disk
on save. The last saved session cannot be deleted.

Changing sessions or leaving with unsaved work opens the guard. Add cancellation
with input opens a discard/keep-editing decision. Normal editor save validates
and shows a diff before writing; choosing save in the guard is itself the
confirmation and uses the save pipeline directly.

**The Explicit Draft Decision Rule.** Name the affected work and show the
available save, discard, or stay action before abandoning it.

## Do's and Don'ts

### Do:

- **Do** reuse the shared frame for the hub, editor, and add flow.
- **Do** pair state marks with words wherever row width permits.
- **Do** keep selection, focused input, and draft state through resizing.
- **Do** keep errors and the next useful action in the current work area and feedback row.
- **Do** preserve literal printable input before applying command shortcuts.

### Don't:

- **Don't** assign fixed colors, fonts, or pixel dimensions to terminal-native roles.
- **Don't** enclose the two workspace areas in individual panel boxes.
- **Don't** dim essential text or depend on color to distinguish session ownership.
- **Don't** split a displayed key/action pair to fit a narrow footer.
- **Don't** discard dirty editor work directly from the discard-edits shortcut.
