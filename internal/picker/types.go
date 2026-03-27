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
}

type Actions struct {
	DeleteWindow  func(session string, windowIndex int) error
	DeleteSession func(session string) error
	RenameWindow  func(session string, windowIndex int, name string) error
	RenameSession func(session string, name string) error
	NewSession    func(name string) error
	NewWindow     func(session string, name string) error
	Reload        func() ([]Session, error)
	Wakeup        func(session string) error
	Sleep         func(session string) error
}
