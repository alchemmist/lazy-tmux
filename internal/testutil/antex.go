package testutil

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	antexStartupTimeout = 20 * time.Second
	antexBindingPoll    = 50 * time.Millisecond
)

// StartAntex returns the published thread ID and binding of a real offline client.
func StartAntex(t *testing.T, home, cwd string, tmuxArgs ...string) (string, map[string]any) {
	t.Helper()
	binary, err := exec.LookPath("antex")
	if err != nil {
		if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" &&
			os.Getenv("REQUIRE_TEST_ANTEX") == "true" {
			t.Fatalf("required Antex release missing: %v", err)
		}
		t.Skip("Antex not installed; the release integration image supplies Linux amd64")
	}
	tmuxArgs = append(
		tmuxArgs,
		"-d",
		"-P",
		"-F",
		"#{pane_id}",
		"-c",
		cwd,
		"sh",
		"-c",
		`exec env -i HOME="$HOME" TERM="$TERM" PATH="$PATH" TMUX="$TMUX" `+
			`TMUX_PANE="$TMUX_PANE" ANTEX_HOME="$1" "$2" -c 'check_for_update_on_startup=false' `+
			`-c 'model_provider="local"' -c 'model_providers.local.name="local"' `+
			`-c 'model_providers.local.base_url="http://127.0.0.1:1/v1"' `+
			`-c 'model_providers.local.wire_api="responses"' -c 'model="local"'`,
		"sh",
		home,
		binary,
	)
	pane := strings.TrimSpace(Tmux(t, tmuxArgs...))
	for deadline := time.Now().Add(antexStartupTimeout); ; {
		raw, err := TmuxTry("show-options", "-pqv", "-t", pane, "@antex_binding")
		var binding map[string]any
		if err == nil && json.Unmarshal([]byte(raw), &binding) == nil {
			if id, ok := binding["thread_id"].(string); ok && id != "" {
				return id, binding
			}
		}
		if time.Now().After(deadline) {
			screen, _ := TmuxTry("capture-pane", "-p", "-t", pane)
			t.Fatalf("no Antex binding for %s: %v %s", pane, err, screen)
		}
		time.Sleep(antexBindingPoll)
	}
}
