## 2026-10-18 - Shell Printf Format String Vulnerability in Host Wrapping
**Vulnerability:** Shell `printf` commands generated for SSH pane holding/retrying embedded hostnames directly into the format string (`printf '[mox] %s: ...'`), which caused format specifiers (like `%` in IPv6 link-local scope IDs `fe80::1%eth0`) to be evaluated as printf format specifiers rather than literal characters.
**Learning:** Even when input values like hostnames pass strict validation, direct interpolation into shell `printf` format string templates can trigger format specifier expansion errors or unexpected output formatting.
**Prevention:** Pass variable parameters as distinct positional arguments (`printf '... %s ...' "$var"`) to shell `printf` instead of interpolating them directly into the format template string.

## 2026-10-20 - Tmux Command Injection via Semicolons in Target Names
**Vulnerability:** Session and window names containing semicolons (`;`) passed validation because reservedNameChars only checked `:.$ \t\r\n`. Semicolons serve as command separators in tmux command syntax, allowing command injection when session/window names were embedded into tmux hook commands (e.g., `set-option -t <session> ...`).
**Learning:** Validation for identifier names used in CLI tool command strings must reject command separator characters specific to the underlying subsystem (like tmux's `;`), not just standard target path separators.
**Prevention:** Include `;` in `reservedNameChars` for session and window validation and always use `--` flag terminators and quoted arguments when generating shell commands like `printf`.

## 2026-10-22 - Terminal Escape Sequence Injection via Config Names and Paths
**Vulnerability:** Session names, window names, layout names, and root directory paths were not consistently validated for ASCII control characters (`unicode.IsControl`). Layout names also bypassed `reservedNameChars` validation. An attacker or malicious config file could inject ANSI escape codes or control characters (like `\x1b` or `\x00`) to manipulate user terminal displays, titles, or status logs.
**Learning:** Config identifiers and filesystem paths displayed in terminal UIs or CLI outputs must be sanitized or validated against control characters, and reserved character checks must apply uniformly to all named configuration abstractions (sessions, windows, layouts).
**Prevention:** Always validate session, window, and layout names as well as root directory paths using `unicode.IsControl` and `reservedNameChars` during config schema validation.

## 2026-10-26 - Shell Command Injection via Unquoted Recovered Process Arguments
**Vulnerability:** Recovering pane process arguments during session import joined `p.argv` slices with spaces (`strings.Join(p.argv, " ")`), leaving spaces, quotes, and shell metacharacters unescaped in generated `commands:` entries.
**Learning:** When process table argument vectors are converted to shell commands for persistence or re-execution, joining arguments with raw spaces loses shell boundaries and allows command injection.
**Prevention:** Use shell-quoting primitives (e.g., `tmux.ShellJoin`) when turning process `argv` slices into single-string shell command templates.
