package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/testutil"
)

func TestPiAutomaticCleanInstallAndOptOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "custom-pi"))
	t.Setenv("LAZY_TMUX_CONFIG", filepath.Join(home, "config.toml"))
	var out bytes.Buffer
	var stdout bytes.Buffer
	if code := runCLI([]string{cmdSetup}, &stdout, &out); code != 0 {
		t.Fatalf("clean CLI setup failed: %d %s", code, &out)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if installed, err := setup.Check("pi", agentHome(cfg, "pi"), ""); err != nil || !installed {
		t.Fatalf("not installed: %v %v", installed, err)
	}
	if out.Len() == 0 {
		t.Fatal("missing pre-existing-client notice")
	}
	out.Reset()
	if err := preparePi(cmdDaemon, nil, &out); err != nil || out.Len() != 0 {
		t.Fatalf("not idempotent: %v %s", err, &out)
	}
	if code, _, errOut := run(t, "integrations", "setup", "pi", "--uninstall"); code != 0 {
		t.Fatal(errOut)
	}
	if err := preparePi(cmdBootstrap, nil, &out); err != nil {
		t.Fatal(err)
	}
	if installed, _ := setup.Check("pi", agentHome(cfg, "pi"), ""); installed {
		t.Fatal("automatic setup ignored uninstall")
	}
}

func TestPiReadOnlyAndDisabledDoNotInstall(t *testing.T) {
	home := t.TempDir()
	piHome := filepath.Join(home, "pi")
	t.Setenv("HOME", home)
	t.Setenv("PI_CODING_AGENT_DIR", piHome)
	conf := filepath.Join(home, "config.toml")
	t.Setenv("LAZY_TMUX_CONFIG", conf)
	var out bytes.Buffer
	for _, command := range []string{cmdVersion, "integrations", cmdHook, cmdList} {
		if err := preparePi(command, nil, &out); err != nil {
			t.Fatal(err)
		}
	}
	if err := preparePi(cmdSave, []string{"--help"}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(piHome); !os.IsNotExist(err) {
		t.Fatal("read-only command wrote Pi home")
	}
	if err := os.WriteFile(conf, []byte("[integrations.pi]\nenabled=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := preparePi(cmdBootstrap, nil, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(piHome); !os.IsNotExist(err) {
		t.Fatal("disabled integration wrote Pi home")
	}
}

func TestSetupBinaryStopsWhenPiHomeIsNotADirectory(t *testing.T) {
	testutil.SkipIfNotIntegration(t)
	binary := filepath.Join(t.TempDir(), "lazy-tmux")
	build := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, out)
	}
	t.Setenv("HOME", t.TempDir())
	isolateCLIPaths(t)
	piHome := os.Getenv("PI_CODING_AGENT_DIR")
	if err := os.WriteFile(piHome, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "setup")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || len(out) != 0 ||
		!strings.Contains(stderr.String(), "install Pi integration") {
		t.Fatalf("blocked setup: %v, stdout=%q, stderr=%q", err, out, stderr.String())
	}
	if contents, err := os.ReadFile(piHome); err != nil || string(contents) != "occupied" {
		t.Fatalf("blocking file changed: %q, %v", contents, err)
	}
	if err := os.Remove(piHome); err != nil {
		t.Fatal(err)
	}
	cmd = exec.CommandContext(t.Context(), binary, "setup")
	stderr.Reset()
	cmd.Stderr = &stderr
	out, err = cmd.Output()
	if err != nil || !strings.Contains(string(out), "lazy-tmux daemon") ||
		!strings.Contains(stderr.String(), "installed Pi identity extension") {
		t.Fatalf("clean setup: %v, stdout=%q, stderr=%q", err, out, stderr.String())
	}
	if installed, err := setup.Check("pi", piHome, ""); err != nil || !installed {
		t.Fatalf("installed extension: %v, %v", installed, err)
	}
}
