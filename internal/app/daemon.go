package app

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (a *App) RunDaemon(interval time.Duration) error {
	if interval <= 0 {
		interval = a.cfg.SaveInterval
	}
	unlock, err := acquireLock(a.tmux.SocketPath())
	if err != nil {
		return err
	}
	defer unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	_ = a.SaveAll()
	for range ticker.C {
		if err := a.SaveAll(); err != nil {
			fmt.Fprintf(os.Stderr, "lazy-tmux daemon save error: %v\n", err)
		}
	}
	return nil
}

func acquireLock(socketPath string) (func(), error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return nil, err
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(socketPath))
	lockPath := filepath.Join(runtimeDir, fmt.Sprintf("lazy-tmux-%x.lock", h.Sum64()))
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("daemon already running")
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
