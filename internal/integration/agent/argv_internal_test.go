package agent

import (
	"reflect"
	"testing"
)

func TestResumeArgumentsPreserveOptionsWithoutReplayingPrompt(t *testing.T) {
	t.Parallel()
	got, err := ResumeArgv(
		"codex",
		"new-id",
		"/workspace",
		[]string{
			"/usr/bin/codex",
			"resume",
			"old-id",
			"--profile",
			"work",
			"--model",
			"model name",
			"initial prompt",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/usr/bin/codex",
		"resume",
		"new-id",
		"--profile",
		"work",
		"--model",
		"model name",
		"--cd",
		"/workspace",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	got, err = ResumeArgv(
		"claude",
		"new-id",
		"/workspace",
		[]string{"claude", "--resume", "old-id", "--fork-session", "--model", "sonnet"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"claude", "--resume", "new-id", "--model", "sonnet"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if _, err := ResumeArgv(
		"codex",
		"id",
		"/workspace",
		[]string{"codex", "--unknown"},
	); err == nil {
		t.Fatal("unknown option silently dropped")
	}
}
