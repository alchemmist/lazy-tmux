package picker

import (
	"errors"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errTUIDisabled = errors.New("TUI picker disabled in fzf-only build")

type Target struct {
	SessionName string
	WindowIndex *int
}

type QuickSession struct {
	Name     string
	Restored bool
	Current  bool
	Working  bool
}

type Session struct {
	Record   snapshot.Record
	Windows  []snapshot.Window
	Restored bool

	Statuses map[int]WindowStatus
}

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
	SetTheme      func(theme string) error
}
