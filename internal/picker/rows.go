//go:build !lazy_fzf

package picker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
			captured:   relativeTime(sess.Record.CapturedAt, time.Now()),
			wins:       strconv.Itoa(sess.Record.Windows),
			state:      sessionStateIcon(sess.Restored),
			selectable: false,
		})

		for idx, win := range matchedWindows {
			branch := "├─"
			if idx == len(matchedWindows)-1 {
				branch = "╰─"
			}

			windowIdx := win.Index

			status := sess.Statuses[win.Index]
			state := statusGlyph(status)

			rows = append(rows, pickerRow{
				target:     Target{SessionName: sess.Record.SessionName, WindowIndex: &windowIdx},
				item:       fmt.Sprintf("  %s [%d] %s", branch, win.Index, win.Name),
				captured:   "",
				wins:       "",
				state:      state,
				status:     status,
				cmd:        windowPreviewCommand(win),
				windowName: win.Name,
				selectable: true,
			})
		}
	}

	return rows
}

func (m pickerModel) modeSessions() []Session {
	switch m.action {
	case actionWake:
		return filterSessions(m.sessions, func(s Session) bool { return !s.Restored })
	case actionSleep:
		return filterSessions(m.sessions, func(s Session) bool { return s.Restored })
	default:
		return m.sessions
	}
}

func filterSessions(sessions []Session, keep func(Session) bool) []Session {
	out := make([]Session, 0, len(sessions))

	for _, s := range sessions {
		if keep(s) {
			out = append(out, s)
		}
	}

	return out
}

func (m pickerModel) decorateRows(rows []pickerRow) []pickerRow {
	switch m.action {
	case actionNew:
		sessionRows := sessionRowsOnly(rows)

		if strings.TrimSpace(m.queryInput.Value()) != "" {
			return sessionRows
		}

		out := make([]pickerRow, 0, len(sessionRows)+1)
		out = append(out, pickerRow{item: "＋ new session", synthetic: true})

		return append(out, sessionRows...)
	case actionWake, actionSleep:
		return sessionRowsOnly(rows)
	default:
		return rows
	}
}

func sessionRowsOnly(rows []pickerRow) []pickerRow {
	out := make([]pickerRow, 0, len(rows))

	for _, r := range rows {
		if r.target.WindowIndex == nil {
			out = append(out, r)
		}
	}

	return out
}

func (m pickerModel) rowSelectable(row pickerRow) bool {
	switch m.action {
	case actionDelete, actionRename:
		return true
	case actionNew, actionWake, actionSleep:
		return row.synthetic || row.target.WindowIndex == nil
	default:
		return row.selectable
	}
}

const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 30 * day
	year  = 365 * day
)

func relativeTime(then, now time.Time) string {
	if then.IsZero() {
		return ""
	}

	delta := max(now.Sub(then), 0)

	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < day:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < week:
		return fmt.Sprintf("%dd ago", int(delta/day))
	case delta < month:
		return fmt.Sprintf("%dw ago", int(delta/week))
	case delta < year:
		return fmt.Sprintf("%dmo ago", int(delta/month))
	default:
		return fmt.Sprintf("%dy ago", int(delta/year))
	}
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
