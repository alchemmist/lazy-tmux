package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

const (
	indexFileName      = "index.json"
	sessionsDirName    = "sessions"
	scrollbackDir      = "scrollback"
	defaultDirPerm     = 0o755
	defaultFilePerm    = 0o644
	scrollbackDirPerm  = 0o700
	scrollbackFilePerm = 0o600
)

type Store struct {
	baseDir string
	mu      sync.Mutex
}

func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func DefaultDataDir() string {
	if v := strings.TrimSpace(os.Getenv("LAZY_TMUX_DATA_DIR")); v != "" {
		return v
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".lazy-tmux"
	}

	return filepath.Join(home, ".local", "share", "lazy-tmux")
}

func (s *Store) SaveSession(sessionSnapshot snapshot.SessionSnapshot) error {
	if sessionSnapshot.SessionName == "" {
		return errors.New("empty session name")
	}

	if sessionSnapshot.CapturedAt.IsZero() {
		sessionSnapshot.CapturedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLayout(); err != nil {
		return err
	}

	safeName, entries, err := s.planScrollbackUnlocked(&sessionSnapshot)
	if err != nil {
		return err
	}

	path := s.sessionPath(sessionSnapshot.SessionName)

	jsonTmp, err := writeJSONTemp(path, sessionSnapshot, defaultFilePerm)
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(jsonTmp) }()

	if err := s.persistScrollbackUnlocked(
		sessionSnapshot.SessionName,
		safeName,
		entries,
	); err != nil {
		return err
	}

	if err := os.Rename(jsonTmp, path); err != nil {
		return fmt.Errorf("rename tmp file: %w", err)
	}

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return err
	}

	panes := 0
	for _, w := range sessionSnapshot.Windows {
		panes += len(w.Panes)
	}

	idx.Sessions[sessionSnapshot.SessionName] = snapshot.Record{
		SessionName:  sessionSnapshot.SessionName,
		File:         path,
		CapturedAt:   sessionSnapshot.CapturedAt.UTC(),
		LastAccessed: idx.Sessions[sessionSnapshot.SessionName].LastAccessed,
		Windows:      len(sessionSnapshot.Windows),
		Panes:        panes,
	}
	idx.Updated = time.Now().UTC()

	return writeJSONAtomic(s.indexPath(), idx)
}

func (s *Store) DeleteSession(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty session name")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove session file: %w", err)
	}

	safeName, err := safeScrollbackSessionName(name)
	if err != nil {
		return err
	}

	scrollRoot := filepath.Clean(filepath.Join(s.baseDir, scrollbackDir))
	sessionDir := filepath.Clean(filepath.Join(scrollRoot, safeName))

	if err := ensureUnderDir(scrollRoot, sessionDir, name); err != nil {
		return err
	}

	if err := os.RemoveAll(sessionDir); err != nil {
		return fmt.Errorf("remove scrollback dir: %w", err)
	}

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return err
	}

	if idx.Sessions != nil {
		delete(idx.Sessions, name)
	}

	idx.Updated = time.Now().UTC()

	return writeJSONAtomic(s.indexPath(), idx)
}

func (s *Store) LoadSession(name string) (snapshot.SessionSnapshot, error) {
	var out snapshot.SessionSnapshot

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)

	b, err := os.ReadFile(path)
	if err != nil {
		return out, fmt.Errorf("read session file: %w", err)
	}

	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("unmarshal session: %w", err)
	}

	if err := s.hydrateScrollback(&out); err != nil {
		return out, err
	}

	return out, nil
}

func (s *Store) SessionPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("empty session name")
	}

	return s.sessionPath(name), nil
}

func (s *Store) SessionExists(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("empty session name")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("stat session file: %w", err)
	}

	return true, nil
}

func (s *Store) ListRecords() ([]snapshot.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}

		return nil, err
	}

	records := make([]snapshot.Record, 0, len(idx.Sessions))
	for _, r := range idx.Sessions {
		records = append(records, r)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].CapturedAt.Equal(records[j].CapturedAt) {
			return records[i].SessionName < records[j].SessionName
		}

		return records[i].CapturedAt.After(records[j].CapturedAt)
	})

	return records, nil
}

func (s *Store) LatestRecord() (snapshot.Record, error) {
	recs, err := s.ListRecords()
	if err != nil {
		return snapshot.Record{}, err
	}

	if len(recs) == 0 {
		return snapshot.Record{}, os.ErrNotExist
	}

	return recs[0], nil
}

