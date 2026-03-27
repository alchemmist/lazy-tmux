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

func (s *Store) SaveSession(ss snapshot.SessionSnapshot) error {
	if ss.SessionName == "" {
		return errors.New("empty session name")
	}

	if ss.CapturedAt.IsZero() {
		ss.CapturedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureLayout(); err != nil {
		return err
	}

	safeName, entries, err := s.planScrollbackUnlocked(&ss)
	if err != nil {
		return err
	}

	path := s.sessionPath(ss.SessionName)

	jsonTmp, err := writeJSONTemp(path, ss, defaultFilePerm)
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(jsonTmp) }()

	if err := s.persistScrollbackUnlocked(ss.SessionName, safeName, entries); err != nil {
		return err
	}

	if err := os.Rename(jsonTmp, path); err != nil {
		return err
	}

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return err
	}

	panes := 0
	for _, w := range ss.Windows {
		panes += len(w.Panes)
	}

	idx.Sessions[ss.SessionName] = snapshot.Record{
		SessionName:  ss.SessionName,
		File:         path,
		CapturedAt:   ss.CapturedAt.UTC(),
		LastAccessed: idx.Sessions[ss.SessionName].LastAccessed,
		Windows:      len(ss.Windows),
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
		return err
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
		return err
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
		return out, err
	}

	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
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

		return false, err
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

func (s *Store) MarkSessionAccessed(name string, at time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("empty session name")
	}

	if at.IsZero() {
		at = time.Now().UTC()
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

	rec.LastAccessed = at.UTC()
	idx.Sessions[name] = rec
	idx.Updated = time.Now().UTC()

	return writeJSONAtomic(s.indexPath(), idx)
}

func (s *Store) ensureLayout() error {
	if err := os.MkdirAll(filepath.Join(s.baseDir, sessionsDirName), defaultDirPerm); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(s.baseDir, scrollbackDir), scrollbackDirPerm); err != nil {
		return err
	}

	return nil
}

func (s *Store) loadIndexUnlocked() (snapshot.Index, error) {
	p := s.indexPath()

	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot.Index{Version: snapshot.FormatVersion, Updated: time.Now().UTC(), Sessions: map[string]snapshot.Record{}}, nil
		}

		return snapshot.Index{}, err
	}

	var idx snapshot.Index
	if err := json.Unmarshal(b, &idx); err != nil {
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

func (s *Store) planScrollbackUnlocked(ss *snapshot.SessionSnapshot) (string, []scrollbackEntry, error) {
	safeName, err := safeScrollbackSessionName(ss.SessionName)
	if err != nil {
		return "", nil, err
	}

	entries := make([]scrollbackEntry, 0)

	for wi := range ss.Windows {
		for pi := range ss.Windows[wi].Panes {
			pane := &ss.Windows[wi].Panes[pi]
			if pane.Scrollback == nil {
				continue
			}

			content := pane.Scrollback.Content
			if strings.TrimSpace(content) == "" {
				pane.Scrollback = nil
				continue
			}

			fileName := fmt.Sprintf("w%d_p%d.log", ss.Windows[wi].Index, pane.Index)
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

func (s *Store) persistScrollbackUnlocked(sessionName, safeName string, entries []scrollbackEntry) error {
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
		return err
	}

	for _, ent := range entries {
		path := filepath.Join(stageDir, ent.FileName)
		if err := os.WriteFile(path, []byte(ent.Content), scrollbackFilePerm); err != nil {
			return err
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
		return err
	}

	if hadSessionDir {
		if err := os.Rename(sessionDir, backupDir); err != nil {
			return err
		}
	}

	if err := os.Rename(stageDir, sessionDir); err != nil {
		if hadSessionDir {
			_ = os.Rename(backupDir, sessionDir)
		}

		return err
	}

	if hadSessionDir {
		_ = os.RemoveAll(backupDir)
	}

	return nil
}

func (s *Store) hydrateScrollback(ss *snapshot.SessionSnapshot) error {
	baseRoot, err := filepath.Abs(filepath.Clean(filepath.Join(s.baseDir, scrollbackDir)))
	if err != nil {
		return err
	}

	for wi := range ss.Windows {
		for pi := range ss.Windows[wi].Panes {
			pane := &ss.Windows[wi].Panes[pi]
			if pane.Scrollback == nil || strings.TrimSpace(pane.Scrollback.Ref) == "" {
				continue
			}

			path, err := safeScrollbackPath(baseRoot, s.baseDir, pane.Scrollback.Ref)
			if err != nil {
				return err
			}

			b, err := os.ReadFile(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}

				return err
			}

			pane.Scrollback.Content = string(b)
			if pane.Scrollback.Bytes == 0 {
				pane.Scrollback.Bytes = len(b)
			}

			if pane.Scrollback.Lines == 0 {
				pane.Scrollback.Lines = countLines(string(b))
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
		return "", err
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
			return "", err
		}
	}

	rel, err := filepath.Rel(baseEval, finalEval)
	if err != nil {
		return "", err
	}

	if rel == "." {
		return finalEval, nil
	}

	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
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
		return err
	}

	childAbs, err := filepath.Abs(filepath.Clean(child))
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(baseAbs, childAbs)
	if err != nil {
		return err
	}

	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(os.PathSeparator)) {
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

	return os.Rename(tmp, path)
}

func writeJSONTemp(path string, v any, perm os.FileMode) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), perm); err != nil {
		return "", err
	}

	return tmp, nil
}
