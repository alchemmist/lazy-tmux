package testutil

import (
	"context"
	"os"
	"os/exec"
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
