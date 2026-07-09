# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `mox list` is now a single aligned table (origin, state, window count,
  attached marker, last activity, and host summary) instead of separate
  Configured/Unmanaged sections, and degrades cleanly under `NO_COLOR` and
  non-TTY output.
- `mox new @name` (with a single cluster argument that matches a configured
  session) now names the created session after the cluster instead of a
  generated `tmp-<timestamp>` name, so `mox new @staging` and `mox -a staging`
  land in the same place.

### Fixed

- `mox import` now recovers SSH connections from a running session instead of
  discarding them. A window whose panes are all plain `ssh <host>` connections
  is imported as a simple-mode `hosts:` list (matching how SSH fan-outs are
  meant to be configured), and any other pane running `ssh` keeps its full
  connection command. Previously these panes imported as anonymous `panes:`,
  losing every host.

### Added

- Bare `mox` on a terminal now opens an interactive two-pane session picker:
  a fuzzy-filterable list of every running, configured, and recent session
  on the left, and a live preview of the highlighted session (state, window
  count, hosts, connect template, sync/arrange/root) on the right. Status
  dots are colored by state, everything draws with the terminal's own
  palette, and narrow terminals collapse to the list alone. Enter attaches
  (building configured sessions as needed), Esc backs out of the filter and
  then the picker. Terminals that can't host it get a numbered prompt, and
  piped/scripted invocations still print help.
- `mox last` — attach to the session you used before this one (the session
  equivalent of `cd -`); bindable inside tmux via `run-shell "mox last"`.
- `-x/--exclude` on `mox new` — drop hosts (or whole `@clusters`) from the
  expanded host list: `mox new @webfarm -x web3`. Exclusions that match
  nothing are an error, catching typos.
- `mox edit` — open the config in `$VISUAL`/`$EDITOR` and validate it when the
  editor exits, reporting schema errors with line numbers.
- Arch Linux packaging: a source-build PKGBUILD under `packaging/aur/`, and
  automated publishing of a `mox-tmux-bin` AUR package on release (activates
  once an `AUR_SSH_KEY` repository secret is configured; releases proceed
  without the AUR step until then).
- `mox recent` (alias `r`) — lists the sessions you most recently created or
  attached to, newest first, with their current state (`running` or `gone`).
  Accepts `-n/--limit` (default 10). Backed by a small best-effort history file
  at `$XDG_STATE_HOME/mox/recent.json` (falls back to
  `~/.local/state/mox/recent.json`); this is the only state mox persists.
- `mox list` now shows recently created/attached sessions in a `Recent:` footer.

## [0.1.0] — 2026-05-28

### Added

- `@cluster` host expansion: `mox new @api-cluster` expands to the hosts
  of a configured session, or to a `clusterssh` `clusters`-file entry
  (nested clusters supported, with cycle detection)
- Shell completion improvements: `--arrange` flag values, `--from` configured
  sessions, `@<TAB>` cluster names, `kill <TAB>` running tmux sessions
- `mox new` command for ad-hoc multi-host sessions (alias: `cssh`).
  Defaults are cssh-style: `--sync`, `--arrange tiled`, `--sudo` on by
  default. Override with `--sync=false`, `--arrange=''`, `--sudo=false`.
  Flags: positional hosts, `--name`, `--connect`, `--user`, `--root`,
  `--from`, `--file`, `--temporary`, `--detach`, `--force`, `--window`
- `--sync` flag and `sync:` config field — enables tmux `synchronize-panes`
  so typing broadcasts to every pane (cssh-style)
- `--arrange tiled|even-horizontal|...` flag and `arrange:` config field —
  applies one of tmux's built-in layouts to a window after panes are created
- `--user USER` flag and `ssh_user:` config field — shortcut for
  `ssh USER@host` without writing a full connect template
- `--sudo` flag — appends `sudo -i` to the per-host commands (pair with
  `--sync` to type the password once)
- `--temporary` flag — sets `destroy-unattached on` so the session vanishes
  when the last client detaches
- `--window` flag — creates a window in the current tmux session instead of
  a new session (only when `$TMUX` is set)
- `mox list` now shows running tmux sessions that aren't in the config,
  in a separate "Unmanaged (tmux only)" section
- Colorized `mox list` output (running=green, stopped=dim, unmanaged=yellow,
  headers=bold). Respects `NO_COLOR` and disables colors when stdout isn't a tty
- Strict YAML decoding (unknown keys now error out)
- `XDG_CONFIG_HOME` support for config path resolution
- `--verbose` / `--quiet` flags using `log/slog` for structured logs
- Configurable per-host connect command template (`connect:` field), defaulting to `ssh {{host}}`
- Signal handling: Ctrl-C cancels session creation cleanly
- `tmux.Tmux` interface to support unit tests
- Unit tests for session manager, builder, config loader
- `.github/workflows/ci.yml` and `release.yml`
- `.goreleaser.yml` cross-compiling linux/darwin x amd64/arm64
- `.golangci.yml` with a curated linter set
- `CONTRIBUTING.md`, `SECURITY.md`

### Changed

- Config file written with mode 0600 (was 0644)
- `version.String()` no longer prefixes "mox " - cobra adds the binary name
- tmux errors now wrap original error with `%w` instead of stringifying
- `split-window` size now uses tmux 3.1 `-l <n>%` syntax

### Fixed

- `SessionExists` returned an error when no tmux server was running (now correctly reports false)
- `attach-session` failed when run from inside tmux (now uses `switch-client` when `$TMUX` is set)
- `Makefile` LDFLAGS injected `VERSION` into `GitCommit` (now uses git rev-parse)
- Session/window names containing `:` or `.` now rejected (would corrupt tmux targets)
- Hostnames now validated against `^[A-Za-z0-9._%@-]+$` to prevent shell injection through `ssh` keystrokes
- `--config ~/path` no longer silently fails; tilde is expanded
- `config view` now prints the raw config file (preserving comments) instead of round-tripped YAML
