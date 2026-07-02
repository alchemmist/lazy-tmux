package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationsDefaults(t *testing.T) {
	cfg := Default()

	if !cfg.Integrations.Enabled || !cfg.Integrations.Claude.Enabled {
		t.Fatal("integrations should default to enabled")
	}

	if cfg.Integrations.Claude.Home != "~/.claude" {
		t.Fatalf("unexpected default claude home: %q", cfg.Integrations.Claude.Home)
	}
}

func TestIntegrationsOverrideAndExpand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}

	path := writeConfig(t, `
[integrations]
enabled = true

[integrations.claude]
enabled = false
home = "~/custom-claude"
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Integrations.Claude.Enabled {
		t.Fatal("claude.enabled should be overridden to false")
	}

	want := filepath.Join(home, "custom-claude")
	if cfg.Integrations.Claude.Home != want {
		t.Fatalf("home should expand ~, got %q want %q", cfg.Integrations.Claude.Home, want)
	}
}

func TestIntegrationsPartialKeepsDefaults(t *testing.T) {
	// Only the master switch is set; claude sub-keys keep their defaults.
	path := writeConfig(t, "[integrations]\nenabled = false\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.Integrations.Enabled {
		t.Fatal("master switch should be false")
	}

	if !cfg.Integrations.Claude.Enabled || cfg.Integrations.Claude.Home != "~/.claude" {
		t.Fatal("omitted claude keys should keep defaults")
	}
}

func TestRenderIncludesIntegrations(t *testing.T) {
	out := Default().Render()

	for _, want := range []string{"[integrations]", "[integrations.claude]", "home    = \"~/.claude\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered config missing %q:\n%s", want, out)
		}
	}
}
