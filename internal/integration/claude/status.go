package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// statusFile is the on-disk shape written by the hook command (status.json per
// project dir) and a superset of Claude's own <home>/sessions/<pid>.json.
type statusFile struct {
	State     string `json:"state,omitempty"`  // hook-written lazy-tmux state
	Status    string `json:"status,omitempty"` // Claude session-file status (busy|idle)
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

// statusFromSessionFile scans <home>/sessions/*.json for the freshest entry
// whose cwd matches, mapping Claude's own busy/idle status.
func (i *Integration) statusFromSessionFile(cwd string) (integration.Status, bool) {
	if strings.TrimSpace(i.home) == "" {
		return integration.StatusUnknown, false
	}

	entries, err := os.ReadDir(filepath.Join(i.home, "sessions"))
	if err != nil {
		return integration.StatusUnknown, false
	}

	var (
		best      statusFile
		bestFound bool
	)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		var file statusFile
		if !readJSONFile(filepath.Join(i.home, "sessions", entry.Name()), &file) {
			continue
		}

		if file.CWD != cwd {
			continue
		}

		if !bestFound || file.UpdatedAt > best.UpdatedAt {
			best = file
			bestFound = true
		}
	}

	if !bestFound {
		return integration.StatusUnknown, false
	}

	switch best.Status {
	case "busy":
		return integration.StatusWorking, true
	case "idle":
		return integration.StatusIdle, true
	default:
		return integration.StatusUnknown, false
	}
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

	err = os.WriteFile(path, data, 0o644)
	if err != nil {
		return fmt.Errorf("write status %s: %w", path, err)
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
