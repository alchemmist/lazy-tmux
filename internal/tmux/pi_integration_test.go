package tmux_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

func TestRealPiSessionSnapshotRestoreAndIsolation(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("real Pi integration runs in the integration-test container")
	}
	testutil.IsolatedTmux(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(t.TempDir(), "work tree")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	otherCWD := filepath.Join(t.TempDir(), "other tree")
	if err := os.Mkdir(otherCWD, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(sessionDir, "A.jsonl")
	writePiSession(t, sessionFile, "A", cwd)
	writePiSession(t, filepath.Join(sessionDir, "B.jsonl"), "B", otherCWD)
	const name = "real-pi"
	launch := "env PI_OFFLINE=1 PI_CODING_AGENT_DIR=" + home +
		" PI_CODING_AGENT_SESSION_DIR=" + sessionDir + " pi --no-tools --session " + sessionFile
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", cwd, launch)
	client := tmux.NewClient("tmux")
	registry := integration.NewRegistry(pi.New(home, sessionDir))
	client.SetRestoreResolver(registry)
	client.SetRestoreTimeout(0)
	before := waitForPiSnapshot(t, client, registry, name, sessionFile, "A")
	if got := before.Windows[0].Panes[0].Meta["pi.session_id"]; got != "A" {
		t.Fatalf("snapshot session id = %q", got)
	}
	if got := before.Windows[0].Panes[0].Meta["pi.session_file"]; got != sessionFile {
		t.Fatalf("snapshot session file = %q", got)
	}
	if err := client.KillSession(name); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSession(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	after := waitForPiSnapshot(t, client, registry, name, sessionFile, "A")
	pane := after.Windows[0].Panes[0]
	if len(pane.ProcessArgv) == 0 || pane.ProcessArgv[0] != "pi" {
		t.Fatalf("restored command = %q, pane output: %q", pane.CurrentCmd, capturePane(t, name))
	}
}

func TestRealPiRestoreUsesShellAssignmentBeforeCd(t *testing.T) {
	testutil.IsolatedTmux(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(t.TempDir(), "space cwd")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(sessionDir, "A.jsonl")
	writePiSession(t, sessionFile, "A", cwd)
	const name = "pi-assignment"
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", cwd, "sh -c 'sleep 1000'")
	client := tmux.NewClient("tmux")
	client.SetRestoreTimeout(0)
	registry := integration.NewRegistry(pi.New(home, sessionDir))
	client.SetRestoreResolver(registry)
	pane := snapshot.Pane{
		Index:       0,
		CurrentPath: cwd,
		CurrentCmd:  "pi",
		Agent: &snapshot.AgentSession{
			Version: 1, Kind: snapshot.AgentPi, ID: "A", Home: home, CWD: cwd,
			Argv: []string{"pi", "--session", "A", sessionFile}, Source: "pi-jsonl-v1",
		},
		Meta: map[string]string{
			"pi.session_id": "A", "pi.session_file": sessionFile, "pi.cwd": cwd,
		},
	}
	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		Windows: []snapshot.Window{{
			Index: 0, Name: "pi", IsActive: true, Panes: []snapshot.Pane{pane},
		}},
	}
	if err := client.KillSession(name); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	command := capturePane(t, name)
	if strings.Contains(command, "env LAZY_TMUX_RESTORE_TOKEN") {
		t.Fatalf("restore token was passed as env command prefix: %q", command)
	}
	if !strings.Contains(command, "LAZY_TMUX_RESTORE_TOKEN=") || !strings.Contains(command, "cd ") {
		t.Fatalf("restore command missing shell assignment/cd: %q", command)
	}
	if os.Getenv("ENABLE_REAL_PI_TESTS") == "true" {
		waitForPiSnapshot(t, client, registry, name, sessionFile, "A")
	}
	if err := client.KillSession(name); err != nil {
		t.Fatal(err)
	}
}

func TestRealPiNoSessionIsUnbound(t *testing.T) {
	testutil.IsolatedTmux(t)
	dir := t.TempDir()
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	testutil.Tmux(
		t, "new-session", "-d", "-s", "pi-no-session", "-c", dir,
		"env PI_OFFLINE=1 PI_CODING_AGENT_DIR="+home+" pi --no-tools --no-session",
	)
	client := tmux.NewClient("tmux")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		captured, err := client.CaptureSession("pi-no-session")
		if err == nil && len(captured.Windows) > 0 &&
			(captured.Windows[0].Panes[0].CurrentCmd == "pi" || captured.Windows[0].Panes[0].CurrentCmd == "node") {
			if _, ok := pi.New(t.TempDir(), t.TempDir()).SessionID(
				captured.Windows[0].Panes[0],
			); ok {
				t.Fatal("--no-session Pi pane was identified")
			}

			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Pi --no-session pane did not start")
}

func TestRealPiMultiplePanesKeepIndependentSessions(t *testing.T) {
	testutil.IsolatedTmux(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firstCWD := filepath.Join(t.TempDir(), "first tree")
	secondCWD := firstCWD
	if err := os.Mkdir(firstCWD, 0o755); err != nil {
		t.Fatal(err)
	}
	firstFile := filepath.Join(sessionDir, "first.jsonl")
	secondFile := filepath.Join(sessionDir, "second.jsonl")
	writePiSession(t, firstFile, "first", firstCWD)
	writePiSession(t, secondFile, "second", secondCWD)
	startPi(t, "pi-many", firstCWD, firstFile, home, sessionDir)
	testutil.Tmux(
		t, "split-window", "-d", "-t", "pi-many:0", "-c", secondCWD,
		"env PI_OFFLINE=1 PI_CODING_AGENT_DIR="+home+
			" PI_CODING_AGENT_SESSION_DIR="+sessionDir+" pi --no-tools --session "+secondFile,
	)
	client := tmux.NewClient("tmux")
	registry := integration.NewRegistry(pi.New(home, sessionDir))
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		captured, err := client.CaptureSession("pi-many")
		if err == nil {
			registry.Enrich(&captured)
			if len(captured.Windows[0].Panes) == 2 {
				ids := map[string]bool{}
				for _, pane := range captured.Windows[0].Panes {
					ids[pane.Meta["pi.session_id"]] = true
				}
				if ids["first"] && ids["second"] {
					return
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("multiple Pi panes did not keep independent session IDs")
}

func TestRealPiMissingSessionRequestsManualReview(t *testing.T) {
	testutil.IsolatedTmux(t)
	home := t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	cwd := t.TempDir()
	file := filepath.Join(home, "missing.jsonl")
	const name = "pi-missing"
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", cwd, "sh -c 'sleep 1000'")
	client := tmux.NewClient("tmux")
	client.SetRestoreTimeout(0)
	client.SetRestoreResolver(integration.NewRegistry(pi.New(home, filepath.Dir(file))))
	pane := snapshot.Pane{
		Index: 0, CurrentPath: cwd, CurrentCmd: "pi",
		Meta: map[string]string{"pi.session_id": "missing", "pi.session_file": file, "pi.cwd": cwd},
	}
	snap := snapshot.SessionSnapshot{
		Version:     snapshot.FormatVersion,
		SessionName: name,
		Windows: []snapshot.Window{{
			Index: 0, Name: "pi", IsActive: true, Panes: []snapshot.Pane{pane},
		}},
	}
	if err := client.KillSession(name); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSession(context.Background(), snap); err != nil {
		t.Fatal(err)
	}
	if output := strings.ReplaceAll(
		capturePane(t, name),
		"\n",
		"",
	); !strings.Contains(
		output,
		"manual review",
	) {
		t.Fatalf("missing session diagnostic: %q", output)
	}
}

func waitForPiSnapshot(
	t *testing.T, client *tmux.Client, registry *integration.Registry, name, file, id string,
) snapshot.SessionSnapshot {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		captured, err := client.CaptureSession(name)
		if err == nil {
			registry.Enrich(&captured)
			if len(captured.Windows) > 0 && len(captured.Windows[0].Panes) > 0 &&
				captured.Windows[0].Panes[0].Meta["agent.restore_pending"] == "" &&
				captured.Windows[0].Panes[0].Meta["pi.session_id"] == id &&
				captured.Windows[0].Panes[0].Meta["pi.session_file"] == file {
				return captured
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if data, err := os.ReadFile(file); err == nil {
		t.Logf("session file header: %.1000s", data)
	}
	captured, _ := client.CaptureSession(name)
	panes := testutil.Tmux(
		t, "list-panes", "-a", "-F", "#{pane_current_command}|#{pane_current_path}|#{pane_pid}",
	) + capturePane(t, name)
	t.Fatalf("Pi session %s was not identified; panes=%q snapshot=%+v", id, panes, captured)

	return snapshot.SessionSnapshot{}
}

func writePiSession(t *testing.T, path, id, cwd string) {
	t.Helper()
	record := map[string]any{"type": "session", "id": id, "cwd": cwd, "version": 3}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func startPi(t *testing.T, name, cwd, file, home, sessionDir string) {
	t.Helper()
	launch := "env PI_OFFLINE=1 PI_CODING_AGENT_DIR=" + home +
		" PI_CODING_AGENT_SESSION_DIR=" + sessionDir + " pi --no-tools --session " + file
	testutil.Tmux(t, "new-session", "-d", "-s", name, "-c", cwd, launch)
}

func capturePane(t *testing.T, session string) string {
	t.Helper()

	return testutil.Tmux(t, "capture-pane", "-p", "-t", session+":0.0")
}

func TestRealPiSwitchSameDirectoryAndRestore(t *testing.T) {
	if os.Getenv("ENABLE_REAL_PI_TESTS") != "true" {
		t.Skip("real Pi required")
	}
	testutil.IsolatedTmux(t)
	home, cwd := t.TempDir(), t.TempDir()
	if _, err := setup.EnsurePi(home); err != nil {
		t.Fatal(err)
	}
	sessionDir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(
		sessionDir,
		"first.jsonl",
	), filepath.Join(
		sessionDir,
		"second.jsonl",
	)
	writePiSession(t, first, "first", cwd)
	writePiSession(t, second, "second", cwd)
	target, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	extension := `export default function(pi) {
 pi.registerCommand("test-switch", {description:"test only",
 handler:async (_args,ctx)=>{ await ctx.switchSession(` + string(
		target,
	) + `); }}); }`
	if err := os.WriteFile(
		filepath.Join(home, "extensions", "test-switch.ts"),
		[]byte(extension),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	startPi(t, "pi-switch", cwd, first, home, sessionDir)
	client := tmux.NewClient("tmux")
	registry := integration.NewRegistry(pi.New(home, sessionDir))
	client.SetRestoreResolver(registry)
	client.SetRestoreTimeout(0)
	waitForPiSnapshot(t, client, registry, "pi-switch", first, "first")
	testutil.Tmux(t, "send-keys", "-t", "pi-switch:0.0", "-l", "/test-switch")
	testutil.Tmux(t, "send-keys", "-t", "pi-switch:0.0", "Enter")
	saved := waitForPiSnapshot(t, client, registry, "pi-switch", second, "second")
	// A stale pane-local value must not survive PID reuse or claim another owner.
	rawBinding := testutil.Tmux(
		t,
		"show-option",
		"-p",
		"-v",
		"-t",
		"pi-switch:0.0",
		"@lazy_tmux_pi_binding",
	)
	for _, field := range []string{"started_at", "pid", "version"} {
		var binding map[string]any
		if err := json.Unmarshal([]byte(rawBinding), &binding); err != nil {
			t.Fatal(err)
		}
		switch field {
		case "started_at":
			binding[field] = "stale process"
		case "pid":
			binding[field] = os.Getpid()
		case "version":
			binding[field] = 99
		}
		data, err := json.Marshal(binding)
		if err != nil {
			t.Fatal(err)
		}
		testutil.Tmux(
			t,
			"set-option",
			"-p",
			"-t",
			"pi-switch:0.0",
			"@lazy_tmux_pi_binding",
			string(data),
		)
		captured, err := client.CaptureSession("pi-switch")
		if err != nil {
			t.Fatal(err)
		}
		if captured.Windows[0].Panes[0].Meta["pi.session_file"] != "" {
			t.Fatalf("accepted stale %s binding", field)
		}
	}
	testutil.Tmux(
		t,
		"set-option",
		"-p",
		"-t",
		"pi-switch:0.0",
		"@lazy_tmux_pi_binding",
		strings.TrimSpace(rawBinding),
	)

	if err := client.KillSession("pi-switch"); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSession(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	waitForPiSnapshot(t, client, registry, "pi-switch", second, "second")
}
