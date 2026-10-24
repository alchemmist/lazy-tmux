package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	source      = "pi-jsonl-v1"
	sessionFlag = "--session"
	jsonlSuffix = ".jsonl"
)

var (
	errUnverified = errors.New(
		"pi session identity is unverified; session metadata was not found",
	)
	errInvalidHeader = errors.New("invalid pi session header")
)

type Integration struct {
	home       string
	sessionDir string
}
type sessionHeader struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	CWD  string `json:"cwd"`
}

func New(home, sessionDir string) *Integration {
	if home == "" {
		home = os.Getenv("PI_CODING_AGENT_DIR")
	}
	if sessionDir == "" {
		sessionDir = os.Getenv("PI_CODING_AGENT_SESSION_DIR")
	}
	if sessionDir == "" {
		sessionDir = filepath.Join(home, "sessions")
	}

	return &Integration{home: home, sessionDir: sessionDir}
}
func (i *Integration) Name() string                   { return snapshot.AgentPi }
func (i *Integration) Scope() integration.Integration { return i }
func (i *Integration) Matches(p snapshot.Pane) bool {
	return agent.PaneKind(p) == snapshot.AgentPi ||
		agent.LaunchKind(p.ProcessArgv) == snapshot.AgentPi
}

func (i *Integration) Capture(p snapshot.Pane) (map[string]string, error) {
	value, ok := i.Session(p, nil)
	if !ok {
		return nil, errUnverified
	}

	return map[string]string{
		"session_id": value.ID, "session_file": value.Argv[3],
		"cwd": value.CWD, "model": value.Status,
	}, nil
}

func (i *Integration) RestoreCommand(p snapshot.Pane, m map[string]string) string {
	v, _ := i.RestoreDecision(p, m)

	return v
}

func (i *Integration) RestoreDecision(
	pane snapshot.Pane,
	metadata map[string]string,
) (string, error) {
	if file := metadata["session_file"]; file != "" {
		_, err := os.Stat(file)
		if err != nil {
			return "", fmt.Errorf(
				"pi session file unavailable (%s): %w; manual review required", file, err,
			)
		}
	}
	value, ok := i.Session(pane, metadata)
	if !ok {
		return "", errUnverified
	}
	_, err := os.Stat(value.Argv[3])
	if err != nil {
		return "", fmt.Errorf(
			"pi session file unavailable (%s): %w; manual review required", value.Argv[3], err,
		)
	}
	words := []string{
		"cd", agent.Quote(value.CWD), "&&", "env",
		agent.Quote("PI_CODING_AGENT_DIR=" + value.Home),
		agent.Quote("PI_CODING_AGENT_SESSION_DIR=" + i.sessionDir),
		agent.Quote("pi"), agent.Quote(sessionFlag), agent.Quote(value.Argv[3]),
	}

	return strings.Join(words, " "), nil
}

func (i *Integration) Status(p snapshot.Pane) (integration.Status, bool) {
	if _, ok := i.Session(p, nil); !ok {
		return integration.StatusUnknown, false
	}

	return integration.StatusUnknown, false
}

func (i *Integration) SessionID(p snapshot.Pane) (snapshot.AgentSession, bool) {
	return i.Session(p, nil)
}

//nolint:cyclop
func (i *Integration) Session(
	pane snapshot.Pane, metadata map[string]string,
) (snapshot.AgentSession, bool) {
	if !i.Matches(pane) || noSession(pane.CurrentCmd) || noSession(pane.RestoreCmd) {
		return snapshot.AgentSession{}, false
	}
	file := ""
	if metadata != nil {
		file = metadata["session_file"]
	}
	if file == "" {
		file = pane.Meta["pi.session_file"]
	}
	if file == "" {
		file = sessionFileFromCommand(pane.RestoreCmd)
	}
	if file == "" {
		file = sessionFileFromArgv(pane.ProcessArgv)
	}
	if file == "" && pane.ProcessPID > 0 {
		file = i.openSessionFile(pane.ProcessPID)
	}
	if file == "" {
		file = i.uniqueFile(pane.CurrentPath)
	}
	if file == "" {
		return snapshot.AgentSession{}, false
	}
	header, model, err := readSession(file)
	if err != nil || header.ID == "" || header.CWD == "" ||
		filepath.Clean(header.CWD) != filepath.Clean(pane.CurrentPath) {
		return snapshot.AgentSession{}, false
	}
	if metadata != nil && metadata["session_id"] != "" && metadata["session_id"] != header.ID {
		return snapshot.AgentSession{}, false
	}
	value := snapshot.AgentSession{
		Version: 1, Kind: snapshot.AgentPi, ID: header.ID, Home: i.home, CWD: header.CWD,
		Argv: []string{"pi", sessionFlag, header.ID, file}, Source: source, Status: model,
	}

	return value, agent.Valid(value) == nil
}

