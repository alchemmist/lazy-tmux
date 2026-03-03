package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type PickerSortOptions struct {
	Session []SessionSortKey
	Window  []WindowSortKey
}

type SessionSortKey struct {
	Field SessionSortField
	Desc  bool
}

type WindowSortKey struct {
	Field WindowSortField
	Desc  bool
}

type SessionSortField string
type WindowSortField string

const (
	SessionSortLastUsed SessionSortField = "last-used"
	SessionSortCaptured SessionSortField = "captured"
	SessionSortName     SessionSortField = "name"
	SessionSortWindows  SessionSortField = "windows"
	SessionSortPanes    SessionSortField = "panes"
)

const (
	WindowSortIndex WindowSortField = "index"
	WindowSortName  WindowSortField = "name"
	WindowSortPanes WindowSortField = "panes"
	WindowSortCmd   WindowSortField = "cmd"
)

func DefaultPickerSortOptions() PickerSortOptions {
	return PickerSortOptions{
		Session: []SessionSortKey{
			{Field: SessionSortLastUsed, Desc: true},
			{Field: SessionSortCaptured, Desc: true},
			{Field: SessionSortName, Desc: false},
		},
		Window: []WindowSortKey{
			{Field: WindowSortIndex, Desc: false},
			{Field: WindowSortName, Desc: false},
		},
	}
}

func ParsePickerSortOptions(sessionExpr, windowExpr string) (PickerSortOptions, error) {
	opts := DefaultPickerSortOptions()

	if strings.TrimSpace(sessionExpr) != "" {
		keys, err := parseSessionSortKeys(sessionExpr)
		if err != nil {
			return PickerSortOptions{}, err
		}
		opts.Session = keys
	}
	if strings.TrimSpace(windowExpr) != "" {
		keys, err := parseWindowSortKeys(windowExpr)
		if err != nil {
			return PickerSortOptions{}, err
		}
		opts.Window = keys
	}
	return opts, nil
}

func parseSessionSortKeys(expr string) ([]SessionSortKey, error) {
	seen := map[SessionSortField]struct{}{}
	parts, err := splitSortExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("session sort: %w", err)
	}
	keys := make([]SessionSortKey, 0, len(parts))
	for _, part := range parts {
		field, desc, err := parseSessionSortPart(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[field]; ok {
			return nil, fmt.Errorf("duplicate session sort field: %s", field)
		}
		seen[field] = struct{}{}
		keys = append(keys, SessionSortKey{Field: field, Desc: desc})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty session sort expression")
	}
	return keys, nil
}

func parseWindowSortKeys(expr string) ([]WindowSortKey, error) {
	seen := map[WindowSortField]struct{}{}
	parts, err := splitSortExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("window sort: %w", err)
	}
	keys := make([]WindowSortKey, 0, len(parts))
	for _, part := range parts {
		field, desc, err := parseWindowSortPart(part)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[field]; ok {
			return nil, fmt.Errorf("duplicate window sort field: %s", field)
		}
		seen[field] = struct{}{}
		keys = append(keys, WindowSortKey{Field: field, Desc: desc})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty window sort expression")
	}
	return keys, nil
}

func splitSortExpr(expr string) ([]string, error) {
	chunks := strings.Split(expr, ",")
	out := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		v := strings.TrimSpace(ch)
		if v == "" {
			return nil, fmt.Errorf("empty sort term in expression")
		}
		out = append(out, v)
	}
	return out, nil
}

func parseSessionSortPart(part string) (SessionSortField, bool, error) {
	name, descToken, hasDesc := splitSortPart(part)
	field, ok := parseSessionField(name)
	if !ok {
		return "", false, fmt.Errorf("unknown session sort field: %s", name)
	}
	desc := defaultSessionDirection(field)
	if hasDesc {
		v, err := parseDirection(descToken)
		if err != nil {
			return "", false, fmt.Errorf("session %s: %w", name, err)
		}
		desc = v
	}
	return field, desc, nil
}

