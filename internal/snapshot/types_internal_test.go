package snapshot

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionSnapshotJSONRoundTrip(t *testing.T) {
	in := SessionSnapshot{
		Version:     FormatVersion,
		SessionName: "dev",
		CapturedAt:  time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		CurrentWin:  1,
		CurrentPane: 2,
		Windows: []Window{
			{
				Index:      1,
				Name:       "code",
				Layout:     "abc",
				IsActive:   true,
				ActivePane: 2,
				Panes: []Pane{
					{
						Index:       2,
						CurrentPath: "/srv",
						CurrentCmd:  "go",
						RestoreCmd:  "go test",
						IsActive:    true,
					},
				},
			},
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out SessionSnapshot
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.SessionName != in.SessionName || out.CurrentWin != 1 || out.CurrentPane != 2 {
		t.Fatalf("round trip mismatch: %+v", out)
	}

	if len(out.Windows) != 1 || out.Windows[0].Panes[0].RestoreCmd != "go test" {
		t.Fatalf("window/pane round trip mismatch: %+v", out.Windows)
	}
}

func TestIndexJSONRoundTrip(t *testing.T) {
	idx := Index{
		Version:  FormatVersion,
		Updated:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Sessions: map[string]Record{"a": {SessionName: "a", Windows: 2, Panes: 4}},
	}

	raw, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out Index
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rec, ok := out.Sessions["a"]
	if !ok || rec.Windows != 2 || rec.Panes != 4 {
		t.Fatalf("index round trip mismatch: %+v", out)
	}
}
