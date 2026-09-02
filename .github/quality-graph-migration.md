# Quality Graph migration inventory

This document records the mapping from the repository's existing GitHub Actions
workflows to Quality Graph.

| Existing check | Command | Runner and setup | Quality Graph node |
| --- | --- | --- | --- |
| `build / build` | `make build` | Ubuntu, checkout, Go from `go.mod` | `build` |
| `coverage / coverage` | `make setup-env`, then `make test-cov` | Ubuntu, checkout, Go from `go.mod`, Podman | `coverage` |
| `golangci-lint / golangci-lint` | `make setup-env`, then `make golangci-lint` | Ubuntu, checkout, Go from `go.mod` | `golangci-lint` |
| `test / test` | `make setup-env`, then `make test` | Ubuntu, checkout, Go from `go.mod` | `test` |
| `test / integration` | `make setup-env`, then `make integration-test` | Ubuntu, checkout, Go from `go.mod`, Podman | `integration` |
| `tmux version test / test-pr` | `make test-sup-versions` | Ubuntu, checkout, Docker | `tmux-versions` |

All migrated checks remain independent, matching the existing workflow
parallelism. They use read-only repository permissions and checkout without
persisted credentials.

## Retained workflows

- `pages.yml` remains the deployment workflow for the documentation site and
  keeps its Pages and OIDC permissions.
- `release.yml` remains the tag-triggered publication workflow and keeps its
  release credentials and write permissions.

Quality Graph 0.1.6 does not support event-specific nodes. The `tmux-versions`
node emits a skipped result on pushes and runs the Docker matrix only for pull
requests. Its native result publishes the complete version table in the job
summary, and any unsupported tested version fails the node.

Rollback is restoring the standalone workflow and its required check.
