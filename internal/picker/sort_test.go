package picker

import "testing"

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