func (s *Store) MarkSessionAccessed(name string, accessTime time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty session name")
	}

	if accessTime.IsZero() {
		accessTime = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return err
	}

	rec, ok := idx.Sessions[name]
	if !ok {
		return os.ErrNotExist
	}

	rec.LastAccessed = accessTime.UTC()
	idx.Sessions[name] = rec
	idx.Updated = time.Now().UTC()

	return writeJSONAtomic(s.indexPath(), idx)
}

func (s *Store) ensureLayout() error {
	if err := os.MkdirAll(filepath.Join(s.baseDir, sessionsDirName), defaultDirPerm); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(s.baseDir, scrollbackDir), scrollbackDirPerm); err != nil {
		return fmt.Errorf("create scrollback dir: %w", err)
	}

	return nil
}

func (s *Store) loadIndexUnlocked() (snapshot.Index, error) {
	p := s.indexPath()

	fileContent, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot.Index{
				Version:  snapshot.FormatVersion,
				Updated:  time.Now().UTC(),
				Sessions: map[string]snapshot.Record{},
			}, nil
		}

		return snapshot.Index{}, fmt.Errorf("read index file: %w", err)
	}

	var idx snapshot.Index
	if err := json.Unmarshal(fileContent, &idx); err != nil {
		return snapshot.Index{}, fmt.Errorf("decode index: %w", err)
	}

	if idx.Sessions == nil {
		idx.Sessions = map[string]snapshot.Record{}
	}

	if idx.Version == 0 {
		idx.Version = snapshot.FormatVersion
	}

	return idx, nil
}

func (s *Store) indexPath() string {
	return filepath.Join(s.baseDir, indexFileName)
}

func (s *Store) sessionPath(name string) string {
	return filepath.Join(s.baseDir, sessionsDirName, sanitizeName(name)+".json")
}

func sanitizeName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")

	out := replacer.Replace(strings.TrimSpace(name))
	if out == "" {
		return "session"
	}

	return out
}

type scrollbackEntry struct {
	FileName string
	Content  string
	Ref      string
	Bytes    int
	Lines    int
}

func (s *Store) planScrollbackUnlocked(
	sessionSnapshot *snapshot.SessionSnapshot,
) (string, []scrollbackEntry, error) {
	safeName, err := safeScrollbackSessionName(sessionSnapshot.SessionName)
	if err != nil {
		return "", nil, err
	}

	entries := make([]scrollbackEntry, 0)

	for windowIndex := range sessionSnapshot.Windows {
		for pi := range sessionSnapshot.Windows[windowIndex].Panes {
			pane := &sessionSnapshot.Windows[windowIndex].Panes[pi]
			if pane.Scrollback == nil {
				continue
			}

			content := pane.Scrollback.Content
			if strings.TrimSpace(content) == "" {
				pane.Scrollback = nil
				continue
			}

			fileName := fmt.Sprintf(
				"w%d_p%d.log",
				sessionSnapshot.Windows[windowIndex].Index,
				pane.Index,
			)
			ref := filepath.Join(scrollbackDir, safeName, fileName)
			pane.Scrollback.Ref = ref
			pane.Scrollback.Bytes = len(content)
			pane.Scrollback.Lines = countLines(content)
			pane.Scrollback.Content = ""

			entries = append(entries, scrollbackEntry{
				FileName: fileName,
				Content:  content,
				Ref:      ref,
				Bytes:    len(content),
				Lines:    countLines(content),
			})
		}
	}

	return safeName, entries, nil
}

func (s *Store) persistScrollbackUnlocked(
	sessionName, safeName string,
	entries []scrollbackEntry,
) error {
	scrollRoot := filepath.Clean(filepath.Join(s.baseDir, scrollbackDir))
	sessionDir := filepath.Clean(filepath.Join(scrollRoot, safeName))

	if err := ensureUnderDir(scrollRoot, sessionDir, sessionName); err != nil {
		return err
	}

	stageDir := sessionDir + ".tmp"
	_ = os.RemoveAll(stageDir)

	defer func() { _ = os.RemoveAll(stageDir) }()

	if len(entries) == 0 {
		_ = os.RemoveAll(sessionDir)
		_ = os.RemoveAll(stageDir)

		return nil
	}

	if err := os.MkdirAll(stageDir, scrollbackDirPerm); err != nil {
		return fmt.Errorf("create stage dir: %w", err)
	}

	for _, ent := range entries {
		path := filepath.Join(stageDir, ent.FileName)
		if err := os.WriteFile(path, []byte(ent.Content), scrollbackFilePerm); err != nil {
			return fmt.Errorf("write scrollback file: %w", err)
		}
	}

	if err := promoteScrollbackStage(sessionDir, stageDir); err != nil {
		return err
	}

	return nil
}

