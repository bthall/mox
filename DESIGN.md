# Terminal interface

Mox uses one operator workspace across the session hub, config editor, and add
flow. The approved direction and acceptance checks live in
`docs/ui-ux-plan.md`.

- A compact header names the view and context. One session rail and one work
  area share a single divider; individual panels do not have enclosing boxes.
- At 80 columns and wider, both areas remain visible. Narrow terminals show
  the focused area with `Tab` to switch. Content scrolls within the viewport.
- The last two rows hold context-sensitive actions and operation/draft status.
  Short hints yield to important actions; `?` exposes the complete key map.
- Primary and secondary content use the terminal's default foreground. Weight
  and spacing establish hierarchy. Cyan marks focus/actions, green success and
  running sessions, yellow warnings and unmanaged sessions, red errors.
- State glyphs have text equivalents: running, stopped, tmux only, and draft.
  Color is optional and never carries a decision alone.
- Add stays in the editor's work area when embedded. Review, errors, and
  confirmations occupy that area without replacing the session rail.
- Text input keeps literal `/` and `?`. Escape returns one level; abandoning
  an unsaved draft requires a save/discard/stay decision.

Terminal text uses the user's own font. Screen fixtures and README captures
are generated from the actual Go models, rather than drawing a separate mock.
