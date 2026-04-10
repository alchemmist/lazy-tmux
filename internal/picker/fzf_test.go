package picker

import (
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestChooseSessionFZF(t *testing.T) {
	testutil.SkipIfNotIntegration(t)
	testutil.RequireFZF(t)

	tests := []struct {
		name    string
		records []snapshot.Record
		want    string
	}{
		{
			name:    "selects first record",
			records: []snapshot.Record{{SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}, {SessionName: "beta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3}},
			want:    "alpha",
		},
		{
			name:    "preserves input order",
			records: []snapshot.Record{{SessionName: "gamma", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}, {SessionName: "delta", CapturedAt: time.Now().UTC(), Windows: 2, Panes: 3}, {SessionName: "alpha", CapturedAt: time.Now().UTC(), Windows: 3, Panes: 5}},
			want:    "gamma",
		},
		{
			name:    "single record",
			records: []snapshot.Record{{SessionName: "solo", CapturedAt: time.Now().UTC(), Windows: 1, Panes: 1}},
			want:    "solo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := ChooseSessionFZF(tt.records)
			if err != nil {
				t.Fatalf("ChooseSessionFZF: %v", err)
			}

			if selected != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, selected)
			}
		})
	}
}

func TestChooseSessionFZFNoSessions(t *testing.T) {
	_, err := ChooseSessionFZF([]snapshot.Record{})
	if err == nil {
		t.Fatal("expected error for empty records")
	}

	if !strings.Contains(err.Error(), "no sessions") {
		t.Fatalf("unexpected error: %v", err)
	}
}
