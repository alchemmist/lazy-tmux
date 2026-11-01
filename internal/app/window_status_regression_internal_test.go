package app

import (
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/picker"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type paneStatusFixture struct{}

func (paneStatusFixture) Name() string { return "fixture" }

func (paneStatusFixture) Matches(snapshot.Pane) bool { return true }

func (paneStatusFixture) Capture(snapshot.Pane) (map[string]string, error) {
	return map[string]string{}, nil
}

func (paneStatusFixture) RestoreCommand(snapshot.Pane, map[string]string) string { return "" }

func (paneStatusFixture) Status(pane snapshot.Pane) (integration.Status, bool) {
	states := map[string]integration.Status{
		"working": integration.StatusWorking, "decision": integration.StatusAwaitingDecision,
		"input": integration.StatusAwaitingInput, "idle": integration.StatusIdle,
		"error": integration.StatusError, "unknown": integration.StatusUnknown,
	}
	status, ok := states[pane.Meta["fixture.status"]]

	return status, ok
}

func (paneStatusFixture) SessionID(pane snapshot.Pane) (snapshot.AgentSession, bool) {
	return snapshot.AgentSession{}, pane.Meta["fixture.verified"] == "1"
}

func TestWindowStatusesAggregatesEveryPaneInPriorityOrder(t *testing.T) {
	t.Parallel()
	registry := integration.NewRegistry(paneStatusFixture{})
	// Increasing priority, independently specifying the expected ordering.
	states := []struct {
		name   string
		pane   snapshot.Pane
		status picker.WindowStatus
	}{
		{name: "shell", pane: snapshot.Pane{CurrentCmd: "zsh"}, status: picker.StatusNone},
		{name: "idle", pane: statusPane("idle"), status: picker.StatusIdle},
		{name: "input", pane: statusPane("input"), status: picker.StatusAwaitingInput},
		{name: "working", pane: statusPane("working"), status: picker.StatusWorking},
		{name: "setup", pane: snapshot.Pane{CurrentCmd: "claude"}, status: picker.StatusNeedsSetup},
		{name: "error", pane: statusPane("error"), status: picker.StatusError},
		{name: "decision", pane: statusPane("decision"), status: picker.StatusAwaitingDecision},
		{
			name:   "pending",
			pane:   snapshot.Pane{Meta: map[string]string{"agent.restore_pending": "1"}},
			status: picker.StatusRestorePending,
		},
	}
	for firstIndex, first := range states {
		for secondIndex, second := range states {
			t.Run(first.name+"/"+second.name, func(t *testing.T) {
				t.Parallel()
				want := first.status
				if secondIndex > firstIndex {
					want = second.status
				}
				windows := []snapshot.Window{
					{Index: 9, Panes: []snapshot.Pane{first.pane, second.pane}},
				}
				if got := windowStatuses(registry, windows)[9]; got != want {
					t.Fatalf("got %v, want %v", got, want)
				}
			})
		}
	}
	// Place the highest-priority pane at every offset among all other states.
	for offset := range states {
		panes := make([]snapshot.Pane, len(states))
		for index := range states {
			panes[index] = states[(index+offset)%len(states)].pane
		}
		windows := []snapshot.Window{{Index: 5, Panes: panes}}
		if got := windowStatuses(registry, windows)[5]; got != picker.StatusRestorePending {
			t.Fatalf("rotation %d: got %v, want pending", offset, got)
		}
	}
}

func TestWindowStatusesPendingOverridesPaneActivity(t *testing.T) {
	t.Parallel()
	registry := integration.NewRegistry(paneStatusFixture{})
	for _, key := range []string{"agent.restore_pending", "antex.restore_pending"} {
		for _, live := range []string{"working", "decision", "input", "idle", "error", "unknown", ""} {
			pane := statusPane(live)
			pane.Meta[key] = "1"
			windows := []snapshot.Window{
				{Index: 1, Panes: []snapshot.Pane{statusPane("idle"), pane}},
			}
			if got := windowStatuses(registry, windows)[1]; got != picker.StatusRestorePending {
				t.Errorf("%s + %s: got %v, want pending", key, live, got)
			}
		}
	}
}

func TestWindowStatusesUnknownStates(t *testing.T) {
	t.Parallel()
	registry := integration.NewRegistry(paneStatusFixture{})
	tests := []struct {
		name string
		pane snapshot.Pane
		want picker.WindowStatus
	}{
		{name: "unknown reporter", pane: statusPane("unknown"), want: picker.StatusNone},
		{name: "no reporter", pane: snapshot.Pane{CurrentCmd: "zsh"}, want: picker.StatusNone},
		{
			name: "verified native without activity",
			pane: snapshot.Pane{
				CurrentCmd: "claude",
				Meta:       map[string]string{"fixture.verified": "1"},
			},
			want: picker.StatusNone,
		},
		{
			name: "unverified agent",
			pane: snapshot.Pane{CurrentCmd: "claude"},
			want: picker.StatusNeedsSetup,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			windows := []snapshot.Window{{Index: 3, Panes: []snapshot.Pane{test.pane}}}
			if got := windowStatuses(registry, windows)[3]; got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			for _, other := range []snapshot.Pane{statusPane("idle"), statusPane("working"), statusPane("decision")} {
				for _, panes := range [][]snapshot.Pane{{test.pane, other}, {other, test.pane}} {
					want := toPickerStatusMust(t, other)
					if test.want == picker.StatusNeedsSetup && want == picker.StatusIdle {
						want = picker.StatusNeedsSetup
					}
					if test.want == picker.StatusNeedsSetup && want == picker.StatusWorking {
						want = picker.StatusNeedsSetup
					}
					if got := windowStatuses(registry, []snapshot.Window{{Index: 3, Panes: panes}})[3]; got != want {
						t.Fatalf("panes %v: got %v, want %v", panes, got, want)
					}
				}
			}
		})
	}
}

func TestWindowStatusesKeepsWindowsIndependent(t *testing.T) {
	t.Parallel()
	windows := []snapshot.Window{
		{Index: 1, Panes: []snapshot.Pane{statusPane("idle"), statusPane("working")}},
		{Index: 7, Panes: []snapshot.Pane{statusPane("decision"), statusPane("idle")}},
		{Index: 11, Panes: []snapshot.Pane{statusPane("idle"), statusPane("error")}},
		{Index: 13, Panes: []snapshot.Pane{{CurrentCmd: "zsh"}}},
		{Index: 15},
	}
	got := windowStatuses(integration.NewRegistry(paneStatusFixture{}), windows)
	want := map[int]picker.WindowStatus{
		1: picker.StatusWorking, 7: picker.StatusAwaitingDecision, 11: picker.StatusError,
		13: picker.StatusNone, 15: picker.StatusNone,
	}
	for index, status := range want {
		if got[index] != status {
			t.Errorf("window %d: got %v, want %v", index, got[index], status)
		}
	}
	if len(windowStatuses(nil, nil)) != 0 {
		t.Fatal("empty windows produced statuses")
	}
	if got := windowStatuses(nil, windows)[1]; got != picker.StatusNone {
		t.Fatalf("nil registry returned %v", got)
	}
}

func statusPane(status string) snapshot.Pane {
	return snapshot.Pane{CurrentCmd: "fixture", Meta: map[string]string{"fixture.status": status}}
}

func toPickerStatusMust(t *testing.T, pane snapshot.Pane) picker.WindowStatus {
	t.Helper()
	status, ok := (paneStatusFixture{}).Status(pane)
	if !ok {
		t.Fatalf("missing fixture status: %v", pane)
	}

	return toPickerStatus(status)
}
