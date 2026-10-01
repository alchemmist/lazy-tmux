package tmux

import (
	"encoding/json"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestHookLifecycleRejectsStaleAndForeignSessions(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"codex", "claude"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			owner := LiveBinding{
				Version:    1,
				PID:        20,
				StartedAt:  "start",
				PaneID:     "%1",
				SocketPath: "/tmp/socket",
			}
			first, changed, err := reduceAgentHook(
				LiveBinding{},
				owner,
				kind,
				"/home/agent",
				AgentHookEvent{Event: "SessionStart", SessionID: "first", CWD: "/workspace"},
				1,
			)
			if err != nil || !changed {
				t.Fatalf("start: %v %v", changed, err)
			}
			second, changed, err := reduceAgentHook(
				first,
				owner,
				kind,
				"/home/agent",
				AgentHookEvent{Event: "SessionStart", SessionID: "second", CWD: "/workspace"},
				2,
			)
			if err != nil || !changed {
				t.Fatal(err)
			}
			for _, event := range []AgentHookEvent{
				{Event: "Stop", SessionID: "first"},
				{Event: "SessionEnd", SessionID: "first"},
				{Event: "SessionStart", SessionID: "child", AgentID: "child", CWD: "/workspace"},
			} {
				if _, changed, err := reduceAgentHook(
					second,
					owner,
					kind,
					"/home/agent",
					event,
					3,
				); err != nil ||
					changed {
					t.Fatal("stale or subagent event changed binding")
				}
			}
			if _, changed, _ := reduceAgentHook(
				second,
				owner,
				kind,
				"/home/agent",
				AgentHookEvent{Event: "SessionStart", SessionID: "first", CWD: "/workspace"},
				1,
			); changed {
				t.Fatal("late start replaced active session")
			}
			ended, changed, err := reduceAgentHook(
				second,
				owner,
				kind,
				"/home/agent",
				AgentHookEvent{Event: "SessionEnd", SessionID: "second"},
				4,
			)
			if err != nil || !changed || !ended.Closed {
				t.Fatal("session end not applied")
			}
			if _, changed, _ := reduceAgentHook(
				ended,
				owner,
				kind,
				"/home/agent",
				AgentHookEvent{Event: "PostToolUse", SessionID: "second"},
				5,
			); changed {
				t.Fatal("late status reopened session")
			}
		})
	}
}

func TestHookBindingMustBelongToExactAgentAndPane(t *testing.T) {
	t.Parallel()
	binding := LiveBinding{
		Version: 1,
		Session: snapshot.AgentSession{
			Version: 1,
			Kind:    "codex",
			ID:      "id",
			Home:    "/home/codex",
			CWD:     "/workspace",
			Argv:    []string{"codex", "resume", "id"},
			Source:  "hook-v1",
		},
		PID:        20,
		StartedAt:  "start",
		PaneID:     "%1",
		SocketPath: "/tmp/socket",
		Generation: 1,
	}
	raw, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, command, start, pane string
		valid                      bool
	}{
		{"valid", "codex", "start", "%1", true},
		{"other agent", "antex", "start", "%1", false},
		{"reused pid", "codex", "later", "%1", false},
		{"other pane", "codex", "start", "%2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newProcessSnapshot([]string{"10 1 Ss zsh", "20 10 S+ " + tc.command})
			got := verifiedAgentBinding(
				string(raw),
				10,
				tc.pane,
				"/tmp/socket",
				p,
				func(int) string { return tc.start },
			)
			if (got != nil) != tc.valid {
				t.Fatalf("valid=%v got=%v", tc.valid, got)
			}
		})
	}
}

func TestHookReceiptRequiresSuccessfulSessionStart(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		event, token string
		want         int
	}{
		{"SessionStart", "attempt123", 13},
		{"SessionEnd", "attempt123", 6},
		{"SessionStart", "invalid;token", 6},
		{"SessionStart", "", 6},
	} {
		if got := hookPublishArgs("%1", "{}", tc.event, tc.token); len(got) != tc.want {
			t.Fatalf("unexpected publish args: %v", got)
		}
	}
}

func TestCodexHookCanTraverseItsOwnedAppServerButNotNestedAgents(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		lines []string
		want  int
		ok    bool
	}{
		{
			"owned app server",
			[]string{"10 1 Ss zsh", "20 10 S+ codex", "30 20 S codex app-server", "40 30 S sh hook"},
			20, true,
		},
		{"shared daemon", []string{"10 1 Ss zsh", "20 10 S+ codex", "30 1 S codex app-server", "40 30 S sh hook"}, 0, false},
		{"nested agent", []string{"10 1 Ss zsh", "20 10 S+ codex", "30 20 S codex exec", "40 30 S sh hook"}, 30, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pid, ok := newProcessSnapshot(tc.lines).hookOwnerProcess(40, 10, "codex")
			if pid != tc.want || ok != tc.ok {
				t.Fatalf("owner=%d valid=%t", pid, ok)
			}
		})
	}
}

func TestCompactionPreservesWorkingState(t *testing.T) {
	t.Parallel()
	owner := LiveBinding{
		Version:    1,
		PID:        20,
		StartedAt:  "start",
		PaneID:     "%1",
		SocketPath: "/tmp/socket",
	}
	start, _, err := reduceAgentHook(
		LiveBinding{},
		owner,
		"codex",
		"/home/codex",
		AgentHookEvent{Event: "SessionStart", SessionID: "id", CWD: "/workspace"},
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	working, _, err := reduceAgentHook(
		start,
		owner,
		"codex",
		"/home/codex",
		AgentHookEvent{Event: "UserPromptSubmit", SessionID: "id"},
		2,
	)
	if err != nil {
		t.Fatal(err)
	}
	compact, changed, err := reduceAgentHook(
		working,
		owner,
		"codex",
		"/home/codex",
		AgentHookEvent{
			Event:     "SessionStart",
			Source:    "compact",
			SessionID: "id",
			CWD:       "/workspace",
		},
		3,
	)
	if err != nil || !changed || compact.Session.Status != "working" {
		t.Fatalf("compaction interrupted status: %+v %v", compact, err)
	}
}

func TestInvalidSessionStartCannotLeavePreviousIdentityTrusted(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	client := NewClientWithRunner("tmux", runner)
	owner := LiveBinding{
		Version:    1,
		PID:        20,
		PaneID:     "%1",
		StartedAt:  "start",
		SocketPath: "/tmp/socket",
	}
	if err := client.invalidateHookBinding(owner, 4, errUnsupportedHookAgent); err == nil {
		t.Fatal("hook failure was hidden")
	}
	if len(runner.calls) != 1 {
		t.Fatal("old binding was not invalidated")
	}
	call := runner.calls[0]
	var record LiveBinding
	if err := json.Unmarshal([]byte(call[len(call)-1]), &record); err != nil {
		t.Fatal(err)
	}
	if !record.Closed || record.Generation != 4 || record.PID != 20 {
		t.Fatalf("unexpected invalidation: %+v", record)
	}
}
