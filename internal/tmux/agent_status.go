package tmux

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const agentStatusFields = 9

// CaptureAgentPanes captures current bindings from this tmux server, independent
// of autosave. One pane listing and process tree are shared by all non-Pi agents.
func (client *Client) CaptureAgentPanes() ([]snapshot.Pane, error) {
	output, err := client.Output(
		"list-panes",
		"-a",
		"-F",
		"#{pane_pid}|#{pane_current_command}|#{pane_current_path}|#{pane_id}|#{socket_path}|"+
			"#{@lazy_tmux_agent_binding}|#{@antex_binding}|#{@lazy_tmux_restore_pending}|"+
			"#{?@lazy_tmux_restore_ack,#{@lazy_tmux_restore_ack},#{@antex_restore_ack}}",
	)
	if err != nil {
		return nil, err
	}
	processes, starts, err := loadAgentStatusProcesses()
	if err != nil {
		return nil, err
	}

	return captureAgentStatus(output, processes, func(pid int) string { return starts[pid] }), nil
}

func captureAgentStatus(
	output string,
	processes processSnapshot,
	started func(int) string,
) []snapshot.Pane {
	var panes []snapshot.Pane
	seen := make(map[string]bool)
	for _, line := range splitLines(output) {
		fields := splitFieldsN(line, agentStatusFields)
		if len(fields) != agentStatusFields {
			continue
		}
		if fields[7] != "" {
			intent, valid := decodeRestoreIntent(fields[7])
			if !valid || intent.Token != fields[8] {
				continue
			}
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pane := snapshot.Pane{
			ProcessPID:   pid,
			CurrentCmd:   fields[1],
			CurrentPath:  fields[2],
			Index:        0,
			RestoreCmd:   "",
			Scrollback:   nil,
			IsActive:     false,
			ProcessArgv:  nil,
			LiveStatus:   "",
			LiveStatusID: "",
			Meta:         nil,
			Agent:        nil,
		}
		pane = liveAgentPane(pane, fields[3], fields[4], fields[5], fields[6], processes, started)
		key := fields[4] + "/" + fields[3]
		if pane.Agent != nil && !seen[key] {
			seen[key] = true
			panes = append(panes, pane)
		}
	}

	return panes
}

func liveAgentPane(pane snapshot.Pane, paneID, socket, raw, legacy string,
	processes processSnapshot, started func(int) string,
) snapshot.Pane {
	if raw != "" {
		pane.Agent = verifiedAgentBinding(raw, pane.ProcessPID, paneID, socket, processes, started)
	}
	if pane.Agent == nil && legacyFallbackAllowed(raw, processes, started) {
		var binding antexBinding
		if json.Unmarshal([]byte(legacy), &binding) != nil ||
			!processes.ownsAgentPane(binding.PID, pane.ProcessPID) {
			return pane
		}
		pane.Meta = validatedAntexMeta(legacy, pane.ProcessPID, paneID, socket, processes, started)
		pane.Agent = typedAntexSession(pane)
	}
	if pane.Agent != nil && (pane.Agent.Kind == snapshot.AgentPi ||
		(agent.Kind(pane.CurrentCmd) != "" && agent.Kind(pane.CurrentCmd) != pane.Agent.Kind)) {
		pane.Agent = nil
	}

	return pane
}

// A dead prior client must not shadow a new legacy owner. A still-live typed
// owner (including a closed same-generation record) remains authoritative.
func legacyFallbackAllowed(raw string, processes processSnapshot, started func(int) string) bool {
	if raw == "" {
		return true
	}
	var binding LiveBinding
	if json.Unmarshal([]byte(raw), &binding) != nil || binding.PID <= 0 || binding.StartedAt == "" {
		return false
	}
	_, exists := processes.processes[binding.PID]

	return !exists || started(binding.PID) != binding.StartedAt
}

// Capture ancestry, foreground state and PID birth identity in the same ps read.
// Calling processStartTime separately for every pane would rescan all processes
// once per client and could mix observations from different process generations.
func loadAgentStatusProcesses() (processSnapshot, map[int]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"ps",
		"-ax",
		"-o",
		"pid=",
		"-o",
		"ppid=",
		"-o",
		"stat=",
		"-o",
		"lstart=",
		"-o",
		"command=",
	)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	output, err := cmd.Output()
	if err != nil {
		return processSnapshot{}, nil, fmt.Errorf("capture live agent processes: %w", err)
	}
	processes, starts := parseAgentStatusProcesses(string(output))

	return processes, starts, nil
}

func parseAgentStatusProcesses(output string) (processSnapshot, map[int]string) {
	const commandField = 8
	const startField = 3
	var lines []string
	starts := make(map[int]string)
	for _, line := range splitLines(output) {
		fields := strings.Fields(line)
		if len(fields) <= commandField {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		starts[pid] = strings.Join(fields[startField:commandField], " ")
		lines = append(
			lines,
			strings.Join(fields[:startField], " ")+" "+strings.Join(fields[commandField:], " "),
		)
	}

	return newProcessSnapshot(lines), starts
}
