package integration

import (
	"sync"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// StatusLoader captures current ownership-verified panes, never stored snapshots.
type StatusLoader func() ([]snapshot.Pane, error)

// StatusSnapshot shares one lazy capture (including failure) across integrations
// and concurrent picker workers in a registry refresh. A new Scope gets a fresh
// snapshot. Integrations sharing it must use the same capture source.
type StatusSnapshot struct {
	once  sync.Once
	panes []snapshot.Pane
}

func (s *StatusSnapshot) Load(loader StatusLoader) []snapshot.Pane {
	if loader == nil {
		return nil
	}
	if s == nil {
		panes, err := loader()
		if err != nil {
			return nil
		}

		return panes
	}
	s.once.Do(func() {
		panes, err := loader()
		if err == nil {
			s.panes = panes
		}
	})

	return s.panes
}

// StatusScoper opts integrations using the common live capture into the shared
// registry-refresh snapshot without changing restore or enrichment behavior.
type StatusScoper interface {
	ScopeWithStatus(status *StatusSnapshot) Integration
}
