//go:build !lazy_fzf

package picker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type rankedSession struct {
	data    Session
	windows []rankedWindow
	score   int
}

type rankedWindow struct {
	window snapshot.Window
	score  int
}

func filteredTreeRows(
	sessions []Session,
	query string,
	windowSort []WindowSortKey,
	spinnerFrame int,
) []pickerRow {
	rows := make([]pickerRow, 0)
	ranked := rankSessions(sessions, query, windowSort)

	for _, match := range ranked {
		sess := match.data

		rows = append(rows, pickerRow{
			target:     Target{SessionName: sess.Record.SessionName},
			item:       sess.Record.SessionName,
			captured:   relativeTime(sess.Record.CapturedAt, time.Now()),
			wins:       strconv.Itoa(sess.Record.Windows),
			state:      sessionStateIcon(sess.Restored),
			selectable: false,
		})

		for idx, rankedWin := range match.windows {
			branch := "├─"
			if idx == len(match.windows)-1 {
				branch = "╰─"
			}

			rows = append(rows, windowRow(sess, rankedWin.window, branch, spinnerFrame))
		}
	}

	return rows
}

func rankSessions(
	sessions []Session,
	query string,
	windowSort []WindowSortKey,
) []rankedSession {
	ranked := make([]rankedSession, 0, len(sessions))

	for _, sess := range sessions {
		match, ok := rankSession(sess, query, windowSort)
		if !ok {
			continue
		}
		ranked = append(ranked, match)
	}

	if query != "" {
		sort.SliceStable(ranked, func(i, j int) bool {
			return ranked[i].score > ranked[j].score
		})
	}

	return ranked
}

func rankSession(
	sess Session,
	query string,
	windowSort []WindowSortKey,
) (rankedSession, bool) {
	windows := make([]snapshot.Window, len(sess.Windows))
	copy(windows, sess.Windows)
	sortWindows(windows, windowSort)

	sessionScore, sessionMatch := fuzzyScore(query, sess.Record.SessionName)
	matchedWindows, bestScore := rankWindows(
		windows,
		sess.Record.SessionName,
		query,
		sessionScore,
		sessionMatch,
	)
	if query != "" && !sessionMatch && len(matchedWindows) == 0 {
		return rankedSession{}, false
	}

	if query != "" {
		sort.SliceStable(matchedWindows, func(i, j int) bool {
			return matchedWindows[i].score > matchedWindows[j].score
		})
	}

	return rankedSession{data: sess, windows: matchedWindows, score: bestScore}, true
}

func rankWindows(
	windows []snapshot.Window,
	sessionName string,
	query string,
	sessionScore int,
	sessionMatch bool,
) ([]rankedWindow, int) {
	matched := make([]rankedWindow, 0, len(windows))
	bestScore := sessionScore

	for _, window := range windows {
		windowScore, windowMatch := fuzzyScore(query, window.Name)
		combinedScore, combinedMatch := fuzzyScore(query, sessionName+" "+window.Name)
		if combinedScore > windowScore {
			windowScore = combinedScore
			windowMatch = combinedMatch
		}
		if query != "" && !sessionMatch && !windowMatch {
			continue
		}
		if !windowMatch {
			windowScore = sessionScore
		}

		matched = append(matched, rankedWindow{window: window, score: windowScore})
		bestScore = max(bestScore, windowScore)
	}

	return matched, bestScore
}

func windowRow(sess Session, win snapshot.Window, branch string, spinnerFrame int) pickerRow {
	windowIdx := win.Index
	status := sess.Statuses[win.Index]

	return pickerRow{
		target:     Target{SessionName: sess.Record.SessionName, WindowIndex: &windowIdx},
		item:       fmt.Sprintf("  %s [%d] %s", branch, win.Index, win.Name),
		state:      statusGlyphFrame(status, spinnerFrame),
		status:     status,
		cmd:        windowPreviewCommand(win),
		windowName: win.Name,
		selectable: true,
	}
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
	_, ok := fuzzyScore(query, target)

	return ok
}

const (
	exactMatchScore       = 1_000_000
	prefixMatchScore      = 900_000
	substringMatchScore   = 800_000
	typoMatchScore        = 700_000
	subsequenceMatchScore = 600_000
	minTypoQueryLength    = 3
	mediumQueryLength     = 4
	longQueryLength       = 8
)

func fuzzyScore(query, target string) (int, bool) {
	query = strings.ToLower(strings.TrimSpace(query))
	target = strings.ToLower(strings.TrimSpace(target))
	if query == "" {
		return 0, true
	}
	if target == "" {
		return 0, false
	}
	if query == target {
		return exactMatchScore, true
	}
	if strings.HasPrefix(target, query) {
		return prefixMatchScore - len([]rune(target)), true
	}
	if idx := strings.Index(target, query); idx >= 0 {
		return substringMatchScore - idx*100 - len([]rune(target)), true
	}
	if score, ok := typoScore(query, target); ok {
		return score, true
	}

	queryRunes := []rune(query)
	targetRunes := []rune(target)
	queryIndex := 0
	first := -1
	last := -1

	for idx, r := range targetRunes {
		if queryIndex >= len(queryRunes) {
			break
		}
		if r != queryRunes[queryIndex] {
			continue
		}
		if first < 0 {
			first = idx
		}
		last = idx
		queryIndex++
	}

	if queryIndex != len(queryRunes) {
		return 0, false
	}

	gaps := last - first + 1 - len(queryRunes)

	return subsequenceMatchScore - gaps*100 - first*10 - len(targetRunes), true
}

func typoScore(query, target string) (int, bool) {
	queryRunes := []rune(query)
	if len(queryRunes) < minTypoQueryLength {
		return 0, false
	}

	maxDistance := 1
	if len(queryRunes) > mediumQueryLength {
		maxDistance = 2
	}
	if len(queryRunes) > longQueryLength {
		maxDistance = 3
	}

	best := maxDistance + 1
	for field := range strings.FieldsSeq(target) {
		fieldRunes := []rune(field)
		candidates := [][]rune{fieldRunes}
		if len(fieldRunes) > len(queryRunes) {
			candidates = append(candidates, fieldRunes[:len(queryRunes)])
		}

		for _, candidate := range candidates {
			distance := damerauLevenshtein(queryRunes, candidate)
			if distance < best {
				best = distance
			}
		}
	}

	if best > maxDistance {
		return 0, false
	}

	return typoMatchScore - best*10_000 - len([]rune(target)), true
}

func damerauLevenshtein(left, right []rune) int {
	rows := len(left) + 1
	cols := len(right) + 1
	matrix := make([][]int, rows)

	for i := range rows {
		matrix[i] = make([]int, cols)
		matrix[i][0] = i
	}
	for j := range cols {
		matrix[0][j] = j
	}

	for row := 1; row < rows; row++ {
		for col := 1; col < cols; col++ {
			cost := 0
			if left[row-1] != right[col-1] {
				cost = 1
			}
			matrix[row][col] = min(
				matrix[row-1][col]+1,
				matrix[row][col-1]+1,
				matrix[row-1][col-1]+cost,
			)
			if row > 1 && col > 1 && left[row-1] == right[col-2] && left[row-2] == right[col-1] {
				matrix[row][col] = min(matrix[row][col], matrix[row-2][col-2]+1)
			}
		}
	}

	return matrix[len(left)][len(right)]
}
