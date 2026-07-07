package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigWritesLoadableTemplate(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sub", "lazy-tmux.toml")

	written, err := GenerateConfig(path, false)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if written != path {
		t.Fatalf("path: got %q want %q", written, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(data) != DefaultConfigTemplate {
		t.Fatal("generated content != embedded template")
	}

	if _, err := LoadFrom(path); err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
}

func TestGenerateConfigRefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "lazy-tmux.toml")

	if _, err := GenerateConfig(path, false); err != nil {
		t.Fatalf("first generate: %v", err)
	}

	if _, err := GenerateConfig(path, false); err == nil {
		t.Fatal("second generate without --force should fail")
	}

	if _, err := GenerateConfig(path, true); err != nil {
		t.Fatalf("generate with --force: %v", err)
	}
}

func TestRenderEffectiveConfig(t *testing.T) {
	t.Parallel()

	cfg := Default()
	out := cfg.Render()

	for _, want := range []string{
		`tmux_bin        = "tmux"`,
		"save_interval",
		"[scrollback]",
		"restore_allowlist not set",
		"restore_denylist not set",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q in:\n%s", want, out)
		}
	}

	cfg.RestoreAllowlist = []string{"nvim", "vim"}
	if got := cfg.Render(); !strings.Contains(got, `restore_allowlist = ["nvim", "vim"]`) {
		t.Fatalf("render allowlist wrong:\n%s", got)
	}

	cfg.RestoreDenylist = []string{"npm", "node"}
	if got := cfg.Render(); !strings.Contains(got, `restore_denylist = ["npm", "node"]`) {
		t.Fatalf("render denylist wrong:\n%s", got)
	}
}
