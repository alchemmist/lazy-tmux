// Package claude integrates Claude Code with lazy-tmux: it captures the resumable
// session id of a running `claude` pane at save time and restores it as
// `claude --resume <id>` so the conversation continues where it left off.
package claude

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	// metaSessionID is the (un-namespaced) metadata key for the resolved session.
	metaSessionID = "session_id"
	transcriptExt = ".jsonl"
)

// Integration resolves and replays Claude Code sessions, and reports their live
// status. home is the Claude data directory (default ~/.claude); transcripts
// live under <home>/projects/<cwd>/. statusDir is lazy-tmux's directory of
// hook-written status files (see status.go).
type Integration struct {
	home      string
	statusDir string
}

// New builds the integration rooted at the given Claude data directory, writing
// and reading live-status files under statusDir.
func New(home, statusDir string) *Integration {
	return &Integration{home: home, statusDir: statusDir}
}

func (i *Integration) Name() string { return "claude" }

// Matches reports whether the pane is running Claude Code. Claude is a Node app,
// so pane_current_command can surface as "node"; matching the executable name or
// a "claude" token in the captured command keeps detection robust, and a missed
// match simply falls back to the default restore.
func (i *Integration) Matches(pane snapshot.Pane) bool {
	for _, cmd := range []string{pane.RestoreCmd, pane.CurrentCmd} {
		cmd = strings.ToLower(strings.TrimSpace(cmd))
		if cmd == "" {
			continue
		}

		if executableName(cmd) == "claude" || strings.Contains(cmd, "claude") {
			return true
		}
	}

	return false
}

// Capture resolves the pane's most recent Claude session id from its working
// directory. A missing project dir or transcript yields nil (no metadata), so
// restore falls back to a plain `claude`.
func (i *Integration) Capture(pane snapshot.Pane) (map[string]string, error) {
	sessionID, ok := i.latestSessionID(pane.CurrentPath)
	if !ok {
		// Empty (not nil) so the no-metadata case never trips the nilnil lint;
		// the registry treats a zero-length map as "nothing captured".
		return map[string]string{}, nil
	}

	return map[string]string{metaSessionID: sessionID}, nil
}

// RestoreCommand replays the captured session, or "" to fall back to default.
func (i *Integration) RestoreCommand(_ snapshot.Pane, meta map[string]string) string {
	id := strings.TrimSpace(meta[metaSessionID])
	if id == "" {
		return ""
	}

	return "claude --resume " + id
}

// latestSessionID maps a working directory to <home>/projects/<encoded-cwd> and
// returns the newest transcript's session id (its filename without .jsonl).
func (i *Integration) latestSessionID(cwd string) (string, bool) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || strings.TrimSpace(i.home) == "" {
		return "", false
	}

	dir := filepath.Join(i.home, "projects", EncodeProjectDir(cwd))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}

	var (
		newestID   string
		newestTime int64
		found      bool
	)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), transcriptExt) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if mod := info.ModTime().UnixNano(); !found || mod > newestTime {
			newestTime = mod
			newestID = strings.TrimSuffix(entry.Name(), transcriptExt)
			found = true
		}
	}

	return newestID, found
}

// EncodeProjectDir mirrors Claude Code's per-project directory naming: the
// absolute cwd with path separators and dots replaced by "-", e.g.
// "/Users/me/code/lazy-tmux" -> "-Users-me-code-lazy-tmux".
func EncodeProjectDir(cwd string) string {
	replacer := strings.NewReplacer("/", "-", ".", "-")
	return replacer.Replace(cwd)
}

func executableName(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}

	base := filepath.Base(fields[0])

	return strings.TrimPrefix(base, "-")
}
