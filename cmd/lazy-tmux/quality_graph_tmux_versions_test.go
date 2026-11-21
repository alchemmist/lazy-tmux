package main

import (
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
		releases    string
		headResults string
		baseResults string
		wantStatus  string
		wantFailure string

		wantSummary []string
	}{
		{
			name: "known incompatibility", releases: "3.5\n3.6",
			headResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			baseResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			wantStatus:  "passed",
			wantSummary: []string{"| `3.5` | ✓ |", "| `3.6` | ✗ |"},
		},
		{
			name: "lost version", releases: "3.5\n3.6",
			headResults: "  tmux 3.5      ✓\n  tmux 3.6      ✗",
			baseResults: "  tmux 3.5      ✓\n  tmux 3.6      ✓",
			wantStatus:  "failed",
			wantFailure: "quality",
			wantSummary: []string{
				"| `3.5` | ✓ |",
				"| `3.6` | ✗ |",
				"Previously supported versions lost: 3.6",
			},
		},
		{
			name: "truncated head", releases: "3.9\n3.10",
			headResults: "  tmux 3.10     ✓",
			baseResults: "  tmux 3.9      ✓\n  tmux 3.10     ✓",
			wantStatus:  "failed",
			wantFailure: "infrastructure",
			wantSummary: []string{"| `3.10` | ✓ |"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runQualityGraphTmuxVersionCase(
				t,
				tc.releases, tc.headResults,
				tc.baseResults,
				tc.wantStatus,
				tc.wantFailure,
				tc.wantSummary,
			)
		})
	}
}

func runQualityGraphTmuxVersionCase(
	t *testing.T,
	releases, headResults, baseResults, wantStatus, wantFailure string,
	wantSummary []string,
) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	temp := t.TempDir()
	for name, contents := range map[string]string{
		"versions.txt":           releases,
		"tmux-versions.txt":      headResults,
		"tmux-versions-base.txt": baseResults,
	} {
		if err := os.WriteFile(
			filepath.Join(temp, name),
			[]byte(contents+"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	eventPath := filepath.Join(temp, "event.json")
	event := []byte(`{
  "pull_request": {
    "number": 42,
    "base": {"sha": "2222222222222222222222222222222222222222"},
    "head": {"sha": "1111111111111111111111111111111111111111"}
  }
}`)
	if err := os.WriteFile(eventPath, event, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(
		t.Context(),
		filepath.Join(root, "scripts", "report-tmux-versions.sh"), "compare", "0", "0", "0",
	)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GITHUB_EVENT_NAME=pull_request",
		"GITHUB_EVENT_PATH="+eventPath,
		"GITHUB_REPOSITORY=alchemmist/lazy-tmux",
		"GITHUB_SHA=3333333333333333333333333333333333333333",
		"GITHUB_RUN_ID=7",
		"GITHUB_RUN_ATTEMPT=2",
		"QUALITY_GRAPH_REPORT_DIR="+temp,
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

	provenance, _ := report["provenance"].(map[string]any)
	headSHA, _ := provenance["headSha"].(string)
	if headSHA != "1111111111111111111111111111111111111111" {
		t.Fatalf("head SHA = %q", headSHA)
	}
}
