package tmux

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func statusBinding() LiveBinding {
	return LiveBinding{
		Version: 1, PID: 20, StartedAt: "start", PaneID: "%1", SocketPath: "/tmp/test.sock",
		Session: snapshot.AgentSession{
			Version: 1,
			Kind:    "claude",
			ID:      "session",
			Home:    "/agents/claude",
			CWD:     "/workspace",
			Argv: []string{
				"claude",
				"--resume",
				"session",
			},
			Source: agent.HookSource,
			Status: "working",
		},
	}
}

func TestAgentStatusCaptureOwnership(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*LiveBinding)
		lines  []string
		valid  bool
	}{
		{"live", func(*LiveBinding) {}, []string{"10 1 Ss zsh", "20 10 S+ claude"}, true},
		{"exited", func(*LiveBinding) {}, []string{"10 1 Ss zsh"}, false},
		{
			"reused pid",
			func(b *LiveBinding) { b.StartedAt = "old" },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
		{
			"closed",
			func(b *LiveBinding) { b.Closed = true },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
		{
			"pane",
			func(b *LiveBinding) { b.PaneID = "%2" },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
		{
			"server",
			func(b *LiveBinding) { b.SocketPath = "/tmp/other" },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
		{
			"missing start",
			func(b *LiveBinding) { b.StartedAt = "" },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
		{"kind switched", func(*LiveBinding) {}, []string{"10 1 Ss zsh", "20 10 S+ codex"}, false},
		{"unrelated", func(*LiveBinding) {}, []string{"10 1 Ss zsh", "20 1 S+ claude"}, false},
		{"stopped", func(*LiveBinding) {}, []string{"10 1 Ss zsh", "20 10 T+ claude"}, false},
		{"background", func(*LiveBinding) {}, []string{"10 1 Ss zsh", "20 10 S claude"}, false},
		{
			"nested agent",
			func(*LiveBinding) {},
			[]string{"10 1 S+ codex", "20 10 S+ claude"},
			false,
		},
		{
			"invalid identity",
			func(b *LiveBinding) { b.Session.Argv[2] = "wrong" },
			[]string{"10 1 Ss zsh", "20 10 S+ claude"},
			false,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			binding := statusBinding()
			test.change(&binding)
			raw, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			output := "10|claude|/workspace|%1|/tmp/test.sock|" + string(raw) + "|||"
			panes := captureAgentStatus(
				output,
				newProcessSnapshot(test.lines),
				func(int) string { return "start" },
			)
			if (len(panes) == 1) != test.valid {
				t.Fatalf("panes: %v", panes)
			}
			if test.valid &&
				(panes[0].Agent.ID != "session" || panes[0].Agent.Status != "working") {
				t.Fatalf("lost live identity: %v", panes[0].Agent)
			}
		})
	}
}

func TestAgentStatusCaptureLegacyAntexAndTypedPrecedence(t *testing.T) {
	t.Parallel()
	legacy := antexBinding{
		Version:          1,
		ThreadID:         "session",
		PID:              20,
		ProcessStartedAt: "start",
		PaneID:           "%1",
		SocketPath:       "/tmp/test.sock",
		Home:             "/agents/antex",
		CWD:              "/workspace",
		ResumeArgv:       []string{"antex", "resume", "session"},
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	processes := newProcessSnapshot([]string{"10 1 Ss zsh", "20 10 S+ antex"})
	for _, raw := range []string{"", "{}", `{"closed":true}`} {
		output := "10|antex|/workspace|%1|/tmp/test.sock|" + raw + "|" + string(encoded) + "||"
		panes := captureAgentStatus(output, processes, func(int) string { return "start" })
		if (len(panes) == 1) != (raw == "") {
			t.Fatalf("typed stale binding fell back to legacy: %q %v", raw, panes)
		}
	}
}

func TestAgentStatusCapturePreservesDuplicateClients(t *testing.T) {
	t.Parallel()
	binding := statusBinding()
	first, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	binding.PaneID, binding.PID = "%2", 40
	second, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	output := "10|claude|/workspace|%1|/tmp/test.sock|" + string(
		first,
	) + "|||\n30|claude|/workspace|%2|/tmp/test.sock|" + string(
		second,
	) + "|||"
	processes := newProcessSnapshot(
		[]string{"10 1 Ss zsh", "20 10 S+ claude", "30 1 Ss zsh", "40 30 S+ claude"},
	)
	panes := captureAgentStatus(output, processes, func(int) string { return "start" })
	if len(panes) != 2 {
		t.Fatalf("duplicates lost: %v", panes)
	}
	reader := agent.LiveStatus{Loader: func() ([]snapshot.Pane, error) { return panes, nil }}
	if _, ok := reader.Status(panes[0], "claude"); ok {
		t.Fatal("ambiguous duplicate client status accepted")
	}
}

func TestAgentStatusCaptureRestorePending(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(statusBinding())
	if err != nil {
		t.Fatal(err)
	}
	intent, err := json.Marshal(pendingRestore{Token: "restore-token"})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(intent)
	processes := newProcessSnapshot([]string{"10 1 Ss zsh", "20 10 S+ claude"})
	for _, ack := range []string{"", "wrong", "restore-token"} {
		output := "10|claude|/workspace|%1|/tmp/test.sock|" + string(
			raw,
		) + "||" + encoded + "|" + ack
		panes := captureAgentStatus(output, processes, func(int) string { return "start" })
		if (len(panes) == 1) != (ack == "restore-token") {
			t.Fatalf("pending restore ack=%q panes=%v", ack, panes)
		}
	}
}

func TestAgentStatusProcessSnapshotIncludesBirthIdentity(t *testing.T) {
	t.Parallel()
	processes, starts := parseAgentStatusProcesses(
		"10 1 Ss Mon Sep  7 12:00:00 2026 zsh\n20 10 S+ Mon Sep  7 12:01:00 2026 claude --resume session\ninvalid",
	)
	if len(processes.processes) != 2 || starts[20] != "Mon Sep 7 12:01:00 2026" ||
		processes.processes[20].cmd != "claude --resume session" {
		t.Fatalf("invalid process snapshot: %v %v", processes, starts)
	}
}

func TestAgentStatusDeduplicatesLinkedPaneRows(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(statusBinding())
	if err != nil {
		t.Fatal(err)
	}
	row := "10|claude|/workspace|%1|/tmp/test.sock|" + string(encoded) + "|||"
	panes := captureAgentStatus(
		row+"\n"+row,
		newProcessSnapshot([]string{"10 1 Ss zsh", "20 10 S+ claude"}),
		func(int) string { return "start" },
	)
	if len(panes) != 1 {
		t.Fatalf("one linked pane became %d clients", len(panes))
	}
}

func TestAgentStatusLegacyAfterExitedTypedOwner(t *testing.T) {
	t.Parallel()
	legacy := antexBinding{
		Version:          1,
		ThreadID:         "session",
		PID:              40,
		ProcessStartedAt: "start",
		PaneID:           "%1",
		SocketPath:       "/tmp/test.sock",
		Home:             "/agents/antex",
		CWD:              "/workspace",
		ResumeArgv:       []string{"antex", "resume", "session"},
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	old := statusBinding()
	old.Closed = true
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	row := "10|antex|/workspace|%1|/tmp/test.sock|" + string(raw) + "|" + string(encoded) + "||"
	panes := captureAgentStatus(
		row,
		newProcessSnapshot([]string{"10 1 Ss zsh", "40 10 S+ antex"}),
		func(int) string { return "start" },
	)
	if len(panes) != 1 || panes[0].Agent.Kind != "antex" {
		t.Fatalf("dead Claude owner hid new Antex: %v", panes)
	}
}
