// Package integration provides an extensible framework for adapting interactive
// programs running in a pane to lazy-tmux's save/restore. Each program (Claude
// Code, editors, REPLs, …) implements the Integration interface and is plugged
// into a Registry; the core save/restore path stays free of program-specific
// special cases.
package integration

import "github.com/alchemmist/lazy-tmux/internal/snapshot"

// Integration adapts one program to lazy-tmux. Implementations must be cheap and
// side-effect-free outside Capture (which runs at save time and may read the
// program's own state on disk).
type Integration interface {
	// Name identifies the integration and namespaces its snapshot metadata
	// (stored on the pane as "<name>.<key>").
	Name() string
	// Matches reports whether this integration applies to a captured pane,
	// judged from its command / path.
	Matches(pane snapshot.Pane) bool
	// Capture gathers integration-specific state at save time (e.g. the Claude
	// session id), returned with its own un-namespaced keys. Returning a nil/empty
	// map (or an error) means "nothing to record" and must never break the save.
	Capture(pane snapshot.Pane) (map[string]string, error)
	// RestoreCommand builds the command to replay from the captured metadata
	// (de-namespaced to this integration's keys). Returning "" falls back to the
	// default restore behavior.
	RestoreCommand(pane snapshot.Pane, meta map[string]string) string
}

// StatusReporter is an optional capability: integrations that can report the
// live state of a running pane implement it. It is the extension point for the
// follow-up TUI picker status dots; defined here so the framework is ready, but
// not yet consumed by the picker.
type StatusReporter interface {
	Status(pane snapshot.Pane) (Status, bool)
}

// Status is the live state of a running, integrated pane.
type Status int

const (
	StatusUnknown          Status = iota
	StatusWorking                 // actively doing work (e.g. Claude is generating)
	StatusAwaitingDecision        // blocked on a question / permission the user must answer
	StatusAwaitingInput           // prompt ready, waiting for the user to type
	StatusIdle                    // running but nothing pending
	StatusError                   // crashed / errored
)
