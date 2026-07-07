//go:build !lazy_fzf

package picker

import "strings"

type actionMode int

const (
	actionBrowse actionMode = iota
	actionDelete
	actionRename
	actionNew
	actionWake
	actionSleep
)

type hint struct{ key, label string }

type pickerCommand struct {
	name   string
	mode   actionMode
	accent string
	label  string
	desc   string
	chord  string
	hints  []hint
}

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

func commandForMode(mode actionMode) (pickerCommand, bool) {
	for _, cmd := range pickerCommands {
		if cmd.mode == mode {
			return cmd, true
		}
	}

	return pickerCommand{}, false
}
