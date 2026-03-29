package picker

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestParsePickerSortOptions(t *testing.T) {
	opts, err := ParseSortOptions("name:asc,last-used:desc", "name:desc,index:asc")
	if err != nil {
		t.Fatalf("ParseSortOptions error: %v", err)
	}

	if len(opts.Session) != 2 || opts.Session[0].Field != SessionSortName || !opts.Session[1].Desc {
		t.Fatalf("unexpected session sort options: %+v", opts.Session)
	}

	if len(opts.Window) != 2 || opts.Window[0].Field != WindowSortName || !opts.Window[0].Desc {
		t.Fatalf("unexpected window sort options: %+v", opts.Window)
	}
}

func TestParsePickerSortOptionsRejectsDuplicateField(t *testing.T) {
	_, err := ParseSortOptions("name,name:desc", "")
	if err == nil {
		t.Fatal("expected duplicate session sort field error")
	}
}

func TestParseWindowSortKeysRejectsDuplicateField(t *testing.T) {
	_, err := parseWindowSortKeys("name,name:desc")
	if err == nil {
		t.Fatal("expected duplicate window sort field error")
	}
}

func TestParsePickerSortOptionsRejectsEmptySessionSortTerm(t *testing.T) {
	_, err := ParseSortOptions("name,,captured", "")
	if err == nil {
		t.Fatal("expected empty session sort term error")
	}
}

func TestParsePickerSortOptionsRejectsEmptyWindowSortTerm(t *testing.T) {
	_, err := ParseSortOptions("", "index,,name")
	if err == nil {
		t.Fatal("expected empty window sort term error")
	}
}

