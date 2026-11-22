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
	for _, name := range []string{"sessions-100x24", "sessions-46x12", "empty-100x24"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var sessions []Session
			if name != "empty-100x24" {
				sessions = []Session{
					makeSession("alpha", true, "editor", "shell"),
					makeSession("studio", false, "notes"),
				}
				for i := range sessions {
					sessions[i].Record.CapturedAt = time.Time{}
				}
			}
			model := newTestModel(t, sessions...)
			if name == "sessions-46x12" {
				model = feed(t, model, tea.WindowSizeMsg{Width: 46, Height: 12})
			}
			actual := ansi.Strip(model.View().Content) + "\n"
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
