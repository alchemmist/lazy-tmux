package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeRollout(t *testing.T, home, date, id, cwd string, mod time.Time) string {
	t.Helper()
	path := filepath.Join(home, "sessions", date, "rollout-"+id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"` +
		id + `","cwd":"` + cwd + `"}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}

	return path
}

func appendRolloutLine(t *testing.T, path, line string) {
	t.Helper()

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureReturnsNewestMatchingSession(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeRollout(t, home, "2026/01/01", "old", cwd, base)
	writeRollout(t, home, "2026/01/02", "other-cwd", "/tmp", base.Add(2*time.Hour))
	writeRollout(t, home, "2026/01/03", "new", cwd, base.Add(time.Hour))

	meta, err := New(home).Capture(snapshot.Pane{CurrentPath: cwd, CurrentCmd: "codex"})
	if err != nil || meta[metaSessionID] != "new" {
		t.Fatalf("Capture() = %v, %v; want newest matching session", meta, err)
	}
}

func TestCaptureInvalidatesIndexWhenExistingRolloutChanges(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/workspace"
	base := time.Unix(100, 0)
	oldPath := writeRollout(t, home, "2026/01/03", "old", cwd, base)
	writeRollout(t, home, "2026/01/03", "new", cwd, base.Add(time.Hour))
	integration := New(home)
	pane := snapshot.Pane{CurrentPath: cwd, CurrentCmd: "codex"}

	meta, err := integration.Capture(pane)
	if err != nil || meta["session_id"] != "new" {
		t.Fatalf("initial capture: meta=%v err=%v", meta, err)
	}
	if err = os.Chtimes(oldPath, base.Add(2*time.Hour), base.Add(2*time.Hour)); err != nil {
		t.Fatalf("update old rollout: %v", err)
	}
	meta, err = integration.Capture(pane)
	if err != nil || meta["session_id"] != "old" {
		t.Fatalf("capture after update: meta=%v err=%v", meta, err)
	}
}

func TestCaptureInvalidatesIndexWhenNewRolloutAppears(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/workspace"
	base := time.Unix(100, 0)
	writeRollout(t, home, "2026/01/03", "old", cwd, base)
	integration := New(home)
	pane := snapshot.Pane{CurrentPath: cwd, CurrentCmd: "codex"}

	meta, err := integration.Capture(pane)
	if err != nil || meta["session_id"] != "old" {
		t.Fatalf("initial capture: meta=%v err=%v", meta, err)
	}
	writeRollout(t, home, "2026/01/03", "new", cwd, base.Add(time.Hour))
	meta, err = integration.Capture(pane)
	if err != nil || meta["session_id"] != "new" {
		t.Fatalf("capture after new rollout: meta=%v err=%v", meta, err)
	}
}

func TestCapturePrefersActivePaneSession(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/Users/me/code/proj"
	writeRollout(t, home, "2026/01/03", "newest", cwd, time.Now())

	meta, err := New(home).Capture(snapshot.Pane{
		CurrentPath: cwd,
		CurrentCmd:  "codex",
		Meta: map[string]string{
			snapshot.CodexSessionIDMetaKey: "active",
		},
	})
	if err != nil || meta[metaSessionID] != "active" {
		t.Fatalf("Capture() = %v, %v; want active pane session", meta, err)
	}
}

func TestMatchesAndRestore(t *testing.T) {
	t.Parallel()

	i := New(t.TempDir())
	if !i.Matches(snapshot.Pane{CurrentCmd: "codex"}) ||
		!i.Matches(snapshot.Pane{RestoreCmd: "codex resume abc"}) {
		t.Fatal("expected codex commands to match")
	}
	if i.Matches(snapshot.Pane{CurrentCmd: "claude"}) {
		t.Fatal("claude must not match codex integration")
	}
	if got := i.RestoreCommand(
		snapshot.Pane{},
		map[string]string{metaSessionID: "abc"},
	); got != "codex resume abc" {
		t.Fatalf("unexpected restore command %q", got)
	}
}

func TestStatusFromRolloutLifecycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		event string
		want  integration.Status
	}{
		{name: "working", event: "task_started", want: integration.StatusWorking},
		{name: "completed", event: "task_complete", want: integration.StatusAwaitingInput},
		{name: "aborted", event: "turn_aborted", want: integration.StatusAwaitingInput},
		{
			name:  "approval",
			event: "exec_approval_request",
			want:  integration.StatusAwaitingDecision,
		},
		{name: "input", event: "request_user_input", want: integration.StatusAwaitingInput},
		{name: "error", event: "error", want: integration.StatusError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := t.TempDir()
			path := writeRollout(t, home, "2026/01/03", tc.name, "/workspace", time.Now())
			appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"`+tc.event+`"}}`)

			got, ok := New(home).Status(snapshot.Pane{
				CurrentCmd: "codex",
				Meta:       map[string]string{snapshot.CodexSessionIDMetaKey: tc.name},
			})
			if !ok || got != tc.want {
				t.Fatalf("Status() = %v, %v; want %v", got, ok, tc.want)
			}
		})
	}
}

