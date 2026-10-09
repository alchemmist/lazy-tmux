package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const claudeResume = "--resume"

const (
	BindingSource = "binding-v1"
	HookSource    = "hook-v1"
	ManualSource  = "manual-v1"
	PendingKey    = "agent.restore_pending"
)

var ErrUnverified = errors.New(
	"agent identity is unverified; configure lifecycle hooks or explicitly repair the saved pane",
)

func Kind(command string) string { return integration.CommandAgent(command) }

func PaneKind(pane snapshot.Pane) string {
	if kind := Kind(pane.CurrentCmd); kind != "" {
		return kind
	}
	if kind := Kind(pane.RestoreCmd); kind != "" {
		return kind
	}
	if pane.Agent != nil {
		return pane.Agent.Kind
	}

	return ""
}

func Matches(pane snapshot.Pane, kind string) bool { return PaneKind(pane) == kind }

func Session(pane snapshot.Pane, kind string) (snapshot.AgentSession, bool) {
	if !Matches(pane, kind) {
		return snapshot.AgentSession{}, false
	}
	if pane.Agent != nil {
		value := *pane.Agent

		return value, value.Kind == kind && Valid(value) == nil
	}
	if kind != snapshot.AgentAntex || pane.Meta["antex.session_id_source"] != BindingSource {
		return snapshot.AgentSession{}, false
	}
	var argv []string
	if json.Unmarshal([]byte(pane.Meta["antex.resume_argv"]), &argv) != nil {
		return snapshot.AgentSession{}, false
	}
	cwd := pane.Meta["antex.cwd"]
	if cwd == "" {
		cwd = pane.CurrentPath
	}
	value := snapshot.AgentSession{
		Version: 1,
		Kind:    kind,
		ID:      pane.Meta[snapshot.AntexSessionIDMetaKey],
		Home:    pane.Meta["antex.home"],
		CWD:     cwd,
		Argv:    argv,
		Source:  BindingSource,
		Status:  "",
	}

	return value, Valid(value) == nil
}

func Valid(value snapshot.AgentSession) error {
	if value.Version != 1 || Kind(value.Kind) != value.Kind || value.Kind == "" ||
		!validID(value.ID) {
		return ErrUnverified
	}
	if !absolutePath(value.Home) || (value.CWD != "" && !absolutePath(value.CWD)) {
		return ErrUnverified
	}
	if !slices.Contains([]string{BindingSource, HookSource, ManualSource}, value.Source) {
		return ErrUnverified
	}
	if !validArgv(value) {
		return ErrUnverified
	}

	return nil
}

func validID(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") &&
		strings.IndexFunc(value, func(char rune) bool {
			return !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '-' && char != '_'
		}) < 0
}

func absolutePath(value string) bool {
	return filepath.IsAbs(value) && strings.IndexFunc(value, unicode.IsControl) < 0
}

func validArgv(value snapshot.AgentSession) bool {
	if len(value.Argv) < 3 || filepath.Base(value.Argv[0]) != value.Kind ||
		value.Argv[2] != value.ID {
		return false
	}
	subcommand := "resume"
	if value.Kind == snapshot.AgentClaude {
		subcommand = claudeResume
	}
	if value.Argv[1] != subcommand {
		return false
	}
	for _, arg := range value.Argv {
		if strings.IndexFunc(arg, unicode.IsControl) >= 0 {
			return false
		}
	}

	return true
}

type RestorePlan struct {
	Argv []string
	Env  []string
	CWD  string
}

func Plan(pane snapshot.Pane, kind string) (RestorePlan, error) {
	value, ok := Session(pane, kind)
	if !ok {
		return RestorePlan{}, fmt.Errorf("%s: %w", kind, ErrUnverified)
	}
	argv := slices.Clone(value.Argv)
	if kind == snapshot.AgentAntex && !slices.ContainsFunc(argv[3:], func(arg string) bool {
		return arg == "--cd" || arg == "-C" || strings.HasPrefix(arg, "--cd=") ||
			arg == `tui.resume_cwd="session"`
	}) {
		argv = append(argv, "-c", `tui.resume_cwd="session"`)
	}
	env := map[string]string{
		snapshot.AgentAntex:  "ANTEX_HOME",
		snapshot.AgentCodex:  "CODEX_HOME",
		snapshot.AgentClaude: "CLAUDE_CONFIG_DIR",
	}[kind]

	return RestorePlan{Argv: argv, Env: []string{env + "=" + value.Home}, CWD: value.CWD}, nil
}

func Restore(pane snapshot.Pane, kind string) (string, error) {
	plan, err := Plan(pane, kind)
	if err != nil {
		return "", err
	}
	words := []string{"env"}
	for _, value := range plan.Env {
		words = append(words, Quote(value))
	}
	for _, value := range plan.Argv {
		words = append(words, Quote(value))
	}

	return strings.Join(words, " "), nil
}

func Quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func Capture(pane snapshot.Pane, kind string) map[string]string {
	value, ok := Session(pane, kind)
	if !ok {
		return nil
	}

	return map[string]string{"session_id": value.ID, "session_id_source": value.Source}
}

func Status(pane snapshot.Pane, kind string) (integration.Status, bool) {
	value, ok := Session(pane, kind)
	if !ok || value.Source == ManualSource || pane.Meta[PendingKey] != "" ||
		pane.Meta[kind+".restore_pending"] != "" {
		return integration.StatusUnknown, false
	}
	states := map[string]integration.Status{
		"working":           integration.StatusWorking,
		"idle":              integration.StatusIdle,
		"awaiting_input":    integration.StatusAwaitingInput,
		"awaiting_decision": integration.StatusAwaitingDecision,
		"error":             integration.StatusError,
	}
	state, ok := states[value.Status]

	return state, ok
}
