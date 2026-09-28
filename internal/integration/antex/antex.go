package antex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	metaSessionID = "session_id"
	bindingSource = "binding-v1"
	commandName   = "antex"
)

type Integration struct {
	home       string
	index      *sessionIndex
	validation *sync.Once
}

type sessionIndex struct {
	indexMu          sync.Mutex
	indexed          bool
	indexBuilds      int
	validationChecks int
	sessionPaths     map[string]string
	fileStates       map[string]fileState
	dirStates        map[string]int64
}

type fileState struct {
	modTime int64
	size    int64
}

func New(home string) *Integration {
	return &Integration{
		home: home,
		index: &sessionIndex{
			indexMu:          sync.Mutex{},
			indexed:          false,
			indexBuilds:      0,
			validationChecks: 0,
			sessionPaths:     make(map[string]string),
			fileStates:       make(map[string]fileState),
			dirStates:        make(map[string]int64),
		},
		validation: nil,
	}
}

func (i *Integration) Scope() integration.Integration {
	return &Integration{home: i.home, index: i.index, validation: &sync.Once{}}
}

func (i *Integration) Name() string { return commandName }

func (i *Integration) Matches(pane snapshot.Pane) bool {
	for _, cmd := range []string{pane.RestoreCmd, pane.CurrentCmd} {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}

		if executableName(cmd) == commandName {
			return true
		}
	}

	return false
}

func (i *Integration) Capture(pane snapshot.Pane) (map[string]string, error) {
	sessionID, ok := i.SessionID(pane)
	if !ok {
		return map[string]string{}, nil
	}

	meta := map[string]string{metaSessionID: sessionID, "session_id_source": bindingSource}
	for _, key := range []string{"home", "resume_argv"} {
		meta[key] = pane.Meta["antex."+key]
	}

	return meta, nil
}

func (i *Integration) SessionID(pane snapshot.Pane) (string, bool) {
	if !i.Matches(pane) || pane.Meta["antex.session_id_source"] != bindingSource {
		return "", false
	}
	if sessionID := strings.TrimSpace(pane.Meta[snapshot.AntexSessionIDMetaKey]); sessionID != "" {
		return sessionID, true
	}

	return "", false
}

func (i *Integration) RestoreCommand(pane snapshot.Pane, meta map[string]string) string {
	command, _ := i.RestoreDecision(pane, meta)

	return command
}

type sessionMetaLine struct {
	Type    string `json:"type"`
	Payload struct {
		ID  string `json:"id"`
		CWD string `json:"cwd"`
	} `json:"payload"`
}

type sessionCandidate struct {
	id      string
	cwd     string
	path    string
	modTime int64
}

func (i *Integration) sessionPath(sessionID string) (string, bool) {
	i.ensureIndex(sessionID)

	i.index.indexMu.Lock()
	defer i.index.indexMu.Unlock()
	path, ok := i.index.sessionPaths[sessionID]

	return path, ok
}

func (i *Integration) ensureIndex(requiredID string) {
	if i.validation != nil {
		i.validation.Do(func() { i.ensureIndexFresh(requiredID) })

		return
	}
	i.ensureIndexFresh(requiredID)
}

func (i *Integration) ensureIndexFresh(requiredID string) {
	i.index.indexMu.Lock()
	defer i.index.indexMu.Unlock()
	i.index.validationChecks++
	if i.index.indexed && !i.indexStale() {
		if requiredID == "" || i.index.sessionPaths[requiredID] != "" {
			return
		}
	}

	root := filepath.Join(i.home, "sessions")
	paths := make(map[string]string)
	files := make(map[string]fileState)
	dirs := make(map[string]int64)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			info, infoErr := entry.Info()
			if infoErr == nil {
				dirs[path] = info.ModTime().UnixNano()
			}

			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr == nil {
			files[path] = fileState{modTime: info.ModTime().UnixNano(), size: info.Size()}
		}
		candidate, ok := readCandidate(path, "")
		if !ok {
			return nil
		}
		relative, relErr := filepath.Rel(i.home, path)
		if relErr != nil {
			return fmt.Errorf("make rollout path relative: %w", relErr)
		}
		candidate.path = filepath.ToSlash(relative)
		paths[candidate.id] = candidate.path

		return nil
	})
	if err == nil {
		i.index.sessionPaths = paths
		i.index.fileStates = files
		i.index.dirStates = dirs
		i.index.indexed = true
		i.index.indexBuilds++
	}
}

func (i *Integration) indexStale() bool {
	for path, state := range i.index.fileStates {
		info, err := os.Stat(path)
		if err != nil || info.ModTime().UnixNano() != state.modTime || info.Size() != state.size {
			return true
		}
	}
	for path, modTime := range i.index.dirStates {
		info, err := os.Stat(path)
		if err != nil || info.ModTime().UnixNano() != modTime {
			return true
		}
	}

	return false
}

func readCandidate(path, cwd string) (sessionCandidate, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return sessionCandidate{}, false
	}

	file, err := os.Open(path) // #nosec G304 -- path is discovered below the configured Antex home
	if err != nil {
		return sessionCandidate{}, false
	}
	defer func() { _ = file.Close() }()

	var meta sessionMetaLine
	err = json.NewDecoder(bufio.NewReader(file)).Decode(&meta)
	if err != nil {
		return sessionCandidate{}, false
	}
	if meta.Type != "session_meta" || strings.TrimSpace(meta.Payload.ID) == "" ||
		(cwd != "" && meta.Payload.CWD != cwd) {
		return sessionCandidate{}, false
	}

	return sessionCandidate{
		id:      meta.Payload.ID,
		cwd:     meta.Payload.CWD,
		path:    path,
		modTime: info.ModTime().UnixNano(),
	}, true
}

func executableName(cmd string) string {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return ""
	}

	base := filepath.Base(fields[0])

	return strings.TrimPrefix(base, "-")
}
