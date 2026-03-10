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
