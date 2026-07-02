package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultConfigTemplate is the commented base config written by `config gen`.
// It is the single source of truth for the documented config schema.
//
//go:embed default.toml
var DefaultConfigTemplate string

// GenerateConfig writes the default config template to path (or DefaultConfigPath
// when path is empty), creating parent directories. Unless force is set it
// refuses to overwrite an existing file. It returns the path written.
func GenerateConfig(path string, force bool) (string, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultConfigPath()
	}

	if strings.TrimSpace(path) == "" {
		return "", errNoConfigPath
	}

	if !force {
		_, statErr := os.Stat(path)

		switch {
		case statErr == nil:
			return path, fmt.Errorf("%s %w", path, errConfigExists)
		case !errors.Is(statErr, fs.ErrNotExist):
			return path, fmt.Errorf("stat %s: %w", path, statErr)
		}
	}

	dir := filepath.Dir(path)
	if dir != "" {
		mkErr := os.MkdirAll(dir, 0o755)
		if mkErr != nil {
			return path, fmt.Errorf("create %s: %w", dir, mkErr)
		}
	}

	writeErr := os.WriteFile(path, []byte(DefaultConfigTemplate), 0o644)
	if writeErr != nil {
		return path, fmt.Errorf("write %s: %w", path, writeErr)
	}

	return path, nil
}

// Render returns the effective configuration as TOML, mirroring the file format
// (durations as strings, scrollback table), so it shows exactly what lazy-tmux
// resolved.
func (c Config) Render() string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "tmux_bin        = %s\n", strconv.Quote(c.TmuxBin))
	fmt.Fprintf(&buf, "data_dir        = %s\n", strconv.Quote(c.DataDir))
	fmt.Fprintf(&buf, "save_interval   = %s\n", strconv.Quote(c.SaveInterval.String()))
	fmt.Fprintf(&buf, "restore_timeout = %s\n", strconv.Quote(c.RestoreTimeout.String()))

	if c.RestoreAllowlist == nil {
		buf.WriteString("# restore_allowlist not set (every command is restored)\n")
	} else {
		quoted := make([]string, len(c.RestoreAllowlist))
		for i, cmd := range c.RestoreAllowlist {
			quoted[i] = strconv.Quote(cmd)
		}

		fmt.Fprintf(&buf, "restore_allowlist = [%s]\n", strings.Join(quoted, ", "))
	}

	if len(c.RestoreDenylist) == 0 {
		buf.WriteString("# restore_denylist not set (no command is blocked)\n")
	} else {
		quoted := make([]string, len(c.RestoreDenylist))
		for i, cmd := range c.RestoreDenylist {
			quoted[i] = strconv.Quote(cmd)
		}

		fmt.Fprintf(&buf, "restore_denylist = [%s]\n", strings.Join(quoted, ", "))
	}

	buf.WriteString("\n[scrollback]\n")
	fmt.Fprintf(&buf, "enabled = %t\n", c.Scrollback.Enabled)
	fmt.Fprintf(&buf, "lines   = %d\n", c.Scrollback.Lines)

	buf.WriteString("\n[integrations]\n")
	fmt.Fprintf(&buf, "enabled = %t\n", c.Integrations.Enabled)
	buf.WriteString("\n[integrations.claude]\n")
	fmt.Fprintf(&buf, "enabled = %t\n", c.Integrations.Claude.Enabled)
	fmt.Fprintf(&buf, "home    = %s\n", strconv.Quote(c.Integrations.Claude.Home))

	return buf.String()
}
