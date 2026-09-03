package claude

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alchemmist/lazy-tmux/internal/integration"
	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	metaSessionID = "session_id"
	transcriptExt = ".jsonl"
)

type Integration struct {
	home        string
	statusDir   string
	index       *projectIndexState
	scopeMu     sync.Mutex
	validations map[string]*sync.Once
}

type projectIndexState struct {
	indexMu          sync.Mutex
	projectIndex     map[string]projectCache
	indexBuilds      int
	validationChecks int
}

type projectCache struct {
	fingerprint uint64
	sessionID   string
}

func New(home, statusDir string) *Integration {
	return &Integration{
		home:      home,
		statusDir: statusDir,
		index: &projectIndexState{
			indexMu:          sync.Mutex{},
			projectIndex:     make(map[string]projectCache),
			indexBuilds:      0,
			validationChecks: 0,
		},
		scopeMu:     sync.Mutex{},
		validations: nil,
	}
}

func (i *Integration) Scope() integration.Integration {
	return &Integration{
		home:        i.home,
		statusDir:   i.statusDir,
		index:       i.index,
		scopeMu:     sync.Mutex{},
		validations: make(map[string]*sync.Once),
	}
}

func (i *Integration) Name() string { return "claude" }

func (i *Integration) Matches(pane snapshot.Pane) bool {
	for _, cmd := range []string{pane.RestoreCmd, pane.CurrentCmd} {
		cmd = strings.ToLower(strings.TrimSpace(cmd))
		if cmd == "" {
			continue
		}

		if executableName(cmd) == "claude" || strings.Contains(cmd, "claude") {
			return true
		}
	}

	return false
}

func (i *Integration) Capture(pane snapshot.Pane) (map[string]string, error) {
	sessionID, ok := i.latestSessionID(pane.CurrentPath)
	if !ok {
		return map[string]string{}, nil
	}

	return map[string]string{metaSessionID: sessionID}, nil
}

func (i *Integration) RestoreCommand(_ snapshot.Pane, meta map[string]string) string {
	id := strings.TrimSpace(meta[metaSessionID])
	if id == "" {
		return ""
	}

	return "claude --resume " + id
}

func (i *Integration) latestSessionID(cwd string) (string, bool) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || strings.TrimSpace(i.home) == "" {
		return "", false
	}

	if i.validations != nil {
		i.scopeMu.Lock()
		once, ok := i.validations[cwd]
		if !ok {
			once = &sync.Once{}
			i.validations[cwd] = once
		}
		i.scopeMu.Unlock()
		once.Do(func() { i.refreshProject(cwd) })

		return i.cachedProject(cwd)
	}
	i.refreshProject(cwd)

	return i.cachedProject(cwd)
}

func (i *Integration) refreshProject(cwd string) {
	dir := filepath.Join(i.home, "projects", EncodeProjectDir(cwd))
	i.index.indexMu.Lock()
	defer i.index.indexMu.Unlock()
	i.index.validationChecks++

	entries, err := os.ReadDir(dir)
	if err != nil {
		delete(i.index.projectIndex, cwd)

		return
	}

	var (
		newestID   string
		newestTime int64
		found      bool
	)
	hash := fnv.New64a()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), transcriptExt) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(
			hash,
			"%s\x00%d\x00%d\x00",
			entry.Name(),
			info.Size(),
			info.ModTime().UnixNano(),
		)

		if mod := info.ModTime().UnixNano(); !found || mod > newestTime {
			newestTime = mod
			newestID = strings.TrimSuffix(entry.Name(), transcriptExt)
			found = true
		}
	}
	fingerprint := hash.Sum64()
	if cached, ok := i.index.projectIndex[cwd]; ok && cached.fingerprint == fingerprint {
		return
	}

	i.index.projectIndex[cwd] = projectCache{fingerprint: fingerprint, sessionID: newestID}
	i.index.indexBuilds++
}

func (i *Integration) cachedProject(cwd string) (string, bool) {
	i.index.indexMu.Lock()
	defer i.index.indexMu.Unlock()
	cached, ok := i.index.projectIndex[cwd]

	return cached.sessionID, ok && cached.sessionID != ""
}

func EncodeProjectDir(cwd string) string {
	replacer := strings.NewReplacer("/", "-", ".", "-")

	return replacer.Replace(cwd)
}

func executableName(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}

	base := filepath.Base(fields[0])

	return strings.TrimPrefix(base, "-")
}
