package tmux

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var (
	errHookPane  = errors.New("invalid hook pane")
	errHookOwner = errors.New("cannot prove hook ownership")
)

const agentBindingOption = "@lazy_tmux_agent_binding"

type LiveBinding struct {
	Version    int                   `json:"version"`
	Session    snapshot.AgentSession `json:"session"`
	PID        int                   `json:"pid"`
	StartedAt  string                `json:"started_at"`
	PaneID     string                `json:"pane_id"`
	SocketPath string                `json:"socket_path"`
	Generation int64                 `json:"generation"`
	Closed     bool                  `json:"closed"`
}

func verifiedAgentBinding(
	raw string,
	panePID int,
	paneID, socket string,
	processes processSnapshot,
	started func(int) string,
) *snapshot.AgentSession {
	var binding LiveBinding
	if json.Unmarshal([]byte(raw), &binding) != nil || binding.Version != 1 || binding.Closed ||
		binding.PaneID != paneID ||
		binding.SocketPath != socket ||
		binding.StartedAt == "" ||
		agent.Valid(binding.Session) != nil ||
		binding.Session.Source != agent.HookSource {
		return nil
	}
	owner, ok := processes.processes[binding.PID]
	if !ok || agent.Kind(owner.cmd) != binding.Session.Kind || !strings.Contains(owner.stat, "+") ||
		strings.ContainsAny(owner.stat, "TZ") ||
		started(binding.PID) != binding.StartedAt {
		return nil
	}
	if !processes.ownsAgentPane(binding.PID, panePID) {
		return nil
	}

	return &binding.Session
}

func (processes processSnapshot) ownsAgentPane(ownerPID, panePID int) bool {
	seen := make(map[int]bool)
	for pid := ownerPID; pid > 0 && !seen[pid]; {
		seen[pid] = true
		process, ok := processes.processes[pid]
		if !ok || (pid != ownerPID && agent.Kind(process.cmd) != "") {
			return false
		}
		if pid == panePID {
			return true
		}
		pid = process.ppid
	}

	return false
}

func typedAntexSession(pane snapshot.Pane) *snapshot.AgentSession {
	value, ok := agent.Session(pane, snapshot.AgentAntex)
	if !ok {
		return nil
	}

	return &value
}

func (client *Client) HookOwner(paneID, kind string) (LiveBinding, error) {
	out, err := client.Output(
		"display-message",
		"-p",
		"-t",
		paneID,
		"#{pane_pid}|#{pane_id}|#{socket_path}",
	)
	if err != nil {
		return LiveBinding{}, err
	}
	fields := strings.Split(strings.TrimSpace(out), "|")
	if len(fields) != 3 || fields[1] != paneID {
		return LiveBinding{}, fmt.Errorf("%w: %s", errHookPane, paneID)
	}
	rootPID, err := strconv.Atoi(fields[0])
	if err != nil {
		return LiveBinding{}, fmt.Errorf("parse pane pid: %w", err)
	}
	processes, err := loadProcessSnapshot()
	if err != nil {
		return LiveBinding{}, err
	}
	pid, ok := processes.hookOwnerProcess(os.Getppid(), rootPID, kind)
	if ok {
		start := processStartTime(pid)
		if start != "" {
			return LiveBinding{
				Version:    1,
				Session:    snapshot.AgentSession{},
				PID:        pid,
				StartedAt:  start,
				PaneID:     paneID,
				SocketPath: fields[2],
				Generation: 0,
				Closed:     false,
			}, nil
		}
	}

	return LiveBinding{}, fmt.Errorf("%w: %s in pane %s", errHookOwner, kind, paneID)
}

func attachAgentBindings(
	pane snapshot.Pane,
	raw, antexRaw string,
	panePID int,
	paneID, socket string,
	processes processSnapshot,
) snapshot.Pane {
	pane.Meta = validatedAntexMeta(antexRaw, panePID, paneID, socket, processes, processStartTime)
	pane.Agent = verifiedAgentBinding(raw, panePID, paneID, socket, processes, processStartTime)
	if pane.Agent == nil {
		pane.Agent = typedAntexSession(pane)
	}
	if pane.Agent != nil && pane.Agent.CWD != "" {
		pane.CurrentPath = pane.Agent.CWD
	}

	return pane
}

func (processes processSnapshot) hookOwnerProcess(parentPID, panePID int, kind string) (int, bool) {
	seen := make(map[int]bool)
	for pid := parentPID; pid > 1 && !seen[pid]; {
		seen[pid] = true
		process, ok := processes.processes[pid]
		if !ok {
			return 0, false
		}
		found := agent.Kind(process.cmd)
		if found != "" {
			fields := strings.Fields(process.cmd)
			helper := found == kind && len(fields) > 1 && fields[1] == "app-server" &&
				kind == snapshot.AgentCodex
			if !helper {
				return pid, found == kind && processes.ownsAgentPane(pid, panePID)
			}
		}
		pid = process.ppid
	}

	return 0, false
}