func parseWindowSortPart(part string) (WindowSortField, bool, error) {
	name, descToken, hasDesc := splitSortPart(part)
	field, ok := parseWindowField(name)
	if !ok {
		return "", false, fmt.Errorf("unknown window sort field: %s", name)
	}
	desc := defaultWindowDirection(field)
	if hasDesc {
		v, err := parseDirection(descToken)
		if err != nil {
			return "", false, fmt.Errorf("window %s: %w", name, err)
		}
		desc = v
	}
	return field, desc, nil
}

func splitSortPart(part string) (name string, dir string, hasDir bool) {
	left, right, ok := strings.Cut(strings.TrimSpace(part), ":")
	if !ok {
		return strings.TrimSpace(part), "", false
	}
	return strings.TrimSpace(left), strings.TrimSpace(right), true
}

func parseDirection(in string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "asc":
		return false, nil
	case "desc":
		return true, nil
	default:
		return false, fmt.Errorf("invalid direction %q (expected asc|desc)", in)
	}
}

func parseSessionField(in string) (SessionSortField, bool) {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "last-used", "last_accessed", "last-accessed":
		return SessionSortLastUsed, true
	case "captured", "captured_at", "captured-at":
		return SessionSortCaptured, true
	case "name":
		return SessionSortName, true
	case "windows":
		return SessionSortWindows, true
	case "panes":
		return SessionSortPanes, true
	default:
		return "", false
	}
}

func parseWindowField(in string) (WindowSortField, bool) {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "index":
		return WindowSortIndex, true
	case "name":
		return WindowSortName, true
	case "panes":
		return WindowSortPanes, true
	case "cmd", "command":
		return WindowSortCmd, true
	default:
		return "", false
	}
}

func defaultSessionDirection(field SessionSortField) bool {
	switch field {
	case SessionSortName:
		return false
	default:
		return true
	}
}

func defaultWindowDirection(field WindowSortField) bool {
	switch field {
	case WindowSortIndex, WindowSortName:
		return false
	default:
		return true
	}
}

func sortSessionRecords(records []snapshot.Record, keys []SessionSortKey) {
	if len(records) == 0 {
		return
	}
	if len(keys) == 0 {
		keys = DefaultPickerSortOptions().Session
	}
	sort.Slice(records, func(i, j int) bool {
		return compareSessionRecord(records[i], records[j], keys) < 0
	})
}

func compareSessionRecord(a, b snapshot.Record, keys []SessionSortKey) int {
	for _, key := range keys {
		var cmp int
		switch key.Field {
		case SessionSortLastUsed:
			cmp = compareTime(a.LastAccessed, b.LastAccessed)
		case SessionSortCaptured:
			cmp = compareTime(a.CapturedAt, b.CapturedAt)
		case SessionSortName:
			cmp = strings.Compare(a.SessionName, b.SessionName)
		case SessionSortWindows:
			cmp = compareInt(a.Windows, b.Windows)
		case SessionSortPanes:
			cmp = compareInt(a.Panes, b.Panes)
		}
		if cmp == 0 {
			continue
		}
		if key.Desc {
			cmp = -cmp
		}
		return cmp
	}
	return strings.Compare(a.SessionName, b.SessionName)
}

func sortWindows(windows []snapshot.Window, keys []WindowSortKey) {
	if len(windows) == 0 {
		return
	}
	if len(keys) == 0 {
		keys = DefaultPickerSortOptions().Window
	}
	sort.Slice(windows, func(i, j int) bool {
		return compareWindow(windows[i], windows[j], keys) < 0
	})
}

func compareWindow(a, b snapshot.Window, keys []WindowSortKey) int {
	for _, key := range keys {
		var cmp int
		switch key.Field {
		case WindowSortIndex:
			cmp = compareInt(a.Index, b.Index)
		case WindowSortName:
			cmp = strings.Compare(a.Name, b.Name)
		case WindowSortPanes:
			cmp = compareInt(len(a.Panes), len(b.Panes))
		case WindowSortCmd:
			cmp = strings.Compare(windowPreviewCommand(a), windowPreviewCommand(b))
		}
		if cmp == 0 {
			continue
		}
		if key.Desc {
			cmp = -cmp
		}
		return cmp
	}
	return compareInt(a.Index, b.Index)
}

func compareInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func compareTime(a, b time.Time) int {
	if a.Before(b) {
		return -1
	}
	if a.After(b) {
		return 1
	}
	return 0
}
