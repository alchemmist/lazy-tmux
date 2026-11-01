package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestCaptureRejectsChangedHeaderIdentity(t *testing.T) {
	t.Parallel()
	for _, evidence := range []string{"live", "saved", "agent", "explicit"} {
		t.Run(evidence, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			file := writeSession(t, root, "original", root)
			pane := snapshot.Pane{
				CurrentCmd:  "pi",
				CurrentPath: root,
				ProcessArgv: []string{"pi", "--session", file},
				Meta:        map[string]string{"pi.session_file": file},
			}
			var metadata map[string]string
			switch evidence {
			case "live":
				pane.LiveStatusID = "original"
			case "saved":
				pane.Meta["pi.session_id"] = "original"
			case "agent":
				pane.Agent = &snapshot.AgentSession{
					Version: 1, Kind: snapshot.AgentPi, ID: "original", CWD: root, Home: root,
					Argv: []string{"pi", "--session", "original", file}, Source: source,
				}
			case "explicit":
				metadata = map[string]string{"session_id": "original"}
			}
			if _, ok := New(root, root).Session(pane, metadata); !ok {
				t.Fatal("valid original intent rejected")
			}
			changed := writeSession(t, root, "replacement", root)
			data, err := os.ReadFile(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, data, 0o600); err != nil {
				t.Fatal(err)
			}
			integ := New(root, root)
			if _, ok := integ.Session(pane, metadata); ok {
				t.Fatal("changed identity accepted")
			}
			if evidence != "explicit" {
				if _, err := integ.Capture(pane); err == nil {
					t.Fatal("capture overwrote original intent")
				}
				if _, ok := integ.SessionID(pane); ok {
					t.Fatal("header-only identity accepted changed ID")
				}
			}
		})
	}
}

func TestLegacyIdentityOnlyBindingAndTruncatedHistory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := writeSession(t, root, "legacy", root)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(
		`{"type":"message","content":"` + strings.Repeat(
			"x",
			200000,
		) + `"}` + "\n" + `{"type":"message","content":"unfinished`,
	)
	if closeErr := f.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	pane := snapshot.Pane{
		CurrentCmd:  "pi",
		CurrentPath: root,
		Meta:        map[string]string{"pi.session_file": file},
	}
	integ := New(root, root)
	value, ok := integ.Session(pane, nil)
	if !ok || value.ID != "legacy" || value.Status != "openai/gpt-5" {
		t.Fatalf("legacy/truncated session = %#v %v", value, ok)
	}
	// Identity and restore probes must not parse history. Deliberately invalid
	// history is distinguishable from a partial append and fails a full capture.
	if err := os.WriteFile(
		file,
		[]byte(`{"type":"session","id":"legacy","cwd":"`+root+`"}`+"\nnot-json\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, ok := integ.SessionID(pane); !ok {
		t.Fatal("identity probe decoded history")
	}
	if _, err := integ.RestoreDecision(pane, nil); err != nil {
		t.Fatalf("restore decoded history: %v", err)
	}
	if _, err := integ.Capture(pane); err == nil {
		t.Fatal("full capture accepted corrupt history")
	}
}

func TestOpenDescendantHistoryDoesNotEstablishIdentity(t *testing.T) {
	t.Parallel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := writeSession(t, root, "unrelated", root)
	// The old process-tree fallback would accept this open descriptor even
	// though no Pi client selected this history.
	opened, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	pane := snapshot.Pane{CurrentCmd: "pi", CurrentPath: root, ProcessPID: os.Getpid()}
	if _, ok := New(root, root).Session(pane, nil); ok {
		t.Fatal("open file was mistaken for client identity")
	}
}

func TestExplicitRestoreIntentMustAgree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	first := writeSession(t, root, "first", root)
	second := writeSession(t, root, "second", root)
	pane := snapshot.Pane{
		CurrentCmd:   "pi",
		CurrentPath:  root,
		ProcessArgv:  []string{"pi", "--session", second},
		LiveStatusID: "second",
	}
	integ := New(root, root)
	for _, meta := range []map[string]string{
		{"session_id": "first", "session_file": first},
		{"session_id": "second", "session_file": first},
		{"session_id": "second", "session_file": second, "cwd": filepath.Join(root, "other")},
	} {
		if _, err := integ.RestoreDecision(pane, meta); err == nil {
			t.Fatalf("accepted conflicting intent %#v", meta)
		}
	}
}
