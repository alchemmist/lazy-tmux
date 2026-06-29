//go:build !lazy_fzf

package picker

import (
	"fmt"
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
			wins:       fmt.Sprintf("%d", sess.Record.Windows),
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
			state := ""

			if status != StatusNone {
				state = statusDot
			}

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

// modeSessions narrows the session list to those valid for the active mode:
// wake shows only sleeping (not live) sessions, sleep only live ones; the other
// modes see every session.
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

// decorateRows adapts the base session/window tree to the active mode: new,
// wake and sleep act on whole sessions, so window rows are dropped; new also
// prepends a synthetic "＋ new session" row. Delete, rename and browse keep the
// full tree.
func (m pickerModel) decorateRows(rows []pickerRow) []pickerRow {
	switch m.action {
	case actionNew:
		sessionRows := sessionRowsOnly(rows)

		// While the user is filtering, drop the synthetic row so Enter binds to
		// the matched session (new window) rather than "new session".
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

// rowSelectable reports whether a row can be acted on in the active mode. Browse
// keeps the inherent window-only selectability; delete and rename also let you
// target session headers; new/wake/sleep target sessions (and the synthetic
// new-session row).
func (m pickerModel) rowSelectable(row pickerRow) bool {
	switch m.action {
	case actionDelete, actionRename:
		return true // sessions and windows are both valid targets
	case actionNew, actionWake, actionSleep:
		return row.synthetic || row.target.WindowIndex == nil
	default:
		return row.selectable
	}
}

// relativeTime renders a capture time as a compact, scannable age such as
// "just now", "3m ago", "2h ago", "5d ago" or "4mo ago".
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
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	case delta < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(delta.Hours())/24)
	case delta < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(delta.Hours())/(24*7))
	case delta < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(delta.Hours())/(24*30))
	default:
		return fmt.Sprintf("%dy ago", int(delta.Hours())/(24*365))
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
