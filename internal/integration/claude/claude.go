package claude

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	metaSessionID = "session_id"
	transcriptExt = ".jsonl"
)

type Integration struct {
	home      string
	statusDir string
}

func New(home, statusDir string) *Integration {
	return &Integration{home: home, statusDir: statusDir}
}

func (i *Integration) Name() string { return "claude" }

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

func (i *Integration) Capture(pane snapshot.Pane) (map[string]string, error) {
	sessionID, ok := i.latestSessionID(pane.CurrentPath)
	if !ok {
		return map[string]string{}, nil
	}

	return map[string]string{metaSessionID: sessionID}, nil
}

func (i *Integration) RestoreCommand(_ snapshot.Pane, meta map[string]string) string {
	id := strings.TrimSpace(meta[metaSessionID])
	if id == "" {
		return ""
	}

	return "claude --resume " + id
}

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