func TestParsePickerSortOptionsUnknownField(t *testing.T) {
	_, err := ParseSortOptions("wat:asc", "")
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestParsePickerSortOptionsInvalidDirection(t *testing.T) {
	_, err := ParseSortOptions("name:up", "")
	if err == nil {
		t.Fatal("expected invalid direction error")
	}
}

func TestParsePickerSortOptionsDefaultDirections(t *testing.T) {
	sess, desc, err := parseSessionSortPart("last-used")
	if err != nil {
		t.Fatalf("parseSessionSortPart: %v", err)
	}

	if sess != SessionSortLastUsed || !desc {
		t.Fatalf("expected last-used to default to desc, got %v desc=%v", sess, desc)
	}

	sess, desc, err = parseSessionSortPart("name")
	if err != nil {
		t.Fatalf("parseSessionSortPart: %v", err)
	}

	if sess != SessionSortName || desc {
		t.Fatalf("expected name to default to asc, got %v desc=%v", sess, desc)
	}

	win, wdesc, err := parseWindowSortPart("panes")
	if err != nil {
		t.Fatalf("parseWindowSortPart: %v", err)
	}

	if win != WindowSortPanes || !wdesc {
		t.Fatalf("expected panes to default to desc, got %v desc=%v", win, wdesc)
	}

	win, wdesc, err = parseWindowSortPart("index")
	if err != nil {
		t.Fatalf("parseWindowSortPart: %v", err)
	}

	if win != WindowSortIndex || wdesc {
		t.Fatalf("expected index to default to asc, got %v desc=%v", win, wdesc)
	}
}

func TestParseSessionField(t *testing.T) {
	tests := []struct {
		input   string
		field   SessionSortField
		success bool
	}{
		{"last-used", SessionSortLastUsed, true},
		{"last_accessed", SessionSortLastUsed, true},
		{"last-accessed", SessionSortLastUsed, true},
		{"captured", SessionSortCaptured, true},
		{"captured_at", SessionSortCaptured, true},
		{"captured-at", SessionSortCaptured, true},
		{"name", SessionSortName, true},
		{"windows", SessionSortWindows, true},
		{"panes", SessionSortPanes, true},
		{"LAST-USED", SessionSortLastUsed, true},
		{"NAME", SessionSortName, true},
		{"  name  ", SessionSortName, true},
		{"invalid", "", false},
		{"", "", false},
		{"unknown-field", "", false},
	}

	for _, testCase := range tests {
		field, ok := parseSessionField(testCase.input)
		if ok != testCase.success {
			t.Fatalf("parseSessionField(%q) success = %v, want %v", testCase.input, ok, testCase.success)
		}

		if ok && field != testCase.field {
			t.Fatalf("parseSessionField(%q) field = %v, want %v", testCase.input, field, testCase.field)
		}
	}
}

func TestParseWindowField(t *testing.T) {
	tests := []struct {
		input   string
		field   WindowSortField
		success bool
	}{
		{"index", WindowSortIndex, true},
		{"name", WindowSortName, true},
		{"panes", WindowSortPanes, true},
		{"cmd", WindowSortCmd, true},
		{"command", WindowSortCmd, true},
		{"INDEX", WindowSortIndex, true},
		{"NAME", WindowSortName, true},
		{"  index  ", WindowSortIndex, true},
		{"invalid", "", false},
		{"", "", false},
		{"unknown-field", "", false},
	}

	for _, testCase := range tests {
		field, ok := parseWindowField(testCase.input)
		if ok != testCase.success {
			t.Fatalf("parseWindowField(%q) success = %v, want %v", testCase.input, ok, testCase.success)
		}

		if ok && field != testCase.field {
			t.Fatalf("parseWindowField(%q) field = %v, want %v", testCase.input, field, testCase.field)
		}
	}
}

func TestCompareInt(t *testing.T) {
	tests := []struct {
		a, b int
		want int
	}{
		{0, 0, 0},
		{1, 2, -1},
		{2, 1, 1},
		{-1, -1, 0},
		{-2, -1, -1},
		{100, 50, 1},
	}

	for _, tt := range tests {
		if got := compareInt(tt.a, tt.b); got != tt.want {
			t.Fatalf("compareInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareSessionField(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-1 * time.Hour)
	later := now.Add(1 * time.Hour)

	rec1 := snapshot.Record{
		SessionName:  "aaa",
		Windows:      2,
		Panes:        5,
		LastAccessed: now,
		CapturedAt:   now,
	}
	rec2 := snapshot.Record{
		SessionName:  "bbb",
		Windows:      3,
		Panes:        4,
		LastAccessed: earlier,
		CapturedAt:   later,
	}

	tests := []struct {
		field SessionSortField
		want  int
	}{
		{SessionSortName, -1},            // "aaa" < "bbb"
		{SessionSortWindows, -1},         // 2 < 3
		{SessionSortPanes, 1},            // 5 > 4
		{SessionSortLastUsed, 1},         // now > earlier
		{SessionSortCaptured, -1},        // now < later
		{SessionSortField("invalid"), 0}, // unknown field
	}

	for _, tt := range tests {
		if got := compareSessionField(rec1, rec2, tt.field); got != tt.want {
			t.Fatalf("compareSessionField(..., %q) = %d, want %d", tt.field, got, tt.want)
		}
	}
}

func TestCompareWindowField(t *testing.T) {
	win1 := snapshot.Window{Index: 1, Name: "aaa", Panes: []snapshot.Pane{{}, {}}}
	win2 := snapshot.Window{Index: 2, Name: "bbb", Panes: []snapshot.Pane{{}, {}, {}}}

	tests := []struct {
		field WindowSortField
		want  int
	}{
		{WindowSortIndex, -1},           // 1 < 2
		{WindowSortName, -1},            // "aaa" < "bbb"
		{WindowSortPanes, -1},           // 2 < 3 panes
		{WindowSortField("invalid"), 0}, // unknown field
	}

	for _, tt := range tests {
		if got := compareWindowField(win1, win2, tt.field); got != tt.want {
			t.Fatalf("compareWindowField(..., %q) = %d, want %d", tt.field, got, tt.want)
		}
	}
}

func TestSortWindows(t *testing.T) {
	windows := []snapshot.Window{
		{Index: 2, Name: "zzz"},
		{Index: 1, Name: "aaa"},
		{Index: 3, Name: "bbb"},
	}

	SortWindows(windows, []WindowSortKey{{Field: WindowSortName, Desc: false}})

	if windows[0].Name != "aaa" || windows[1].Name != "bbb" || windows[2].Name != "zzz" {
		t.Fatalf("SortWindows by name failed: got %v", windows)
	}
}
