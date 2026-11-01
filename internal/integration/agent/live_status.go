package agent

import (
	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// LiveStatus reads only panes returned by an ownership-verifying capture source.
// Persisted Agent.Status is deliberately not evidence of a running client.
type LiveStatus struct {
	Loader   integration.StatusLoader
	Snapshot *integration.StatusSnapshot
}

func (s LiveStatus) Scope(cache *integration.StatusSnapshot) LiveStatus {
	return LiveStatus{Loader: s.Loader, Snapshot: cache}
}

// Match requires a unique live owner of the saved identity, including exact home
// and cwd. Manual identities remain usable for restore, but not live status.
func (s LiveStatus) Match(pane snapshot.Pane, kind string) (snapshot.AgentSession, bool) {
	saved, ok := Session(pane, kind)
	if !ok || saved.Source == ManualSource || pending(pane, kind) || s.Loader == nil {
		return snapshot.AgentSession{}, false
	}
	var match snapshot.AgentSession
	count := 0
	for _, live := range s.Snapshot.Load(s.Loader) {
		value, valid := Session(live, kind)
		if !valid || value.Source == ManualSource || pending(live, kind) ||
			value.ID != saved.ID || value.Home != saved.Home {
			continue
		}
		count++
		match = value
	}
	if count != 1 || match.CWD != saved.CWD {
		return snapshot.AgentSession{}, false
	}

	return match, true
}

func pending(pane snapshot.Pane, kind string) bool {
	return pane.Meta[PendingKey] != "" || pane.Meta[kind+".restore_pending"] != ""
}

func (s LiveStatus) Status(pane snapshot.Pane, kind string) (integration.Status, bool) {
	value, ok := s.Match(pane, kind)
	if !ok {
		return integration.StatusUnknown, false
	}

	return State(value.Status)
}

func State(value string) (integration.Status, bool) {
	states := map[string]integration.Status{
		"working":           integration.StatusWorking,
		"idle":              integration.StatusIdle,
		"awaiting_input":    integration.StatusAwaitingInput,
		"awaiting_decision": integration.StatusAwaitingDecision,
		"error":             integration.StatusError,
	}
	state, ok := states[value]

	return state, ok
}
