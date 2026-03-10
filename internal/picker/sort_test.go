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
