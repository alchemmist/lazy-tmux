package snapshot

import "time"

const FormatVersion = 1

type SessionSnapshot struct {
	Version     int       `json:"version"`
	SessionName string    `json:"session_name"`
	CapturedAt  time.Time `json:"captured_at"`
	CurrentWin  int       `json:"current_window"`
	CurrentPane int       `json:"current_pane"`
	Windows     []Window  `json:"windows"`
}

type Window struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Layout     string `json:"layout"`
	IsActive   bool   `json:"is_active"`
	ActivePane int    `json:"active_pane"`
	Panes      []Pane `json:"panes"`
}

type Pane struct {
	Index        int    `json:"index"`
	CurrentPath  string `json:"current_path"`
	CurrentCmd   string `json:"current_cmd"`
	StartCommand string `json:"start_command"`
	IsActive     bool   `json:"is_active"`
}

type Index struct {
	Version  int               `json:"version"`
	Updated  time.Time         `json:"updated"`
	Sessions map[string]Record `json:"sessions"`
}

type Record struct {
	SessionName string    `json:"session_name"`
	File        string    `json:"file"`
	CapturedAt  time.Time `json:"captured_at"`
	Windows     int       `json:"windows"`
	Panes       int       `json:"panes"`
}
