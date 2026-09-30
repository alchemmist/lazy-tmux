package claude

import (
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestLegacyStatusFileCannotIdentifyAPane(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteStatus(dir, "/workspace", StateWorking, "other", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok := New(
		t.TempDir(),
		dir,
	).Status(snapshot.Pane{CurrentCmd: "claude", CurrentPath: "/workspace"}); ok {
		t.Fatal("cwd-scoped hook leaked status")
	}
}

func TestValidState(t *testing.T) {
	t.Parallel()
	for _, state := range []string{StateWorking, StateIdle, StateAwaitingInput, StateAwaitingDecision} {
		if !ValidState(state) {
			t.Fatal(state)
		}
	}
	if ValidState("invalid") {
		t.Fatal("invalid state accepted")
	}
}
