package config

import (
	"os"
	"path/filepath"
	"strconv"
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

	if !strings.Contains(string(data), `restore_handler = ""`) {
		t.Fatal("generated config does not document restore_handler")
	}

	if !strings.Contains(string(data), `restore_handler_source = "saved"`) {
		t.Fatal("generated config does not document restore_handler_source")
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}

	if cfg.RestoreHandler != "" {
		t.Fatalf("generated config restore_handler: got %q", cfg.RestoreHandler)
	}

	if cfg.RestoreHandlerSource != RestoreHandlerSourceSaved {
		t.Fatalf("generated config restore_handler_source: got %q", cfg.RestoreHandlerSource)
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
		`restore_handler = ""`,
		`restore_handler_source = "saved"`,
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

func TestRenderRestoreHandlerSourceRoundTrips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "default", source: RestoreHandlerSourceSaved, want: RestoreHandlerSourceSaved},
		{
			name:   "resolved",
			source: RestoreHandlerSourceResolved,
			want:   RestoreHandlerSourceResolved,
		},
		{name: "zero", source: "", want: RestoreHandlerSourceSaved},
		{name: "invalid", source: "bogus", want: RestoreHandlerSourceSaved},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			cfg := Default()
			cfg.RestoreHandlerSource = test.source
			out := cfg.Render()
			wantLine := `restore_handler_source = ` + strconv.Quote(test.want)
			if strings.Count(out, "restore_handler_source = ") != 1 {
				t.Fatalf("render must contain exactly one source line:\n%s", out)
			}
			if !strings.Contains(out, wantLine) {
				t.Fatalf("render missing %q in:\n%s", wantLine, out)
			}

			loaded, err := LoadFrom(writeConfig(t, out))
			if err != nil {
				t.Fatalf("load rendered config: %v", err)
			}
			if loaded.RestoreHandlerSource != test.want {
				t.Fatalf("round trip: got %q want %q", loaded.RestoreHandlerSource, test.want)
			}
		})
	}
}

func TestRenderRestoreHandlerRoundTrips(t *testing.T) {
	t.Parallel()

	for name, handler := range map[string]string{
		"empty":     "",
		"non-empty": "cowsay -f tux",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfg := Default()
			cfg.RestoreHandler = handler

			out := cfg.Render()
			wantLine := `restore_handler = ` + strconv.Quote(handler)
			if !strings.Contains(out, wantLine) {
				t.Fatalf("render missing %q in:\n%s", wantLine, out)
			}

			loaded, err := LoadFrom(writeConfig(t, out))
			if err != nil {
				t.Fatalf("load rendered config: %v", err)
			}

			if loaded.RestoreHandler != handler {
				t.Fatalf("round trip: got %q want %q", loaded.RestoreHandler, handler)
			}
		})
	}
}
