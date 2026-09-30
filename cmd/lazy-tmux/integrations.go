package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/config"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/setup"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
	"github.com/alchemmist/lazy-tmux/internal/store"
	"github.com/alchemmist/lazy-tmux/internal/tmux"
)

const maxHookInput = 1024 * 1024

var (
	errIntegrationSubcommand = errors.New("unknown integrations subcommand")
	errIntegrationKind       = errors.New("specify codex or claude")
	errRepairTarget          = errors.New("specify session, window and pane")
)

func agentHome(cfg config.Config, kind string) string {
	switch kind {
	case snapshot.AgentCodex:
		return config.ExpandHome(cfg.Integrations.Codex.Home)
	case snapshot.AgentAntex:
		return config.ExpandHome(cfg.Integrations.Antex.Home)
	case snapshot.AgentClaude:
		return config.ExpandHome(cfg.Integrations.Claude.Home)
	default:
		return ""
	}
}

func integrationsHelp(w io.Writer) {
	_, _ = fmt.Fprint(w, `Usage: lazy-tmux integrations <setup|doctor|repair>
  setup <codex|claude> [--uninstall]  install owned lifecycle hooks; client trust remains explicit
  doctor                           report installed hooks and live pane verification
  repair --session NAME --window N --pane N --agent NAME --sessionID ID [--home PATH] [--apply]
                                   preview a saved pane repair; --apply saves it with a backup
`)
}

func runIntegrations(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" {
		integrationsHelp(stdout)

		return 0
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	var err error
	switch args[0] {
	case "setup":
		err = setupIntegration(args[1:], cfg, stdout)
	case "doctor":
		err = doctorIntegrations(cfg, stdout)
	case "repair":
		err = repairIntegration(args[1:], cfg, stdout)
	default:
		err = errIntegrationSubcommand
	}
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	return 0
}

func setupIntegration(args []string, cfg config.Config, stdout io.Writer) error {
	if len(args) == 0 {
		return errIntegrationKind
	}
	kind := args[0]
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	uninstall := flags.Bool("uninstall", false, "remove owned hooks")
	err := flags.Parse(args[1:])
	if err != nil {
		return fmt.Errorf("parse setup: %w", err)
	}
	if flags.NArg() != 0 {
		return errUnexpectedArguments
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	changed, err := setup.Apply(kind, agentHome(cfg, kind), binary, *uninstall)
	if err != nil {
		return fmt.Errorf("configure integration: %w", err)
	}
	_, _ = fmt.Fprintf(
		stdout,
		"%s hooks: changed=%t removed=%t; review and trust hooks in the client before use\n",
		kind,
		changed,
		*uninstall,
	)

	return nil
}

func doctorIntegrations(cfg config.Config, stdout io.Writer) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	for _, kind := range []string{snapshot.AgentCodex, snapshot.AgentAntex, snapshot.AgentClaude} {
		executable, lookErr := exec.LookPath(kind)
		installed := false
		if kind != snapshot.AgentAntex {
			installed, err = setup.Check(kind, agentHome(cfg, kind), binary)
			if err != nil {
				return fmt.Errorf("inspect %s: %w", kind, err)
			}
		}
		_, _ = fmt.Fprintf(
			stdout,
			"%s: executable=%q available=%t hooks_installed=%t version=%q; "+
				"live binding required; compatibility/trust not certified\n",
			kind,
			executable,
			lookErr == nil,
			installed,
			probeClientVersion(kind),
		)
	}
	client := tmux.NewClient(cfg.TmuxBin)
	output, err := client.Output("list-panes", "-a", "-F", "#{pane_id}")
	if err != nil {
		return fmt.Errorf("integration operation: %w", err)
	}
	for sessionID := range strings.FieldsSeq(output) {
		pane, err := client.CapturePane(sessionID)
		if err != nil {
			return fmt.Errorf("integration operation: %w", err)
		}
		kind := agent.PaneKind(pane)
		if kind == "" {
			continue
		}
		value, verified := agent.Session(pane, kind)
		_, _ = fmt.Fprintf(
			stdout,
			"%s %s: verified=%t session=%q\n",
			sessionID,
			kind,
			verified,
			value.ID,
		)
	}

	return nil
}

