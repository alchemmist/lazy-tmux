package codex

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const statusReadBlockSize int64 = 64 * 1024

type rolloutEvent struct {
	Type    string `json:"type"`
	Payload struct {
		Type string `json:"type"`
	} `json:"payload"`
}

func (i *Integration) Status(pane snapshot.Pane) (integration.Status, bool) {
	if !i.Matches(pane) {
		return integration.StatusUnknown, false
	}

	root, path, ok := i.sessionFile(pane)
	if !ok {
		return integration.StatusUnknown, false
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(path)
	if err != nil {
		return integration.StatusUnknown, false
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return integration.StatusUnknown, false
	}

	return latestRolloutStatus(file, info.Size())
}

func (i *Integration) sessionFile(pane snapshot.Pane) (*os.Root, string, bool) {
	sessionID, ok := i.SessionID(pane)
	if !ok || strings.TrimSpace(i.home) == "" {
		return nil, "", false
	}

	root, err := os.OpenRoot(i.home)
	if err != nil {
		return nil, "", false
	}
	if path, ok := i.cachedSessionPath(sessionID); ok {
		return root, path, true
	}

	pattern := filepath.ToSlash(filepath.Join("sessions", "*", "*", "*", "*"+sessionID+".jsonl"))
	matches, err := fs.Glob(root.FS(), pattern)
	if err != nil || len(matches) == 0 {
		_ = root.Close()

		return nil, "", false
	}

	path := matches[len(matches)-1]
	i.cacheSessionPath(sessionID, path)

	return root, path, true
}

func (i *Integration) cachedSessionPath(sessionID string) (string, bool) {
	i.pathsMu.RLock()
	defer i.pathsMu.RUnlock()

	path, ok := i.sessionPaths[sessionID]

	return path, ok
}

func (i *Integration) cacheSessionPath(sessionID, path string) {
	i.pathsMu.Lock()
	defer i.pathsMu.Unlock()

	i.sessionPaths[sessionID] = path
}

func latestRolloutStatus(file *os.File, size int64) (integration.Status, bool) {
	offset := size
	remainder := []byte{}

	for offset > 0 {
		blockSize := min(statusReadBlockSize, offset)
		offset -= blockSize
		block := make([]byte, blockSize)
		_, err := file.ReadAt(block, offset)
		if err != nil {
			return integration.StatusUnknown, false
		}

		data := make([]byte, 0, len(block)+len(remainder))
		data = append(data, block...)
		data = append(data, remainder...)
		lines := bytes.Split(data, []byte{'\n'})
		firstComplete := 0
		if offset > 0 {
			remainder = bytes.Clone(lines[0])
			firstComplete = 1
		}

		for lineIndex := len(lines) - 1; lineIndex >= firstComplete; lineIndex-- {
			if status, ok := rolloutLineStatus(lines[lineIndex]); ok {
				return status, true
			}
		}
	}

	return integration.StatusIdle, true
}

func rolloutLineStatus(line []byte) (integration.Status, bool) {
	if !bytes.Contains(line, []byte(`"type":"`)) {
		return integration.StatusUnknown, false
	}

	var event rolloutEvent
	if json.Unmarshal(line, &event) != nil || event.Type != "event_msg" {
		return integration.StatusUnknown, false
	}

	switch event.Payload.Type {
	case "task_started", "turn_started", "exec_command_begin", "exec_command_end",
		"patch_apply_begin", "patch_apply_end", "web_search_begin", "web_search_end":
		return integration.StatusWorking, true
	case "exec_approval_request", "apply_patch_approval_request":
		return integration.StatusAwaitingDecision, true
	case "request_user_input":
		return integration.StatusAwaitingInput, true
	case "task_complete", "turn_complete", "turn_aborted":
		return integration.StatusAwaitingInput, true
	case "error":
		return integration.StatusError, true
	default:
		return integration.StatusUnknown, false
	}
}
