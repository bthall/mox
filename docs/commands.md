# Commands

```
Session lifecycle:
  mox                   interactive picker over running/configured/recent sessions
  mox -a <session>      attach to a configured session (builds it if not running)
                        also attaches to any running tmux session by name
  mox new [hosts...]    ad-hoc session or window (alias: cssh)
  mox list | ls         list configured and running sessions
  mox recent | r        sessions you recently created or attached to
  mox last              attach to the session you used before this one
  mox kill <session>    destroy a running session
  mox import <session>  capture a running tmux session into the config

Configuration:
  mox init              scaffold a default config
  mox edit              open the config in $EDITOR, validate on exit
  mox validate          check config syntax
  mox config path       print resolved config path
  mox config view       print the raw config file

Other:
  mox completion <sh>   emit a shell-completion script
  mox --version         print the build version
```

`mox <command> --help` shows the full flag list for any command, including
shorthands and defaults.

## The session picker

Bare `mox` on a terminal opens an interactive two-pane picker: a
fuzzy-filterable list of every running, configured, and recent session on
the left, and a live preview of the highlighted session (state, windows,
hosts, connect template) on the right.

- Type to filter, arrows or `Ctrl-J`/`Ctrl-K` to move
- `Enter` attaches (building configured sessions as needed)
- `Esc` backs out of the filter, then out of the picker

Terminals that can't host the interactive UI get a numbered prompt instead;
piped and scripted invocations print help, so scripts never hang.

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
