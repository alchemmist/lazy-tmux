package app

import "github.com/alchemmist/lazy-tmux/internal/picker"

type PickerSortOptions = picker.SortOptions

type SessionSortKey = picker.SessionSortKey

type WindowSortKey = picker.WindowSortKey

type SessionSortField = picker.SessionSortField

type WindowSortField = picker.WindowSortField

const (
	SessionSortLastUsed = picker.SessionSortLastUsed
	SessionSortCaptured = picker.SessionSortCaptured
	SessionSortName     = picker.SessionSortName
	SessionSortWindows  = picker.SessionSortWindows
	SessionSortPanes    = picker.SessionSortPanes
)

const (
	WindowSortIndex = picker.WindowSortIndex
	WindowSortName  = picker.WindowSortName
	WindowSortPanes = picker.WindowSortPanes
	WindowSortCmd   = picker.WindowSortCmd
)

func DefaultPickerSortOptions() PickerSortOptions {
	return picker.DefaultSortOptions()
}

func ParsePickerSortOptions(sessionExpr, windowExpr string) (PickerSortOptions, error) {
	return picker.ParseSortOptions(sessionExpr, windowExpr)
}
