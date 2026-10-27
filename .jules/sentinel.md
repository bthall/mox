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

## 2026-10-24 - Unquoted Shell Arguments in Session Import Command Recovery
**Vulnerability:** SSH commands recovered from the OS process table during `mox import` were joined with `strings.Join(p.argv, " ")`, stripping shell quotes and argument boundaries. When the imported session was later launched, shell metacharacters (such as `;` or `&&`) in arguments were executed by the local shell instead of being passed as arguments to SSH.
**Learning:** Reconstructing executable command strings from argument vectors (`argv`) for subsequent execution by a shell requires full shell-quoting (`ShellJoin`), not simple space concatenation.
**Prevention:** Always use `tmux.ShellJoin` when converting an argument slice (`[]string`) into a single shell command string to preserve argument boundaries and prevent local command execution.
## 2026-10-24 - Tmux Option Injection via Positional Arguments Starting with Hyphen
**Vulnerability:** Positional arguments passed to tmux commands (`send-keys`, `rename-window`, `select-layout`) without an option terminator (`--`) caused strings starting with hyphens (such as `-t`, `-R`, `--help`, or window names like `-mywin`) to be parsed as tmux command flags rather than literal positional values, causing command failures or unintended option execution.
**Learning:** CLI wrappers executing external tools like `tmux` must use `--` option terminators before positional arguments that originate from user input or configuration to ensure they are interpreted as positional arguments rather than CLI flags.
**Prevention:** Always place `--` before positional arguments in `tmux` command invocations (e.g. `send-keys -t target -- cmd C-m` or `rename-window -t target -- name`).

## 2026-10-26 - Shell Command Injection via Unquoted Recovered Process Arguments
**Vulnerability:** Recovering pane process arguments during session import joined `p.argv` slices with spaces (`strings.Join(p.argv, " ")`), leaving spaces, quotes, and shell metacharacters unescaped in generated `commands:` entries.
**Learning:** When process table argument vectors are converted to shell commands for persistence or re-execution, joining arguments with raw spaces loses shell boundaries and allows command injection.
**Prevention:** Use shell-quoting primitives (e.g., `tmux.ShellJoin`) when turning process `argv` slices into single-string shell command templates.
