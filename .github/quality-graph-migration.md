# Quality Graph migration inventory

This document records the mapping from the repository's existing GitHub Actions
workflows to Quality Graph.

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

All migrated checks remain independent, matching the existing workflow
parallelism. They use read-only repository permissions and checkout without
persisted credentials. `docs-build` checks that the site compiles. Publishing
stays in `pages.yml`.

## Retained workflows

- `pages.yml` remains the deployment workflow for the documentation site and
  keeps its Pages and OIDC permissions.
- `release.yml` remains the tag-triggered publication workflow and keeps its
  release credentials and write permissions.

Quality Graph 0.1.6 does not support event-specific nodes. The `tmux-versions`
node emits a skipped result on pushes and runs the Docker matrix only for pull
requests. Its native result publishes the complete version table in the job
summary. The node compares supported versions with the pull request's base and
fails when a version previously supported on the base is no longer supported.

Rollback is restoring the standalone workflow and its required check.
