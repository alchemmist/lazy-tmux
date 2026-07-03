package app

import (
	"fmt"

	"github.com/alchemmist/lazy-tmux/internal/picker"
)

// PickerSortOptions aliases picker.SortOptions so CLI code does not import
// picker directly.
type PickerSortOptions = picker.SortOptions

// SessionSortKey aliases picker.SessionSortKey so CLI code does not import
// picker directly.
type SessionSortKey = picker.SessionSortKey

// WindowSortKey aliases picker.WindowSortKey so CLI code does not import
// picker directly.
type WindowSortKey = picker.WindowSortKey

// SessionSortField aliases picker.SessionSortField so CLI code does not import
// picker directly.
type SessionSortField = picker.SessionSortField

// WindowSortField aliases picker.WindowSortField so CLI code does not import
// picker directly.
type WindowSortField = picker.WindowSortField

// Session sort fields re-exported from picker so CLI code does not import
// picker directly.
const (
	SessionSortLastUsed = picker.SessionSortLastUsed
	SessionSortCaptured = picker.SessionSortCaptured
	SessionSortName     = picker.SessionSortName
	SessionSortWindows  = picker.SessionSortWindows
	SessionSortPanes    = picker.SessionSortPanes
)

// Window sort fields re-exported from picker so CLI code does not import
// picker directly.
const (
	WindowSortIndex = picker.WindowSortIndex
	WindowSortName  = picker.WindowSortName
	WindowSortPanes = picker.WindowSortPanes
	WindowSortCmd   = picker.WindowSortCmd
)

// DefaultPickerSortOptions aliases picker.DefaultSortOptions so CLI code does
// not import picker directly.
func DefaultPickerSortOptions() PickerSortOptions {
	return picker.DefaultSortOptions()
}

// ParsePickerSortOptions wraps picker.ParseSortOptions, parsing the session and
// window sort expressions from CLI flags so CLI code does not import picker
// directly.
func ParsePickerSortOptions(sessionExpr, windowExpr string) (PickerSortOptions, error) {
	opts, err := picker.ParseSortOptions(sessionExpr, windowExpr)
	if err != nil {
		return picker.SortOptions{}, fmt.Errorf("parse sort options: %w", err)
	}

	return opts, nil
}
