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
		baseHas36   string
		wantStatus  string
		wantFailure string
	}{
		{name: "known incompatibility", baseHas36: "0", wantStatus: "passed"},
		{name: "lost version", baseHas36: "1", wantStatus: "failed", wantFailure: "quality"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runQualityGraphTmuxVersionCase(t, tc.baseHas36, tc.wantStatus, tc.wantFailure)
		})
	}
}

func runQualityGraphTmuxVersionCase(t *testing.T, baseHas36, wantStatus, wantFailure string) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	temp := t.TempDir()
	fakeMake := filepath.Join(temp, "make")
	makeBody := `#!/bin/sh
status=✗
if [ "${1:-}" = "-C" ] && [ "$FAKE_BASE_HAS_36" = "1" ]; then
    status=✓
fi
printf '  tmux 3.5      ✓\n  tmux 3.6      %s\n' "$status"
exit 1
`
	if err := os.WriteFile(fakeMake, []byte(makeBody), 0o755); err != nil {
		t.Fatal(err)
	}

	eventPath := filepath.Join(temp, "event.json")
	event := []byte(
		`{"pull_request":{"number":42,"base":{"sha":"2222222222222222222222222222222222222222"}}}`,
	)
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
		"FAKE_BASE_HAS_36="+baseHas36,
		"GITHUB_EVENT_NAME=pull_request",
		"GITHUB_EVENT_PATH="+eventPath,
		"GITHUB_REPOSITORY=alchemmist/lazy-tmux",
		"GITHUB_SHA=1111111111111111111111111111111111111111",
		"GITHUB_RUN_ID=7",
		"GITHUB_RUN_ATTEMPT=2",
		"QUALITY_GRAPH_REPORT_DIR="+temp,
		"QUALITY_GRAPH_BASE_DIR="+temp,
	)

	runErr := cmd.Run()
	if wantStatus == "passed" && runErr != nil {
		t.Fatalf("script failed: %v", runErr)
	}
	if wantStatus == "failed" && runErr == nil {
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
