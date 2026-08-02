package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const metaSessionID = "session_id"

type Integration struct {
	home string
}

func New(home string) *Integration { return &Integration{home: home} }

func (i *Integration) Name() string { return "codex" }

func (i *Integration) Matches(pane snapshot.Pane) bool {
	for _, cmd := range []string{pane.RestoreCmd, pane.CurrentCmd} {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		if executableName(cmd) == "codex" || strings.Contains(strings.ToLower(cmd), "codex") {
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

	return "codex resume " + id
}

type sessionMetaLine struct {
	Type    string `json:"type"`
	Payload struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"payload"`
}

type sessionCandidate struct {
	id      string
	modTime int64
}

func (i *Integration) latestSessionID(cwd string) (string, bool) {
	cwd = strings.TrimSpace(cwd)
	root := filepath.Join(i.home, "sessions")
	if cwd == "" || strings.TrimSpace(i.home) == "" {
		return "", false
	}

	var newest sessionCandidate
	found := false
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}

		candidate, ok := readCandidate(path, cwd)
		if !ok || (found && candidate.modTime <= newest.modTime) {
			return nil
		}

		newest = candidate
		found = true
		return nil
	})
	if err != nil || !found {
		return "", false
	}

	return newest.id, true
}

func readCandidate(path, cwd string) (sessionCandidate, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return sessionCandidate{}, false
	}

	file, err := os.Open(path) // #nosec G304 -- path is discovered below the configured Codex home
	if err != nil {
		return sessionCandidate{}, false
	}
	defer func() { _ = file.Close() }()

	var meta sessionMetaLine
	if err := json.NewDecoder(bufio.NewReader(file)).Decode(&meta); err != nil {
		return sessionCandidate{}, false
	}
	if meta.Type != "session_meta" || strings.TrimSpace(meta.Payload.ID) == "" || meta.Payload.CWD != cwd {
		return sessionCandidate{}, false
	}

	return sessionCandidate{id: meta.Payload.ID, modTime: info.ModTime().UnixNano()}, true
}

func executableName(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}

	base := filepath.Base(fields[0])
	return strings.TrimPrefix(base, "-")
}