func noSession(command string) bool {
	return slices.Contains(strings.Fields(command), "--no-session")
}

func sessionFileFromCommand(command string) string {
	fields := strings.Fields(command)
	for index, field := range fields {
		if field == sessionFlag && index+1 < len(fields) {
			return strings.Trim(fields[index+1], "'\"")
		}
		if value, ok := strings.CutPrefix(field, sessionFlag+"="); ok {
			return value
		}
	}

	return ""
}

func sessionFileFromArgv(argv []string) string {
	for index, field := range argv {
		if field == sessionFlag && index+1 < len(argv) {
			return argv[index+1]
		}
		if value, ok := strings.CutPrefix(field, sessionFlag+"="); ok {
			return value
		}
	}

	return ""
}

func readSession(path string) (sessionHeader, string, error) {
	f, err := os.Open(path) //nolint:gosec // path comes from Pi's session directory
	if err != nil {
		return sessionHeader{}, "", fmt.Errorf("open pi session: %w", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	var header sessionHeader
	model := ""
	first := true
	for scanner.Scan() {
		var record map[string]any
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if first {
			first = false
			data, marshalErr := json.Marshal(record)
			if marshalErr != nil {
				return sessionHeader{}, "", fmt.Errorf("encode pi session header: %w", marshalErr)
			}
			unmarshalErr := json.Unmarshal(data, &header)
			if unmarshalErr != nil {
				return sessionHeader{}, "", fmt.Errorf("decode pi session header: %w", unmarshalErr)
			}
			if header.Type != "session" {
				return sessionHeader{}, "", errInvalidHeader
			}
		}
		if record["type"] == "model_change" {
			provider, _ := record["provider"].(string)
			modelID, _ := record["modelId"].(string)
			if provider != "" || modelID != "" {
				model = strings.Trim(provider+"/"+modelID, "/")
			}
		}
	}
	err = scanner.Err()
	if err != nil {
		return sessionHeader{}, "", fmt.Errorf("read pi session: %w", err)
	}

	return header, model, nil
}

func (i *Integration) openSessionFile(pid int) string {
	for _, candidate := range processTree(pid) {
		//nolint:gosec // fixed executable and numeric PID
		out, err := exec.CommandContext(
			context.Background(), "lsof", "-p", strconv.Itoa(candidate), "-Fn",
		).Output()
		if err == nil {
			for line := range strings.SplitSeq(string(out), "\n") {
				path := strings.TrimPrefix(line, "n")
				if isSessionPath(path, i.sessionDir) {
					_, statErr := os.Stat(path)
					if statErr == nil {
						return path
					}
				}
			}
		}
	}
	for _, candidate := range processTree(pid) {
		entry := fmt.Sprintf("/proc/%d/fd", candidate)
		fdEntries, readErr := os.ReadDir(entry)
		if readErr != nil {
			continue
		}
		for _, fd := range fdEntries {
			path, evalErr := filepath.EvalSymlinks(filepath.Join(entry, fd.Name()))
			if evalErr == nil && filepath.Ext(path) == ".jsonl" && within(path, i.sessionDir) {
				return path
			}
		}
	}

	return ""
}

func processTree(root int) []int {
	children := make(map[int][]int)
	out, err := exec.CommandContext(context.Background(), "ps", "-axo", "pid=,ppid=").Output()
	if err == nil {
		for line := range strings.SplitSeq(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			child, childErr := strconv.Atoi(fields[0])
			parent, parentErr := strconv.Atoi(fields[1])
			if childErr == nil && parentErr == nil {
				children[parent] = append(children[parent], child)
			}
		}
	}
	result := []int{root}
	for index := 0; index < len(result); index++ {
		result = append(result, children[result[index]]...)
	}

	return result
}

func (i *Integration) uniqueFile(cwd string) string {
	var found string
	count := 0
	var anyFound string
	anyCount := 0
	_ = filepath.WalkDir(i.sessionDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != jsonlSuffix {
			return nil
		}
		h, _, err := readSession(path)
		if err == nil {
			anyFound = path
			anyCount++
			if filepath.Clean(h.CWD) == filepath.Clean(cwd) {
				found = path
				count++
			}
		}

		return nil
	})
	if count == 1 {
		return found
	}
	if count == 0 && anyCount == 1 {
		return anyFound
	}

	return ""
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isSessionPath(path, root string) bool {
	return filepath.Ext(path) == jsonlSuffix && within(path, root)
}
