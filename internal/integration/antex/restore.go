package antex

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errUnverifiedSession = errors.New(
	"antex session identity is unverified; resume the intended conversation manually and save it with an updated Antex",
)

func (i *Integration) RestoreDecision(_ snapshot.Pane, meta map[string]string) (string, error) {
	sessionID := meta[metaSessionID]
	if meta["session_id_source"] != bindingSource || !validSessionID(sessionID) ||
		!filepath.IsAbs(meta["home"]) {
		return "", errUnverifiedSession
	}
	var argv []string
	if json.Unmarshal([]byte(meta["resume_argv"]), &argv) != nil || len(argv) < 3 ||
		argv[0] != commandName || argv[1] != "resume" || argv[2] != sessionID {
		return "", errUnverifiedSession
	}
	quoted := make([]string, 0, len(argv)+2)
	quoted = append(quoted, "env", quote("ANTEX_HOME="+meta["home"]))
	for _, arg := range argv {
		if strings.ContainsRune(arg, '\x00') || strings.ContainsAny(arg, "\r\n") {
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
