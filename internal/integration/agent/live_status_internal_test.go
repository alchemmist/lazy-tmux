package agent

import (
	"os"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func statusPane(kind, id, state string) snapshot.Pane {
	flag := "resume"
	if kind == "claude" {
		flag = "--resume"
	}

	return snapshot.Pane{CurrentCmd: kind, Agent: &snapshot.AgentSession{
		Version: 1, Kind: kind, ID: id, Home: "/agents/" + kind, CWD: "/workspace",
		Argv: []string{kind, flag, id}, Source: HookSource, Status: state,
	}}
}

func TestLiveStatusNeverUsesSavedActivity(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"claude", "codex", "antex"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			saved := statusPane(kind, "session", "error")
			current := []snapshot.Pane{statusPane(kind, "session", "working")}
			reader := LiveStatus{Loader: func() ([]snapshot.Pane, error) { return current, nil }}
			for _, state := range []string{"working", "idle", "awaiting_input", "awaiting_decision", "error"} {
				current[0].Agent.Status = state
				want, _ := State(state)
				if got, ok := reader.Status(saved, kind); !ok || got != want {
					t.Fatalf("%s: %v %v", state, got, ok)
				}
			}
			current = nil
			if _, ok := reader.Status(saved, kind); ok {
				t.Fatal("exited client retained saved status")
			}
			if _, ok := Status(saved, kind); ok {
				t.Fatal("persisted status is not live evidence")
			}
		})
	}
}

func TestLiveStatusIdentityAndAmbiguity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*snapshot.Pane, *[]snapshot.Pane)
	}{
		{
			"session switched",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0] = statusPane("codex", "other", "working") },
		},
		{
			"home",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0].Agent.Home = "/other" },
		},
		{"cwd", func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0].Agent.CWD = "/other" }},
		{
			"kind",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0] = statusPane("claude", "session", "working") },
		},
		{
			"duplicate",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { *live = append(*live, (*live)[0]) },
		},
		{"duplicate other cwd", func(_ *snapshot.Pane, live *[]snapshot.Pane) {
			other := statusPane("codex", "session", "idle")
			other.Agent.CWD = "/other"
			*live = append(*live, other)
		}},
		{
			"manual saved",
			func(saved *snapshot.Pane, _ *[]snapshot.Pane) { saved.Agent.Source = ManualSource },
		},
		{
			"manual live",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0].Agent.Source = ManualSource },
		},
		{
			"saved pending",
			func(saved *snapshot.Pane, _ *[]snapshot.Pane) { saved.Meta = map[string]string{PendingKey: "1"} },
		},
		{"kind pending", func(saved *snapshot.Pane, _ *[]snapshot.Pane) {
			saved.Meta = map[string]string{"codex.restore_pending": "1"}
		}},
		{
			"live pending",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0].Meta = map[string]string{PendingKey: "1"} },
		},
		{
			"invalid argv",
			func(_ *snapshot.Pane, live *[]snapshot.Pane) { (*live)[0].Agent.Argv[2] = "wrong" },
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			saved := statusPane("codex", "session", "working")
			live := []snapshot.Pane{statusPane("codex", "session", "working")}
			test.change(&saved, &live)
			reader := LiveStatus{Loader: func() ([]snapshot.Pane, error) { return live, nil }}
			if got, ok := reader.Status(saved, "codex"); ok || got != integration.StatusUnknown {
				t.Fatalf("unsafe status %v %v", got, ok)
			}
		})
	}
}

func TestLiveStatusLoaderFailure(t *testing.T) {
	t.Parallel()
	saved := statusPane("codex", "session", "working")
	readers := []LiveStatus{{}, {Loader: func() ([]snapshot.Pane, error) {
		return []snapshot.Pane{saved}, os.ErrNotExist
	}}}
	for _, reader := range readers {
		if _, ok := reader.Status(saved, "codex"); ok {
			t.Fatal("unavailable source returned status")
		}
	}
}
