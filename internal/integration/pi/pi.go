package pi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/integration/agent"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	source      = "pi-jsonl-v1"
	sessionFlag = "--session"
)

var (
	errUnverified = errors.New(
		"pi session identity is unverified; session metadata was not found",
	)
	errInvalidHeader = errors.New("invalid pi session header")
)

type Integration struct {
	home        string
	sessionDir  string
	loadStatus  StatusLoader
	statusCache *statusSnapshot
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

	return &Integration{home: home, sessionDir: sessionDir, loadStatus: nil, statusCache: nil}
}
func (i *Integration) Name() string { return snapshot.AgentPi }
func (i *Integration) Scope() integration.Integration {
	scoped := New(i.home, i.sessionDir)
	scoped.loadStatus = i.loadStatus
	scoped.statusCache = &statusSnapshot{}

	return scoped
}

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
	value, ok := i.session(pane, metadata, false)
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

func (i *Integration) SessionID(p snapshot.Pane) (snapshot.AgentSession, bool) {
	return i.session(p, nil, false)
}

func (i *Integration) Session(
	pane snapshot.Pane, metadata map[string]string,
) (snapshot.AgentSession, bool) {
	return i.session(pane, metadata, true)
}

//nolint:cyclop
func (i *Integration) session(
	pane snapshot.Pane, metadata map[string]string, history bool,
) (snapshot.AgentSession, bool) {
	if !i.Matches(pane) || noSession(pane.CurrentCmd) || noSession(pane.RestoreCmd) ||
		nonInteractive(pane.ProcessArgv) {
		return snapshot.AgentSession{}, false
	}
	file := sessionFileFromArgv(pane.ProcessArgv)
	if file == "" && metadata != nil {
		file = metadata["session_file"]
	}
	if file == "" {
		file = pane.Meta["pi.session_file"]
	}
	if file == "" {
		file = sessionFileFromCommand(pane.RestoreCmd)
	}

	if file == "" {
		return snapshot.AgentSession{}, false
	}
	header, err := readSessionHeader(file)
	if err != nil || header.ID == "" || header.CWD == "" ||
		!sameDirectory(header.CWD, pane.CurrentPath) {
		return snapshot.AgentSession{}, false
	}
	if !sessionIntentMatches(pane, metadata, file, header) {
		return snapshot.AgentSession{}, false
	}
	model := ""
	if history {
		var current sessionHeader
		current, model, err = readSession(file)
		if err != nil || current != header {
			return snapshot.AgentSession{}, false
		}
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
	decoder := json.NewDecoder(f)
	var header sessionHeader
	model := ""
	first := true
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if err != nil {
			if errors.Is(err, io.EOF) || (!first && errors.Is(err, io.ErrUnexpectedEOF)) {
				break
			}

			return sessionHeader{}, "", fmt.Errorf("read pi session: %w", err)
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

	return header, model, nil
}

// An open JSONL in a descendant is not proof of the interactive client's
// identity: tools and RPC children can open unrelated histories. Only explicit
// arguments, saved intent or the ownership-verified tmux binding select a file.
func sessionIntentMatches(
	pane snapshot.Pane,
	metadata map[string]string,
	file string,
	header sessionHeader,
) bool {
	for _, expected := range []string{
		pane.LiveStatusID, pane.Meta["pi.session_id"], metadata["session_id"],
	} {
		if expected != "" && expected != header.ID {
			return false
		}
	}
	if metadata["session_file"] != "" && !sameDirectory(metadata["session_file"], file) {
		return false
	}
	for _, cwd := range []string{metadata["cwd"], pane.Meta["pi.cwd"]} {
		if cwd != "" && !sameDirectory(cwd, header.CWD) {
			return false
		}
	}
	if pane.Agent != nil &&
		(agent.Valid(*pane.Agent) != nil || pane.Agent.Kind != snapshot.AgentPi ||
			pane.Agent.ID != header.ID || !sameDirectory(pane.Agent.CWD, header.CWD) ||
			!sameDirectory(pane.Agent.Argv[3], file)) {
		return false
	}

	return true
}

// VerifySessionFile checks identity without decoding potentially large history
// records. An empty ID supports legacy version-1 identity-only bindings.
func VerifySessionFile(path, id, cwd string) bool {
	header, err := readSessionHeader(path)

	return err == nil && header.ID != "" && (id == "" || header.ID == id) &&
		header.CWD != "" && sameDirectory(header.CWD, cwd)
}

func readSessionHeader(path string) (sessionHeader, error) {
	file, err := os.Open(path) //nolint:gosec // explicit Pi session identity
	if err != nil {
		return sessionHeader{}, fmt.Errorf("open pi session: %w", err)
	}
	defer func() { _ = file.Close() }()
	var header sessionHeader
	err = json.NewDecoder(file).Decode(&header)
	if err != nil {
		return sessionHeader{}, fmt.Errorf("read pi session header: %w", err)
	}
	if header.Type != "session" {
		return sessionHeader{}, errInvalidHeader
	}

	return header, nil
}

func sameDirectory(first, second string) bool {
	if filepath.Clean(first) == filepath.Clean(second) {
		return true
	}
	left, err := filepath.EvalSymlinks(first)
	if err != nil {
		return false
	}
	right, err := filepath.EvalSymlinks(second)

	return err == nil && left == right
}

func nonInteractive(argv []string) bool {
	for index, arg := range argv {
		if slices.Contains([]string{"--no-session", "--print", "-p"}, arg) {
			return true
		}
		if strings.HasPrefix(arg, "--mode=") && arg != "--mode=text" {
			return true
		}
		if arg == "--mode" && index+1 < len(argv) && argv[index+1] != "text" {
			return true
		}
	}

	return false
}
