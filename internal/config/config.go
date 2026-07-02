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
	Integrations   IntegrationsConfig

	// RestoreAllowlist limits which commands are replayed on restore, matched by
	// executable name. A nil slice means no allowlist is configured and every
	// command is restored (the default). A non-nil slice — including an empty
	// one — activates the allowlist: only the listed commands are restored, and
	// an empty list restores none.
	RestoreAllowlist []string

	// RestoreDenylist blocks specific commands from being replayed on restore,
	// matched by executable name. It is the inverse of RestoreAllowlist: instead
	// of enumerating everything you trust, you list only the few programs to
	// exclude. The denylist wins over the allowlist — a command is replayed only
	// when it is not denied and (no allowlist is set or it is allowed). A nil or
	// empty slice blocks nothing (the default).
	RestoreDenylist []string
}

type ScrollbackConfig struct {
	Enabled bool
	Lines   int
}

// IntegrationsConfig controls the program-integration framework: a master switch
// plus per-integration settings.
type IntegrationsConfig struct {
	Enabled bool
	Claude  ClaudeIntegrationConfig
}

// ClaudeIntegrationConfig configures the Claude Code integration (restore a
// `claude` pane as `claude --resume <session-id>`).
type ClaudeIntegrationConfig struct {
	Enabled bool
	// Home is the Claude Code data directory; transcripts live under
	// <Home>/projects/<cwd>/<session-id>.jsonl.
	Home string
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
		Integrations: IntegrationsConfig{
			Enabled: true,
			Claude: ClaudeIntegrationConfig{
				Enabled: true,
				Home:    "~/.claude",
			},
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

	meta, err := toml.Decode(string(data), &file)
	if err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Reject unknown keys so typos (e.g. "tmux_binn") fail loudly instead of
	// being silently ignored.
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		return cfg, fmt.Errorf("config %s: unknown keys: %v", path, undecoded)
	}

	return cfg.withFile(file), nil
}

// fileConfig mirrors the TOML schema. Every field is a pointer so an absent key
// leaves the corresponding default untouched, rather than overwriting it with a
// zero value.
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

// withFile returns a copy of cfg with every value set in file applied on top.
func (cfg Config) withFile(file fileConfig) Config {
	if file.TmuxBin != nil {
		// Expand a leading ~ so e.g. tmux_bin = "~/bin/tmux.appimage" works
		// (matches data_dir handling).
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
		// Keep the slice non-nil even when empty so a configured-but-empty
		// allowlist stays distinguishable from "no allowlist configured".
		allowlist := *file.RestoreAllowlist
		if allowlist == nil {
			allowlist = []string{}
		}

		cfg.RestoreAllowlist = allowlist
	}

	if file.RestoreDenylist != nil {
		// Unlike the allowlist, the denylist has no "configured-but-empty"
		// meaning: an empty list simply blocks nothing, so a plain copy is fine.
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

// ExpandHome resolves a leading ~ in a path to the user's home directory, so
// e.g. "~/snapshots" (data_dir) or "~/bin/tmux.appimage" (tmux_bin, including
// the --tmux-bin flag) works as expected.
func ExpandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}

	return path
}
