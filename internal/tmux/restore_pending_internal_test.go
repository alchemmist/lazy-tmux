package tmux

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestFailedAntexBootstrapKeepsRestoreIntent(t *testing.T) {
	t.Parallel()
	id := "01a09f96-a7d1-75f1-b8d8-6df25aaa7e6c"
	pane := snapshot.Pane{
		Index:       0,
		CurrentCmd:  "antex",
		CurrentPath: "/workspace",
		Meta: map[string]string{
			"antex.session_id":        id,
			"antex.session_id_source": "binding-v1",
			"antex.home":              "/tmp/antex",
			"antex.resume_argv":       `["antex","resume","` + id + `"]`,
		},
	}
	raw, err := json.Marshal(map[string]any{"token": "attempt-1", "pane": pane})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	output := "0|layout|1|work|0|1|10|/dev/pts/1|zsh|/home/test|%1|/tmp/socket|" + encoded + "|||"
	windows := parseCapturedPanes(output, newProcessSnapshot([]string{"10 1 S+ zsh"}))
	if len(windows) != 1 || len(windows[0].Panes) != 1 {
		t.Fatalf("missing pane: %v", windows)
	}
	got := windows[0].Panes[0]
	if got.CurrentCmd != "antex" || got.Meta[snapshot.AntexSessionIDMetaKey] != id {
		t.Fatalf("failed startup erased original restore intent: %+v", got)
	}
}

func TestAntexRestoreStagesIntentBeforeLaunch(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	client := NewClientWithRunner("tmux", runner)
	client.SetRestoreResolver(integration.NewRegistry(antex.New(t.TempDir())))
	pane := snapshot.Pane{CurrentCmd: "antex"}
	if err := client.restoreWindowCommands(
		"work",
		snapshot.Window{Panes: []snapshot.Pane{pane}},
		0,
	); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), "@lazy_tmux_restore_pending") {
			return
		}
	}
	t.Fatal("restore did not preserve its intent before reporting an unverified session")
}

func TestRestoreReceiptDistinguishesSuccessfulExitFromFailedStartup(t *testing.T) {
	t.Parallel()
	original := snapshot.Pane{CurrentCmd: "antex", RestoreCmd: "antex resume saved"}
	body, err := json.Marshal(map[string]any{"token": "attempt", "pane": original})
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(body)
	for _, tc := range []struct{ name, command, ack, want string }{
		{"failed startup", "zsh", "", "antex"},
		{"env wrapper still starting", "env", "", "antex"},
		{"shell still spawning", "", "", "antex"},
		{"normal exit after ready", "zsh", "attempt", "zsh"},
		{"old receipt", "zsh", "older-attempt", "antex"},
		{"user replaced program", "nvim", "", "nvim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := preserveRestoreIntent(snapshot.Pane{CurrentCmd: tc.command}, encoded, tc.ack)
			if got.CurrentCmd != tc.want {
				t.Fatalf("got %s, want %s", got.CurrentCmd, tc.want)
			}
		})
	}
}
