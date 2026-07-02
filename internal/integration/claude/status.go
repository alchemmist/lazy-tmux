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

// State names written by the `lazy-tmux hook claude-status` command and read
// back here. They map 1:1 onto Claude Code's hook events.
const (
	StateWorking          = "working"           // UserPromptSubmit — Claude is generating
	StateAwaitingDecision = "awaiting_decision" // Notification/permission_prompt
	StateAwaitingInput    = "awaiting_input"    // Notification/idle_prompt
	StateIdle             = "idle"              // Stop — turn finished
)

// statusFile is the on-disk shape written by lazy-tmux's hook command (one per
// project dir) and read back by statusFromHook.
type statusFile struct {
	State     string `json:"state,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

// Status reports the live state of a Claude pane. It prefers lazy-tmux's
// hook-written status file (precise: working / awaiting decision / awaiting
// input / idle), and falls back to Claude's own session file (busy / idle) when
// the hook is not installed. Returns ok=false when nothing is found (no dot).
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

// statusFromHook reads <statusDir>/<encoded cwd>.json written by the hook.
func (i *Integration) statusFromHook(cwd string) (integration.Status, bool) {
	if strings.TrimSpace(i.statusDir) == "" {
		return integration.StatusUnknown, false
	}

	path := filepath.Join(i.statusDir, EncodeProjectDir(cwd)+".json")

	var file statusFile
	if !readJSONFile(path, &file) {
		return integration.StatusUnknown, false
	}

	// Guard against encoded-path collisions: only trust the file when its
	// recorded cwd matches this pane's. Older files without a cwd are accepted.
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

// claudeSession is Claude Code's own per-process session file
// (<home>/sessions/<pid>.json). Note the camelCase keys — these differ from
// lazy-tmux's hook status file.
type claudeSession struct {
	PID       int    `json:"pid"`
	CWD       string `json:"cwd"`
	Status    string `json:"status"`    // busy | waiting | idle
	UpdatedAt int64  `json:"updatedAt"` //nolint:tagliatelle // external format owned by Claude Code
}

// statusFromSessionFile reads Claude Code's own session files as a zero-setup
// source of truth: among the files matching cwd whose process is still alive, it
// takes the freshest and maps Claude's busy/waiting/idle. Dead sessions are
// skipped so stale files never show a misleading dot.
func (i *Integration) statusFromSessionFile(cwd string) (integration.Status, bool) {
	if strings.TrimSpace(i.home) == "" {
		return integration.StatusUnknown, false
	}

	entries, err := os.ReadDir(filepath.Join(i.home, "sessions"))
	if err != nil {
		return integration.StatusUnknown, false
	}

	var (
		best      claudeSession
		bestFound bool
	)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(i.home, "sessions", entry.Name()))
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

	if !bestFound {
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

// processAlive reports whether a pid refers to a running process (signal 0
// probe). EPERM means the process exists but is owned by another user.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}

// WriteStatus records a Claude pane's live state under statusDir, keyed by the
// project-dir encoding of cwd, so the picker can read it back. It is called by
// the `lazy-tmux hook claude-status` command from Claude Code hooks.
func WriteStatus(statusDir, cwd, state, sessionID string, now time.Time) error {
	err := os.MkdirAll(statusDir, 0o755)
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

	// Write to a temp file in the same dir and rename, so a concurrent picker
	// read never observes partial JSON.
	tmp := path + ".tmp"

	err = os.WriteFile(tmp, data, 0o644)
	if err != nil {
		return fmt.Errorf("write temp status %s: %w", tmp, err)
	}

	err = os.Rename(tmp, path)
	if err != nil {
		return fmt.Errorf("replace status %s: %w", path, err)
	}

	return nil
}

// ValidState reports whether s is one of the recognized hook states.
func ValidState(s string) bool {
	switch s {
	case StateWorking, StateAwaitingDecision, StateAwaitingInput, StateIdle:
		return true
	default:
		return false
	}
}

func readJSONFile(path string, into *statusFile) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	return json.Unmarshal(data, into) == nil
}
