# Terminal UI/UX plan

Status: proposed. Audience: experienced tmux and SSH users. Scope: the
interactive hub, config editor, and add wizard. The command interface and
session semantics remain the product contract.

## What the current UI shows

- The hub and editor share a panel renderer, but their list widths, footer
  actions, and narrow-terminal behavior differ. The add wizard replaces the
  editor view with a small panel at the top left; [issue #21](https://github.com/bthall/mox/issues/21)
  records the broken handoff.
- ANSI color 8 carries borders, stopped sessions, metadata, field labels,
  instructions, and the wizard's YAML preview. In the committed screenshots,
  these distinct meanings all read as similarly dim text.
- The hub clips rendered lines to terminal width, while the editor and wizard
  do not use the same final clamp. Their 24- and 32-column minimum widths can
  exceed the actual terminal width.
- The shortcut footers are useful for experts but can truncate on narrow
  terminals. Actions available only in certain states still occupy space in
  the default hint row.

## Direction

Build a compact operator workspace: one shared shell across hub, editor, and
add. Keep the live session preview as the hub's focal point, and make
configured/running/unmanaged state legible in text as well as glyph and color.
Use fewer box borders and a stronger type hierarchy to give the list, detail,
and current action clear roles. The add flow should use the editor's detail
pane, with a short progress label and an in-place YAML review.

This is a broader redesign of the terminal presentation. Familiar tmux key
paths stay fast: navigation, attach, start, kill, edit, and import should not
gain extra confirmation steps except where data or a running session is
destroyed. Color remains terminal-native and optional.

## Work sequence

1. **Lock the interaction map.** Inventory actions and states across the three
   views. Define one rule for Enter, Esc, Tab, `/`, `?`, and status feedback;
   retain existing expert shortcuts where they have no conflict. Write the
   rules before changing rendering.
2. **Build shared chrome and visual roles.** Add a common responsive layout,
   focus treatment, status area, and context-sensitive key hints. Separate
   primary text, secondary text, inactive state, warning, success, and border
   styles; check dark, light, and no-color terminals.
3. **Unify the flows.** Apply the shell to the hub and editor, then embed the
   add wizard in the editor detail pane. Make empty, loading, capture-failure,
   validation-error, unsaved, and changed-on-disk states explain the next
   useful action in the same place.
4. **Verify real terminal constraints.** Exercise 40×12, 80×24, and 120×35
   terminals with long session names, many hosts, wide Unicode, and captured
   ANSI output. No rendered line should exceed the terminal width; selection
   and save state must survive resize and handoff. Refresh the README captures
   only after the flows pass their interaction tests.

## Acceptance checks

- An operator can open `mox`, find a running or stopped session, and attach
  without consulting docs; unmanaged sessions are unmistakable.
- `mox edit` → add → review → save/start stays in one coherent layout, with a
  clear return path and no lost draft.
- The same state and key mean the same thing wherever possible; unavailable
  actions do not appear as active hints.
- Every supported width stays within terminal bounds, and essential text is
  readable in dark, light, and no-color modes.
