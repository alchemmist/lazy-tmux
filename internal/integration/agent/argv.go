package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func ResumeArgv(kind, sessionID, cwd string, launch []string) ([]string, error) {
	subcommand := "resume"
	if kind == snapshot.AgentClaude {
		subcommand = claudeResume
	}
	executable := restorationExecutable(kind, launch)
	result := []string{executable, subcommand, sessionID}
	if len(launch) == 0 {
		return nil, ErrUnverified
	}
	remaining := launch[1:]
	for len(remaining) > 0 {
		arg := remaining[0]
		remaining = remaining[1:]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flag, value, attached := strings.Cut(arg, "=")
		skip, hasValue := transientOption(flag)
		if skip {
			if hasValue && !attached {
				if len(remaining) == 0 {
					return nil, ErrUnverified
				}
				remaining = remaining[1:]
			}

			continue
		}
		arity, known := launchOption(kind, flag)
		if !known {
			return nil, fmt.Errorf("unsupported launch option %q: %w", flag, ErrUnverified)
		}
		if arity == 0 {
			result = append(result, arg)

			continue
		}
		if !attached {
			if len(remaining) == 0 {
				return nil, ErrUnverified
			}
			value = remaining[0]
			remaining = remaining[1:]
		}
		result = append(result, flag, value)
	}
	if kind != snapshot.AgentClaude {
		result = append(result, "--cd", cwd)
	}

	return result, nil
}

func launchOption(kind, flag string) (int, bool) {
	common := map[string]int{"--model": 1, "-m": 1}
	if arity, ok := common[flag]; ok {
		return arity, true
	}
	if kind == snapshot.AgentClaude {
		options := map[string]int{
			"--permission-mode":              1,
			"--dangerously-skip-permissions": 0,
			"--allowedTools":                 1,
			"--disallowedTools":              1,
			"--add-dir":                      1,
			"--settings":                     1,
			"--setting-sources":              1,
			"--verbose":                      0,
			"--effort":                       1,
		}
		arity, ok := options[flag]

		return arity, ok
	}
	options := map[string]int{
		"--profile":          1,
		"-p":                 1,
		"--sandbox":          1,
		"-s":                 1,
		"--ask-for-approval": 1,
		"-a":                 1,
		"--config":           1,
		"-c":                 1,
		"--add-dir":          1,
		"--oss":              0,
		"--local-provider":   1,
		"--no-alt-screen":    0,
		"--search":           0,
		"--full-auto":        0,
		"--dangerously-bypass-approvals-and-sandbox": 0,
		"--yolo":    0,
		"--enable":  1,
		"--disable": 1,
	}
	arity, ok := options[flag]

	return arity, ok
}

func LaunchKind(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	if filepath.Base(argv[0]) == "node" && len(argv) > 1 {
		return Kind(argv[0] + " " + argv[1])
	}

	return Kind(argv[0])
}

func transientOption(flag string) (bool, bool) {
	switch flag {
	case claudeResume, "-r", "--cd", "-C", "--image", "-i":
		return true, true
	case "--continue", "--last", "--fork-session", "--worktree", "--all":
		return true, false
	default:
		return false, false
	}
}

func restorationExecutable(kind string, launch []string) string {
	if len(launch) > 0 && filepath.Base(launch[0]) == kind {
		return launch[0]
	}

	return kind
}
