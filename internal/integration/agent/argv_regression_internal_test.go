package agent

import (
	"errors"
	"reflect"
	"testing"
)

func TestResumeArgvClaudeVariadicOptions(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{
		"--allowedTools", "--allowed-tools", "--disallowedTools", "--disallowed-tools", "--add-dir",
	} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			for _, operands := range [][]string{
				{"Bash"}, {"Bash", "Write"}, {"Bash(git *)", "Write", "path with spaces"},
			} {
				launch := append([]string{"/opt/bin/claude", flag}, operands...)
				launch = append(launch, "--model", "model", "old prompt")
				want := append([]string{"/opt/bin/claude", "--resume", "new-id", flag}, operands...)
				want = append(want, "--model", "model")
				assertResumeArgv(t, "claude", launch, want)
				assertResumeArgv(t, "claude", launch[:len(launch)-3], want[:len(want)-2])
			}
			// Commander does not activate variadic consumption for --flag=value.
			// The next operand is a prompt, not a second tool/directory restriction.
			assertResumeArgv(t, "claude",
				[]string{"claude", flag + "=Bash,Write", "prompt", "--verbose"},
				[]string{"claude", "--resume", "new-id", flag + "=Bash,Write", "--verbose"})
			assertResumeArgv(t, "claude",
				[]string{"claude", flag, "Bash", "Write", "--", "prompt", "--unknown"},
				[]string{"claude", "--resume", "new-id", flag, "Bash", "Write"})
			assertResumeArgv(t, "claude",
				[]string{"claude", flag, "Bash", "--model=m", flag, "Write"},
				[]string{"claude", "--resume", "new-id", flag, "Bash", "--model=m", flag, "Write"})
		})
	}
}

func TestResumeArgvPreservesSupportedOptions(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"claude", "codex", "antex"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			valueFlags := []string{"--model", "-m"}
			var switches []string
			if kind == "claude" {
				valueFlags = append(
					valueFlags,
					"--permission-mode",
					"--settings",
					"--setting-sources",
					"--effort",
				)
				switches = []string{"--dangerously-skip-permissions", "--verbose"}
			} else {
				valueFlags = append(
					valueFlags,
					"--profile",
					"-p",
					"--sandbox",
					"-s",
					"--ask-for-approval",
					"-a",
					"--config",
					"-c",
					"--add-dir",
					"--local-provider",
					"--enable",
					"--disable",
				)
				switches = []string{
					"--oss", "--no-alt-screen", "--search", "--full-auto",
					"--dangerously-bypass-approvals-and-sandbox", "--yolo",
				}
			}
			for _, flag := range valueFlags {
				for _, invalid := range [][]string{{kind, flag}, {kind, flag, "--model", "m"}, {kind, flag, ""}} {
					assertUnverifiedResumeArgv(t, kind, invalid)
				}
				assertResumeArgv(t, kind, []string{kind, flag, "value with spaces", "prompt"},
					resumeArgvForTest(kind, flag, "value with spaces"))
				if len(flag) > 2 {
					assertUnverifiedResumeArgv(t, kind, []string{kind, flag + "="})
					assertResumeArgv(t, kind, []string{kind, flag + "=-literal", "prompt"},
						resumeArgvForTest(kind, flag+"=-literal"))
				}
			}
			for _, flag := range switches {
				assertUnverifiedResumeArgv(t, kind, []string{kind, flag + "=false"})
				assertResumeArgv(
					t,
					kind,
					[]string{kind, flag, "prompt"},
					resumeArgvForTest(kind, flag),
				)
			}
		})
	}
}

func TestResumeArgvTransientOptionsAndTermination(t *testing.T) {
	t.Parallel()
	claudeWant := []string{"claude", "--resume", "new-id", "--disallowedTools", "Bash", "Write"}
	for _, transient := range [][]string{
		{"--resume", "old"},
		{"-r", "old"},
		{"--resume=old"},
		{"--resume"},
		{"-r"},
		{"--continue"},
		{"-c"},
		{"--fork-session"},
		{"--worktree", "branch"},
		{"--worktree=branch"},
		{"--worktree"},
		{"-w", "branch"},
		{"-w"},
	} {
		launch := append([]string{"claude"}, transient...)
		launch = append(launch, "--disallowedTools", "Bash", "Write")
		assertResumeArgv(t, "claude", launch, claudeWant)
	}
	for _, kind := range []string{"codex", "antex"} {
		for _, transient := range [][]string{
			{"--cd", "/old"},
			{"-C", "/old"},
			{"--cd=/old"},
			{"--image", "old.png"},
			{"-i", "old.png"},
			{"--image=one.png,two.png"},
			{"--last"},
			{"--all"},
		} {
			launch := append([]string{kind, "resume", "old-id"}, transient...)
			launch = append(launch, "--sandbox", "read-only", "old prompt")
			assertResumeArgv(t, kind, launch,
				[]string{kind, "resume", "new-id", "--sandbox", "read-only", "--cd", "/workspace"})
		}
		// Codex add-dir is unary/repeatable, not Claude's variadic option.
		assertResumeArgv(
			t,
			kind,
			[]string{kind, "--add-dir", "/one", "prompt", "--add-dir=/two"},
			[]string{
				kind,
				"resume",
				"new-id",
				"--add-dir",
				"/one",
				"--add-dir=/two",
				"--cd",
				"/workspace",
			},
		)
		assertResumeArgv(
			t,
			kind,
			[]string{kind, "--", "--unknown", "--sandbox", "danger-full-access"},
			[]string{kind, "resume", "new-id", "--cd", "/workspace"},
		)
	}
	assertResumeArgv(
		t,
		"claude",
		[]string{"claude", "--", "--unknown", "--disallowedTools", "Bash"},
		[]string{"claude", "--resume", "new-id"},
	)
	assertResumeArgv(t, "claude", []string{"./tools/claude", "prompt"},
		[]string{"./tools/claude", "--resume", "new-id"})
}

