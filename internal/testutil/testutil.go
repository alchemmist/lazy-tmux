// Package testutil holds shared helpers for the integration test suite.
//
// The integration tests in this repository talk to a real tmux server and,
// where relevant, a real fzf binary. They are gated behind the
// ENABLE_INTEGRATION_TESTS environment variable so that the plain `make test`
// run (which has neither tmux nor fzf available) skips them, while the
// docker-based `make integration-test` / `make test-cov` runs execute them
// against the real tools baked into docker/test.Dockerfile.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// SkipIfNotIntegration skips the calling test unless ENABLE_INTEGRATION_TESTS
// is set to a truthy value. This keeps the real-tmux tests out of the plain
// unit test run.
func SkipIfNotIntegration(t *testing.T) {
	t.Helper()

	v, err := strconv.ParseBool(os.Getenv("ENABLE_INTEGRATION_TESTS"))
	if err != nil || !v {
		t.Skip(
			"ENABLE_INTEGRATION_TESTS is not true. Run integration tests with `make integration-test`",
		)
	}
}

// RequireTMux skips the test when tmux is not installed.
func RequireTMux(t *testing.T) {
	t.Helper()

	if !HasTMux() {
		t.Skip("tmux not installed")
	}
}

// RequireFZF skips the test when fzf is not installed.
func RequireFZF(t *testing.T) {
	t.Helper()

	if !HasFZF() {
		t.Skip("fzf not installed")
	}
}

// IsolatedTmux prepares an isolated, private tmux server for the test. It points
// TMUX_TMPDIR at a fresh temp directory (so the server socket can never collide
// with a developer's real tmux), clears TMUX so the client never tries to switch
// an attached client, and registers a cleanup that tears the server down.
//
// All exec.CommandContext(context.Background(), "tmux", ...) calls made by the production code and by the
// Tmux helper below inherit this environment, so they share the same private
// server.
func IsolatedTmux(t *testing.T) {
	t.Helper()

	SkipIfNotIntegration(t)
	RequireTMux(t)

	// tmux uses a unix domain socket at $TMUX_TMPDIR/tmux-<uid>/default, and
	// unix socket paths are limited (~104 chars). t.TempDir() can be very long
	// on macOS, so anchor the socket dir under /tmp to stay within the limit on
	// every platform.
	// t.TempDir() would be ideal, but on macOS it lives under a long $TMPDIR
	// that overflows the ~104 char unix socket path limit; /tmp keeps it short.
	base, err := os.MkdirTemp("/tmp", "lztmux") //nolint:usetesting
	if err != nil {
		t.Fatalf("create tmux tmpdir: %v", err)
	}

	t.Setenv("TMUX_TMPDIR", base)
	t.Setenv("TMUX", "")

	// Make sure no stale server lingers from a previous test in this process.
	_ = exec.CommandContext(context.Background(), "tmux", "kill-server").Run()

	// Start the server from a controlled config so indexing is deterministic
	// regardless of the developer's ~/.tmux.conf (which may set base-index 1).
	// exit-empty off keeps the otherwise session-less server alive so that the
	// production code's later tmux calls attach to *this* configured server
	// instead of spawning a fresh one that would load the user config.
	conf := filepath.Join(base, "tmux.conf")
	confBody := "set -g base-index 0\nset -g pane-base-index 0\nset -g exit-empty off\n"

	err = os.WriteFile(conf, []byte(confBody), 0o644)
	if err != nil {
		t.Fatalf("write tmux config: %v", err)
	}

	startServer := exec.CommandContext(context.Background(), "tmux", "-f", conf, "start-server")

	out, err := startServer.CombinedOutput()
	if err != nil {
		t.Fatalf("start tmux server: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "tmux", "kill-server").Run()
		_ = os.RemoveAll(base)
	})
}

// Tmux runs a raw tmux command against the isolated server and returns its
// combined output, failing the test on error. Use it for test setup and for
// asserting on real server state.
func Tmux(t *testing.T, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(context.Background(), "tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v failed: %v\n%s", args, err, out)
	}

	return string(out)
}

// TmuxTry runs a raw tmux command and returns its combined output and error
// without failing the test. Use it when an error is an expected outcome.
func TmuxTry(args ...string) (string, error) {
	out, err := exec.CommandContext(context.Background(), "tmux", args...).CombinedOutput()
	return string(out), err
}

func probeCommandWithTimeout(name string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}

// HasTMux reports whether tmux is available on PATH.
func HasTMux() bool {
	return probeCommandWithTimeout("tmux", "-V")
}

// HasFZF reports whether fzf is available on PATH.
func HasFZF() bool {
	return probeCommandWithTimeout("fzf", "--version")
}
