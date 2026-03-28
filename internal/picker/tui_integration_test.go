//go:build integration && !lazy_fzf

package picker

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestPickerTUISelectsWindow(t *testing.T) {
	sessions := []Session{
		{
			Record: snapshot.Record{SessionName: "work", CapturedAt: time.Now().UTC(), Windows: 1},
			Windows: []snapshot.Window{
				{Index: 0, Name: "editor"},
			},
		},
	}

	model := newPickerModel(sessions, DefaultSortOptions().Window, Actions{})
	var out bytes.Buffer
	prog := tea.NewProgram(
		model,
		tea.WithOutput(&out),
		tea.WithInput(nil),
		tea.WithWindowSize(80, 24),
	)

	resultCh := make(chan tea.Model, 1)
	errCh := make(chan error, 1)
	go func() {
		final, err := prog.Run()
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- final
	}()

	prog.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	prog.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	select {
	case err := <-errCh:
		t.Fatalf("program error: %v", err)
	case final := <-resultCh:
		res, ok := final.(pickerModel)
		if !ok {
			t.Fatalf("unexpected final model type: %T", final)
		}
		if res.selected.SessionName != "work" {
			t.Fatalf("expected session work to be selected, got %q", res.selected.SessionName)
		}
		if res.selected.WindowIndex == nil || *res.selected.WindowIndex != 0 {
			t.Fatalf("expected window index 0 selected, got %+v", res.selected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for program to finish")
	}

	if !strings.Contains(out.String(), "work") {
		t.Fatalf("expected output to contain session name, got:\n%s", out.String())
	}
}
