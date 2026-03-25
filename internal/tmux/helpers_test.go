package tmux

import "testing"

func TestSessionTargets(t *testing.T) {
	if got := sessionTarget("demo"); got != "=demo" {
		t.Fatalf("unexpected sessionTarget: %q", got)
	}
	if got := sessionTarget("=demo"); got != "=demo" {
		t.Fatalf("expected existing prefix to be preserved, got %q", got)
	}
	if got := sessionWindowTarget("demo", 3); got != "=demo:3" {
		t.Fatalf("unexpected sessionWindowTarget: %q", got)
	}
	if got := sessionPaneTarget("demo", 3, 2); got != "=demo:3.2" {
		t.Fatalf("unexpected sessionPaneTarget: %q", got)
	}
}

func TestStripOptionPair(t *testing.T) {
	args := []string{"new-window", "-d", "-c", "/tmp", "-t", "=demo:1", "-n", "name"}
	got := stripOptionPair(args, "-c")
	want := []string{"new-window", "-d", "-t", "=demo:1", "-n", "name"}
	if len(got) != len(want) {
		t.Fatalf("unexpected len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected arg at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestParsePSLine(t *testing.T) {
	if _, _, _, ok := parsePSLine("bad"); ok {
		t.Fatal("expected invalid ps line to fail")
	}
	pid, stat, cmd, ok := parsePSLine("2002 S docker compose up")
	if !ok {
		t.Fatal("expected parsePSLine to succeed")
	}
	if pid != 2002 || stat != "S" || cmd != "docker compose up" {
		t.Fatalf("unexpected parsed values: %d %q %q", pid, stat, cmd)
	}
}

