## 2025-03-08 - TUI Confirmation Dialog Key Handling
**Learning:** In Bubbletea modal prompts displaying "y/n" or "Enter/y / Esc", users intuitively press 'n' or 'N' for "no". Failing to handle 'n' leaves the prompt open and makes the key feel unresponsive.
**Action:** Always handle 'n'/'N' alongside 'Esc' in confirmation dialog Update functions.

## 2025-03-08 - TUI Modal Repeat Key & Case-Insensitive Navigation
**Learning:** Users in terminal interfaces often double-tap the hotkey that opened a confirmation modal (e.g. pressing 'K' then 'K' to kill, or 'D' then 'D' to delete/discard). Additionally, Shift or Caps Lock state should not make single-letter modal prompts ('s'/'d'/'r') feel unresponsive.
**Action:** Accept trigger keys ('k'/'K', 'd'/'D') alongside 'y'/'Y'/'Enter' in modal confirm handlers, and handle case variations for prompt hotkeys.
