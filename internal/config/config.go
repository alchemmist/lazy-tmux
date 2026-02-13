package config

import (
	"time"

	"github.com/alchemmist/lazy-tmux/internal/store"
)

type Config struct {
	TmuxBin      string
	DataDir      string
	SaveInterval time.Duration
}

func Default() Config {
	return Config{
		TmuxBin:      "tmux",
		DataDir:      store.DefaultDataDir(),
		SaveInterval: 5 * time.Minute,
	}
}
