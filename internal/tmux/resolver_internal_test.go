package tmux

import (
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/antex"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestEffectiveRestoreCommandFallsBack(t *testing.T) {
	t.Parallel()
	client := NewClient("tmux")
	pane := snapshot.Pane{CurrentCmd: "vim", RestoreCmd: "vim main.go"}
	for _, reg := range []*integration.Registry{nil, integration.NewRegistry(antex.New(t.TempDir()))} {
		client.SetRestoreResolver(reg)
		if got := client.effectiveRestoreCommand(pane); got != "vim main.go" {
			t.Fatalf("fallback: %q", got)
		}
	}
}

func TestRestoreResolverRespectsAllowlist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	testutil.IsolatedTmux(t)
	testutil.Tmux(t, "new-session", "-d", "-s", "work")
	client := NewClient("tmux")
	client.SetRestoreResolver(integration.NewRegistry(antex.New(t.TempDir())))
	const id = "01a09f96-a7d1-75f1-b8d8-6df25aaa7e6c"
	pane := snapshot.Pane{
		CurrentCmd: "antex",
		RestoreCmd: "cat",
		Meta: map[string]string{
			"antex.session_id":        id,
			"antex.session_id_source": "binding-v1",
			"antex.home":              t.TempDir(),
			"antex.resume_argv":       `["antex","resume","` + id + `"]`,
		},
	}
	command, err := client.checkedRestoreCommand(pane)
	if err != nil || !strings.Contains(command, "'antex' 'resume' '"+id+"'") {
		t.Fatalf("resolved: %q %v", command, err)
	}
	client.SetRestoreAllowlist([]string{"^cat$"})
	if err := client.restoreWindowCommands(
		"work",
		snapshot.Window{Panes: []snapshot.Pane{pane}},
		0,
	); err != nil {
		t.Fatal(err)
	}
	if raw := testutil.Tmux(
		t,
		"display-message",
		"-p",
		"-t",
		"=work:0.0",
		"#{"+pendingRestoreOption+"}",
	); strings.TrimSpace(
		raw,
	) != "" {
		t.Fatal("disallowed resume was staged")
	}
}
