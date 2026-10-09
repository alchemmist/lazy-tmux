package tmux

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	antexCommand       = "antex"
	antexBindingSource = "binding-v1"
)

type antexBinding struct {
	Version          int      `json:"version"`
	ThreadID         string   `json:"thread_id"`
	PID              int      `json:"pid"`
	ProcessStartedAt string   `json:"process_started_at"`
	PaneID           string   `json:"pane_id"`
	SocketPath       string   `json:"socket_path"`
	Home             string   `json:"home"`
	CWD              string   `json:"cwd,omitempty"`
	ResumeArgv       []string `json:"resume_argv"`
}

func validatedAntexMeta(
	raw string,
	panePID int,
	paneID, socketPath string,
	processes processSnapshot,
	startedAt func(int) string,
) map[string]string {
	var binding antexBinding
	if json.Unmarshal([]byte(raw), &binding) != nil || binding.Version != 1 ||
		binding.PaneID != paneID || binding.SocketPath != socketPath ||
		binding.PID <= 0 || binding.ProcessStartedAt == "" || !filepath.IsAbs(binding.Home) {
		return nil
	}
	if binding.CWD != "" && (!filepath.IsAbs(binding.CWD) || strings.IndexFunc(binding.CWD, unicode.IsControl) >= 0) {
		return nil
	}
	owner, exists := processes.processes[binding.PID]
	if !exists || executableName(owner.cmd) != antexCommand ||
		strings.ContainsAny(owner.stat, "TZ") ||
		!strings.Contains(owner.stat, "+") {
		return nil
	}
	if !processes.ownsAntexPane(binding.PID, panePID) ||
		startedAt(binding.PID) != binding.ProcessStartedAt {
		return nil
	}
	argv, err := json.Marshal(binding.ResumeArgv)
	if err != nil {
		return nil
	}

	meta := map[string]string{
		snapshot.AntexSessionIDMetaKey: binding.ThreadID,
		"antex.session_id_source":      antexBindingSource,
		"antex.home":                   binding.Home,
		"antex.resume_argv":            string(argv),
	}
	if binding.CWD != "" {
		meta["antex.cwd"] = binding.CWD
	}

	return meta
}

func (processes processSnapshot) ownsAntexPane(ownerPID, panePID int) bool {
	seen := make(map[int]bool)
	for pid := ownerPID; pid > 0 && !seen[pid]; {
		seen[pid] = true
		process, exists := processes.processes[pid]
		if !exists || (pid != ownerPID && executableName(process.cmd) == antexCommand) {
			return false
		}
		if pid == panePID {
			return true
		}
		pid = process.ppid
	}

	return false
}

func processStartTime(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-ax", "-o", "pid=", "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && fields[0] == strconv.Itoa(pid) {
			return strings.Join(fields[1:], " ")
		}
	}

	return ""
}
