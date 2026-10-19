## 2025-03-08 - TUI Confirmation Dialog Key Handling
**Learning:** In Bubbletea modal prompts displaying "y/n" or "Enter/y / Esc", users intuitively press 'n' or 'N' for "no". Failing to handle 'n' leaves the prompt open and makes the key feel unresponsive.
**Action:** Always handle 'n'/'N' alongside 'Esc' in confirmation dialog Update functions.
