package tmux_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

//nolint:paralleltest // Uses an isolated tmux server and process-wide test environment.
func TestRealPiLiveWorkingCompletionAndError(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("requires real Pi")
	}
	testutil.IsolatedTmux(t)
	home, cwd := t.TempDir(), t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	started, finish := make(chan struct{}, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(finish) })

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) > 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(
				w,
				`{"error":{"message":"fixture error","type":"invalid_request_error"}}`,
			)

			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(
			w,
			"data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"fixture\","+
				"\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\n",
		)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		started <- struct{}{}
		select {
		case <-finish:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprint(
			w,
			"data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"fixture\","+
				"\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
		)
	}))
	defer func() { release(); server.Close() }()
	// All model traffic stays on this fixture. No credentials or external models.
	models := map[string]any{"providers": map[string]any{"status-test": map[string]any{
		"baseUrl": server.URL + "/v1",
		"api":     "openai-completions",
		"apiKey":  "fixture-only",
		"models": []map[string]any{
			{"id": "fixture", "name": "Fixture", "contextWindow": 32000, "maxTokens": 512},
		},
	}}}
	data, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "models.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sessionDir, "status.jsonl")
	writePiSession(t, file, "status-session", cwd)
	launch := "env PI_OFFLINE=1 PI_CODING_AGENT_DIR=" + home +
		" PI_CODING_AGENT_SESSION_DIR=" + sessionDir +
		" pi --no-tools --provider status-test --model fixture --session " + file
	testutil.Tmux(t, "new-session", "-d", "-s", "pi-status", "-c", cwd, launch)
	client := tmux.NewClient("tmux")
	registry := integration.NewRegistry(
		pi.New(home, sessionDir).WithStatusLoader(client.CapturePiPanes),
	)
	saved := waitForPiSnapshot(t, client, registry, "pi-status", file, "status-session").Windows[0].Panes[0]
	waitForPiState(t, registry, saved, integration.StatusIdle)
	scoped := registry.Scope()
	if got, ok := scoped.StatusFor("pi", saved); !ok || got != integration.StatusIdle {
		t.Fatalf("named Pi status: %v %v", got, ok)
	}
	if got, ok := scoped.StatusFor("claude", saved); ok || got != integration.StatusUnknown {
		t.Fatalf("wrong provider returned Pi status: %v %v", got, ok)
	}
	testutil.Tmux(t, "send-keys", "-t", "pi-status:0.0", "-l", "fixture input")
	testutil.Tmux(t, "send-keys", "-t", "pi-status:0.0", "Enter")
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatalf("fixture model not called: %s", capturePane(t, "pi-status"))
	}
	// The snapshot is unchanged throughout the turn; status must come from live Pi.
	waitForPiState(t, registry, saved, integration.StatusWorking)
	release()
	waitForPiState(t, registry, saved, integration.StatusAwaitingInput)
	testutil.Tmux(t, "send-keys", "-t", "pi-status:0.0", "-l", "fixture failure")
	testutil.Tmux(t, "send-keys", "-t", "pi-status:0.0", "Enter")
	waitForPiState(t, registry, saved, integration.StatusError)
	if err := client.KillSession("pi-status"); err != nil {
		t.Fatal(err)
	}
	if state, ok := registry.Scope().Status(saved); ok || state != integration.StatusUnknown {
		t.Fatal("exited client retained a live status")
	}
}

func waitForPiState(
	t *testing.T,
	registry *integration.Registry,
	pane snapshot.Pane,
	want integration.Status,
) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var got integration.Status
	var ok bool
	for time.Now().Before(deadline) {
		got, ok = registry.Scope().Status(pane)
		if ok && got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Pi state = %v %v, want %v", got, ok, want)
}
