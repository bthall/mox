# Configuration

mox builds tmux sessions from a YAML config file.

## Config file location

The default config path follows the XDG Base Directory spec:

- `$XDG_CONFIG_HOME/mox/config.yml` if set, otherwise
- `~/.config/mox/config.yml`

Override via the `--config` flag (which expands `~`).

The file is created with mode `0600` (owner read/write only) since it can
list hostnames and shell commands.

### Editor autocomplete and validation

A JSON Schema for the config format is published at
[`schema/mox.schema.json`](../schema/mox.schema.json). Configs scaffolded by
`mox init` start with a modeline that LSP-aware editors (VS Code with the
YAML extension, neovim with yamlls, and others) pick up automatically:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/bthall/mox/main/schema/mox.schema.json
```

That gives field completion, typo squiggles, and hover documentation while
editing. Add the line to an existing config to get the same. It's a plain
YAML comment — editors without LSP support ignore it, and mox's own
validation remains the source of truth.

### Project-local config

When a `.mox.yml` exists in the current directory and no `--config` was
given, it wins over the global config for every command (a stderr notice
says so). Project session definitions can live in the repo they belong to,
versioned with the code. An explicit `--config` always overrides.

## Simple session

One window, one pane per host:

```yaml
sessions:
  devenv:
    hosts: [api, web, worker]
    root: ~/projects/myapp
    commands:
      - echo "Welcome!"
```

## Custom connect command

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

## Synchronized panes and tiled layouts (cssh-style)

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

## Lifecycle hooks and pre commands

`on_start` runs locally (in order) before the session is built — a failing
command aborts creation. `on_stop` runs after `mox kill` destroys the
session. `pre` commands are prepended to every pane's command list; a
window-level `pre` runs after the session-level one.

```yaml
sessions:
  staging:
    on_start:
      - vpn-up staging          # non-zero exit aborts creation
    on_stop:
      - vpn-down staging        # best-effort, never blocks the kill
    pre:
      - export DEPLOY_ENV=staging
    hosts: [app1, app2]
```

## Connection holding and retry

When a host pane's connection ends — failure or clean exit — the pane
prints a notice and waits for Enter before closing, so a `sync` window can
never broadcast keystrokes into a local shell by accident. `hold: false`
restores the old drop-to-shell behavior; `retry: N` re-attempts failed
connections (3s apart, clean exits never retry):

```yaml
sessions:
  flaky-lab:
    hosts: [lab1, lab2]
    retry: 3                    # 4 attempts total per host
  quick-look:
    hosts: [box1]
    hold: false                 # ended connection drops to a local shell
```

Both keys work at the window level too, and `mox new` accepts them as
`--hold=false` / `--retry N`.

## Multiple windows

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

## Custom layout

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

## Reusable layouts

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

See [`examples/config.yml`](../examples/config.yml) for more.

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
- `retry` must be between 0 and 10
- Multiple windows in a session may share the same name (tmux addresses
  windows by id, not name)

## Security

The config file is treated as trusted user input — `commands:` and the
`connect:` template are executed in the spawned tmux pane via `send-keys`.
Sharing a config is equivalent to sharing arbitrary shell commands.

See [`SECURITY.md`](../SECURITY.md) for vulnerability reporting.
