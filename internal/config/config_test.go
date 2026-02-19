package config

import (
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.TmuxBin != "tmux" {
		t.Fatalf("expected tmux binary, got %q", cfg.TmuxBin)
	}
	if cfg.DataDir == "" {
		t.Fatal("expected non-empty data dir")
	}
	if cfg.SaveInterval != 5*time.Minute {
		t.Fatalf("expected 5m interval, got %s", cfg.SaveInterval)
	}
}
