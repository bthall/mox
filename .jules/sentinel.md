## 2026-10-18 - Shell Printf Format String Vulnerability in Host Wrapping
**Vulnerability:** Shell `printf` commands generated for SSH pane holding/retrying embedded hostnames directly into the format string (`printf '[mox] %s: ...'`), which caused format specifiers (like `%` in IPv6 link-local scope IDs `fe80::1%eth0`) to be evaluated as printf format specifiers rather than literal characters.
**Learning:** Even when input values like hostnames pass strict validation, direct interpolation into shell `printf` format string templates can trigger format specifier expansion errors or unexpected output formatting.
**Prevention:** Pass variable parameters as distinct positional arguments (`printf '... %s ...' "$var"`) to shell `printf` instead of interpolating them directly into the format template string.

## 2026-10-20 - Tmux Command Injection via Semicolons in Target Names
**Vulnerability:** Session and window names containing semicolons (`;`) passed validation because reservedNameChars only checked `:.$ \t\r\n`. Semicolons serve as command separators in tmux command syntax, allowing command injection when session/window names were embedded into tmux hook commands (e.g., `set-option -t <session> ...`).
**Learning:** Validation for identifier names used in CLI tool command strings must reject command separator characters specific to the underlying subsystem (like tmux's `;`), not just standard target path separators.
**Prevention:** Include `;` in `reservedNameChars` for session and window validation and always use `--` flag terminators and quoted arguments when generating shell commands like `printf`.
