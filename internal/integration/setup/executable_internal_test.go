package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableExecutableSurvivesCellarCleanup(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "Cellar", "lazy-tmux", "0.3.6", "bin", "lazy-tmux")
	stable := filepath.Join(root, "bin", "lazy-tmux")
	for _, path := range []string{binary, stable} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(binary, []byte("original"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(stable))
	// An unrelated executable earlier in PATH must never be installed as a hook.
	if err := os.WriteFile(stable, []byte("unrelated"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := StableExecutable(binary, stable); got != binary {
		t.Fatalf("selected foreign executable %q", got)
	}
	if err := os.Remove(stable); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, stable); err != nil {
		t.Fatal(err)
	}
	selected := StableExecutable(binary, binary)
	if selected != stable {
		t.Fatalf("selected %q want stable %q", selected, stable)
	}
	home := filepath.Join(root, "home")
	for _, kind := range []string{"claude", "codex"} {
		if _, err := Apply(kind, home, selected, false); err != nil {
			t.Fatal(err)
		}
		name := "hooks.json"
		if kind == "claude" {
			name = "settings.json"
		}
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), binary) || !strings.Contains(string(data), stable) {
			t.Fatalf("unstable hook: %s", data)
		}
	}
	newer := filepath.Join(root, "Cellar", "lazy-tmux", "0.3.7", "bin", "lazy-tmux")
	if err := os.MkdirAll(filepath.Dir(newer), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("upgraded"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stable); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(newer, stable); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(selected)
	if err != nil || string(data) != "upgraded" {
		t.Fatalf("hook path did not survive cleanup: %s %v", data, err)
	}
}

func TestStableExecutableAcceptsRelativeLaunchSymlink(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("PATH", "")
	binary := filepath.Join(root, "releases", "current")
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("running executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(root, "launch", "renamed-client")
	if err := os.MkdirAll(filepath.Dir(stable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, stable); err != nil {
		t.Fatal(err)
	}
	if got := StableExecutable(binary, "./launch/renamed-client"); got != stable {
		t.Fatalf("selected %q want %q", got, stable)
	}
	if err := os.Remove(stable); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("unrelated executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := StableExecutable(binary, "./launch/renamed-client"); got != binary {
		t.Fatalf("accepted unrelated relative executable %q", got)
	}
}
