// Package snapshot defines the serialized session-state model shared by the
// capture, store and restore paths.
package snapshot

import "time"

// FormatVersion is the current on-disk schema version written into every
// SessionSnapshot and Index.
const FormatVersion = 1

// SessionSnapshot is the full serialized state of one tmux session: its
// windows and panes plus which window/pane were active at capture time.
type SessionSnapshot struct {
	Version     int       `json:"version"`
	SessionName string    `json:"session_name"`
	CapturedAt  time.Time `json:"captured_at,omitzero"`
	CurrentWin  int       `json:"current_window"`
	CurrentPane int       `json:"current_pane"`
	Windows     []Window  `json:"windows"`
}

// Window is the captured state of one tmux window: its identity, pane layout
// string, panes, and whether it was the session's active window.
type Window struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Layout     string `json:"layout"`
	IsActive   bool   `json:"is_active"`
	ActivePane int    `json:"active_pane"`
	Panes      []Pane `json:"panes"`
}

// Pane is the captured state of one tmux pane: its working directory, running
// command, the command to re-run on restore, and an optional scrollback ref.
type Pane struct {
	Index       int            `json:"index"`
	CurrentPath string         `json:"current_path"`
	CurrentCmd  string         `json:"current_cmd"`
	RestoreCmd  string         `json:"restore_cmd,omitempty"`
	Scrollback  *ScrollbackRef `json:"scrollback,omitempty"`
	IsActive    bool           `json:"is_active"`

	// Meta carries program-integration metadata captured at save time, keyed by
	// "<integration>.<key>" (e.g. "claude.session_id"). It is opaque to the core
	// save/restore path; integrations read it back to build a resume command.
	// Omitted when empty, so older snapshots and JSON stay unaffected.
	Meta map[string]string `json:"meta,omitempty"`
}

// ScrollbackRef points at a pane's captured scrollback stored outside the
// snapshot JSON; Content carries the text in memory and is never serialized.
type ScrollbackRef struct {
	Ref     string `json:"ref,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Content string `json:"-"`
}

// Index is the on-disk catalog of all saved sessions, mapping session name to
// its Record.
type Index struct {
	Version  int               `json:"version"`
	Updated  time.Time         `json:"updated,omitzero"`
	Sessions map[string]Record `json:"sessions"`
}

// Record is one Index entry: where a session's snapshot file lives plus summary
// metadata (capture/access times, window and pane counts) used for listing and
// sorting without loading the snapshot itself.
type Record struct {
	SessionName  string    `json:"session_name"`
	File         string    `json:"file"`
	CapturedAt   time.Time `json:"captured_at,omitzero"`
	LastAccessed time.Time `json:"last_accessed,omitzero"`
	Windows      int       `json:"windows"`
	Panes        int       `json:"panes"`
}
