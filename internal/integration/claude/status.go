package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	StateWorking          = "working"
	StateAwaitingDecision = "awaiting_decision"
	StateAwaitingInput    = "awaiting_input"
	StateIdle             = "idle"
)

type statusFile struct {
	State     string `json:"state,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

func (i *Integration) Status(pane snapshot.Pane) (integration.Status, bool) {
	cwd := strings.TrimSpace(pane.CurrentPath)
	if cwd == "" {
		return integration.StatusUnknown, false
	}

	if status, ok := i.statusFromHook(cwd); ok {
		return status, true
	}

	return i.statusFromSessionFile(cwd)
}

func (i *Integration) statusFromHook(cwd string) (integration.Status, bool) {
	if strings.TrimSpace(i.statusDir) == "" {
		return integration.StatusUnknown, false
	}

	path := filepath.Join(i.statusDir, EncodeProjectDir(cwd)+".json")

	var file statusFile
	if !readJSONFile(path, &file) {
		return integration.StatusUnknown, false
	}

	if file.CWD != "" && file.CWD != cwd {
		return integration.StatusUnknown, false
	}

	switch file.State {
	case StateWorking:
		return integration.StatusWorking, true
	case StateAwaitingDecision:
		return integration.StatusAwaitingDecision, true
	case StateAwaitingInput:
		return integration.StatusAwaitingInput, true
	case StateIdle:
		return integration.StatusIdle, true
	default:
		return integration.StatusUnknown, false
	}
}

type claudeSession struct {
	PID       int    `json:"pid"`
	CWD       string `json:"cwd"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"` //nolint:tagliatelle // external format owned by Claude Code
}

func (i *Integration) statusFromSessionFile(cwd string) (integration.Status, bool) {
	if strings.TrimSpace(i.home) == "" {
		return integration.StatusUnknown, false
	}

	entries, err := os.ReadDir(filepath.Join(i.home, "sessions"))
	if err != nil {
		return integration.StatusUnknown, false
	}

	best, found := freshestLiveSession(filepath.Join(i.home, "sessions"), entries, cwd)
	if !found {
		return integration.StatusUnknown, false
	}

	switch best.Status {
	case "busy":
		return integration.StatusWorking, true
	case "waiting":
		return integration.StatusAwaitingDecision, true
	case "idle":
		return integration.StatusIdle, true
	default:
		return integration.StatusUnknown, false
	}
}

func freshestLiveSession(
	dir string,
	entries []os.DirEntry,
	cwd string,
) (claudeSession, bool) {
	var (
		best      claudeSession
		bestFound bool
	)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(
			filepath.Join(dir, entry.Name()),
		) // #nosec G304 -- session files under the user's Claude home
		if err != nil {
			continue
		}

		var sess claudeSession
		if json.Unmarshal(data, &sess) != nil {
			continue
		}

		if sess.CWD != cwd || !processAlive(sess.PID) {
			continue
		}

		if !bestFound || sess.UpdatedAt > best.UpdatedAt {
			best = sess
			bestFound = true
		}
	}

	return best, bestFound
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

func WriteStatus(statusDir, cwd, state, sessionID string, now time.Time) error {
	err := os.MkdirAll(statusDir, 0o750)
	if err != nil {
		return fmt.Errorf("create status dir: %w", err)
	}

	data, err := json.Marshal(statusFile{
		State:     state,
		CWD:       cwd,
		SessionID: sessionID,
		UpdatedAt: now.Unix(),
	})
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}

	path := filepath.Join(statusDir, EncodeProjectDir(cwd)+".json")

	tmp := path + ".tmp"

	err = os.WriteFile(tmp, data, 0o600)
	if err != nil {
		return fmt.Errorf("write temp status %s: %w", tmp, err)
	}

	err = os.Rename(tmp, path)
	if err != nil {
		return fmt.Errorf("replace status %s: %w", path, err)
	}

	return nil
}

func ValidState(s string) bool {
	switch s {
	case StateWorking, StateAwaitingDecision, StateAwaitingInput, StateIdle:
		return true
	default:
		return false
	}
}

func readJSONFile(path string, into *statusFile) bool {
	data, err := os.ReadFile(
		path,
	) // #nosec G304 -- path is <statusDir>/<encoded cwd>.json under the data dir
	if err != nil {
		return false
	}

	return json.Unmarshal(data, into) == nil
}
