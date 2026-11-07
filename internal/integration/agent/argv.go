package agent

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	resumeSubcommand = "resume"
	optionVariadic   = -1
	optionOptional   = -2
)

func ResumeArgv(kind, sessionID, cwd string, launch []string) ([]string, error) {
	launch = normalizedLaunch(kind, launch)
	if !validResumeLaunch(kind, launch) {
		return nil, ErrUnverified
	}
	if split := codexResumeScope(kind, launch); split > 1 {
		return scopedCodexResume(sessionID, cwd, launch, split)
	}
	subcommand := resumeSubcommand
	if kind == snapshot.AgentClaude {
		subcommand = claudeResume
	}
	result := []string{launch[0], subcommand, sessionID}
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
		flag, _, _ := strings.Cut(arg, "=")
		arity, transient := transientOption(kind, flag)
		if !transient {
			var known bool
			arity, known = launchOption(kind, flag)
			if !known {
				return nil, fmt.Errorf("unsupported launch option %q: %w", flag, ErrUnverified)
			}
		}
		values, rest, err := launchOptionArgs(arg, remaining, arity)
		if err != nil {
			return nil, err
		}
		remaining = rest
		if !transient {
			result = appendResumeOption(result, kind, flag, values)
		}
	}
	if kind != snapshot.AgentClaude {
		result = append(result, "--cd", cwd)
	}

	return result, nil
}

func validResumeLaunch(kind string, launch []string) bool {
	return slices.Contains([]string{snapshot.AgentClaude, snapshot.AgentCodex, snapshot.AgentAntex}, kind) &&
		len(launch) > 0 &&
		filepath.Base(launch[0]) == kind
}

// codexResumeScope locates the subcommand without treating option values as commands.
func codexResumeScope(kind string, launch []string) int {
	if kind != snapshot.AgentCodex {
		return 0
	}
	for index := 1; index < len(launch); {
		arg := launch[index]
		if arg == resumeSubcommand {
			return index
		}
		if !strings.HasPrefix(arg, "-") || arg == "--" {
			return 0
		}
		flag, _, _ := strings.Cut(arg, "=")
		arity, known := launchOption(kind, flag)
		if !known {
			arity, known = transientOption(kind, flag)
		}
		if !known {
			return 0
		}
		_, rest, err := launchOptionArgs(arg, launch[index+1:], arity)
		if err != nil {
			return 0
		}
		index = len(launch) - len(rest)
	}

	return 0
}

// scopedCodexResume validates each scope separately, preserving local precedence.
func scopedCodexResume(sessionID, cwd string, launch []string, split int) ([]string, error) {
	root, err := ResumeArgv(snapshot.AgentCodex, sessionID, cwd, launch[:split])
	if err != nil {
		return nil, err
	}
	localLaunch := append([]string{launch[0]}, launch[split:]...)
	local, err := ResumeArgv(snapshot.AgentCodex, sessionID, cwd, localLaunch)
	if err != nil {
		return nil, err
	}
	// Strip the synthetic resume/id prefix and cwd suffix from root arguments.
	result := append([]string{launch[0]}, root[3:len(root)-2]...)

	return append(result, local[1:]...), nil
}

// appendResumeOption keeps Codex's global and resume-local execution mode in one scope.
func appendResumeOption(argv []string, kind, flag string, values []string) []string {
	if kind == snapshot.AgentCodex && flag == "--no-daemon" && slices.Contains(argv, flag) {
		return argv
	}

	return append(argv, values...)
}

func launchOptionArgs(arg string, remaining []string, arity int) ([]string, []string, error) {
	flag, value, attached := strings.Cut(arg, "=")
	if attached {
		// Commander only consumes the attached value, even for variadic options.
		// Keep its spelling: separating a dash-prefixed value changes its meaning.
		if arity == 0 || value == "" || !strings.HasPrefix(flag, "--") {
			return nil, nil, fmt.Errorf("unsupported attached value %q: %w", arg, ErrUnverified)
		}

		return []string{arg}, remaining, nil
	}
	count := 0
	if arity != 0 && len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		count = 1
		if arity == optionVariadic {
			for count < len(remaining) && !strings.HasPrefix(remaining[count], "-") {
				count++
			}
		}
	}
	if arity != 0 && arity != optionOptional && (count == 0 || remaining[0] == "") {
		return nil, nil, fmt.Errorf("missing launch option value for %q: %w", arg, ErrUnverified)
	}

	return append([]string{arg}, remaining[:count]...), remaining[count:], nil
}

// launchOption returns the argument count for persistent options supported on resume.
func launchOption(kind, flag string) (int, bool) {
	common := map[string]int{"--model": 1, "-m": 1}
	if arity, ok := common[flag]; ok {
		return arity, true
	}
	if kind == snapshot.AgentClaude {
		options := map[string]int{
			"--permission-mode":              1,
			"--dangerously-skip-permissions": 0,
			"--allowedTools":                 optionVariadic,
			"--allowed-tools":                optionVariadic,
			"--disallowedTools":              optionVariadic,
			"--disallowed-tools":             optionVariadic,
			"--add-dir":                      optionVariadic,
			"--settings":                     1,
			"--setting-sources":              1,
			"--verbose":                      0,
			"--effort":                       1,
		}
		arity, ok := options[flag]

		return arity, ok
	}
	// Keep Codex's execution mode when resuming instead of switching back to a daemon.
	if kind == snapshot.AgentCodex && flag == "--no-daemon" {
		return 0, true
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

func transientOption(kind, flag string) (int, bool) {
	if kind == snapshot.AgentClaude {
		switch flag {
		case claudeResume, "-r", "--worktree", "-w":
			return optionOptional, true
		case "--continue", "-c", "--fork-session":
			return 0, true
		default:
			return 0, false
		}
	}
	switch flag {
	case "--cd", "-C", "--image", "-i":
		return 1, true
	case "--last", "--all":
		return 0, true
	default:
		return 0, false
	}
}

func normalizedLaunch(kind string, launch []string) []string {
	if len(launch) > 1 && filepath.Base(launch[0]) == "node" && LaunchKind(launch) == kind {
		return append([]string{kind}, launch[2:]...)
	}

	return launch
}
