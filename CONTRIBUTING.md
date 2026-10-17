# Contributing to mox

Thanks for your interest in contributing! This document covers the basics for
getting a development environment set up and the conventions used in this
repository.

## Prerequisites

- **Go** - the version declared in `go.mod` (CI builds with the same
  version).
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

`make screenshots` regenerates the README captures from deterministic demo
fixtures in the actual UI models. It requires `freeze` v0.2.2 and
`rsvg-convert` (the `librsvg2-bin` package on Debian/Ubuntu), and Python 3.
Each PNG records its source commit, model-frame hash, and renderer in PNG text
metadata. Regenerate captures from a clean checkout to keep that source accurate.
The UI preview
workflow also captures 40×12, 80×24, and 120×35 terminals, light and no-color
views, and review/error states as downloadable PR artifacts.

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

## Releases

Move the completed changelog entries into a dated release section and add the
release notes at `.github/release-notes/vX.Y.Z.md`. Merge the preparation and
wait for every main CI job to pass on that exact commit.

The Release workflow accepts a version and full commit SHA through **Run
workflow**. A connected tool can also commit `.github/release-request.json` on
main with the same inputs:

```json
{"version": "vX.Y.Z", "commit": "<full SHA of the prepared, verified main commit>"}
```

The workflow checks the requested commit's CI, verifies it belongs to main,
tests it, creates its tag, and runs GoReleaser in the same job. A tag created
with `GITHUB_TOKEN` does not trigger another release job. Existing tag-push
releases continue to work. The workflow refuses to move an existing tag.

After publishing, it downloads and checks all release archives, verifies the
Linux binary's version and commit, and prints the source-archive SHA256. Use
that hash to update `packaging/aur/PKGBUILD` after the tag exists.

The release then calls the Arch packaging workflow. It packages the verified
Linux binaries and publishes native packages on the same GitHub release.
The signed pacman repository additionally requires `ARCH_REPO_SIGNING_KEY`;
see [Arch distribution](docs/arch-linux.md) for setup and backfilling a release.

## Reporting security issues

Please do **not** report security issues in public. See [SECURITY.md](./SECURITY.md).
