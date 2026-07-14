<p align="center">
  <img src="assets/logo.png" alt="mox logo" width="200">
</p>

<h1 align="center">mox</h1>

<p align="center">
  <a href="https://github.com/bthall/mox/actions/workflows/ci.yml"><img src="https://github.com/bthall/mox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/bthall/mox/releases/latest"><img src="https://img.shields.io/github/v/release/bthall/mox" alt="Latest release"></a>
  <a href="https://pkg.go.dev/github.com/bthall/mox"><img src="https://pkg.go.dev/badge/github.com/bthall/mox.svg" alt="Go Reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/bthall/mox" alt="License"></a>
</p>

A CLI tool for creating and managing tmux sessions declaratively from YAML
configuration files — with cssh-style multi-host broadcast built in.

Run `mox` and pick a session:

<p align="center">
  <img src="assets/screenshot-picker.png" alt="mox session picker" width="850">
</p>

Or see everything at a glance with `mox list`:

<p align="center">
  <img src="assets/screenshot-list.png" alt="mox list output" width="700">
</p>

## Features

- **Declarative YAML config** — one window per host, full custom layouts, or anything between; project-local `.mox.yml` overrides
- **Cssh-style broadcast** — `sync: true` for synchronized typing; tiled layouts; `sudo -i` once for every pane
- **Ad-hoc sessions** — `mox new @cluster` or `mox new host1 host2` without touching config; `-x` excludes hosts
- **Session picker** — bare `mox` opens a fuzzy-filterable list with a live preview pane
- **Import** — capture a running session's structure *and its SSH connections* into config
- **Broadcast safety** — an ended connection holds its pane instead of dropping to a local shell; optional retry
- **Lifecycle hooks** — `on_start`/`on_stop` run locally around a session; `pre` seeds every pane
- **Recents** — `mox recent` remembers what you used; `mox last` is `cd -` for sessions
- **Dry-run** — `--print` shows the exact tmux commands without running them
- **Honest defaults** — single binary, no daemon, strict config validation; the only state is a small recents history

## Install

```bash
# Go
go install github.com/bthall/mox/cmd/mox@latest

# From source (also installs shell completion)
git clone https://github.com/bthall/mox.git && cd mox && make install

# Arch Linux (source build)
cd packaging/aur && makepkg -si
```

Pre-built archives for linux/macOS × amd64/arm64 are attached to each
[release](https://github.com/bthall/mox/releases) with `checksums.txt`.

## Quick start

```bash
mox init                        # scaffold a config at ~/.config/mox/config.yml
mox edit                        # open it in $EDITOR, validated on save
mox -a example                  # build + attach to the "example" session
mox                             # or pick a session interactively
mox new @webfarm                # ad-hoc broadcast session on a cluster
mox kill example                # destroy a running session
```

## Documentation

- **[Configuration](docs/configuration.md)** — the YAML schema: sessions, windows, layouts, connect templates, hooks, holding/retry, validation rules
- **[Commands](docs/commands.md)** — every command and flag, the picker, cluster expansion, dry-run, shell completion
- **[Recipes](docs/recipes.md)** — copy-paste workflows, from quick local sessions to clusterssh migration

## Contributing

`make test`, `make lint`, and `make integration` cover the local loop; CI
runs all three plus govulncheck on every push. See
[`CONTRIBUTING.md`](CONTRIBUTING.md) and [`SECURITY.md`](SECURITY.md).

## License

MIT
