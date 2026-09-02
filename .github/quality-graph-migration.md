# Quality Graph migration inventory

This document records the bootstrap mapping from the repository's existing
GitHub Actions workflows to Quality Graph 0.1.2.

| Existing check | Command | Runner and setup | Quality Graph node |
| --- | --- | --- | --- |
| `build / build` | `make build` | Ubuntu, checkout, Go from `go.mod` | `build` |
| `coverage / coverage` | `make setup-env`, then `make test-cov` | Ubuntu, checkout, Go from `go.mod`, Podman | `coverage` |
| `golangci-lint / golangci-lint` | `make setup-env`, then `make golangci-lint` | Ubuntu, checkout, Go from `go.mod` | `golangci-lint` |
| `test / test` | `make setup-env`, then `make test` | Ubuntu, checkout, Go from `go.mod` | `test` |
| `test / integration` | `make setup-env`, then `make integration-test` | Ubuntu, checkout, Go from `go.mod`, Podman | `integration` |

All migrated checks remain independent, matching the existing workflow
parallelism. They use read-only repository permissions and checkout without
persisted credentials. The old workflows remain active during parity testing.

## Retained workflows

- `tmux-versions.yml` remains outside the graph. Its pull-request-only jobs
  compare the head and base revisions, exchange custom artifacts, and update a
  pull-request comment with write permission. Quality Graph 0.1.2 cannot
  preserve that trigger and reporting behavior without changing semantics or
  duplicating the expensive version test.
- `pages.yml` remains the deployment workflow for the documentation site and
  keeps its Pages and OIDC permissions.
- `release.yml` remains the tag-triggered publication workflow and keeps its
  release credentials and write permissions.

After a representative parity pull request and a post-bootstrap probe succeed,
branch protection can move to the aggregate `Quality Graph` check. The old
`build.yml`, `coverage.yml`, `golangci-lint.yml`, and `test.yml` workflows can
then be removed in a separate cleanup change. Rollback is restoring those
workflow checks as required checks.
