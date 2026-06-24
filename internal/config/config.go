package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/alchemmist/lazy-tmux/internal/store"
)

type Config struct {
	TmuxBin        string
	DataDir        string
	SaveInterval   time.Duration
	RestoreTimeout time.Duration
	Scrollback     ScrollbackConfig
}

type ScrollbackConfig struct {
	Enabled bool
	Lines   int
}

func Default() Config {
	return Config{
		TmuxBin:        "tmux",
		DataDir:        store.DefaultDataDir(),
		SaveInterval:   5 * time.Minute,
		RestoreTimeout: 5 * time.Second,
		Scrollback: ScrollbackConfig{
			Enabled: false,
			Lines:   5000,
		},
	}
}

// DefaultConfigPath returns the path lazy-tmux reads its TOML config from.
//
// Resolution order: the LAZY_TMUX_CONFIG override, then
// $XDG_CONFIG_HOME/lazy-tmux/lazy-tmux.toml, then
// ~/.config/lazy-tmux/lazy-tmux.toml. It returns "" only when the home
// directory cannot be determined and no override is set.
func DefaultConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("LAZY_TMUX_CONFIG")); v != "" {
		return v
	}

	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return filepath.Join(v, "lazy-tmux", "lazy-tmux.toml")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "lazy-tmux", "lazy-tmux.toml")
}

// Load builds the effective configuration: built-in defaults overlaid with the
// values found in the TOML config file at DefaultConfigPath (if it exists).
// A missing file is not an error; an unreadable or malformed file is.
func Load() (Config, error) {
	return LoadFrom(DefaultConfigPath())
}

// LoadFrom is Load with an explicit config file path, primarily for testing.
func LoadFrom(path string) (Config, error) {
	cfg := Default()

	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}

		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	var file fileConfig

	err = toml.Unmarshal(data, &file)
	if err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	return cfg.withFile(file), nil
}

// fileConfig mirrors the TOML schema. Every field is a pointer so an absent key
// leaves the corresponding default untouched, rather than overwriting it with a
// zero value.
type fileConfig struct {
	TmuxBin        *string             `toml:"tmux_bin"`
	DataDir        *string             `toml:"data_dir"`
	SaveInterval   *duration           `toml:"save_interval"`
	RestoreTimeout *duration           `toml:"restore_timeout"`
	Scrollback     *fileScrollbackConf `toml:"scrollback"`
}

type fileScrollbackConf struct {
	Enabled *bool `toml:"enabled"`
	Lines   *int  `toml:"lines"`
}

// withFile returns a copy of cfg with every value set in file applied on top.
func (cfg Config) withFile(file fileConfig) Config {
	if file.TmuxBin != nil {
		cfg.TmuxBin = *file.TmuxBin
	}

	if file.DataDir != nil {
		cfg.DataDir = expandHome(*file.DataDir)
	}

	if file.SaveInterval != nil {
		cfg.SaveInterval = file.SaveInterval.Duration
	}

	if file.RestoreTimeout != nil {
		cfg.RestoreTimeout = file.RestoreTimeout.Duration
	}

	if file.Scrollback != nil {
		if file.Scrollback.Enabled != nil {
			cfg.Scrollback.Enabled = *file.Scrollback.Enabled
		}

		if file.Scrollback.Lines != nil {
			cfg.Scrollback.Lines = *file.Scrollback.Lines
		}
	}

	return cfg
}

// duration lets the TOML decoder accept Go duration strings like "5m" or "10s".
type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}

	d.Duration = parsed

	return nil
}

// expandHome resolves a leading ~ in a config-provided path to the user's home
// directory, so data_dir = "~/snapshots" works as expected.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}

	return path
}
