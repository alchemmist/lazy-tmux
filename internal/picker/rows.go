//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func filteredTreeRows(sessions []Session, query string, windowSort []WindowSortKey) []pickerRow {
	rows := make([]pickerRow, 0)
	for _, s := range sessions {
		windows := make([]snapshot.Window, len(s.Windows))
		copy(windows, s.Windows)
		sortWindows(windows, windowSort)

		sessionMatch := query == "" || fuzzyMatch(query, strings.ToLower(s.Record.SessionName))
		matchedWindows := make([]snapshot.Window, 0, len(windows))
		for _, w := range windows {
			target := strings.ToLower(s.Record.SessionName + " " + w.Name)
			if query == "" || sessionMatch || fuzzyMatch(query, target) {
				matchedWindows = append(matchedWindows, w)
			}
		}

		if !sessionMatch && len(matchedWindows) == 0 {
			continue
		}

		rows = append(rows, pickerRow{
			target:     Target{SessionName: s.Record.SessionName},
			item:       s.Record.SessionName,
			captured:   s.Record.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			wins:       fmt.Sprintf("%d", s.Record.Windows),
			state:      sessionStateIcon(s.Restored),
			selectable: false,
		})

		for i, w := range matchedWindows {
			branch := "├─"
			if i == len(matchedWindows)-1 {
				branch = "╰─"
			}
			wi := w.Index
			rows = append(rows, pickerRow{
				target:     Target{SessionName: s.Record.SessionName, WindowIndex: &wi},
				item:       fmt.Sprintf("  %s [%d] %s", branch, w.Index, w.Name),
				captured:   "",
				wins:       "",
				state:      "",
				cmd:        windowPreviewCommand(w),
				windowName: w.Name,
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
	qr := []rune(query)
	qi := 0
	for _, r := range target {
		if qi >= len(qr) {
			break
		}
		if r == qr[qi] {
			qi++
		}
	}
	return qi == len(qr)
}
