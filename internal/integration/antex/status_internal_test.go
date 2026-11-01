package antex

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestStatusRequiresCurrentOwnerAndExactRollout(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := writeRollout(t, home, "2026/01/03", "session", "/workspace", time.Now())
	appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_started"}}`)
	saved := boundStatusPane(home, "session")
	identity, ok := agent.Session(saved, "antex")
	if !ok {
		t.Fatal("invalid legacy fixture")
	}
	typed := snapshot.Pane{CurrentCmd: "antex", Agent: &identity}
	typed.Agent.Source = agent.HookSource
	typed.Agent.Status = "idle"
	for _, pane := range []snapshot.Pane{saved, typed} {
		current := []snapshot.Pane{typed}
		reader := New(
			home,
		).WithStatusLoader(func() ([]snapshot.Pane, error) { return current, nil })
		if got, ok := reader.Status(pane); !ok || got != integration.StatusWorking {
			t.Fatalf("rollout status %v %v", got, ok)
		}
		appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_complete"}}`)
		if got, ok := reader.Status(pane); !ok || got != integration.StatusAwaitingInput {
			t.Fatalf("live rollout change %v %v", got, ok)
		}
		current = nil
		if _, ok := reader.Status(pane); ok {
			t.Fatal("exited owner retained rollout status")
		}
		current = []snapshot.Pane{typed, typed}
		if _, ok := reader.Status(pane); ok {
			t.Fatal("duplicate owner accepted")
		}
		current = []snapshot.Pane{typed}
		otherCWD := *typed.Agent
		otherCWD.CWD = "/different"
		current[0].Agent = &otherCWD
		if _, ok := reader.Status(pane); ok {
			t.Fatal("mismatched owner cwd accepted")
		}
		appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_started"}}`)
	}
}

func TestStatusRejectsWrongRolloutHeaderAndPendingRestore(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeRollout(t, home, "2026/01/03", "session", "/other", time.Now())
	pane := boundStatusPane(home, "session")
	reader := New(
		home,
	).WithStatusLoader(func() ([]snapshot.Pane, error) { return []snapshot.Pane{pane}, nil })
	if _, ok := reader.Status(pane); ok {
		t.Fatal("rollout cwd mismatch accepted")
	}
	pane.Meta[agent.PendingKey] = "1"
	if _, ok := reader.Status(pane); ok {
		t.Fatal("pending restore reported status")
	}
	delete(pane.Meta, agent.PendingKey)
	if _, ok := New(home).Status(pane); ok {
		t.Fatal("saved binding alone reported status")
	}
}

func TestStatusRejectsDuplicateRolloutIdentity(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeRollout(t, home, "2026/01/03", "session", "/workspace", time.Now())
	writeRollout(t, home, "2026/01/04", "session", "/workspace", time.Now())
	pane := boundStatusPane(home, "session")
	reader := New(
		home,
	).WithStatusLoader(func() ([]snapshot.Pane, error) { return []snapshot.Pane{pane}, nil })
	if _, ok := reader.Status(pane); ok {
		t.Fatal("duplicate rollout identity accepted")
	}
}
