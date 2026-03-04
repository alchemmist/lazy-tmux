package config

import (
	"time"

	"github.com/alchemmist/lazy-tmux/internal/store"
)

type Config struct {
	TmuxBin      string
	DataDir      string
	SaveInterval time.Duration
	Scrollback   ScrollbackConfig
}

type ScrollbackConfig struct {
	Enabled bool
	Lines   int
}

func Default() Config {
	return Config{
		TmuxBin:      "tmux",
		DataDir:      store.DefaultDataDir(),
		SaveInterval: 5 * time.Minute,
		Scrollback: ScrollbackConfig{
			Enabled: false,
			Lines:   5000,
		},
	}
}
