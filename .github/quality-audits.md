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

## Performance and screen regression

Instead of copying monori's browser performance score or comparing timings
from different machines, run `python3 scripts/performance_diff.py origin/main`.
It measures the real process snapshot/foreground lookup for 2,000 processes
and the large picker's spinner update. After warmup it alternates base/head
on the same runner. A median paired increase over 25% blocks only when at
least four of five pairs exceed 15%. This is a coarse regression gate, not an
end-to-end latency SLA. Review benchmark fixture edits as measurement changes.

For screen regressions run `go test ./internal/picker -run TestPickerScreens`.
Goldens capture Bubble Tea view content without ANSI styling, not screenshots
of a specific emulator. They preserve borders, whitespace and clipping, but
not colors or cursor timing. Update intentionally outside CI with
`UPDATE_PICKER_SCREENS=1 go test ./internal/picker -run TestPickerScreens -count=1`
and inspect `git diff -- internal/picker/testdata/screens`.
