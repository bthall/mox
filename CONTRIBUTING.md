# Contributing to mox

Thanks for your interest in contributing! This document covers the basics for
getting a development environment set up and the conventions used in this
repository.

## Prerequisites

- **Go 1.24+** - the module declares `go 1.24` and CI runs on 1.24.
- **tmux 3.1+** - required at runtime and for the integration test suite.
  `mox` uses tmux 3.1 features such as `split-window -l <n>%`.
- **golangci-lint** - used by `make lint` and by CI. Install from
  <https://golangci-lint.run/usage/install/>.
- **goreleaser** *(optional)* - only needed to cut releases or run
  `make release-snapshot` locally. Install from <https://goreleaser.com>.

## Common tasks

```sh
make build         # build the binary into ./build/mox
make test          # run the unit test suite
make integration   # run integration tests (requires tmux on PATH)
make lint          # run golangci-lint
make fmt           # gofmt the tree
make vuln          # run govulncheck against the module
```

Integration tests are gated behind a build tag so they don't run in plain
`go test ./...`. To run them directly:

```sh
go test -tags=integration ./...
```

## Pull requests

- Open PRs against `main`.
- Please ensure `make test` and `make lint` pass before requesting review.
- New behavior should come with a test where practical.
- Update `CHANGELOG.md` under `## [Unreleased]` for any user-visible change.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org/) are encouraged
but not required - they make the goreleaser-generated changelog cleaner. The
release changelog excludes commits prefixed with `chore:`, `docs:`, and
`test:`.

## Reporting security issues

Please do **not** report security issues in public. See [SECURITY.md](./SECURITY.md).
