<p align="center">
  <img src="assets/logo.png" alt="mox logo" width="200">
</p>

<h1 align="center">mox</h1>

<p align="center">
  <a href="https://github.com/bthall/mox/actions/workflows/ci.yml"><img src="https://github.com/bthall/mox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/bthall/mox/releases/latest"><img src="https://img.shields.io/github/v/release/bthall/mox" alt="Latest release"></a>
  <a href="https://goreportcard.com/report/github.com/bthall/mox"><img src="https://goreportcard.com/badge/github.com/bthall/mox" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/bthall/mox" alt="License"></a>
</p>

A CLI tool for creating and managing tmux sessions declaratively from YAML
configuration files.

## Features

- **Declarative YAML config** — one window per host, full custom layouts, or anything between
- **Cssh-style broadcast** — `sync: true` for synchronized typing across panes; tiled layouts
- **Ad-hoc multi-host sessions** — `mox new @cluster` or `mox new host1 host2 host3` without editing config
- **Cluster expansion** — `@name` resolves to a configured session's hosts or a clusterssh `clusters` file entry (with nested-cluster expansion)
- **Import existing tmux sessions** — capture a running session's structure and SSH connections into the config (`mox import`)
- **Local quick sessions** — `mox new` with no args opens a single-pane local session; `-t` makes it self-destruct on detach
- **In-tmux mode** — `mox new -w @cluster` opens a window in your current session instead of a new session
- **Configurable connect** — defaults to `ssh {{host}}`; override per session/window; `ssh_user:` shortcut
- **Reusable named layouts** — define once, reference from any window
- **Shell completion** — bash, zsh, fish; completes sessions, clusters, layouts, and running tmux sessions
- **Session picker** — bare `mox` opens a fuzzy-filterable list of every running, configured, and recent session
- **Recent sessions** — `mox list` and `mox recent` remember what you created or attached to; `mox last` bounces back to the previous one
- **Host exclusion** — `mox new @webfarm -x web3` broadcasts to a cluster minus the hosts you name
- **Edit with a net** — `mox edit` opens the config in `$EDITOR` and validates it on save
- **Strict validation** — typos in config keys error with line numbers
- **Honest defaults** — single binary, no daemon; the only state is a small recents history

## Install

### From source

```bash
git clone https://github.com/bthall/mox.git
cd mox
make install        # installs binary to $GOPATH/bin + shell completion for $SHELL
```

`make install` also installs completion for your current shell (bash, zsh,
or fish). If you only want completion (e.g. after `go install`), run
`make install-completion`.

### Using `go install`

```bash
go install github.com/bthall/mox/cmd/mox@latest
```

### Pre-built binaries

