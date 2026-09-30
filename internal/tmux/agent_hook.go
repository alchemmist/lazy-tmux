package tmux

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const sessionStartEvent = "SessionStart"

const idleHookState = "idle"

type AgentHookEvent struct {
	SessionID        string `json:"session_id"`
	CWD              string `json:"cwd"`
	Event            string `json:"hook_event_name"`
	Source           string `json:"source"`
	AgentID          string `json:"agent_id"`
	AgentType        string `json:"agent_type"`
	NotificationType string `json:"notification_type"`
}

var errUnsupportedHookAgent = errors.New("unsupported hook agent")

func (client *Client) ApplyAgentHook(kind, home, pane string, event AgentHookEvent) error {
	if kind != snapshot.AgentCodex && kind != snapshot.AgentClaude {
		return errUnsupportedHookAgent
	}
	if event.AgentID != "" || strings.HasPrefix(event.Event, "Subagent") {
		return nil
	}
	received := time.Now().UnixNano()
	owner, err := client.HookOwner(pane, kind)
	if err != nil {
		return err
	}
	unlock, err := lockAgentHooks()
	if err != nil {
		return err
	}
	defer unlock()
	raw, err := client.Output("show-options", "-pqv", "-t", pane, agentBindingOption)
	if err != nil {
		return err
	}
	var previous LiveBinding
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &previous)
	next, changed, err := reduceAgentHook(previous, owner, kind, home, event, received)
	if err != nil {
		return client.invalidateHookBinding(owner, received, err)
	}
	if !changed {
		return nil
	}
	err = populateHookArgs(&next, owner, event, kind)
	if err != nil {
		return client.invalidateHookBinding(owner, received, err)
	}
	body, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("encode hook binding: %w", err)
	}
	_, err = client.Output(
		hookPublishArgs(pane, string(body), event.Event, os.Getenv("LAZY_TMUX_RESTORE_TOKEN"))...)

	return err
}

func reduceAgentHook(
	previous, owner LiveBinding,
	kind, home string,
	event AgentHookEvent,
	received int64,
) (LiveBinding, bool, error) {
	sameOwner := sameHookOwner(previous, owner, kind)
	if event.AgentID != "" {
		return previous, false, nil
	}
	if sameOwner && received <= previous.Generation {
		return previous, false, nil
	}
	if event.Event != sessionStartEvent {
		if !sameOwner || previous.Closed || previous.Session.ID != event.SessionID {
			return previous, false, nil
		}
		owner = previous
	} else {
		session, err := newHookSession(kind, home, event)
		if err != nil {
			return previous, false, err
		}
		owner.Session = session
	}

	status, closed, recognized := hookState(event)
	if !recognized {
		return previous, false, nil
	}
	if event.Event == sessionStartEvent && event.Source == "compact" && sameOwner {
		if previous.Session.ID != event.SessionID {
			return previous, false, nil
		}
		status = previous.Session.Status
	}
	owner.Session.Status = status
	owner.Closed = closed
	owner.Generation = received

	return owner, true, nil
}

func hookState(event AgentHookEvent) (string, bool, bool) {
	switch event.Event {
	case sessionStartEvent:
		return idleHookState, false, true
	case "SessionEnd":
		return idleHookState, true, true
	case "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure":
		return "working", false, true
	case "PermissionRequest":
		return "awaiting_decision", false, true
	case "Stop", "Interrupt":
		return "awaiting_input", false, true
	case "Notification":
		switch event.NotificationType {
		case "permission_prompt":
			return "awaiting_decision", false, true
		case "idle_prompt":
			return "awaiting_input", false, true
		}
	}

	return "", false, false
}

func newHookSession(kind, home string, event AgentHookEvent) (snapshot.AgentSession, error) {
	if event.AgentID != "" {
		return snapshot.AgentSession{}, errUnsupportedHookAgent
	}
	subcommand := "resume"
	if kind == snapshot.AgentClaude {
		subcommand = "--resume"
	}
	session := snapshot.AgentSession{
		Version: 1,
		Kind:    kind,
		ID:      event.SessionID,
		Home:    filepath.Clean(home),
		CWD:     event.CWD,
		Argv:    []string{kind, subcommand, event.SessionID},
		Source:  agent.HookSource,
		Status:  idleHookState,
	}
	err := agent.Valid(session)
	if err != nil {
		return snapshot.AgentSession{}, fmt.Errorf("invalid hook session: %w", err)
	}

	return session, nil
}

func populateHookArgs(
	next *LiveBinding,
	owner LiveBinding,
	event AgentHookEvent,
	kind string,
) error {
	if event.Event != sessionStartEvent {
		return nil
	}
	argv, err := processArgv(owner.PID)
	if err != nil {
		return err
	}
	if agent.LaunchKind(argv) != kind {
		return errHookOwner
	}
	resume, err := agent.ResumeArgv(kind, event.SessionID, event.CWD, argv)
	if err != nil {
		return fmt.Errorf("capture launch arguments: %w", err)
	}
	next.Session.Argv = resume

	return nil
}

func hookPublishArgs(pane, body, event, token string) []string {
	const publishArgsCapacity = 13
	args := make([]string, 0, publishArgsCapacity)
	args = append(args, "set-option", "-p", "-t", pane, agentBindingOption, body)
	if event != sessionStartEvent || token == "" {
		return args
	}
	if strings.IndexFunc(token, func(char rune) bool {
		return char < '0' || (char > '9' && char < 'A') || (char > 'Z' && char < 'a') || char > 'z'
	}) >= 0 {
		return args
	}

	return append(args, ";", "set-option", "-p", "-t", pane, restoreAckOption, token)
}

func sameHookOwner(previous, owner LiveBinding, kind string) bool {
	return previous.PID == owner.PID && previous.StartedAt == owner.StartedAt &&
		previous.PaneID == owner.PaneID &&
		previous.SocketPath == owner.SocketPath &&
		previous.Session.Kind == kind
}

func (client *Client) invalidateHookBinding(
	owner LiveBinding,
	generation int64,
	cause error,
) error {
	owner.Closed = true
	owner.Generation = generation
	data, err := json.Marshal(owner)
	if err != nil {
		return fmt.Errorf("invalidate hook binding: %w", err)
	}
	_, err = client.Output("set-option", "-p", "-t", owner.PaneID, agentBindingOption, string(data))
	if err != nil {
		return errors.Join(cause, err)
	}

	return cause
}

func lockAgentHooks() (func(), error) {
	root, err := os.OpenRoot(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("open hook lock directory: %w", err)
	}
	lock, err := root.OpenFile(
		fmt.Sprintf("lazy-tmux-agent-hook-%d.lock", os.Getuid()),
		os.O_CREATE|os.O_RDWR,
		0o600,
	)
	if err != nil {
		_ = root.Close()

		return nil, fmt.Errorf("open hook lock: %w", err)
	}
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
	if err != nil {
		_ = lock.Close()
		_ = root.Close()

		return nil, fmt.Errorf("lock hook: %w", err)
	}

	return func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); _ = lock.Close(); _ = root.Close() }, nil
}
