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
		headResults string
		baseResults string
		wantStatus  string
		wantFailure string
		wantMissing string
		wantSummary []string
	}{
		{
			name:        "known incompatibility",
			headResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			baseResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			wantStatus:  "passed",
			wantSummary: []string{"| `3.5` | ✓ |", "| `3.6` | ✗ |"},
		},
		{
			name:        "lost version",
			headResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			baseResults: "  tmux 3.5      ✓\n  tmux 3.6      ✓",
			wantStatus:  "failed",
			wantFailure: "quality",
			wantMissing: "3.6\n",
			wantSummary: []string{"| `3.5` | ✓ |", "| `3.6` | ✗ |"},
		},
		{
			name:        "lexical version ordering",
			headResults: "  tmux 3.10     ✓",
			baseResults: "  tmux 3.9      ✓\n  tmux 3.10     ✓",
			wantStatus:  "failed",
			wantFailure: "quality",
			wantMissing: "3.9\n",
			wantSummary: []string{"| `3.10` | ✓ |"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runQualityGraphTmuxVersionCase(
				t,
				tc.headResults,
				tc.baseResults,
				tc.wantStatus,
				tc.wantFailure,
				tc.wantMissing,
				tc.wantSummary,
			)
		})
	}
}

func runQualityGraphTmuxVersionCase(
	t *testing.T,
	headResults, baseResults, wantStatus, wantFailure, wantMissing string,
	wantSummary []string,
) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	temp := t.TempDir()
	fakeMake := filepath.Join(temp, "make")
	makeBody := `#!/bin/sh
results=$FAKE_HEAD_RESULTS
if [ "${1:-}" = "-C" ]; then
    results=$FAKE_BASE_RESULTS
fi
printf '%s\n' "$results"
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
		"FAKE_HEAD_RESULTS="+headResults,
		"FAKE_BASE_RESULTS="+baseResults,
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
		wantSummary,
	)

	missing, err := os.ReadFile(filepath.Join(temp, "missing.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(missing) != wantMissing {
		t.Fatalf("missing versions = %q, want %q", missing, wantMissing)
	}
}

func assertQualityGraphTmuxReport(
	t *testing.T,
	path, wantStatus, wantFailure string,
	wantSummary []string,
) {
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
	for _, want := range wantSummary {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary does not contain %q:\n%s", want, summary)
		}
	}
}
