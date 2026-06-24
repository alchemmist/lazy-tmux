package config

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/data")

	cfg := Default()

	if cfg.TmuxBin != "tmux" {
		t.Fatalf("expected default tmux bin, got %q", cfg.TmuxBin)
	}

	if cfg.DataDir != "/data" {
		t.Fatalf("expected data dir from env, got %q", cfg.DataDir)
	}

	if cfg.SaveInterval != 5*time.Minute {
		t.Fatalf("expected 5m default interval, got %s", cfg.SaveInterval)
	}

	if cfg.Scrollback.Enabled {
		t.Fatal("expected scrollback disabled by default")
	}

	if cfg.Scrollback.Lines != 5000 {
		t.Fatalf("expected 5000 default scrollback lines, got %d", cfg.Scrollback.Lines)
	}
}
