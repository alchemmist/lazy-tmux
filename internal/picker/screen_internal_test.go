//go:build !lazy_fzf

package picker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPickerScreens(t *testing.T) {
	t.Parallel()
	alpha := makeSession("alpha", true, "editor", "shell")
	beta := makeSession("studio", false, "notes")
	alpha.Record.CapturedAt = time.Time{}
	beta.Record.CapturedAt = time.Time{}
	model := newTestModel(t, &recordingActions{}, alpha, beta)
	for name, frame := range map[string]string{
		"sessions-100x24": model.View().Content,
		"sessions-46x12":  feed(t, model, tea.WindowSizeMsg{Width: 46, Height: 12}).View().Content,
		"empty-100x24":    newTestModel(t, &recordingActions{}).View().Content,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			actual := ansi.Strip(frame) + "\n"
			path := filepath.Join("testdata", "screens", name+".txt")
			if os.Getenv("UPDATE_PICKER_SCREENS") == "1" {
				if os.Getenv("CI") != "" {
					t.Fatal("refusing to rewrite fixtures in CI")
				}
				if err := os.WriteFile(path, []byte(actual), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != actual {
				t.Fatalf("screen differs from %s\nwant:\n%s\ngot:\n%s", path, want, actual)
			}
		})
	}
}