func TestResumeArgvRejectsUnsupportedAndAmbiguousForms(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"claude", "codex", "antex"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			launches := [][]string{
				nil,
				{},
				{""},
				{"other"},
				{"node", "/cli/" + kind},
				{"sh", "-c", kind},
				{kind, "--unknown"},
				{kind, "--model"},
				{kind, "--model", "--unknown"},
				{kind, "--model", "--"},
				{kind, "--model", ""},
				{kind, "--model="},
				{kind, "-mvalue"},
				{kind, "-m=value"},
				{kind, "-xyz"},
				{kind, "-"},
			}
			if kind == "claude" {
				for _, flag := range []string{"--disallowedTools", "--allowedTools", "--add-dir"} {
					launches = append(
						launches,
						[]string{kind, flag},
						[]string{kind, flag, "--model", "m"},
						[]string{kind, flag + "="},
						[]string{kind, flag, "--"},
					)
				}
				launches = append(
					launches,
					[]string{kind, "--verbose=false"},
					[]string{kind, "--continue=false"},
					[]string{
						kind,
						"--image",
						"file",
					},
					[]string{kind, "--last"},
					[]string{kind, "--cd", "/old"},
				)
			} else {
				launches = append(
					launches,
					[]string{kind, "--oss=false"},
					[]string{kind, "--all=false"},
					[]string{kind, "--image"},
					[]string{kind, "--image", "--sandbox", "read-only"},
					[]string{
						kind,
						"--cd",
					},
					[]string{kind, "--resume", "old"},
					[]string{kind, "--worktree"},
					[]string{
						kind,
						"--continue",
					},
					[]string{kind, "--disallowedTools", "Bash", "Write"},
				)
			}
			for _, launch := range launches {
				got, err := ResumeArgv(kind, "new-id", "/workspace", launch)
				if !errors.Is(err, ErrUnverified) || got != nil {
					t.Errorf(
						"launch %q: got %q, error %v; want nil and ErrUnverified",
						launch,
						got,
						err,
					)
				}
			}
		})
	}
	for _, kind := range []string{"", "unknown", "pi"} {
		if got, err := ResumeArgv(
			kind,
			"id",
			"/workspace",
			[]string{kind},
		); got != nil ||
			!errors.Is(err, ErrUnverified) {
			t.Errorf("unsupported kind %q: got %v, error %v", kind, got, err)
		}
	}
}

func assertUnverifiedResumeArgv(t *testing.T, kind string, launch []string) {
	t.Helper()
	got, err := ResumeArgv(kind, "new-id", "/workspace", launch)
	if !errors.Is(err, ErrUnverified) || got != nil {
		t.Errorf("launch %q: got %q, error %v; want nil and ErrUnverified", launch, got, err)
	}
}

func resumeArgvForTest(kind string, args ...string) []string {
	result := make([]string, 0, len(args)+5)
	result = append(result, kind, "resume", "new-id")
	if kind == "claude" {
		result[1] = "--resume"
	}
	result = append(result, args...)
	if kind != "claude" {
		result = append(result, "--cd", "/workspace")
	}

	return result
}

func assertResumeArgv(t *testing.T, kind string, launch, want []string) {
	t.Helper()
	before := append([]string{}, launch...)
	got, err := ResumeArgv(kind, "new-id", "/workspace", launch)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("launch %q: got %q, error %v; want %q", launch, got, err, want)
	}
	if !reflect.DeepEqual(launch, before) {
		t.Fatalf("launch mutated: got %q, want %q", launch, before)
	}
}

func TestResumeRecognizedNodeClaudeLaunch(t *testing.T) {
	t.Parallel()
	argv, err := ResumeArgv(
		"claude",
		"session",
		"/workspace",
		[]string{
			"/usr/bin/node",
			"/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js",
			"--disallowedTools",
			"Bash",
			"Write",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "--resume", "session", "--disallowedTools", "Bash", "Write"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("got %q want %q", argv, want)
	}
}

// TestResumeArgvCodexPreservesNoDaemon covers execution mode across launch layouts.
func TestResumeArgvCodexPreservesNoDaemon(t *testing.T) {
	t.Parallel()
	for _, launch := range [][]string{
		{"codex", "--no-daemon", "--no-alt-screen"},
		{"codex", "--no-daemon", "resume", "old-id"},
		{"codex", "resume", "old-id", "--no-daemon"},
		{"codex", "--no-daemon", "resume", "old-id", "--no-daemon"},
		{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js", "--no-daemon"},
	} {
		want := []string{"codex", "resume", "new-id", "--no-daemon"}
		if len(launch) == 3 && launch[2] == "--no-alt-screen" {
			want = append(want, "--no-alt-screen")
		}
		want = append(want, "--cd", "/workspace")
		assertResumeArgv(t, "codex", launch, want)
	}
	assertResumeArgv(t, "codex",
		[]string{"codex", "--no-daemon", "--model", "m", "resume", "old-id", "--no-daemon", "--sandbox", "read-only"},
		[]string{"codex", "resume", "new-id", "--no-daemon", "--model", "m", "--sandbox", "read-only", "--cd", "/workspace"})
	assertUnverifiedResumeArgv(t, "codex", []string{"codex", "--no-daemon", "resume", "old-id", "--no-daemon=false"})
	assertUnverifiedResumeArgv(t, "codex", []string{"codex", "--no-daemon=false"})
	assertUnverifiedResumeArgv(t, "claude", []string{"claude", "--no-daemon"})
}
