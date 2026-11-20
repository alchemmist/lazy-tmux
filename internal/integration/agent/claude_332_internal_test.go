package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestClaudeConfigOverrideProvenance(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	defaultHome := filepath.Join(home, ".claude")
	unset, explicit, custom := "", defaultHome, filepath.Join(t.TempDir(), "custom")
	for _, tc := range []struct {
		name, home string
		override   *string
		want       string
	}{
		{"unset", defaultHome, &unset, "env -u CLAUDE_CONFIG_DIR"},
		{"explicit default", defaultHome, &explicit, "env 'CLAUDE_CONFIG_DIR=" + defaultHome + "'"},
		{"explicit custom", custom, &custom, "env 'CLAUDE_CONFIG_DIR=" + custom + "'"},
		{"legacy default", defaultHome, nil, "env -u CLAUDE_CONFIG_DIR"},
		{"legacy custom", custom, nil, "env 'CLAUDE_CONFIG_DIR=" + custom + "'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := snapshot.AgentSession{
				Version:         1,
				Kind:            "claude",
				ID:              "exact-id",
				Home:            tc.home,
				CWD:             home,
				Argv:            []string{"claude", "--resume", "exact-id"},
				Source:          HookSource,
				ClaudeConfigDir: tc.override,
			}
			raw, err := json.Marshal(session)
			if err != nil {
				t.Fatal(err)
			}
			var restored snapshot.AgentSession
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(session, restored) {
				t.Fatal("snapshot lost override provenance")
			}
			command, err := Restore(snapshot.Pane{Agent: &restored}, "claude")
			if err != nil || !strings.HasPrefix(command, tc.want+" ") {
				t.Fatalf("command=%q err=%v want prefix %q", command, err, tc.want)
			}
		})
	}
}

func TestClaudeCommonLaunchOptionsPreserveBoundaries(t *testing.T) {
	t.Parallel()
	for _, options := range [][]string{
		{"-n", "my project"},
		{"--name", "my project"},
		{"--name=my project"},
		{"--append-system-prompt", "additional context with 'quotes'"},
		{"--append-system-prompt=-literal"},
		{"--mcp-config", "/tmp/first config.json", "{\"mcpServers\":{}}", "--model", "sonnet"},
		{"--mcp-config=/tmp/first config.json", "--name", "work"},
	} {
		t.Run(strings.Join(options, " "), func(t *testing.T) {
			t.Parallel()
			argv, err := ResumeArgv(
				"claude",
				"exact-id",
				"/work",
				append([]string{"claude"}, options...),
			)
			want := append([]string{"claude", "--resume", "exact-id"}, options...)
			if err != nil || !reflect.DeepEqual(argv, want) {
				t.Fatalf("argv=%q err=%v want=%q", argv, err, want)
			}
		})
	}
	for _, flag := range []string{"-n", "--name", "--append-system-prompt", "--mcp-config", "--unknown-flag"} {
		if _, err := ResumeArgv(
			"claude",
			"exact-id",
			"/work",
			[]string{"claude", flag},
		); err == nil {
			t.Fatalf("accepted missing/unknown option %s", flag)
		}
	}
}
