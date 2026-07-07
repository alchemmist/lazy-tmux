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

func SkipIfNotIntegration(t *testing.T) {
	t.Helper()

	v, err := strconv.ParseBool(os.Getenv("ENABLE_INTEGRATION_TESTS"))
	if err != nil || !v {
		t.Skip(
			"ENABLE_INTEGRATION_TESTS is not true. Run integration tests with `make integration-test`",
		)
	}
}

func RequireTMux(t *testing.T) {
	t.Helper()

	if !HasTMux() {
		t.Skip("tmux not installed")
	}
}

func RequireFZF(t *testing.T) {
	t.Helper()

	if !HasFZF() {
		t.Skip("fzf not installed")
	}
}

func IsolatedTmux(t *testing.T) {
	t.Helper()

	SkipIfNotIntegration(t)
	RequireTMux(t)

	base, err := os.MkdirTemp("/tmp", "lztmux") //nolint:usetesting
	if err != nil {
		t.Fatalf("create tmux tmpdir: %v", err)
	}

	t.Setenv("TMUX_TMPDIR", base)
	t.Setenv("TMUX", "")

	_ = exec.CommandContext(context.Background(), "tmux", "kill-server").Run()

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

func Tmux(t *testing.T, args ...string) string {
	t.Helper()

	out, err := exec.CommandContext(context.Background(), "tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v failed: %v\n%s", args, err, out)
	}

	return string(out)
}

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

func HasTMux() bool {
	return probeCommandWithTimeout("tmux", "-V")
}

func HasFZF() bool {
	return probeCommandWithTimeout("fzf", "--version")
}
