package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StableExecutable accepts only paths referring to the running executable.
// Linux os.Executable resolves Homebrew symlinks into the removable Cellar.
func StableExecutable(current, invoked string) string {
	candidates := []string{}
	if prefix, _, ok := strings.Cut(filepath.ToSlash(current), "/Cellar/lazy-tmux/"); ok {
		candidates = append(candidates, filepath.Join(prefix, "bin", "lazy-tmux"))
	}
	if filepath.IsAbs(invoked) {
		candidates = append(candidates, invoked)
	}
	path, lookErr := exec.LookPath("lazy-tmux")
	if lookErr == nil {
		candidates = append(candidates, path)
	}
	original, err := os.Stat(current)
	if err != nil {
		return current
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err == nil && os.SameFile(original, info) {
			return absolute
		}
	}

	return current
}
