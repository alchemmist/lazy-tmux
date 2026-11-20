# Dependency and secret gates

Unlike monori's Python environment and regenerated npm locks, these gates check
what the PR actually resolves. `scripts/audit-quality.sh go` verifies downloaded
modules and checks reachable vulnerable symbols in both Go build variants
against the public Go vulnerability database. Use the Go version in `go.mod`
(including its security patch level) on PATH. `npm` audits the committed docs
lock, including development/build dependencies, and blocks high/critical
findings. A clean gate does not imply there are no lower-severity advisories.
At adoption the React Router v6 SSG chain has moderate advisories
GHSA-wrjc-x8rr-h8h6 and GHSA-337j-9hxr-rhxg; npm offers no compatible fix.
Review the router/SSG migration separately rather than forcing incompatible
peer dependencies or hiding advisories. `secrets` runs checksum-pinned Gitleaks
against files in the HEAD tree with full redaction. It is not a history scan.
All three gates need network access; a network/tool error fails the command
rather than passing it as a clean audit. No deployment credentials are used.
