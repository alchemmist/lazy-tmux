//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func filteredTreeRows(sessions []Session, query string, windowSort []WindowSortKey) []pickerRow {
	rows := make([]pickerRow, 0)

	for _, sess := range sessions {
		windows := make([]snapshot.Window, len(sess.Windows))
		copy(windows, sess.Windows)
		sortWindows(windows, windowSort)

		sessionMatch := query == "" || fuzzyMatch(query, strings.ToLower(sess.Record.SessionName))
		matchedWindows := make([]snapshot.Window, 0, len(windows))

		for _, w := range windows {
			target := strings.ToLower(sess.Record.SessionName + " " + w.Name)
			if query == "" || sessionMatch || fuzzyMatch(query, target) {
				matchedWindows = append(matchedWindows, w)
			}
		}

		if !sessionMatch && len(matchedWindows) == 0 {
			continue
		}

		rows = append(rows, pickerRow{
			target:     Target{SessionName: sess.Record.SessionName},
			item:       sess.Record.SessionName,
			captured:   sess.Record.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			wins:       fmt.Sprintf("%d", sess.Record.Windows),
			state:      sessionStateIcon(sess.Restored),
			selectable: false,
		})

		for idx, win := range matchedWindows {
			branch := "├─"
			if idx == len(matchedWindows)-1 {
				branch = "╰─"
			}

			wi := win.Index
			rows = append(rows, pickerRow{
				target:     Target{SessionName: sess.Record.SessionName, WindowIndex: &wi},
				item:       fmt.Sprintf("  %s [%d] %s", branch, win.Index, win.Name),
				captured:   "",
				wins:       "",
				state:      "",
				cmd:        windowPreviewCommand(win),
				windowName: win.Name,
				selectable: true,
			})
		}
	}

	return rows
}

func sessionStateIcon(restored bool) string {
	if restored {
		return "✓"
	}

	return ""
}

func fuzzyMatch(query, target string) bool {
	if query == "" {
		return true
	}

	queryRunes := []rune(query)
	queryIndex := 0

	for _, r := range target {
		if queryIndex >= len(queryRunes) {
			break
		}

		if r == queryRunes[queryIndex] {
			queryIndex++
		}
	}

	return queryIndex == len(queryRunes)
}
