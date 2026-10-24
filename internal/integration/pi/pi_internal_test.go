package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func writeSession(t *testing.T, root, id, cwd string) string {
	t.Helper()
	path := filepath.Join(root, id+".jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"session","id":"`+id+`","cwd":"`+cwd+`"}
{"type":"model_change","provider":"openai","modelId":"gpt-5"}
`), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestSessionMetadataAndExactRestore(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "work tree")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	file := writeSession(t, root, "session-a", cwd)
	i := New(filepath.Join(root, "agent"), root)
	pane := snapshot.Pane{
		CurrentCmd: "pi", CurrentPath: cwd,
		Meta: map[string]string{"pi.session_file": file},
	}
	meta, err := i.Capture(pane)
	if err != nil {
		t.Fatal(err)
	}
	if meta["session_id"] != "session-a" || meta["session_file"] != file ||
		meta["cwd"] != cwd || meta["model"] != "openai/gpt-5" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	command, err := i.RestoreDecision(pane, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command, "'pi' '--session' "+quote(file)) ||
		!strings.Contains(command, "cd "+quote(cwd)) {
		t.Fatal(command)
	}
}

func TestNoSessionAndMissingFile(t *testing.T) {
	t.Parallel()
	i := New(t.TempDir(), t.TempDir())
	pane := snapshot.Pane{CurrentCmd: "pi --no-session", CurrentPath: "/tmp"}
	if _, ok := i.Session(pane, nil); ok {
		t.Fatal("--no-session must not bind")
	}
	pane.CurrentCmd = "pi"
	pane.Meta = map[string]string{"pi.session_file": filepath.Join(t.TempDir(), "missing.jsonl")}
	if _, ok := i.Session(pane, nil); ok {
		t.Fatal("missing session must not bind")
	}
	_, err := i.RestoreDecision(
		pane, map[string]string{"session_file": pane.Meta["pi.session_file"]},
	)
	if err == nil || !strings.Contains(err.Error(), "manual review") {
		t.Fatal("missing file should require manual review")
	}
}

func TestMultipleSessionsRequireProcessBinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	if err := os.Mkdir(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = writeSession(t, root, "a", cwd)
	_ = writeSession(t, root, "b", cwd)
	pane := snapshot.Pane{CurrentCmd: "pi", CurrentPath: cwd}
	if _, ok := New(root, root).Session(pane, nil); ok {
		t.Fatal("ambiguous sessions must not bind")
	}
}

func TestPiMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	snap := snapshot.SessionSnapshot{
		Version: snapshot.FormatVersion,
		Windows: []snapshot.Window{{Panes: []snapshot.Pane{{Meta: map[string]string{
			"pi.session_id": "session-a", "pi.session_file": "/tmp/with space/session-a.jsonl",
			"pi.cwd": "/tmp/with space", "pi.model": "openai/gpt-5",
		}}}}},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var restored snapshot.SessionSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	got := restored.Windows[0].Panes[0].Meta
	if got["pi.session_id"] != "session-a" ||
		got["pi.session_file"] != "/tmp/with space/session-a.jsonl" ||
		got["pi.cwd"] != "/tmp/with space" || got["pi.model"] != "openai/gpt-5" {
		t.Fatalf("metadata lost: %#v", got)
	}
}

func quote(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }
