package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/config"
)

// claudeHookCommandMarker identifies the hook commands this tool installs, so
// install is idempotent and uninstall is surgical.
const claudeHookCommandMarker = "hook claude-status"

// claudeHookSpec is one hook lazy-tmux installs into Claude Code's settings.
type claudeHookSpec struct {
	event   string
	matcher string // "" for events without a matcher
	state   string
}

func claudeHookSpecs() []claudeHookSpec {
	return []claudeHookSpec{
		{event: "Notification", matcher: "permission_prompt", state: "awaiting_decision"},
		{event: "Notification", matcher: "idle_prompt", state: "awaiting_input"},
		{event: "UserPromptSubmit", matcher: "", state: "working"},
		{event: "Stop", matcher: "", state: "idle"},
	}
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

func runClaudeHooks(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(cmdClaudeHooks, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	uninstall := flags.Bool("uninstall", false, "remove the hooks instead of installing them")

	err := flags.Parse(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			claudeHooksHelp(stdout)

			return 0
		}

		writeErr(stderr, fmt.Errorf("parse flags: %w", err))

		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		writeErr(stderr, fmt.Errorf("load config: %w", err))

		return 1
	}

	path := filepath.Join(config.ExpandHome(cfg.Integrations.Claude.Home), "settings.json")

	binary, err := os.Executable()
	if err != nil || strings.TrimSpace(binary) == "" {
		binary = "lazy-tmux"
	}

	changed, err := applyClaudeHooks(path, binary, *uninstall)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	action := "installed"
	if *uninstall {
		action = "uninstalled"
	}

	if !changed {
		action = "already up to date —"
	}

	_, _ = fmt.Fprintf(stdout, "Claude status hooks %s (%s)\n", action, path)

	return 0
}

// mergeClaudeHookSpecs rewrites each of our hook events inside hooks in place:
// existing lazy-tmux groups (matched by marker) are dropped, and unless
// uninstalling a fresh group pointing at binary is appended. User-owned groups
// under the same events are never touched.
func mergeClaudeHookSpecs(hooks map[string][]json.RawMessage, binary string, uninstall bool) error {
	for _, spec := range claudeHookSpecs() {
		command := fmt.Sprintf("%s hook claude-status --state %s", binary, spec.state)
		marker := fmt.Sprintf("%s --state %s", claudeHookCommandMarker, spec.state)

		kept := dropOurGroups(hooks[spec.event], marker)

		if !uninstall {
			group := hookGroup{
				Matcher: spec.matcher,
				Hooks:   []hookEntry{{Type: "command", Command: command}},
			}

			raw, err := json.Marshal(group)
			if err != nil {
				return fmt.Errorf("marshal hook group: %w", err)
			}

			kept = append(kept, raw)
		}

		if len(kept) == 0 {
			delete(hooks, spec.event)
		} else {
			hooks[spec.event] = kept
		}
	}

	return nil
}

// applyClaudeHooks merges (or removes) lazy-tmux's status hooks in the Claude
// settings.json at path, preserving every other key and the user's own hooks. It
// backs the file up before writing and is idempotent. Returns whether the file
// changed.
func applyClaudeHooks(path, binary string, uninstall bool) (bool, error) {
	root, original, err := readSettings(path)
	if err != nil {
		return false, err
	}

	// Nothing to remove from a Claude install that has no settings file yet.
	if uninstall && len(original) == 0 {
		return false, nil
	}

	hooks := map[string][]json.RawMessage{}

	if raw, ok := root["hooks"]; ok {
		err := json.Unmarshal(raw, &hooks)
		if err != nil {
			return false, fmt.Errorf("parse hooks in %s: %w", path, err)
		}
	}

	err = mergeClaudeHookSpecs(hooks, binary, uninstall)
	if err != nil {
		return false, err
	}

	if len(hooks) == 0 {
		delete(root, "hooks")
	} else {
		raw, err := json.Marshal(hooks)
		if err != nil {
			return false, fmt.Errorf("marshal hooks: %w", err)
		}

		root["hooks"] = raw
	}

	updated, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal settings: %w", err)
	}

	updated = append(updated, '\n')

	if string(updated) == string(original) {
		return false, nil
	}

	if len(original) > 0 {
		err := os.WriteFile(path+".lazy-tmux.bak", original, 0o600)
		if err != nil {
			return false, fmt.Errorf("back up %s: %w", path, err)
		}
	}

	err = os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		return false, fmt.Errorf("create config dir: %w", err)
	}

	err = os.WriteFile(path, updated, 0o600)
	if err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}

	return true, nil
}

// dropOurGroups removes only lazy-tmux-owned hook entries (those whose command
// contains marker), preserving any other commands a user added to the same
// matcher group. A group left empty afterwards is dropped. Groups that don't
// parse round-trip untouched.
func dropOurGroups(groups []json.RawMessage, marker string) []json.RawMessage {
	kept := make([]json.RawMessage, 0, len(groups))

	for _, raw := range groups {
		var group hookGroup

		err := json.Unmarshal(raw, &group)
		if err != nil {
			kept = append(kept, raw)

			continue
		}

		filtered := group.Hooks[:0]

		for _, entry := range group.Hooks {
			if !strings.Contains(entry.Command, marker) {
				filtered = append(filtered, entry)
			}
		}

		group.Hooks = filtered
		if len(group.Hooks) == 0 {
			continue
		}

		pruned, err := json.Marshal(group)
		if err != nil {
			return kept
		}

		kept = append(kept, pruned)
	}

	return kept
}

// readSettings reads path as a top-level JSON object preserving key order-free
// fidelity (unknown keys round-trip). A missing file yields an empty object.
func readSettings(path string) (map[string]json.RawMessage, []byte, error) {
	data, err := os.ReadFile(
		path,
	) // #nosec G304 -- fixed settings.json path under the user's own Claude home
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]json.RawMessage{}, nil, nil
		}

		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	root := map[string]json.RawMessage{}

	if len(strings.TrimSpace(string(data))) > 0 {
		err := json.Unmarshal(data, &root)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	return root, data, nil
}

func claudeHooksHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux claude-hooks [--uninstall]

Install (or remove) Claude Code hooks that report each session's live status to
lazy-tmux, so the TUI picker can show a colored status dot per Claude window.
Merges into ~/.claude/settings.json, preserving your existing hooks (a .bak is
written first).
`)
}
