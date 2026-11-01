package tmux

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/integration/pi"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type piBinding struct {
	PID       int    `json:"pid"`
	File      string `json:"file"`
	Version   int    `json:"version"`
	StartedAt string `json:"started_at"`
	SessionID string `json:"session_id"`
	State     string `json:"state"`
}

func (processes processSnapshot) attachPiBinding(pane snapshot.Pane, encoded string) snapshot.Pane {
	var binding piBinding
	if json.Unmarshal([]byte(encoded), &binding) != nil || !binding.valid() {
		return pane
	}
	candidates := processes.descendants(pane.ProcessPID)
	if root, ok := processes.processes[pane.ProcessPID]; ok {
		candidates = append(candidates, root)
	}
	for _, process := range candidates {
		if process.pid != binding.PID || !piForegroundOwner(process) {
			continue
		}
		argv, err := processArgv(process.pid)
		if err != nil || agent.LaunchKind(argv) != "pi" ||
			processStartTime(binding.PID) != binding.StartedAt {
			return pane
		}
		// Keep authoritative owner identity even when the file was replaced.
		// Otherwise Session could fall back to argv and adopt the changed ID.
		pane.LiveStatusID = binding.SessionID
		if binding.File != "" &&
			!pi.VerifySessionFile(binding.File, binding.SessionID, pane.CurrentPath) {
			return pane
		}
		if !piRestoreIntentMatches(pane, binding.File) {
			return pane
		}

		return applyPiBinding(pane, binding)
	}

	return pane
}

func (binding piBinding) valid() bool {
	return binding.Version == 1 && binding.StartedAt != "" &&
		(binding.File == "" || filepath.IsAbs(binding.File))
}

func applyPiBinding(pane snapshot.Pane, binding piBinding) snapshot.Pane {
	if pane.Meta == nil {
		pane.Meta = make(map[string]string)
	}
	delete(pane.Meta, agent.PendingKey)
	delete(pane.Meta, "pi.restore_pending")
	delete(pane.Meta, "pi.session_id")
	delete(pane.Meta, "pi.cwd")
	delete(pane.Meta, "pi.model")
	pane.Agent = nil
	pane.LiveStatus = ""
	pane.LiveStatusID = ""
	if binding.File == "" {
		delete(pane.Meta, "pi.session_file")
		pane.ProcessArgv = []string{"pi", "--no-session"}
		pane.RestoreCmd = "pi --no-session"

		return pane
	}
	pane.Meta["pi.session_file"] = binding.File
	pane.LiveStatus = binding.State
	pane.LiveStatusID = binding.SessionID
	pane.ProcessArgv = []string{"pi", "--session", binding.File}
	pane.RestoreCmd = "pi"

	return pane
}

func piRestoreIntentMatches(pane snapshot.Pane, file string) bool {
	if pane.Meta[agent.PendingKey] == "" && pane.Meta["pi.restore_pending"] == "" {
		return true
	}
	// A live client does not acknowledge a different saved restore intent.
	if file == "" || (pane.Meta["pi.session_file"] != "" && pane.Meta["pi.session_file"] != file) {
		return false
	}
	if pane.Agent != nil && (len(pane.Agent.Argv) < 4 || pane.Agent.Argv[3] != file ||
		!pi.VerifySessionFile(file, pane.Agent.ID, pane.Agent.CWD)) {
		return false
	}

	return pi.VerifySessionFile(file, pane.Meta["pi.session_id"], pane.CurrentPath)
}

func piForegroundOwner(process psProcess) bool {
	return strings.Contains(process.stat, "+") && !strings.ContainsAny(process.stat, "TZ")
}

const piStatusFields = 5

// CapturePiPanes takes one process snapshot per picker refresh, independent of
// stored snapshots and autosave cadence. attachPiBinding performs owner checks.
func (client *Client) CapturePiPanes() ([]snapshot.Pane, error) {
	output, err := client.Output(
		"list-panes",
		"-a",
		"-F",
		"#{pane_pid}|#{pane_current_command}|#{pane_current_path}|#{@lazy_tmux_pi_binding}|#{pane_id}",
	)
	if err != nil {
		return nil, err
	}
	processes, err := loadProcessSnapshot()
	if err != nil {
		return nil, err
	}
	var panes []snapshot.Pane
	seen := make(map[string]bool)
	for _, line := range splitLines(output) {
		fields := splitFieldsN(line, piStatusFields)
		if len(fields) != piStatusFields || fields[3] == "" {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pane := processes.attachPiBinding(snapshot.Pane{
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
		}, fields[3])
		if pane.Meta["pi.session_file"] != "" && !seen[fields[4]] {
			seen[fields[4]] = true
			panes = append(panes, pane)
		}
	}

	return panes, nil
}
