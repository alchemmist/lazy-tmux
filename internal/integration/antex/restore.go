package antex

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errUnverifiedSession = errors.New(
	"antex session identity is unverified; resume the intended conversation manually and save it with an updated Antex",
)

func (i *Integration) RestoreDecision(pane snapshot.Pane, meta map[string]string) (string, error) {
	if pane.Agent != nil {
		command, err := agent.Restore(pane, commandName)
		if err != nil {
			return "", fmt.Errorf("restore agent: %w", err)
		}

		return command, nil
	}
	sessionID := meta[metaSessionID]
	if meta["session_id_source"] != bindingSource || !validSessionID(sessionID) ||
		!filepath.IsAbs(meta["home"]) || strings.IndexFunc(meta["home"], unicode.IsControl) >= 0 {
		return "", errUnverifiedSession
	}
	var argv []string
	if json.Unmarshal([]byte(meta["resume_argv"]), &argv) != nil || len(argv) < 3 ||
		argv[0] != commandName || argv[1] != "resume" || argv[2] != sessionID {
		return "", errUnverifiedSession
	}
	argv = resumeArgsWithCWD(argv)
	quoted := make([]string, 0, len(argv)+2)
	quoted = append(quoted, "env", quote("ANTEX_HOME="+meta["home"]))
	for _, arg := range argv {
		if strings.IndexFunc(arg, unicode.IsControl) >= 0 {
			return "", errUnverifiedSession
		}
		quoted = append(quoted, quote(arg))
	}

	return strings.Join(quoted, " "), nil
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func validSessionID(value string) bool {
	const uuidLength = 36
	if len(value) != uuidLength {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}

	return true
}

func resumeArgsWithCWD(argv []string) []string {
	explicit := slices.ContainsFunc(argv[3:], func(arg string) bool {
		return arg == "--cd" || arg == "-C" || strings.HasPrefix(arg, "--cd=")
	})
	if explicit || slices.Contains(argv, `tui.resume_cwd="session"`) {
		return argv
	}

	return append(argv, "-c", `tui.resume_cwd="session"`)
}
