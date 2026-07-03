//go:build !lazy_fzf

package picker

import "strings"

// actionMode is the picker's colored modal flow: a resting browse mode plus one
// dedicated mode per in-place action. It is orthogonal to the text-prompt
// pickerMode (rename/new still pop a themed prompt once a target is picked).
type actionMode int

const (
	actionBrowse actionMode = iota
	actionDelete
	actionRename
	actionNew
	actionWake
	actionSleep
)

// hint is one footer key/label pair, e.g. {"↵", "remove"}.
type hint struct{ key, label string }

// pickerCommand binds a typed slash-command to its mode, accent color, palette
// description and mode-specific footer hints.
type pickerCommand struct {
	name   string
	mode   actionMode
	accent string
	label  string // short uppercase mode label shown in title/footer
	desc   string // palette description
	chord  string // direct keyboard shortcut shown in the palette and help panel
	hints  []hint // mode-specific footer hints (esc is appended by helpHints)
}

// pickerCommands is the ordered command palette. Wake/sleep share the cyan
// accent; delete is the only multi-select mode (space marks targets). chord is
// the window-scoped direct shortcut (the session-scoped ⌥ variants are shown in
// the help panel).
//
//nolint:gochecknoglobals // static command-palette table, never mutated
var pickerCommands = []pickerCommand{
	{
		name:   "delete",
		mode:   actionDelete,
		accent: colDelete,
		label:  "DELETE",
		desc:   "mark windows/sessions, remove them",
		chord:  "^d",
		hints:  []hint{{"space", "mark"}, {"↵", "remove"}},
	},
	{
		name:   "rename",
		mode:   actionRename,
		accent: colRename,
		label:  "RENAME",
		desc:   "rename a session or window",
		chord:  "^r",
		hints:  []hint{{"↵", "rename"}},
	},
	{
		name:   "new",
		mode:   actionNew,
		accent: colNew,
		label:  "NEW",
		desc:   "create a session or a window",
		chord:  "^n",
		hints:  []hint{{"↵", "create"}},
	},
	{
		name:   "wake",
		mode:   actionWake,
		accent: colSleepWake,
		label:  "WAKE",
		desc:   "wake a sleeping session",
		chord:  "⌥w",
		hints:  []hint{{"↵", "wake"}},
	},
	{
		name:   "sleep",
		mode:   actionSleep,
		accent: colSleepWake,
		label:  "SLEEP",
		desc:   "sleep a live session",
		chord:  "⌥s",
		hints:  []hint{{"↵", "sleep"}},
	},
}

// matchCommands returns the commands whose name starts with prefix (the text
// after the leading "/"). An empty prefix returns the whole palette.
func matchCommands(prefix string) []pickerCommand {
	prefix = strings.ToLower(strings.TrimSpace(prefix))

	out := make([]pickerCommand, 0, len(pickerCommands))
	for _, cmd := range pickerCommands {
		if strings.HasPrefix(cmd.name, prefix) {
			out = append(out, cmd)
		}
	}

	return out
}

// commandForMode returns the command backing an active action mode (and false
// for browse, which has no command).
func commandForMode(mode actionMode) (pickerCommand, bool) {
	for _, cmd := range pickerCommands {
		if cmd.mode == mode {
			return cmd, true
		}
	}

	return pickerCommand{}, false
}
