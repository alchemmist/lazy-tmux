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

var (
	errUnknownConfigKeys = errors.New("unknown keys")
	errNoConfigPath      = errors.New(
		"could not determine a config path (pass --path or set $HOME)",
	)
	errConfigExists = errors.New("already exists (use --force to overwrite)")
)

type Config struct {
	TmuxBin        string
	DataDir        string
	SaveInterval   time.Duration
	RestoreTimeout time.Duration
	Scrollback     ScrollbackConfig
	Integrations   IntegrationsConfig

	RestoreAllowlist []string

	RestoreDenylist []string
}

type ScrollbackConfig struct {
	Enabled bool
	Lines   int
}

type IntegrationsConfig struct {
	Enabled bool
	Claude  ClaudeIntegrationConfig
}

type ClaudeIntegrationConfig struct {
	Enabled bool
	Home    string
}

const (
	defaultTmuxBin        = "tmux"
	defaultSaveInterval   = 5 * time.Minute
	defaultRestoreTimeout = 5 * time.Second
	defaultClaudeHome     = "~/.claude"

	DefaultScrollbackLines = 5000
)

func Default() Config {
	return Config{
		TmuxBin:        defaultTmuxBin,
		DataDir:        store.DefaultDataDir(),
		SaveInterval:   defaultSaveInterval,
		RestoreTimeout: defaultRestoreTimeout,
		Scrollback: ScrollbackConfig{
			Enabled: false,
			Lines:   DefaultScrollbackLines,
		},
		Integrations: IntegrationsConfig{
			Enabled: true,
			Claude: ClaudeIntegrationConfig{
				Enabled: true,
				Home:    defaultClaudeHome,
			},
		},
	}
}

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

func Load() (Config, error) {
	return LoadFrom(DefaultConfigPath())
}

func LoadFrom(path string) (Config, error) {
	cfg := Default()

	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(
		path,
	) // #nosec G304 -- config path is chosen by the user via flag/env by design
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}

		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}

	var file fileConfig

	meta, err := toml.Decode(string(data), &file)
	if err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("config %s: %w: %v", path, errUnknownConfigKeys, undecoded)
	}

	return cfg.withFile(file), nil
}

type fileConfig struct {
	TmuxBin          *string               `toml:"tmux_bin"`
	DataDir          *string               `toml:"data_dir"`
	SaveInterval     *duration             `toml:"save_interval"`
	RestoreTimeout   *duration             `toml:"restore_timeout"`
	RestoreAllowlist *[]string             `toml:"restore_allowlist"`
	RestoreDenylist  *[]string             `toml:"restore_denylist"`
	Scrollback       *fileScrollbackConf   `toml:"scrollback"`
	Integrations     *fileIntegrationsConf `toml:"integrations"`
}

type fileScrollbackConf struct {
	Enabled *bool `toml:"enabled"`
	Lines   *int  `toml:"lines"`
}

type fileIntegrationsConf struct {
	Enabled *bool                  `toml:"enabled"`
	Claude  *fileClaudeIntegration `toml:"claude"`
}

type fileClaudeIntegration struct {
	Enabled *bool   `toml:"enabled"`
	Home    *string `toml:"home"`
}

func (cfg Config) withFile(file fileConfig) Config {
	if file.TmuxBin != nil {
		cfg.TmuxBin = ExpandHome(*file.TmuxBin)
	}

	if file.DataDir != nil {
		cfg.DataDir = ExpandHome(*file.DataDir)
	}

	if file.SaveInterval != nil {
		cfg.SaveInterval = file.SaveInterval.Duration
	}

	if file.RestoreTimeout != nil {
		cfg.RestoreTimeout = file.RestoreTimeout.Duration
	}

	if file.RestoreAllowlist != nil {
		allowlist := *file.RestoreAllowlist
		if allowlist == nil {
			allowlist = []string{}
		}

		cfg.RestoreAllowlist = allowlist
	}

	if file.RestoreDenylist != nil {
		cfg.RestoreDenylist = *file.RestoreDenylist
	}

	if file.Scrollback != nil {
		if file.Scrollback.Enabled != nil {
			cfg.Scrollback.Enabled = *file.Scrollback.Enabled
		}

		if file.Scrollback.Lines != nil {
			cfg.Scrollback.Lines = *file.Scrollback.Lines
		}
	}

	if file.Integrations != nil {
		cfg.Integrations = cfg.Integrations.withFile(*file.Integrations)
	}

	return cfg
}

func (ic IntegrationsConfig) withFile(file fileIntegrationsConf) IntegrationsConfig {
	if file.Enabled != nil {
		ic.Enabled = *file.Enabled
	}

	if file.Claude != nil {
		if file.Claude.Enabled != nil {
			ic.Claude.Enabled = *file.Claude.Enabled
		}

		if file.Claude.Home != nil {
			ic.Claude.Home = ExpandHome(*file.Claude.Home)
		}
	}

	return ic
}

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

func ExpandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}

	return path
}
