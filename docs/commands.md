# Commands

```
Session lifecycle:
  mox                   the session hub: browse, preview, and act on every
                        running/configured/recent session
  mox -a <session>      attach to a configured session (builds it if not running)
                        also attaches to any running tmux session by name
  mox new [hosts...]    ad-hoc session or window (alias: cssh)
  mox list | ls         list configured and running sessions
  mox recent | r        sessions you recently created or attached to
  mox last              attach to the session you used before this one
  mox kill <session>    destroy a running session
  mox import [session]  capture a running tmux session into the config
                        (no argument: the session you're inside)

Configuration:
  mox init              scaffold a default config
  mox add [name]        interactive wizard: build a session, save it to config
  mox edit [session]    full-screen session editor (optionally pre-selected);
                        invalid configs and non-terminals fall back to $EDITOR
  mox validate          check config syntax
  mox config path       print resolved config path
  mox config view       print the raw config file

Other:
  mox completion <sh>   emit a shell-completion script
  mox --version         print the build version
```

`mox <command> --help` shows the full flag list for any command, including
shorthands and defaults.

## The session hub

Bare `mox` on a terminal opens a full-screen hub: every running,
configured, and recent session on the left; a preview of the highlighted
one on the right. For a **running** session the preview is a live capture
of its active pane (refreshed every second while highlighted) with a
window summary line. For a **stopped** session it's the config summary.

| Key | Action |
| --- | --- |
| `↑↓` / `j k` | move |
| `/` | filter the list |
| `enter` | attach (building configured sessions as needed) |
| `S` | start the highlighted configured session detached |
| `K` | kill the highlighted running session (confirm, runs `on_stop`) |
| `ctrl+e` | open the session in the config editor; quitting returns here |
| `i` | import the highlighted unmanaged session into the config |
| `q` / `esc` | quit (esc clears an active filter first) |

Start and kill run in the background with a status line while they work;
the list refreshes in place when they finish. Import and edit hand off to
their own flows and return to a fresh hub over the updated config.

The status glyphs are shared with `mox list`: shape carries the origin,
color the state. `●` green is a running configured session; `◆` yellow is
a session running outside the config (`tmux only` in the preview title; it
can be attached, killed, or imported, but not started or edited); `○` is
stopped. Terminals that can't host the UI get a numbered prompt instead;
piped and scripted invocations print help, so scripts never hang.

## The session editor

`mox edit` (or `ctrl+e` in the hub) opens a full-screen editor:
configured sessions on the left, the selected session's fields on the
right, with the focused field's documentation always visible. Name a
session (`mox edit webfarm`) to open with it selected. A config the editor
can't load, or a non-terminal invocation, falls back to `$EDITOR` with
validation on exit — fixing a broken config by hand still works.

| Key | Action |
| --- | --- |
| `↑↓` / `j k` | move (list or form) |
| `tab` | switch pane |
| `/` | filter the session list |
| `enter` | edit the focused field (text input, toggle, or list editor) |
| `space` | cycle toggle/enum fields (`sync`, `arrange`, `hold`) |
| `a` | add a session (runs the `mox add` wizard; *save* lands as a draft, *save + start now* opens the diff preview and starts the session detached once the save lands) |
| `r` / `y` / `D` | rename / duplicate / delete (all buffered until save) |
| `o` | open the config in `$EDITOR` (window/pane structure) |
| `s` | save: validate → diff preview → write |
| `q` / `esc` | quit (prompts if there are unsaved changes) |

Edits buffer into a draft; nothing touches the file until you confirm the
diff preview. The save rewrites only the edited session's block — comments
and ordering elsewhere in the file are preserved (comments *inside* the
edited session's block are re-generated). If the file changed on disk while
the editor was open, the save is refused and `R` reloads.

In the `hosts` list editor, committing an entry that starts with `@`
expands the cluster into its members on the spot — config-stored hosts are
literal, so this matches how `mox add` and `mox new --save` behave.

Complex sessions (windows/panes) expose their session-level fields in the
form, and each *simple-mode* window (hosts, no panes) contributes editable
`<name> hosts` / `<name> cmds` rows — so imported sessions edit like any
other. Pane geometry itself is one `o` away in `$EDITOR`.
Changes apply the next time the session is built — a running session is
never touched.

## Cluster expansion (`@name`)

Anywhere `mox new` accepts a host, `@name` expands to a host list.
Lookup order:

1. The `hosts:` of a mox configured session named `name` (complex sessions
   are flattened across all their simple-mode windows)
2. The `name` entry in `~/.clusterssh/clusters` (or `$CSSH_CLUSTERS`, or
   `/etc/clusters`). Nested clusters are expanded recursively with cycle
   detection.

When `name` exists in both, the mox config wins. Mix freely with literal
hosts on the same line, and drop hosts back out with `-x/--exclude`:

```bash
mox new @webfarm -x web3        # the cluster minus a host that's mid-deploy
```

## Getting sessions into the config

Three routes, by decreasing interactivity:

1. **`mox add`** — a short wizard: name, hosts (with live `@cluster`
   expansion), ssh user, sync, arrangement, directory, commands, then a
   YAML preview with *save to config* or *save + start now*. Simple-mode
   sessions only.
2. **`mox new ... --save`** — you already expressed the session in flags;
   `--save` persists that definition to the config (requires `-n`) and
   creates the session as usual. Refuses to overwrite an existing entry.
3. **`mox import [session]`** — capture a *running* session: window/pane
   structure with real split directions and sizes, working directories,
   and SSH connections recovered from the process table. Run it with no
   argument from inside tmux to capture the session you're in, or press
   `i` in the hub on a highlighted `◆` tmux-only session for the same
   capture without leaving the hub.

The build-by-doing loop for custom layouts: `mox new`, split and arrange
panes by hand until the window looks right, then `mox import` from inside
it. A layout that can't be expressed as mox's sequential splits falls back
to a plain stack with a notice on stderr.

## Dry-run (`--print`)

`mox new --print` and `mox -a <session> --print` emit the exact tmux
commands a build would run, one copy-pasteable line each, without executing
anything (tmux doesn't even need to be installed). Hooks are printed rather
than run, and nothing is recorded in the recents history.

## Global flags

- `-c, --config <path>` — override the config path (`~` expanded)
- `-v, --verbose` — debug logging to stderr
- `-q, --quiet` — only warnings and errors
- `--force` — `mox -a <session> --force` rebuilds the session even if it's running

## Shell completion

`make install` does this automatically for your `$SHELL`. To install it
standalone, or after `go install`:

```bash
make install-completion         # detects $SHELL, installs to user dir

# or manually:
mox completion bash > ~/.local/share/bash-completion/completions/mox
mox completion zsh  > ~/.zsh/completions/_mox
mox completion fish > ~/.config/fish/completions/mox.fish
```

`<TAB>` then works for every argument that takes a name:

```bash
mox <TAB>                       # subcommands
mox -a <TAB>                    # configured + running tmux sessions
mox new @<TAB>                  # cluster names (config + clusterssh)
mox new --arrange <TAB>         # tmux layout names
mox kill <TAB>                  # running tmux sessions
```

## Behavior notes

When `-a` attaches from **inside** an existing tmux client (`$TMUX` set),
mox uses `switch-client` instead of `attach-session`, so nested-session
errors are avoided automatically.

`Ctrl-C` during session creation cancels cleanly: the partial session is
killed before the program exits.

The only state mox persists is a small recents history at
`$XDG_STATE_HOME/mox/recent.json` (fallback `~/.local/state/mox/recent.json`).
A missing or corrupt history file never breaks a command.
