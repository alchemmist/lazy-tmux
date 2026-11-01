package pi

import (
	"sync"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// StatusLoader returns current, ownership-verified panes from the configured
// tmux server. Saved status fields are never a source of live activity.
type StatusLoader func() ([]snapshot.Pane, error)

type statusSnapshot struct {
	once  sync.Once
	panes []snapshot.Pane
}

func (i *Integration) statusPanes() []snapshot.Pane {
	if i.statusCache == nil {
		panes, err := i.loadStatus()
		if err != nil {
			return nil
		}

		return panes
	}
	i.statusCache.once.Do(func() {
		panes, err := i.loadStatus()
		if err == nil {
			i.statusCache.panes = panes
		}
	})

	return i.statusCache.panes
}

func (i *Integration) WithStatusLoader(loader StatusLoader) *Integration {
	i.loadStatus = loader

	return i
}

func (i *Integration) Status(pane snapshot.Pane) (integration.Status, bool) {
	if !i.canReadStatus(pane) {
		return integration.StatusUnknown, false
	}
	file, sessionID := pane.Meta["pi.session_file"], pane.Meta["pi.session_id"]
	if file == "" || sessionID == "" {
		return integration.StatusUnknown, false
	}
	livePanes := i.statusPanes()
	var match *snapshot.Pane
	for index := range livePanes {
		live := &livePanes[index]
		if live.Meta["pi.session_file"] != file || live.LiveStatusID != sessionID {
			continue
		}
		// Multiple live clients for the same history cannot be attributed to a
		// particular saved window. Do not borrow a different client's spinner.
		if match != nil {
			return integration.StatusUnknown, false
		}
		match = live
	}
	if match == nil || !statusHeaderMatches(file, sessionID, pane.CurrentPath, match.CurrentPath) {
		return integration.StatusUnknown, false
	}

	return piState(match.LiveStatus)
}

func statusHeaderMatches(path, sessionID, savedCWD, liveCWD string) bool {
	return VerifySessionFile(path, sessionID, savedCWD) && sameDirectory(savedCWD, liveCWD)
}

func piState(state string) (integration.Status, bool) {
	switch state {
	case "working":
		return integration.StatusWorking, true
	case "idle":
		return integration.StatusIdle, true
	case "awaiting_input":
		return integration.StatusAwaitingInput, true
	case "awaiting_decision":
		return integration.StatusAwaitingDecision, true
	case "error":
		return integration.StatusError, true
	default:
		return integration.StatusUnknown, false
	}
}

func (i *Integration) canReadStatus(pane snapshot.Pane) bool {
	return i.Matches(pane) && pane.Meta[agent.PendingKey] == "" &&
		pane.Meta["pi.restore_pending"] == "" &&
		!noSession(pane.CurrentCmd) &&
		!noSession(pane.RestoreCmd) &&
		!nonInteractive(pane.ProcessArgv) &&
		i.loadStatus != nil
}
