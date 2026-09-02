package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestQualityGraphTmuxVersionsResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		exitCode    string
		wantStatus  string
		wantFailure string
	}{
		{name: "supported", exitCode: "0", wantStatus: "passed"},
		{name: "lost version", exitCode: "1", wantStatus: "failed", wantFailure: "quality"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runQualityGraphTmuxVersionCase(t, tc.exitCode, tc.wantStatus, tc.wantFailure)
		})
	}
}

func runQualityGraphTmuxVersionCase(t *testing.T, exitCode, wantStatus, wantFailure string) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	temp := t.TempDir()
	fakeMake := filepath.Join(temp, "make")
	makeBody := "#!/bin/sh\nprintf '  tmux 3.5      ✓\\n  tmux 3.6      ✗\\n'\nexit \"$FAKE_EXIT_CODE\"\n"
	if err := os.WriteFile(fakeMake, []byte(makeBody), 0o755); err != nil {
		t.Fatal(err)
	}

	eventPath := filepath.Join(temp, "event.json")
	event := []byte(`{"pull_request":{"number":42}}`)
	if err := os.WriteFile(eventPath, event, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(
		context.Background(),
		filepath.Join(root, "scripts", "quality-graph-tmux-versions.sh"),
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+temp+":"+os.Getenv("PATH"),
		"FAKE_EXIT_CODE="+exitCode,
		"GITHUB_EVENT_NAME=pull_request",
		"GITHUB_EVENT_PATH="+eventPath,
		"GITHUB_REPOSITORY=alchemmist/lazy-tmux",
		"GITHUB_SHA=1111111111111111111111111111111111111111",
		"GITHUB_RUN_ID=7",
		"GITHUB_RUN_ATTEMPT=2",
		"QUALITY_GRAPH_REPORT_DIR="+temp,
	)

	runErr := cmd.Run()
	if exitCode == "0" && runErr != nil {
		t.Fatalf("script failed: %v", runErr)
	}
	if exitCode != "0" && runErr == nil {
		t.Fatal("script succeeded after a lost tmux version")
	}

	assertQualityGraphTmuxReport(
		t,
		filepath.Join(temp, "tmux-versions.json"),
		wantStatus,
		wantFailure,
	)
}

func assertQualityGraphTmuxReport(t *testing.T, path, wantStatus, wantFailure string) {
	t.Helper()

	reportBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var report map[string]any
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}

	status, _ := report["status"].(string)
	failure, _ := report["failureKind"].(string)
	if status != wantStatus || failure != wantFailure {
		t.Fatalf("result = %q/%q, want %q/%q", status, failure, wantStatus, wantFailure)
	}

	summary, _ := report["summary"].(string)
	if !strings.Contains(summary, "| `3.5` | ✓ |") ||
		!strings.Contains(summary, "| `3.6` | ✗ |") {
		t.Fatalf("summary does not contain the tmux table:\n%s", summary)
	}
}
