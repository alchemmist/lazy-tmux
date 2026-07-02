// Package picker implements the interactive session and window pickers: a
// bubbletea TUI (default build) and an fzf-driven engine (lazy_fzf build tag).
package picker

import "github.com/alchemmist/lazy-tmux/internal/snapshot"

// Target identifies what the user picked: a session and, when a specific
// window was chosen, its index. A nil WindowIndex means "the whole session".
type Target struct {
	SessionName string
	WindowIndex *int
}

// Session is one pickable entry: the index record of a saved session, its
// windows, whether it is currently restored (live in tmux), and any live
// window statuses.
type Session struct {
	Record   snapshot.Record
	Windows  []snapshot.Window
	Restored bool

	// Statuses holds the live program status per window index (e.g. a Claude
	// window that is working / awaiting a decision / idle). Only populated for
	// live sessions; windows without a status are absent.
	Statuses map[int]WindowStatus
}

// WindowStatus is a live program status surfaced as a colored dot in the picker.
type WindowStatus int

// Window statuses in escalating order of attention required; StatusNone means
// no integration reported anything for the window.
const (
	StatusNone WindowStatus = iota
	StatusWorking
	StatusAwaitingDecision
	StatusAwaitingInput
	StatusIdle
	StatusError
)

// Actions holds the callbacks the picker invokes to mutate sessions and
// windows. Any callback may be nil, in which case the picker reports the
// corresponding action as unavailable instead of calling it.
type Actions struct {
	DeleteWindow  func(session string, windowIndex int) error
	DeleteSession func(session string) error
	RenameWindow  func(session string, windowIndex int, name string) error
	RenameSession func(session, name string) error
	NewSession    func(name string) error
	NewWindow     func(session, name string) error
	Reload        func() ([]Session, error)
	Wakeup        func(session string) error
	Sleep         func(session string) error
}