Archives for linux/macOS × amd64/arm64 are attached to each
[GitHub release](https://github.com/bthall/mox/releases) along with
`checksums.txt`:

```bash
curl -LO https://github.com/bthall/mox/releases/latest/download/mox_0.1.0_linux_amd64.tar.gz
tar xzf mox_0.1.0_linux_amd64.tar.gz && install -Dm755 mox ~/.local/bin/mox
```

### Arch Linux

A source-build `PKGBUILD` is provided under [`packaging/aur/`](packaging/aur/):

```bash
cd packaging/aur && makepkg -si
```

## Quick start

```bash
mox init                        # scaffold a default config at ~/.config/mox/config.yml
mox -a example                  # build + attach to the "example" session
mox                             # or: pick a session interactively
mox list                        # configured + running tmux sessions
mox recent                      # sessions you recently created or attached to
mox kill example                # destroy a running session
```

## Recipes

Common workflows by example. Every short flag has a long form (`mox new --help` for the full table).

### Attach to a configured session

```bash
mox -a dev                      # build it if it isn't running, then attach
mox -a dev --force              # tear down the running one and rebuild
```

`-a` also attaches to **any** running tmux session, even ones not in your
mox config — useful for hand-rolled sessions you started by hand.

### Quick local session

```bash
mox new                         # plain shell in a new tmux session (auto-named tmp-<timestamp>)
mox new -n work                 # named "work"
mox new -n work -r ~/proj       # named "work", starting in ~/proj
mox new -t                      # destroyed automatically when you detach
```

### Multi-host admin (cssh-style)

`mox new` with hosts gives you a tiled grid, synchronized typing across
panes, and `sudo -i` auto-sent so you type the root password once.

```bash
mox new host1 host2 host3       # 3 hosts, broadcast typing, sudo on connect
mox new @api-cluster                # same, using a configured session's host list
mox new @api-cluster -x api2        # the cluster minus a host that's mid-deploy
mox new -u root @api-cluster        # ssh as root
mox new -S=false @api-cluster       # turn off broadcast typing for this one
mox new --sudo=false @api-cluster   # skip sudo
```

### Open as a window in your current tmux session

If you're already inside tmux and don't want a separate session:

```bash
mox new -w @api-cluster             # new window in the current session
```

### Clone a configured session, override hosts

```bash
mox new --from api-cluster host42   # use api-cluster's settings, but just this one host
```

### Capture a hand-rolled tmux session into config

If you built a session interactively and want to recreate it later:

```bash
mox import work                 # add 'work' to your mox config under the same name
mox import work -n my-work      # rename on import
mox import work -p              # preview the YAML on stdout without saving
mox import work -F              # overwrite an existing config entry
```

SSH connections are recovered from the OS process table: a window whose
panes are all plain `ssh host` connections imports as a simple `hosts:`
list, and other ssh panes keep their connection as a `commands:` entry.
Anything else (editors, REPLs, scripts you typed) isn't recoverable from
tmux's running state — add `commands:` entries afterward to make those
panes fully reproducible.

### Migrate from clusterssh

If you already maintain `~/.clusterssh/clusters`, you don't have to
duplicate it. mox reads that file directly:

```bash
mox new @monitoring         # expands the clusterssh entry by that name
mox new @pve                    # nested clusterssh tags work too
```

Configured sessions in `~/.config/mox/config.yml` take precedence when a
name exists in both places.

### Tab completion

After `make install`, `<TAB>` works for every argument that takes a name:

```bash
mox <TAB>                       # subcommands
mox -a <TAB>                    # configured + running tmux sessions
mox new @<TAB>                  # cluster names (config + clusterssh)
mox new --arrange <TAB>         # tmux layout names
mox kill <TAB>                  # running tmux sessions
```

## Configuration

The default config path follows the XDG Base Directory spec:

- `$XDG_CONFIG_HOME/mox/config.yml` if set, otherwise
- `~/.config/mox/config.yml`

Override via the `--config` flag (which expands `~`).

The file is created with mode `0600` (owner read/write only) since it can
list hostnames and shell commands.

### Simple session

One window, one pane per host:

```yaml
sessions:
  devenv:
    hosts: [api, web, worker]
    root: ~/projects/myapp
    commands:
      - echo "Welcome!"
```

### Custom connect command

The default connect template is `ssh {{host}}`. Override per session or per
window with the `connect:` field, or use the `ssh_user:` shortcut to prefix
the default with `USER@`:

```yaml
sessions:
  prod:
    connect: "ssh -p 2222 ops@{{host}}"
    hosts: [api1, api2]

  as-root:
    ssh_user: root                    # same as connect: "ssh root@{{host}}"
    hosts: [a, b, c]

  mixed:
    windows:
      - name: shells
        connect: "mosh {{host}}"      # window-level override
        hosts: [bastion]
      - name: local
        hosts: [localhost]            # uses default ssh
```

### Synchronized panes and tiled layouts (cssh-style)

For multi-host administration where you want one keystroke to broadcast to
every pane:

```yaml
sessions:
  mongo-cluster:
    sync: true                        # synchronize-panes on
    arrange: tiled                    # grid layout instead of vertical strips
    hosts: [mongo-1, mongo-2, mongo-3]
    commands:
      - sudo -i                       # password typed once, applied to all
```

`arrange:` accepts any of tmux's built-in layouts: `tiled`, `even-horizontal`,
`even-vertical`, `main-horizontal`, `main-vertical`. Both `sync:` and
`arrange:` work at the session and window level (window overrides session).

### Multiple windows

```yaml
sessions:
  staging:
    root: ~/staging
    windows:
      - name: backend
        hosts: [api1, api2, api3]
      - name: frontend
        hosts: [web1, web2]
```

### Custom layout

`split: root` marks the first pane (no actual split — it's the pane that
exists when the window is created). `split: horizontal` stacks the new pane
under the previous; `split: vertical` places it side-by-side. `size:` is
a percentage of the parent pane (1–99).

```yaml
sessions:
  monitoring:
    windows:
      - name: system
        panes:
          - split: root
            commands: [htop]
          - split: horizontal
            size: 30
            commands: [df -h]
```

### Reusable layouts

```yaml
layouts:
  two-pane:
    panes:
      - split: root
      - split: vertical
        size: 30

sessions:
  dev:
    windows:
      - name: logs
        layout: two-pane
```

See `examples/config.yml` for more.

## Commands

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

### Cluster expansion (`@name`)

Anywhere `mox new` accepts a host, `@name` expands to a host list.
Lookup order:

1. The `hosts:` of a mox configured session named `name` (complex sessions
   are flattened across all their simple-mode windows)
2. The `name` entry in `~/.clusterssh/clusters` (or `$CSSH_CLUSTERS`, or
   `/etc/clusters`). Nested clusters are expanded recursively with cycle
   detection.

When `name` exists in both, the mox config wins. Mix freely with literal
hosts on the same line.

### Shell completion

`make install` does this automatically for your `$SHELL`. To install it
standalone, or after `go install`:

```bash
make install-completion         # detects $SHELL, installs to user dir

# or manually:
mox completion bash > ~/.local/share/bash-completion/completions/mox
mox completion zsh  > ~/.zsh/completions/_mox
mox completion fish > ~/.config/fish/completions/mox.fish
```

### Global flags

- `-c, --config <path>` — override the config path (`~` expanded)
- `-v, --verbose` — debug logging to stderr
- `-q, --quiet` — only warnings and errors
- `--force` — `mox -a <session> --force` rebuilds the session even if it's running

### Behavior notes

When `-a` attaches from **inside** an existing tmux client (`$TMUX` set),
mox uses `switch-client` instead of `attach-session`, so nested-session
errors are avoided automatically.

`Ctrl-C` during session creation cancels cleanly: the partial session is
killed before the program exits.

## Validation rules

- Session and window names cannot contain `: . $` or whitespace (these are
  special in tmux's target syntax)
- Hostnames in the default `ssh {{host}}` template must match
  `^[A-Za-z0-9._%@-]+$`. To use other characters, override `connect:` with
  a template that handles your own escaping
- Unknown YAML keys are an error (catches typos like `hots:` for `hosts:`)
- A session can define `hosts`, `windows`, or **neither** (a session with
  neither opens a single local pane — useful for `commands:`-only workflows)
- Inside a window, `hosts` is mutually exclusive with `panes` and `layout`,
  and `panes` is mutually exclusive with `layout`
- Multiple windows in a session may share the same name (tmux addresses
  windows by id, not name)

## Security

The config file is treated as trusted user input — `commands:` and the
`connect:` template are executed in the spawned tmux pane via `send-keys`.
Sharing a config is equivalent to sharing arbitrary shell commands.

See `SECURITY.md` for vulnerability reporting.

## Development

```bash
make build         # build to ./build/mox
make test          # unit tests
make integration   # tests requiring a live tmux server
make lint          # golangci-lint
make vuln          # govulncheck scan
```

CI runs the unit tests + lint + govulncheck on every push, plus integration
tests on Linux. See `CONTRIBUTING.md` for details.

## License

MIT
