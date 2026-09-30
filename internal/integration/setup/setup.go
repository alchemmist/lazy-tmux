package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"

	"github.com/alchemmist/lazy-tmux/internal/atomicfile"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
)

const (
	marker      = " --integration-token lazy-tmux-v1"
	commandType = "command"
)

var (
	errAgentKind = errors.New("setup supports codex and claude")
	errHookShape = errors.New("invalid hook configuration")
)

type Document map[string]json.RawMessage

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}
type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func location(kind string) (string, error) {
	switch kind {
	case "codex":
		return "hooks.json", nil
	case "claude":
		return "settings.json", nil
	default:
		return "", errAgentKind
	}
}

func Apply(kind, home, binary string, uninstall bool) (bool, error) {
	name, err := location(kind)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(binary) {
		return false, errHookShape
	}
	err = os.MkdirAll(home, 0o700)
	if err != nil {
		return false, fmt.Errorf("create hook directory: %w", err)
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return false, fmt.Errorf("open hook directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	lock, err := root.OpenFile(".lazy-tmux-hooks.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, fmt.Errorf("open hook lock: %w", err)
	}
	defer func() { _ = lock.Close() }()
	err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX)
	if err != nil {
		return false, fmt.Errorf("lock hooks: %w", err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	data, err := root.ReadFile(name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read hook configuration: %w", err)
	}
	updated, err := merge(data, kind, home, binary, uninstall)
	if err != nil {
		return false, err
	}
	if bytes.Equal(data, updated) {
		return false, nil
	}
	err = writeConfig(root, name, data, updated)
	if err != nil {
		return false, err
	}

	return true, nil
}

func merge(data []byte, kind, home, binary string, uninstall bool) ([]byte, error) {
	document, err := readDocument(data)
	if err != nil {
		return nil, err
	}
	hooks := map[string][]json.RawMessage{}
	if raw, ok := document["hooks"]; ok {
		err := json.Unmarshal(raw, &hooks)
		if err != nil {
			return nil, fmt.Errorf("decode hooks: %w", err)
		}
	}
	changed, err := removeOwned(hooks)
	if err != nil {
		return nil, err
	}
	if !uninstall {
		entry, err := json.Marshal(
			hookGroup{
				Matcher: "",
				Hooks:   []hookCommand{{Type: commandType, Command: hookLine(kind, home, binary)}},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("encode hook: %w", err)
		}
		for _, event := range events(kind) {
			hooks[event] = append(hooks[event], entry)
		}
		changed = true
	}
	if !changed {
		return data, nil
	}
	raw, err := json.Marshal(hooks)
	if err != nil {
		return nil, fmt.Errorf("encode hooks: %w", err)
	}
	if len(hooks) > 0 {
		document["hooks"] = raw
	} else {
		delete(document, "hooks")
	}
	result, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("format hooks: %w", err)
	}
	result = append(result, '\n')
	var before, after any
	_ = json.Unmarshal(data, &before)
	_ = json.Unmarshal(result, &after)
	if reflect.DeepEqual(before, after) {
		return data, nil
	}

	return result, nil
}

func removeOwned(hooks map[string][]json.RawMessage) (bool, error) {
	changed := false
	for event, groups := range hooks {
		kept := make([]json.RawMessage, 0, len(groups))
		for _, raw := range groups {
			filtered, removed, err := filterOwnedGroup(raw)
			if err != nil {
				return false, err
			}
			changed = changed || removed
			if len(filtered) > 0 {
				kept = append(kept, filtered)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}

	return changed, nil
}

func writeConfig(root *os.Root, name string, old, data []byte) error {
	if len(old) > 0 {
		err := atomicfile.Backup(root, name+".lazy-tmux.bak", old)
		if err != nil {
			return fmt.Errorf("backup hooks: %w", err)
		}
	}
	temporary := name + ".lazy-tmux.tmp"
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("stage hooks: %w", err)
	}
	defer func() { _ = file.Close(); _ = root.Remove(temporary) }()
	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("write hooks: %w", err)
	}
	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync hooks: %w", err)
	}
	err = file.Close()
	if err != nil {
		return fmt.Errorf("close hooks: %w", err)
	}
	err = root.Rename(temporary, name)
	if err != nil {
		return fmt.Errorf("publish hooks: %w", err)
	}

	return nil
}

func events(kind string) []string {
	result := []string{
		"SessionStart",
		"SessionEnd",
		"UserPromptSubmit",
		"PreToolUse",
		"PostToolUse",
		"PermissionRequest",
		"Stop",
	}
	if kind == "codex" {
		return append(result, "Interrupt")
	}

	return append(result, "Notification", "PostToolUseFailure")
}

func hookLine(kind, home, binary string) string {
	return agent.Quote(
		binary,
	) + " hook agent --agent " + kind + " --home " + agent.Quote(
		home,
	) + marker
}

func Check(kind, home, binary string) (bool, error) {
	name, err := location(kind)
	if err != nil {
		return false, err
	}
	root, err := os.OpenRoot(home)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open hook directory: %w", err)
	}
	defer func() { _ = root.Close() }()
	data, err := root.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read hooks: %w", err)
	}
	var doc struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	err = json.Unmarshal(data, &doc)
	if err != nil {
		return false, fmt.Errorf("decode hooks: %w", err)
	}
	for _, event := range events(kind) {
		found := false
		for _, group := range doc.Hooks[event] {
			for _, hook := range group.Hooks {
				if hook.Type == commandType && hook.Command == hookLine(kind, home, binary) {
					found = true
				}
			}
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}

func hooksNull(document Document) bool { return string(document["hooks"]) == "null" }

func readDocument(data []byte) (Document, error) {
	document := Document{}
	if len(data) > 0 {
		err := json.Unmarshal(data, &document)
		if err != nil {
			return nil, fmt.Errorf("decode hook configuration: %w", err)
		}
	}
	if document == nil || hooksNull(document) {
		return nil, errHookShape
	}

	return document, nil
}

func filterOwnedGroup(raw json.RawMessage) (json.RawMessage, bool, error) {
	var document Document
	err := json.Unmarshal(raw, &document)
	if err != nil {
		return nil, false, fmt.Errorf("invalid hook group: %w", err)
	}
	var hooks []json.RawMessage
	err = json.Unmarshal(document["hooks"], &hooks)
	if err != nil {
		return nil, false, fmt.Errorf("invalid hook list: %w", err)
	}
	kept := make([]json.RawMessage, 0, len(hooks))
	for _, entry := range hooks {
		var hook hookCommand
		err := json.Unmarshal(entry, &hook)
		if err != nil {
			return nil, false, fmt.Errorf("invalid hook entry: %w", err)
		}
		if hook.Type != commandType || !ownedCommand(hook.Command) {
			kept = append(kept, entry)
		}
	}
	if len(kept) == len(hooks) {
		return raw, false, nil
	}
	if len(kept) == 0 {
		return nil, true, nil
	}
	updated, err := json.Marshal(kept)
	if err != nil {
		return nil, false, fmt.Errorf("encode retained hooks: %w", err)
	}
	document["hooks"] = updated
	group, err := json.Marshal(document)
	if err != nil {
		return nil, false, fmt.Errorf("encode retained group: %w", err)
	}

	return group, true, nil
}

func ownedCommand(command string) bool {
	if strings.HasSuffix(command, marker) && strings.Contains(command, " hook agent --agent ") {
		return true
	}
	fields := strings.Fields(command)
	if len(fields) != 5 || filepath.Base(strings.Trim(fields[0], "'\"")) != "lazy-tmux" {
		return false
	}
	if fields[1] != "hook" || fields[2] != "claude-status" || fields[3] != "--state" {
		return false
	}

	return fields[4] == "working" || fields[4] == "idle" || fields[4] == "awaiting_input" ||
		fields[4] == "awaiting_decision"
}
