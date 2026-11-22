package app

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func (a *App) RunDaemon(ctx context.Context, interval time.Duration) error {
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

	for ctx.Err() == nil {
		_, err := a.SaveAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "lazy-tmux daemon save error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}

	return nil
}

func acquireLock(socketPath string) (func(), error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}

	// #nosec G703 -- XDG_RUNTIME_DIR is the standard location for lock files
	err := os.MkdirAll(runtimeDir, 0o750)
	if err != nil {
		return nil, fmt.Errorf("create runtime dir: %w", err)
	}

	hash := fnv.New64a()
	_, _ = hash.Write([]byte(socketPath))
	lockPath := filepath.Join(runtimeDir, fmt.Sprintf("lazy-tmux-%x.lock", hash.Sum64()))

	// #nosec G304,G703 -- fixed name with a socket-path hash under the runtime dir
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = file.Close()

		return nil, errDaemonRunning
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
