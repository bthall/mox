# Security Policy

## Supported Versions

Only the latest published tag and the `main` branch receive security fixes.
If you are running an older release, please upgrade before reporting an issue.

| Version       | Supported          |
| ------------- | ------------------ |
| latest tag    | yes                |
| `main` branch | yes                |
| anything else | no                 |

## Reporting a Vulnerability

Please report security vulnerabilities **privately** via
[GitHub Security Advisories](https://github.com/bthall/mox/security/advisories/new).

Do not open a public issue, pull request, or discussion thread for a suspected
vulnerability. We will acknowledge your report within a few days and work with
you on a coordinated disclosure timeline.

## Threat Model

`mox` treats its configuration file as **trusted user input**. The config
file describes sessions, windows, panes, hostnames, and shell commands that
will be executed verbatim inside a tmux session on the user's machine.

As a consequence:

- **Sharing a `mox` config with someone is equivalent to sharing arbitrary
  shell commands with them.** Inspect any config you did not author before
  loading it, exactly as you would inspect a shell script before running it.
- `mox` does not sandbox user-provided commands, hostnames, or pane
  contents, and it is not designed to.
- Input validation in `mox` (e.g. hostname character restrictions, session
  name restrictions) exists to prevent *accidental* corruption of tmux state
  or unintended keystroke injection - not to defend against a hostile config
  author.

If you find a way for a `mox` config to escape the user's expected level
of trust (for example: a config that appears benign but causes commands to
run outside the documented session/pane it describes), that is a security
issue and should be reported per the section above.
