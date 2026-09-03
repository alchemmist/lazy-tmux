package integration

import "github.com/alchemmist/lazy-tmux/internal/snapshot"

type Integration interface {
	Name() string
	Matches(pane snapshot.Pane) bool
	Capture(pane snapshot.Pane) (map[string]string, error)
	RestoreCommand(pane snapshot.Pane, meta map[string]string) string
}

type StatusReporter interface {
	Status(pane snapshot.Pane) (Status, bool)
}

type Scoper interface {
	Scope() Integration
}

type Status int

const (
	StatusUnknown Status = iota
	StatusWorking
	StatusAwaitingDecision
	StatusAwaitingInput
	StatusIdle
	StatusError
)
