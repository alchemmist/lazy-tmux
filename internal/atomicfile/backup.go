package atomicfile

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
)

func Backup(root *os.Root, name string, data []byte) error {
	_, err := root.Stat(name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect backup: %w", err)
	}
	temporary := ".lazy-tmux-backup-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("stage backup: %w", err)
	}
	defer func() { _ = file.Close(); _ = root.Remove(temporary) }()
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync backup: %w", err)
	}
	err = root.Link(temporary, name)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("publish backup: %w", err)
	}

	return nil
}
