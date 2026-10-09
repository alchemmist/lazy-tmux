package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestAntexBindingWorktreeRestore(t *testing.T) {
	t.Parallel()
	for _, cwd := range []string{"", "/worktrees/task with spaces"} {
		t.Run(cwd, func(t *testing.T) {
			t.Parallel()
			pane := snapshot.Pane{
				CurrentCmd:  "antex",
				CurrentPath: "/original",
				Meta: map[string]string{
					"antex.session_id":        "thread-id",
					"antex.session_id_source": BindingSource,
					"antex.home":              "/home/.antex",
					"antex.resume_argv":       `["antex","resume","thread-id"]`,
					"antex.cwd":               cwd,
				},
			}
			wantCWD := cwd
			if wantCWD == "" {
				wantCWD = pane.CurrentPath
			}
			got, err := Plan(pane, snapshot.AgentAntex)
			if err != nil {
				t.Fatal(err)
			}
			want := RestorePlan{
				Argv: []string{"antex", "resume", "thread-id", "-c", `tui.resume_cwd="session"`},
				Env:  []string{"ANTEX_HOME=/home/.antex"},
				CWD:  wantCWD,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v; want %+v", got, want)
			}
		})
	}
}

func TestThreeAgentsNeverCrossRestoreIdentity(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"codex", "antex", "claude"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			sub := "resume"
			if kind == "claude" {
				sub = "--resume"
			}
			p := snapshot.Pane{
				CurrentCmd:  kind,
				CurrentPath: "/workspace",
				Agent: &snapshot.AgentSession{
					Version: 1,
					Kind:    kind,
					ID:      "same-id",
					Home:    "/home/" + kind,
					CWD:     "/workspace",
					Argv:    []string{kind, sub, "same-id", "--model", "model with spaces"},
					Source:  HookSource,
					Status:  "idle",
				},
			}
			got, err := Restore(p, kind)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, Quote(kind)+" "+Quote(sub)+" 'same-id'") ||
				!strings.Contains(got, "'model with spaces'") {
				t.Fatal(got)
			}
			for _, other := range []string{"codex", "antex", "claude"} {
				if other == kind {
					continue
				}
				if _, err := Restore(p, other); err == nil {
					t.Fatal("cross-agent restore accepted")
				}
			}
		})
	}
}

func TestExecutableMatchingDoesNotMatchIncidentalText(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"echo claude", "vim codex.go", "/tmp/my-antex-helper", "node /tmp/claude/cli.js"} {
		if Kind(command) != "" {
			t.Fatal(command)
		}
	}
	for _, command := range []string{
		"/usr/bin/codex resume id",
		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js",
	} {
		if Kind(command) == "" {
			t.Fatal(command)
		}
	}
}
