package tmux_test

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestCaptureDiscoveryChild(t *testing.T) {
	t.Parallel()
	if args := flag.Args(); len(args) != 2 || args[0] != "capture-discovery" {
		t.Skip("subprocess entry point")
	}
	snap, err := tmux.NewClient("tmux").CaptureSession("discovery")
	if err != nil {
		t.Fatal(err)
	}
	if strconv.Itoa(len(snap.Windows)) != flag.Arg(1) {
		t.Fatalf("capture returned %d windows, want %s", len(snap.Windows), flag.Arg(1))
	}
}

func TestCaptureSessionUsesConstantDiscoveryRoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{1, 8} {
		testutil.Tmux(t, "new-session", "-d", "-s", "discovery")
		for n := 1; n < size; n++ {
			testutil.Tmux(t, "new-window", "-d", "-t", "=discovery:")
		}
		trace := filepath.Join(t.TempDir(), "exec.log")
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		cmd := exec.CommandContext(
			ctx,
			"strace",
			"-f",
			"-qq",
			"-e",
			"trace=execve",
			"-o",
			trace,
			exe,
			"-test.run=^TestCaptureDiscoveryChild$",
			"--",
			"capture-discovery",
			strconv.Itoa(size),
		)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("trace %d-window capture: %v: %s", size, err, out)
		}
		data, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		for _, prefix := range []string{`["tmux", "list-panes"`, `["ps", "-ax"`} {
			if got := strings.Count(string(data), prefix); got != 1 {
				t.Fatalf("%d windows: %s execs=%d; trace:\n%s", size, prefix, got, data)
			}
		}
		testutil.Tmux(t, "kill-session", "-t", "=discovery")
	}
}
