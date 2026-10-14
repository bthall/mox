# Mox

<!-- impeccable:product-schema 1 -->

## Platform

Terminal application for Linux and macOS, built in Go with Bubble Tea.

## Users

Experienced tmux and SSH users who manage local workspaces and multi-host
sessions. The interface should keep frequent operations fast from the keyboard.

## Product Purpose

Build and manage tmux sessions from declarative YAML, including synchronized
SSH fan-outs. Operators can find, start, attach, edit, and import sessions
without retyping their setup.

## Capabilities and Constraints

- One binary, no daemon. Configuration is trusted user input.
- The hub previews running tmux panes; the editor buffers and validates drafts.
- Add builds simple sessions; custom pane structures remain available through
  YAML and importing a running session.
- Respect terminal dimensions, the user's terminal palette, and `NO_COLOR`.
- Preserve existing command semantics and expert shortcuts during the redesign.

## Brand Commitments

Keep the name Mox and the existing logo. A broader terminal visual redesign is
authorized; the shared hub/editor/add direction is in `docs/ui-ux-plan.md`.

## Product Principles

- Make session state and ownership clear without relying on color.
- Keep a stable place for navigation, the current task, and feedback.
- Protect unsaved drafts and make destructive decisions explicit.
- Use the available terminal space without hiding the next useful action.
