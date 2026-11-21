# Quality Graph migration inventory

All repository-owned quality jobs are declared in `qg.yaml`. The legacy build,
coverage, golangci-lint and test workflows have been removed; the old check
names below are historical. Do not add parallel standalone CI jobs.

| Existing check | Command | Runner and setup | Quality Graph node |
| --- | --- | --- | --- |
| `build / build` | `make build-all` (default and `lazy_fzf` binaries) | Ubuntu, checkout, Go from `go.mod` | `build` |
| `coverage / coverage` | `make setup-env`, then `make test-cov` | Ubuntu, checkout, Go from `go.mod`, Podman | `coverage` |
| `golangci-lint / golangci-lint` | `make setup-env`, then `make golangci-lint` | Ubuntu, checkout, Go from `go.mod` | `golangci-lint` |
| `test / test` | `make setup-env`, then `make test` (JUnit `.cache/quality-graph/test.xml`) | Ubuntu, checkout, Go from `go.mod` | `test` |
| fzf variant tests | `make setup-env`, then `make test-fzf` (JUnit `.cache/quality-graph/test-fzf.xml`) | Ubuntu, checkout, Go from `go.mod` | `test-fzf` |
| docs site build | `make docs-install && make docs-build` | Ubuntu, checkout, Node 26 with `docs/package-lock.json` npm cache | `docs-build` |
| `test / integration` | `make integration-test` | Ubuntu, checkout only. The container supplies Go, tmux, and tools; the host does not install a Go toolchain | `integration` |
| `tmux version test / test-pr` | `make test-sup-versions` | Ubuntu, checkout, Docker | `tmux-versions` |

## Why these edges exist

`workflow-graph` validates the declaration first. Cheap, independent signals
then fan out: `build` (both binaries), `fast-tests` (including picker goldens),
`workflow-shell-lint`, `nolint-exclusions`, and `secrets`.

- **Go policy:** validate new exceptions before GolangCI-Lint. A lint failure
  doesn't hide runtime test results, but prevents release rehearsal.
- **Runtime:** default race tests follow compilation and fast tests. Fzf race
  tests only require compilation: a broken TUI golden should not hide fzf
  results. Integration needs working binaries, smoke tests and a sound harness;
  only then start the costly upstream tmux matrix. Coverage reruns the default
  suite in the integration environment, so waits for both suites. Performance
  needs passing default tests and a checked measurement script, not coverage.
- **Dependencies/docs:** shell lint precedes the scripted audits. Docs build
  follows the npm lock audit, independently of Go tests and Go advisories.
- **Packaging:** both variants' tests, Go lint, Go audit and secrets must pass
  before rehearsing publication. Packaging, coverage and the tmux matrix do
  not wait on each other. Pages and release publication stay separate.

Edges gate eligibility; GitHub runs jobs on independent runners, so `needs`
never implies that binary files or coverage profiles are shared. The graph
trades roughly one integration run of extra latency on a green PR for not
spending the entire version matrix on a known broken integration environment.

## Retained workflows

- `pages.yml` remains the deployment workflow for the documentation site and
  keeps its Pages and OIDC permissions.
- `release.yml` remains the tag-triggered publication workflow and keeps its
  release credentials and write permissions.

This graph retains the same nodes across events. The `tmux-versions`
node emits a skipped result on pushes and compares the Docker matrix on pull
requests. On workflow dispatch it runs the full repeated matrix instead. Its native result publishes the complete version table in the job
summary. The node compares supported versions with the pull request's base and
fails when a version previously supported on the base is no longer supported.

`scheduled-verification.yml` is a timer only: it dispatches the generated graph
on main and never checks out or runs project code. For a manual full run use
the Quality Graph workflow itself. GitHub CodeQL default setup is a separately
managed platform security service, not a repository workflow.

## Workflow and script lint

`workflow-shell-lint` runs inside Quality Graph.
`make test-lint-actions-shell` downloads checksum-pinned actionlint and
shellcheck, then lints workflows and shell scripts. Generated Quality Graph
workflows stay owned by `qg generate`.

`coverage` reuses `cover.out` for an informational changed-line summary (Go
cover block spans, not a statement-by-statement measurement). The existing
50% repository threshold still blocks. `nolint-exclusions` compares against
merge-base, requiring a named linter and reason on new directives. Added or
modified exclusion entries fail conservatively and need explicit policy review.
`packaging` exercises the snapshot on packaging configuration changes. Daily
and manual scheduled verification also tests snapshots and repeats the full
compatibility matrix. Neither workflow publishes release artifacts.
