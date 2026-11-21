package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/integration/claude"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func paneSnap(panes ...snapshot.Pane) *snapshot.SessionSnapshot {
	return &snapshot.SessionSnapshot{Windows: []snapshot.Window{{Panes: panes}}}
}

func TestRegistryRefreshesPiMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "history.jsonl")
	reg := integration.NewRegistry(pi.New(dir, dir))
	state := paneSnap(
		snapshot.Pane{
			CurrentCmd:  "pi",
			CurrentPath: dir,
			ProcessArgv: []string{"pi", "--session", file},
		},
		snapshot.Pane{CurrentCmd: "zsh"},
	)
	pane := &state.Windows[0].Panes[0]
	reg.Enrich(state)
	if pane.Meta != nil {
		t.Fatalf("missing session yielded metadata: %v", pane.Meta)
	}
	for _, model := range []string{"first", "second"} {
		body := fmt.Sprintf(`{"type":"session","id":"saved","cwd":%q}
{"type":"model_change","provider":"test","modelId":%q}
`, dir, model)
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		reg.Enrich(state)
		if pane.Meta["pi.session_id"] != "saved" || pane.Meta["pi.model"] != "test/"+model ||
			pane.Meta["pi.session_file"] != file {
			t.Fatalf("unexpected metadata: %v", pane.Meta)
		}
	}
	shell := state.Windows[0].Panes[1]
	if shell.Meta != nil || reg.Resolve(shell) != "" {
		t.Fatalf("unexpected shell enrichment: %+v", shell)
	}
}

func TestRegistryResolveUsesOnlyMatchingProviderMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := integration.NewRegistry(antex.New(dir))
	const id = "019fc30a-6732-7c63-9732-c76949907c98"
	pane := snapshot.Pane{
		CurrentCmd: "antex",
		Meta: map[string]string{
			"antex.session_id":        id,
			"antex.session_id_source": "binding-v1",
			"antex.home":              dir,
			"antex.resume_argv":       `["antex","resume","` + id + `"]`,
			"other.session_id":        "irrelevant",
		},
	}
	if command, err := reg.ResolveChecked(
		pane,
	); err != nil ||
		!strings.Contains(command, "'antex' 'resume' '"+id+"'") {
		t.Fatalf("unexpected resume: %q %v", command, err)
	}
	delete(pane.Meta, "antex.session_id")
	pane.Meta["other.session_id"] = id
	if command, err := reg.ResolveChecked(pane); err == nil || command != "" {
		t.Fatalf("foreign metadata accepted: %q %v", command, err)
	}
}

func TestEnrichForSaveReportsEveryFailedPane(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg := integration.NewRegistry(pi.New(dir, dir), claude.New(dir, dir))
	state := paneSnap(
		snapshot.Pane{Index: 1, CurrentCmd: "pi"},
		snapshot.Pane{Index: 2, CurrentCmd: "claude"},
	)
	state.SessionName = "kit"
	state.Windows[0].Name = "work"
	err := reg.EnrichForSave(state)
	if err == nil {
		t.Fatal("unverified agents accepted")
	}
	for _, want := range []string{
		`session "kit" window 0 ("work") pane 1: capture pi`,
		`session "kit" window 0 ("work") pane 2: resolve restore`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q: %v", want, err)
		}
	}
	reg.Enrich(state)
	for _, pane := range state.Windows[0].Panes {
		if pane.Meta != nil {
			t.Fatalf("unverified metadata: %v", pane.Meta)
		}
	}
}

func TestRegistryNilSafe(t *testing.T) {
	t.Parallel()

	var reg *integration.Registry

	reg.Enrich(paneSnap(snapshot.Pane{CurrentCmd: "x"}))

	if got := reg.Resolve(snapshot.Pane{CurrentCmd: "x"}); got != "" {
		t.Fatalf("nil registry resolve should be empty, got %q", got)
	}
}

func TestDisabledAgentCannotFallBackToStartingNewConversation(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"codex", "antex", "claude"} {
		command, err := integration.NewRegistry().ResolveChecked(snapshot.Pane{CurrentCmd: kind})
		if command != "" || err == nil {
			t.Fatalf("disabled %s allowed fallback: %q %v", kind, command, err)
		}
	}
}

func TestEnrichForSaveAllowsDisabledIntegrationsAndShells(t *testing.T) {
	t.Parallel()
	state := paneSnap(snapshot.Pane{CurrentCmd: "pi"}, snapshot.Pane{CurrentCmd: "zsh"})
	if err := integration.NewRegistry().EnrichForSave(state); err != nil {
		t.Fatal(err)
	}
}
