## 2025-03-08 - TUI Confirmation Dialog Key Handling
**Learning:** In Bubbletea modal prompts displaying "y/n" or "Enter/y / Esc", users intuitively press 'n' or 'N' for "no". Failing to handle 'n' leaves the prompt open and makes the key feel unresponsive.
**Action:** Always handle 'n'/'N' alongside 'Esc' in confirmation dialog Update functions.

## 2025-03-08 - TUI Modal Repeat Key & Case-Insensitive Navigation
**Learning:** Users in terminal interfaces often double-tap the hotkey that opened a confirmation modal (e.g. pressing 'K' then 'K' to kill, or 'D' then 'D' to delete/discard). Additionally, Shift or Caps Lock state should not make single-letter modal prompts ('s'/'d'/'r') feel unresponsive.
**Action:** Accept trigger keys ('k'/'K', 'd'/'D') alongside 'y'/'Y'/'Enter' in modal confirm handlers, and handle case variations for prompt hotkeys.

## 2025-03-08 - TUI Selection List Vim Navigation & Page Scrolling
**Learning:** Terminal users expect vim-style navigation ('j'/'k') and page scrolling ('PgUp'/'PgDn') to work consistently across selection lists and modal diff previews. Swallowing 'j'/'k' or 'PgUp'/'PgDn' in non-text-entry steps makes controls feel unresponsive.
**Action:** Handle 'j'/'k' (and 'J'/'K') in list selection steps and 'PgUp'/'PgDn' in modal diff views, updating key hints to match supported controls.
