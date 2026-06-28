package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	if cfg.RestoreTimeout != 5*time.Second {
		t.Fatalf("expected 5s default restore timeout, got %s", cfg.RestoreTimeout)
	}

	if cfg.Scrollback.Enabled {
		t.Fatal("expected scrollback disabled by default")
	}

	if cfg.Scrollback.Lines != 5000 {
		t.Fatalf("expected 5000 default scrollback lines, got %d", cfg.Scrollback.Lines)
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Setenv("LAZY_TMUX_CONFIG", "/custom/lazy.toml")

	if got := DefaultConfigPath(); got != "/custom/lazy.toml" {
		t.Fatalf("LAZY_TMUX_CONFIG override: got %q", got)
	}

	t.Setenv("LAZY_TMUX_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	want := filepath.Join("/xdg", "lazy-tmux", "lazy-tmux.toml")
	if got := DefaultConfigPath(); got != want {
		t.Fatalf("XDG_CONFIG_HOME path: got %q want %q", got, want)
	}
}

func TestLoadFromMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/data")

	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}

	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("missing file should yield defaults, got %+v", cfg)
	}
}

func TestLoadFromFullFile(t *testing.T) {
	path := writeConfig(t, `
tmux_bin = "/usr/local/bin/tmux"
data_dir = "/snapshots"
save_interval = "10m"
restore_timeout = "0s"

[scrollback]
enabled = true
lines = 200
`)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.TmuxBin != "/usr/local/bin/tmux" {
		t.Fatalf("tmux_bin: got %q", cfg.TmuxBin)
	}

	if cfg.DataDir != "/snapshots" {
		t.Fatalf("data_dir: got %q", cfg.DataDir)
	}

	if cfg.SaveInterval != 10*time.Minute {
		t.Fatalf("save_interval: got %s", cfg.SaveInterval)
	}

	if cfg.RestoreTimeout != 0 {
		t.Fatalf("restore_timeout: got %s", cfg.RestoreTimeout)
	}

	if !cfg.Scrollback.Enabled || cfg.Scrollback.Lines != 200 {
		t.Fatalf("scrollback: got %+v", cfg.Scrollback)
	}
}

func TestLoadFromPartialFileKeepsDefaults(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/data")

	path := writeConfig(t, "restore_timeout = \"2s\"\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Only restore_timeout is overridden; everything else keeps its default.
	if cfg.RestoreTimeout != 2*time.Second {
		t.Fatalf("restore_timeout: got %s", cfg.RestoreTimeout)
	}

	if cfg.TmuxBin != "tmux" || cfg.DataDir != "/data" || cfg.SaveInterval != 5*time.Minute {
		t.Fatalf("untouched defaults changed: %+v", cfg)
	}

	if cfg.Scrollback.Lines != 5000 {
		t.Fatalf("scrollback lines default lost: %d", cfg.Scrollback.Lines)
	}
}

func TestLoadFromExpandsHomeInDataDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}

	path := writeConfig(t, "data_dir = \"~/snapshots\"\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if want := filepath.Join(home, "snapshots"); cfg.DataDir != want {
		t.Fatalf("home expansion: got %q want %q", cfg.DataDir, want)
	}
}

func TestLoadFromExpandsHomeInTmuxBin(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}

	path := writeConfig(t, "tmux_bin = \"~/bin/tmux.appimage\"\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if want := filepath.Join(home, "bin/tmux.appimage"); cfg.TmuxBin != want {
		t.Fatalf("home expansion: got %q want %q", cfg.TmuxBin, want)
	}
}

func TestLoadFromMalformedFileErrors(t *testing.T) {
	path := writeConfig(t, "this is not = valid = toml\n")

	if _, err := LoadFrom(path); err == nil {
		t.Fatal("expected error for malformed config")
	}
}

func TestLoadFromInvalidDurationErrors(t *testing.T) {
	path := writeConfig(t, "save_interval = \"not-a-duration\"\n")

	if _, err := LoadFrom(path); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoadFromRestoreAllowlist(t *testing.T) {
	// Absent key -> nil (allowlist disabled, restore everything).
	cfg, err := LoadFrom(writeConfig(t, "tmux_bin = \"tmux\"\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.RestoreAllowlist != nil {
		t.Fatalf("absent allowlist should be nil, got %#v", cfg.RestoreAllowlist)
	}

	// Populated list.
	cfg, err = LoadFrom(writeConfig(t, "restore_allowlist = [\"nvim\", \"htop\"]\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(cfg.RestoreAllowlist) != 2 ||
		cfg.RestoreAllowlist[0] != "nvim" ||
		cfg.RestoreAllowlist[1] != "htop" {
		t.Fatalf("allowlist: got %#v", cfg.RestoreAllowlist)
	}

	// Explicitly empty list -> non-nil empty (allowlist active, restore nothing).
	cfg, err = LoadFrom(writeConfig(t, "restore_allowlist = []\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.RestoreAllowlist == nil || len(cfg.RestoreAllowlist) != 0 {
		t.Fatalf("empty allowlist should be non-nil empty, got %#v", cfg.RestoreAllowlist)
	}
}

func TestLoadFromUnknownKeyErrors(t *testing.T) {
	// A typo'd key must fail loudly rather than be silently ignored.
	path := writeConfig(t, "tmux_binn = \"tmux\"\n")

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for unknown config key")
	}

	if !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("expected unknown keys error, got %v", err)
	}
}

// writeConfig writes body to a temp .toml file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}
