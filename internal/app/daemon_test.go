package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

type testDaemonTicker struct {
	ch chan time.Time
}

func (t *testDaemonTicker) Chan() <-chan time.Time { return t.ch }

func (t *testDaemonTicker) Stop() {}

func TestRunDaemonLogsErrors(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	fake := writeFakeTmuxForApp(t, `
if [ "$1" = "display-message" ]; then
  if [ "$2" = "-p" ] && [ "$3" = "#{socket_path}" ]; then
    echo "/tmp/fake.sock"
    exit 0
  fi
fi
exit 0
`)
	a := &App{
		cfg:  config.Config{SaveInterval: time.Second},
		tmux: tmux.NewClient(fake),
	}
	var calls int
	a.saveAllFn = func() error {
		calls++
		if calls == 2 {
			return fmt.Errorf("boom")
		}
		return nil
	}

	origTicker := newDaemonTicker
	defer func() { newDaemonTicker = origTicker }()
	ticker := &testDaemonTicker{ch: make(chan time.Time)}
	newDaemonTicker = func(time.Duration) daemonTicker {
		go func() {
			ticker.ch <- time.Now()
			close(ticker.ch)
		}()
		return ticker
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("open pipe: %v", err)
	}
	origErr := os.Stderr
	os.Stderr = w
	defer func() {
		os.Stderr = origErr
		w.Close()
	}()

	if err := a.RunDaemon(10 * time.Millisecond); err != nil {
		t.Fatalf("RunDaemon error: %v", err)
	}
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	r.Close()
	if !strings.Contains(string(out), "lazy-tmux daemon save error: boom") {
		t.Fatalf("expected logged error, got %q", string(out))
	}
	if calls != 2 {
		t.Fatalf("unexpected saveAll calls: %d", calls)
	}
}
