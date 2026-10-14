# Terminal UI/UX plan

Status: implemented in [PR #33](https://github.com/bthall/mox/pull/33).
Audience: experienced tmux and SSH users. Scope: the
interactive hub, config editor, and add wizard. The command interface and
session semantics remain the product contract.

## Before the redesign

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

## Shared screen model

At 80 columns and wider, reserve a stable session list on the left and a
larger work area on the right. The work area shows a live preview in the hub,
session fields in the editor, and the current step plus YAML review in add.
Keep the view name, selected session, operation status, and current key hints
in the same locations during handoffs. The add wizard should replace only
the work area when launched from the editor; the list and selection remain.

```text
mox / edit                                      config.yml · 12 sessions
  Sessions              │  new session · hosts                 step 2/8
  ● api-prod            │  Hosts, space separated
  ○ web-prod            │  @cluster expands before review
  ◆ scratch  tmux only  │  ▸ api web worker
  ...                   │
                        │  Review shows the YAML before save/start.
  Tab focus  / filter   │  Enter next  Esc back
  Draft not saved                                                80 × 24
```

At 40 columns, show the focused pane alone with a short breadcrumb and
`Tab` to switch list/detail. At 12 rows, keep the focused content, one status
line, and only the keys usable in that state; scroll content rather than
expanding the frame beyond the terminal. Never enforce a minimum width
larger than the actual viewport. Full-screen capture may be reduced to a
short preview or omitted at narrow widths, but session identity and state
must remain visible.

## Interaction contract

| Key | Browse/list | Field or wizard | Confirmation/review |
| --- | --- | --- | --- |
| `Enter` | Attach in hub; focus selected field in editor | Commit a field or advance one wizard step | Perform the labeled action |
| `Esc` | Clear an active filter, otherwise go back or quit | Return one level without losing a draft | Cancel and return to the draft |
| `Tab` | Move focus between list and work area | Move focus only when more than one area is shown | No hidden action |
| `/` | Filter the session list | Insert a literal slash in text input | No effect |
| `?` | Show help for the current view | Insert a literal question mark in text input; show help otherwise | Show help for the decision |

Preserve `j/k`, `S`, `K`, `i`, and `Ctrl+E` in the hub, plus editor
`a`, `s`, `D`, and `o`. Surface these shortcuts only when their target
action is available. Killing a running session, deleting a saved session,
and abandoning an unsaved draft require an explicit decision; attach,
start, and switching focus do not. `Ctrl+C` exits immediately only when
there is no unsaved draft.

The editor also provides `u` to open save/discard/stay for unsaved edits.
Discarding edits keeps the saved session; `D` explicitly deletes a saved
session or asks before discarding a never-saved session.

## Work sequence

1. **Validate the interaction contract.** Inventory every mode across the
   three views, resolve any conflict with the proposed key table, and turn
   the agreed transitions and status feedback into interaction tests before
   changing rendering.
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

## Implementation and verification

The hub, editor, and standalone/embedded add flow now use the shared shell in
`internal/cli/workspace_ui.go`. Key-path tests cover focus, resize, available
actions, literal text-input keys, cancellation, draft discard, and backtracking.
Rendered bounds are checked at 40×12, 80×24, 120×35 and defensive tiny sizes.

[CI](https://github.com/bthall/mox/actions/runs/38599593497) passed Linux/macOS
race tests, lint, tmux integration, vulnerability checks, and release builds.
The [preview workflow](https://github.com/bthall/mox/actions/runs/38599593504)
generated 29 captures covering the supported sizes, light/dark/no-color views,
and review, help, validation, stale-file, and discard states. These captures use
deterministic demo data in the actual Go models. README images carry the source
commit, frame hash, and renderer in PNG metadata; `make screenshots` regenerates
them.

The independent finish review checked the written terminal plan and captures.
It found three draft/recovery issues; the follow-up verdict scored all three
fixes resolved (`ship` at that correction scope). The approved written plan was
the composition reference; no separate bitmap comp or visual benchmark was used.
`DESIGN.md` and `.impeccable/design.json` describe the built terminal system.

## Direction contract

**Mode:** Operate. **Thesis:** a compact keyboard workspace for experienced
tmux/SSH users, with one stable session rail and one current task.

**Own world:** terminal foreground for text and key hints; weight and
underline for hierarchy; thin rules, restrained state glyph colors, and
no enclosing panel boxes. The user's terminal font and theme remain native.

**Path:** find a session, inspect or act, edit/add in the same frame, review
the result, then save or start. Unsaved work survives resizing and going back.

**First viewport:** one header, a session rail beside a larger work area,
and two fixed bottom rows for actions and feedback. At narrow widths, Tab
switches areas while selection and drafts stay intact.

**Form:** the user-approved shared workspace in this plan; a code-led terminal
implementation. The signature interaction is editor → add → review without
losing the rail or draft.

**Finish:** unreviewed and undocumented is unfinished; this build ends with
the finish review, the verdict, DESIGN.md, and every shipping raster carrying
its provenance.
