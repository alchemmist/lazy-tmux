package tmux

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestAntexBindingOwnership(t *testing.T) {
	t.Parallel()
	const id = "01a0d3e1-d263-75c0-8189-c98bc7266f6c"
	base := antexBinding{
		Version:          1,
		ThreadID:         id,
		PID:              20,
		ProcessStartedAt: "Mon Sep 28 12:00:00 2026",
		PaneID:           "%19",
		SocketPath:       "/tmp/socket",
		Home:             "/home/test/.antex",
		ResumeArgv:       []string{"antex", "resume", id},
	}
	for _, tc := range []struct {
		name   string
		change func(*antexBinding)
		lines  []string
		valid  bool
	}{
		{"live owner", func(*antexBinding) {}, []string{"10 1 Ss zsh", "20 10 S+ antex resume old"}, true},
		{"adopted worktree", func(b *antexBinding) { b.CWD = "/worktrees/task with spaces" }, []string{"10 1 Ss zsh", "20 10 S+ antex"}, true},
		{"relative worktree", func(b *antexBinding) { b.CWD = "worktree" }, []string{"10 1 Ss zsh", "20 10 S+ antex"}, false},
		{"invalid worktree", func(b *antexBinding) { b.CWD = "/worktree\nother" }, []string{"10 1 Ss zsh", "20 10 S+ antex"}, false},
		{"exec replaces shell", func(b *antexBinding) { b.PID = 10 }, []string{"10 1 S+ antex"}, true},
		{"other pane", func(b *antexBinding) { b.PaneID = "%20" }, []string{"10 1 Ss zsh", "20 10 S+ antex"}, false},
		{
			"other server", func(b *antexBinding) { b.SocketPath = "/tmp/other" },
			[]string{"10 1 Ss zsh", "20 10 S+ antex"},
			false,
		},
		{
			"reused pid", func(b *antexBinding) { b.ProcessStartedAt = "yesterday" },
			[]string{"10 1 Ss zsh", "20 10 S+ antex"},
			false,
		},
		{"dead owner", func(*antexBinding) {}, []string{"10 1 Ss zsh"}, false},
		{"different process", func(*antexBinding) {}, []string{"10 1 Ss zsh", "20 10 S+ vim"}, false},
		{"nested agent", func(*antexBinding) {}, []string{"10 1 Ss zsh", "15 10 S+ antex", "20 15 S antex"}, false},
		{"nested direct pane", func(*antexBinding) {}, []string{"10 1 S+ antex", "20 10 S+ antex"}, false},
		{"unrelated process", func(*antexBinding) {}, []string{"10 1 Ss zsh", "20 1 S+ antex"}, false},
		{"suspended process", func(*antexBinding) {}, []string{"10 1 Ss zsh", "20 10 T antex"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := base
			tc.change(&b)
			raw, err := json.Marshal(b)
			if err != nil {
				t.Fatal(err)
			}
			got := validatedAntexMeta(
				string(raw),
				10,
				"%19",
				"/tmp/socket",
				newProcessSnapshot(tc.lines),
				func(int) string { return base.ProcessStartedAt },
			)
			if !tc.valid {
				if got != nil {
					t.Fatalf("unowned binding accepted: %v", got)
				}

				return
			}
			want := map[string]string{
				snapshot.AntexSessionIDMetaKey: id,
				"antex.session_id_source":      "binding-v1",
				"antex.home":                   base.Home,
				"antex.resume_argv":            `["antex","resume","` + id + `"]`,
			}
			if b.CWD != "" {
				want["antex.cwd"] = b.CWD
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
		})
	}
}

func TestRestoreNeverLaunchesGuessedAntexConversation(t *testing.T) {
	for _, source := range []string{"cwd", ""} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("SHELL", "/bin/sh")
			testutil.IsolatedTmux(t)
			testutil.Tmux(t, "new-session", "-d", "-s", "work")
			testutil.Tmux(t, "split-window", "-d", "-t", "=work:0")
			client := NewClient("tmux")
			client.SetRestoreResolver(integration.NewRegistry(antex.New(t.TempDir())))
			pane := snapshot.Pane{
				CurrentCmd: "antex",
				RestoreCmd: "antex resume 01a0d3e1-d263-75c0-8189-c98bc7266f6c",
				Meta: map[string]string{
					"antex.session_id":        "01a0cee1-2987-7b13-81cc-777ec4861be7",
					"antex.session_id_source": source,
				},
			}
			if command, err := client.checkedRestoreCommand(pane); err == nil || command != "" {
				t.Fatalf("ambiguous identity restored: %q %v", command, err)
			}
			if err := client.restoreWindowCommands(
				"work",
				snapshot.Window{Panes: []snapshot.Pane{pane, {Index: 1, CurrentCmd: "cat"}}},
				0,
			); err != nil {
				t.Fatal(err)
			}
			for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
				out := testutil.Tmux(t, "capture-pane", "-pJ", "-t", "=work:0.0")
				sibling := strings.TrimSpace(
					testutil.Tmux(
						t,
						"display-message",
						"-p",
						"-t",
						"=work:0.1",
						"#{pane_current_command}",
					),
				)
				if strings.Contains(out, "lazy-tmux: automatic resume skipped") &&
					sibling == "cat" {
					return
				}
				time.Sleep(25 * time.Millisecond)
			}
			t.Fatal("missing refusal or sibling failed to start")
		})
	}
}