func agentSessionHelp(w io.Writer) {
	_, _ = fmt.Fprintln(
		w,
		"Usage: lazy-tmux agent-session --agent <codex|antex|claude> [--pane TARGET]",
	)
}

func runAgentSession(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent-session", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("agent", "", "agent")
	pane := flags.String("pane", os.Getenv("TMUX_PANE"), "pane")
	parseErr := flags.Parse(args)
	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			agentSessionHelp(stdout)

			return 0
		}
		writeErr(stderr, parseErr)

		return 1
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	live, err := tmux.NewClient(cfg.TmuxBin).CapturePane(*pane)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}
	value, valid := agent.Session(live, *kind)
	if !valid {
		writeErr(stderr, agent.ErrUnverified)

		return 1
	}
	_, _ = fmt.Fprintln(stdout, value.ID)

	return 0
}

func runAgentHook(args []string, stdin io.Reader, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("agent", "", "agent")
	home := flags.String("home", "", "home")
	_ = flags.String("integration-token", "", "owned hook marker")
	err := flags.Parse(args)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}
	cfg, ok := loadConfig(stderr)
	if !ok {
		return 1
	}
	if *home == "" {
		*home = agentHome(cfg, *kind)
	}
	var event tmux.AgentHookEvent
	err = json.NewDecoder(io.LimitReader(stdin, maxHookInput)).Decode(&event)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}
	err = tmux.NewClient(cfg.TmuxBin).
		ApplyAgentHook(*kind, *home, os.Getenv("TMUX_PANE"), event)
	if err != nil {
		writeErr(stderr, err)

		return 1
	}

	return 0
}

func repairIntegration(args []string, cfg config.Config, stdout io.Writer) error {
	flags := flag.NewFlagSet("repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	session := flags.String("session", "", "session")
	window := flags.Int("window", -1, "window")
	pane := flags.Int("pane", -1, "pane")
	kind := flags.String("agent", "", "agent")
	sessionID := flags.String("id", "", "id")
	home := flags.String("home", "", "home")
	apply := flags.Bool("apply", false, "apply")
	parseErr := flags.Parse(args)
	if parseErr != nil {
		return fmt.Errorf("parse repair: %w", parseErr)
	}
	if flags.NArg() != 0 || *session == "" || *window < 0 || *pane < 0 {
		return errRepairTarget
	}
	if *home == "" {
		*home = agentHome(cfg, *kind)
	}
	storage := store.New(cfg.DataDir)
	state, err := storage.LoadSessionMetadata(*session)
	if err != nil {
		return fmt.Errorf("integration operation: %w", err)
	}
	cwd := ""
	for _, w := range state.Windows {
		if w.Index == *window {
			for _, p := range w.Panes {
				if p.Index == *pane {
					cwd = p.CurrentPath
				}
			}
		}
	}
	subcommand := "resume"
	if *kind == snapshot.AgentClaude {
		subcommand = "--resume"
	}
	value := snapshot.AgentSession{
		Version: 1,
		Kind:    *kind,
		ID:      *sessionID,
		Home:    *home,
		CWD:     cwd,
		Argv:    []string{*kind, subcommand, *sessionID},
		Source:  agent.ManualSource,
		Status:  "",
	}
	changed, err := storage.RepairAgent(*session, *window, *pane, value, *apply)
	if err != nil {
		return fmt.Errorf("integration operation: %w", err)
	}
	err = json.NewEncoder(stdout).Encode(changed)
	if err != nil {
		return fmt.Errorf("print repair: %w", err)
	}

	return nil
}

func probeClientVersion(kind string) string {
	const versionTimeout = 3 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	var command *exec.Cmd
	switch kind {
	case snapshot.AgentCodex:
		command = exec.CommandContext(ctx, "codex", "--version")
	case snapshot.AgentAntex:
		command = exec.CommandContext(ctx, "antex", "--version")
	case snapshot.AgentClaude:
		command = exec.CommandContext(ctx, "claude", "--version")
	default:
		return "unknown"
	}
	output, err := command.Output()
	if err != nil {
		return "unavailable"
	}

	return strings.Join(strings.Fields(string(output)), " ")
}
