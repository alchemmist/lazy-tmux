package picker

import "github.com/alchemmist/lazy-tmux/internal/snapshot"

type Target struct {
	SessionName string
	WindowIndex *int
}

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

const (
	StatusNone WindowStatus = iota
	StatusWorking
	StatusAwaitingDecision
	StatusAwaitingInput
	StatusIdle
	StatusError
)

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
