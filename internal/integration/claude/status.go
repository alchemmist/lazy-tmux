package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func EncodeProjectDir(cwd string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
}
