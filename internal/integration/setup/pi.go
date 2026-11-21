package setup

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

//go:embed pi-extension.ts
var piExtension []byte

const (
	piExtensionName = "lazy-tmux-pi.ts"
	piDisabled      = ".lazy-tmux-pi-disabled"
	piMarker        = "// Managed by lazy-tmux: pi-extension-v1\n"
)

var errPiExtensionOwnership = errors.New("pi extension is not a regular file owned by lazy-tmux")

// EnsurePi installs the embedded extension, respecting an explicit uninstall.
func EnsurePi(home string) (bool, error) { return applyPi(home, false, true) }

func applyPi(home string, uninstall, automatic bool) (bool, error) {
	if !filepath.IsAbs(home) {
		return false, errHookShape
	}
	err := os.MkdirAll(home, 0o700)
	if err != nil {
		return false, fmt.Errorf("create Pi home: %w", err)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return false, fmt.Errorf("open Pi home: %w", err)
	}
	defer func() { _ = root.Close() }()
	const lockName = ".lazy-tmux-pi.lock"
	lock, err := root.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		lock, err = root.OpenFile(lockName, os.O_RDWR, 0)
	}
	if err != nil {
		return false, fmt.Errorf("open Pi lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
	if err != nil {
		return false, fmt.Errorf("lock Pi extension: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if automatic {
		_, err = root.Stat(piDisabled)
		if err == nil {
			return false, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("inspect Pi opt-out: %w", err)
		}
	}

	return updatePi(root, home, uninstall, automatic)
}

func openPiExtensions(home string) (*os.Root, error) {
	path := filepath.Join(home, "extensions")
	err := os.MkdirAll(path, 0o700)
	if err != nil {
		return nil, fmt.Errorf("create Pi extensions: %w", err)
	}
	// The extension directory may intentionally point to a dotfiles checkout.
	dir, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("open Pi extensions: %w", err)
	}

	return dir, nil
}

func readOwnedPi(dir *os.Root) ([]byte, error) {
	info, err := dir.Lstat(piExtensionName)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect Pi extension: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errPiExtensionOwnership
	}
	data, err := dir.ReadFile(piExtensionName)
	if err != nil {
		return nil, fmt.Errorf("read Pi extension: %w", err)
	}
	if !bytes.HasPrefix(data, []byte(piMarker)) {
		return nil, errPiExtensionOwnership
	}

	return data, nil
}

func updatePi(root *os.Root, home string, uninstall, automatic bool) (bool, error) {
	dir, err := openPiExtensions(home)
	if err != nil {
		return false, err
	}
	defer func() { _ = dir.Close() }()
	data, err := readOwnedPi(dir)
	if err != nil {
		return false, err
	}
	if uninstall {
		err = root.WriteFile(
			piDisabled,
			[]byte("disabled by integrations setup pi --uninstall\n"),
			0o600,
		)
		if err != nil {
			return false, fmt.Errorf("disable Pi auto-setup: %w", err)
		}
		if data == nil {
			return false, nil
		}
		err = dir.Remove(piExtensionName)
		if err != nil {
			return false, fmt.Errorf("remove Pi extension: %w", err)
		}

		return true, nil
	}
	if !automatic {
		err = root.Remove(piDisabled)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("enable Pi auto-setup: %w", err)
		}
	}
	if bytes.Equal(data, piExtension) {
		return false, nil
	}

	return true, writeConfig(dir, piExtensionName, data, piExtension)
}

func checkPi(home string) (bool, error) {
	dir, err := os.OpenRoot(filepath.Join(home, "extensions"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Pi extensions: %w", err)
	}
	defer func() { _ = dir.Close() }()
	data, err := readOwnedPi(dir)
	if err != nil {
		return false, err
	}

	return bytes.Equal(data, piExtension), nil
}
