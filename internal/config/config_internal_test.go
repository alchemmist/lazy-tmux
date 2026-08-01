package config

import (
	"errors"
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
	t.Parallel()

	path := writeConfig(t, `
tmux_bin = "/usr/local/bin/tmux"
data_dir = "/snapshots"
save_interval = "10m"
restore_timeout = "0s"
theme = "light"

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
	if cfg.Theme != "light" {
		t.Fatalf("theme: got %q", cfg.Theme)
	}

	if !cfg.Scrollback.Enabled || cfg.Scrollback.Lines != 200 {
		t.Fatalf("scrollback: got %+v", cfg.Scrollback)
	}
}

func TestSetThemePreservesConfig(t *testing.T) {
	path := writeConfig(t, "# keep me\ndata_dir = \"/data\"\n")

	if err := SetTheme(path, "light"); err != nil {
		t.Fatalf("set theme: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "# keep me") || !strings.Contains(got, `theme = "light"`) {
		t.Fatalf("theme update did not preserve config: %s", got)
	}
}

func TestLoadFromInvalidThemeErrors(t *testing.T) {
	_, err := LoadFrom(writeConfig(t, `theme = "solarized"`))
	if err == nil || !strings.Contains(err.Error(), "invalid theme") {
		t.Fatalf("expected invalid theme error, got %v", err)
	}
}

func TestLoadFromPartialFileKeepsDefaults(t *testing.T) {
	t.Setenv("LAZY_TMUX_DATA_DIR", "/data")

	path := writeConfig(t, "restore_timeout = \"2s\"\n")

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	path := writeConfig(t, "this is not = valid = toml\n")

	if _, err := LoadFrom(path); err == nil {
		t.Fatal("expected error for malformed config")
	}
}

func TestLoadFromInvalidDurationErrors(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "save_interval = \"not-a-duration\"\n")

	if _, err := LoadFrom(path); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoadFromInvalidPatternErrors(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"restore_allowlist", "restore_denylist"} {
		path := writeConfig(t, key+" = [\"node .*\", \"(\"]\n")

		_, err := LoadFrom(path)
		if err == nil {
			t.Fatalf("%s: expected error for an invalid regex", key)
		}

		if !errors.Is(err, errInvalidPattern) {
			t.Fatalf("%s: expected errInvalidPattern, got %v", key, err)
		}
	}
}

func TestLoadFromRestoreAllowlist(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(writeConfig(t, "tmux_bin = \"tmux\"\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.RestoreAllowlist != nil {
		t.Fatalf("absent allowlist should be nil, got %#v", cfg.RestoreAllowlist)
	}

	cfg, err = LoadFrom(writeConfig(t, "restore_allowlist = [\"nvim\", \"htop\"]\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(cfg.RestoreAllowlist) != 2 ||
		cfg.RestoreAllowlist[0] != "nvim" ||
		cfg.RestoreAllowlist[1] != "htop" {
		t.Fatalf("allowlist: got %#v", cfg.RestoreAllowlist)
	}

	cfg, err = LoadFrom(writeConfig(t, "restore_allowlist = []\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.RestoreAllowlist == nil || len(cfg.RestoreAllowlist) != 0 {
		t.Fatalf("empty allowlist should be non-nil empty, got %#v", cfg.RestoreAllowlist)
	}
}

func TestLoadFromRestoreDenylist(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(writeConfig(t, "tmux_bin = \"tmux\"\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.RestoreDenylist != nil {
		t.Fatalf("absent denylist should be nil, got %#v", cfg.RestoreDenylist)
	}

	cfg, err = LoadFrom(writeConfig(t, "restore_denylist = [\"npm\", \"node\"]\n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(cfg.RestoreDenylist) != 2 ||
		cfg.RestoreDenylist[0] != "npm" ||
		cfg.RestoreDenylist[1] != "node" {
		t.Fatalf("denylist: got %#v", cfg.RestoreDenylist)
	}
}

func TestLoadFromUnknownKeyErrors(t *testing.T) {
	t.Parallel()

	path := writeConfig(t, "tmux_binn = \"tmux\"\n")

	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for unknown config key")
	}

	if !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("expected unknown keys error, got %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "lazy-tmux.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return path
}