func promoteScrollbackStage(sessionDir, stageDir string) error {
	backupDir := sessionDir + ".bak"
	_ = os.RemoveAll(backupDir)

	hadSessionDir := false
	if _, err := os.Stat(sessionDir); err == nil {
		hadSessionDir = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat session dir: %w", err)
	}

	if hadSessionDir {
		if err := os.Rename(sessionDir, backupDir); err != nil {
			return fmt.Errorf("backup session dir: %w", err)
		}
	}

	if err := os.Rename(stageDir, sessionDir); err != nil {
		if hadSessionDir {
			_ = os.Rename(backupDir, sessionDir)
		}

		return fmt.Errorf("promote stage dir: %w", err)
	}

	if hadSessionDir {
		_ = os.RemoveAll(backupDir)
	}

	return nil
}

func (s *Store) hydrateScrollback(sessionSnapshot *snapshot.SessionSnapshot) error {
	baseRoot, err := filepath.Abs(filepath.Clean(filepath.Join(s.baseDir, scrollbackDir)))
	if err != nil {
		return fmt.Errorf("get base root: %w", err)
	}

	for wi := range sessionSnapshot.Windows {
		for pi := range sessionSnapshot.Windows[wi].Panes {
			pane := &sessionSnapshot.Windows[wi].Panes[pi]
			if pane.Scrollback == nil || strings.TrimSpace(pane.Scrollback.Ref) == "" {
				continue
			}

			path, err := safeScrollbackPath(baseRoot, s.baseDir, pane.Scrollback.Ref)
			if err != nil {
				return err
			}

			fileContent, err := os.ReadFile(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}

				return fmt.Errorf("read scrollback file: %w", err)
			}

			pane.Scrollback.Content = string(fileContent)
			if pane.Scrollback.Bytes == 0 {
				pane.Scrollback.Bytes = len(fileContent)
			}

			if pane.Scrollback.Lines == 0 {
				pane.Scrollback.Lines = countLines(string(fileContent))
			}
		}
	}

	return nil
}

func safeScrollbackPath(baseRoot, baseDir, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty scrollback ref")
	}

	candidate := filepath.Clean(filepath.Join(baseDir, ref))

	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}

	baseEval, err := filepath.EvalSymlinks(baseRoot)
	if err != nil {
		baseEval = baseRoot
	}

	candidateDirEval, err := filepath.EvalSymlinks(filepath.Dir(candidateAbs))
	if err != nil {
		candidateDirEval = filepath.Dir(candidateAbs)
	}

	candidateEval := filepath.Join(candidateDirEval, filepath.Base(candidateAbs))

	finalEval, err := filepath.EvalSymlinks(candidateEval)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			finalEval = candidateEval
		} else {
			return "", fmt.Errorf("eval symlinks: %w", err)
		}
	}

	rel, err := filepath.Rel(baseEval, finalEval)
	if err != nil {
		return "", fmt.Errorf("get relative path: %w", err)
	}

	if rel == "." {
		return finalEval, nil
	}

	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) || cleanRel == ".." ||
		strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid scrollback ref outside base dir: %s", ref)
	}

	return finalEval, nil
}

func safeScrollbackSessionName(sessionName string) (string, error) {
	name := sanitizeName(sessionName)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid session name for scrollback: %q", sessionName)
	}

	if strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("invalid session name for scrollback: %q", sessionName)
	}

	if filepath.Separator != '/' && strings.Contains(name, "/") {
		return "", fmt.Errorf("invalid session name for scrollback: %q", sessionName)
	}

	if filepath.Separator != '\\' && strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid session name for scrollback: %q", sessionName)
	}

	return name, nil
}

func ensureUnderDir(baseDir, child, ref string) error {
	baseAbs, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return fmt.Errorf("get base absolute path: %w", err)
	}

	childAbs, err := filepath.Abs(filepath.Clean(child))
	if err != nil {
		return fmt.Errorf("get child absolute path: %w", err)
	}

	rel, err := filepath.Rel(baseAbs, childAbs)
	if err != nil {
		return fmt.Errorf("get relative path: %w", err)
	}

	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) || cleanRel == ".." ||
		strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("invalid path outside base dir: %s", ref)
	}

	return nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}

	return strings.Count(s, "\n") + 1
}

func writeJSONAtomic(path string, v any) error {
	tmp, err := writeJSONTemp(path, v, defaultFilePerm)
	if err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp file: %w", err)
	}

	return nil
}

func writeJSONTemp(path string, v any, perm os.FileMode) (string, error) {
	jsonData, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(jsonData, '\n'), perm); err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}

	return tmp, nil
}
