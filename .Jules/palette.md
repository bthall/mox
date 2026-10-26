## 2025-03-08 - TUI Confirmation Dialog Key Handling
**Learning:** In Bubbletea modal prompts displaying "y/n" or "Enter/y / Esc", users intuitively press 'n' or 'N' for "no". Failing to handle 'n' leaves the prompt open and makes the key feel unresponsive.
**Action:** Always handle 'n'/'N' alongside 'Esc' in confirmation dialog Update functions.

## 2025-03-08 - TUI Modal Repeat Key & Case-Insensitive Navigation
**Learning:** Users in terminal interfaces often double-tap the hotkey that opened a confirmation modal (e.g. pressing 'K' then 'K' to kill, or 'D' then 'D' to delete/discard). Additionally, Shift or Caps Lock state should not make single-letter modal prompts ('s'/'d'/'r') feel unresponsive.
**Action:** Accept trigger keys ('k'/'K', 'd'/'D') alongside 'y'/'Y'/'Enter' in modal confirm handlers, and handle case variations for prompt hotkeys.

## 2025-03-08 - TUI Guard Modal Double-Ctrl+C and Trigger Keys
**Learning:** When a dirty-draft exit guard prompt appears after pressing 'q' or 'Ctrl+C', users frequently press 'Ctrl+C' or 'q' a second time to confirm quitting. Ignoring 'Ctrl+C' in guard prompts makes the key feel dead and unresponsive. Additionally, users press 'c'/'C' intuitively for "Cancel" or "Continue editing".
**Action:** In guard prompts with a pending quit action, handle double 'Ctrl+C' and repeat 'q'/'Q' to complete the quit, handle 'Ctrl+C' on non-quit guards to stay/cancel, and accept 'c'/'C' alongside 'n'/'N'/'Esc'.
