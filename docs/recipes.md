# Recipes

Common workflows by example. Every short flag has a long form
(`mox new --help` for the full table).

## Attach to a configured session

```bash
mox -a dev                      # build it if it isn't running, then attach
mox -a dev --force              # tear down the running one and rebuild
```

`-a` also attaches to **any** running tmux session, even ones not in your
mox config — useful for hand-rolled sessions you started by hand.

## Quick local session

```bash
mox new                         # plain shell in a new tmux session (auto-named tmp-<timestamp>)
mox new -n work                 # named "work"
mox new -n work -r ~/proj       # named "work", starting in ~/proj
mox new -t                      # destroyed automatically when you detach
```

## Multi-host admin (cssh-style)

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

## Open as a window in your current tmux session

If you're already inside tmux and don't want a separate session:

```bash
mox new -w @api-cluster             # new window in the current session
```

## Clone a configured session, override hosts

```bash
mox new --from api-cluster host42   # use api-cluster's settings, but just this one host
```

## Capture a hand-rolled tmux session into config

If you built a session interactively and want to recreate it later:

```bash
mox import work                 # add 'work' to your mox config under the same name
mox import work -n my-work      # rename on import
mox import work -p              # preview the YAML on stdout without saving
mox import work -F              # overwrite an existing config entry
```

The same capture is one key away in the hub: run bare `mox`, highlight a
`◆` tmux-only session, and press `i`.

SSH connections are recovered from the OS process table: a window whose
panes are all plain `ssh host` connections imports as a simple `hosts:`
list, and other ssh panes keep their connection as a `commands:` entry.
Anything else (editors, REPLs, scripts you typed) isn't recoverable from
tmux's running state — add `commands:` entries afterward to make those
panes fully reproducible.

## Preview what mox will do

`--print` emits the tmux commands a build would run, one line each,
without touching your tmux server:

```bash
mox new @api-cluster --print        # inspect the full build
mox -a dev --print              # same for a configured session
```

## Project-local config

Drop a `.mox.yml` in a project directory and every mox command run from
there uses it instead of the global config (a stderr notice says so).
An explicit `--config` always wins. Great for keeping a project's session
layout versioned with its code.

## Bounce between two sessions

`mox last` attaches to the session you used before this one — the session
equivalent of `cd -`. Bind it inside tmux to flip back and forth:

```
bind-key L run-shell "mox last"
```

## Migrate from clusterssh

If you already maintain `~/.clusterssh/clusters`, you don't have to
duplicate it. mox reads that file directly:

```bash
mox new @monitoring             # expands the clusterssh entry by that name
mox new @pve                    # nested clusterssh tags work too
```

Configured sessions in `~/.config/mox/config.yml` take precedence when a
name exists in both places.