func TestStatusConcurrentReadersShareSessionPath(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := writeRollout(t, home, "2026/01/03", "shared", "/workspace", time.Now())
	appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_started"}}`)
	codexIntegration := New(home)
	pane := snapshot.Pane{
		Index:       0,
		CurrentPath: "/workspace",
		CurrentCmd:  "codex",
		RestoreCmd:  "",
		Scrollback:  nil,
		IsActive:    true,
		Meta:        map[string]string{snapshot.CodexSessionIDMetaKey: "shared"},
	}

	errs := make(chan string, 32)
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			status, ok := codexIntegration.Status(pane)
			if !ok || status != integration.StatusWorking {
				errs <- fmt.Sprintf("status=%v ok=%v", status, ok)
			}
		})
	}
	group.Wait()
	close(errs)
	for result := range errs {
		t.Fatal(result)
	}
	if codexIntegration.index.indexBuilds != 1 {
		t.Fatalf("rollout tree scanned %d times, want 1", codexIntegration.index.indexBuilds)
	}
}

func TestScopedCaptureValidatesRolloutTreeOnce(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	cwd := "/workspace"
	writeRollout(t, home, "2026/01/03", "session", cwd, time.Now())
	base := New(home)
	pane := snapshot.Pane{CurrentPath: cwd, CurrentCmd: "codex"}

	scoped, ok := base.Scope().(*Integration)
	if !ok {
		t.Fatal("Codex scope has unexpected type")
	}
	for range 20 {
		if _, err := scoped.Capture(pane); err != nil {
			t.Fatalf("scoped capture: %v", err)
		}
	}
	if base.index.validationChecks != 1 {
		t.Fatalf("scope validation checks = %d, want 1", base.index.validationChecks)
	}

	next, ok := base.Scope().(*Integration)
	if !ok {
		t.Fatal("next Codex scope has unexpected type")
	}
	if _, err := next.Capture(pane); err != nil {
		t.Fatalf("next scope capture: %v", err)
	}
	if base.index.validationChecks != 2 {
		t.Fatalf("next scope validation checks = %d, want 2", base.index.validationChecks)
	}
}

func TestStatusUsesLatestLifecycleEvent(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := writeRollout(t, home, "2026/01/03", "active", "/workspace", time.Now())
	appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_started"}}`)
	appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_complete"}}`)

	got, ok := New(home).Status(snapshot.Pane{
		CurrentCmd: "codex",
		Meta:       map[string]string{snapshot.CodexSessionIDMetaKey: "active"},
	})
	if !ok || got != integration.StatusAwaitingInput {
		t.Fatalf("Status() = %v, %v; want awaiting input", got, ok)
	}
}

func TestStatusReadsAcrossLargeRolloutTail(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	path := writeRollout(t, home, "2026/01/03", "active", "/workspace", time.Now())
	appendRolloutLine(t, path, `{"type":"event_msg","payload":{"type":"task_started"}}`)
	appendRolloutLine(t, path, `{"type":"response_item","payload":{"text":"`+
		strings.Repeat("x", int(statusReadBlockSize*2))+`"}}`)

	got, ok := New(home).Status(snapshot.Pane{
		CurrentCmd: "codex",
		Meta:       map[string]string{snapshot.CodexSessionIDMetaKey: "active"},
	})
	if !ok || got != integration.StatusWorking {
		t.Fatalf("Status() = %v, %v; want working", got, ok)
	}
}

func TestStatusWithoutLifecycleIsIdle(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	writeRollout(t, home, "2026/01/03", "idle", "/workspace", time.Now())

	got, ok := New(home).Status(snapshot.Pane{
		CurrentCmd: "codex",
		Meta:       map[string]string{snapshot.CodexSessionIDMetaKey: "idle"},
	})
	if !ok || got != integration.StatusIdle {
		t.Fatalf("Status() = %v, %v; want idle", got, ok)
	}
}

func TestStatusRejectsNonCodexAndMissingSession(t *testing.T) {
	t.Parallel()

	i := New(t.TempDir())

	if _, ok := i.Status(snapshot.Pane{CurrentCmd: "codex"}); ok {
		t.Fatal("Codex pane without a rollout should not have a status")
	}
	if _, ok := i.Status(snapshot.Pane{CurrentCmd: "zsh"}); ok {
		t.Fatal("non-Codex pane should not have a status")
	}
}
