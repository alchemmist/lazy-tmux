// Package store persists session snapshots on disk under the data dir: one
// JSON file per session, pane scrollback in a sibling directory, and an index
// with per-session records.
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

// Sentinel errors of the on-disk store; paths and names wrap them at the call
// sites so messages stay unchanged.
var (
	errEmptySessionName         = errors.New("empty session name")
	errEmptyScrollbackRef       = errors.New("empty scrollback ref")
	errScrollbackRefOutsideBase = errors.New("invalid scrollback ref outside base dir")
	errInvalidScrollbackSession = errors.New("invalid session name for scrollback")
	errPathOutsideBase          = errors.New("invalid path outside base dir")
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

// Store is the on-disk snapshot store rooted at a base dir. A mutex serializes
// all mutations, and every write goes through a temp file plus rename so a
// crash never leaves a half-written snapshot or index behind.
type Store struct {
	baseDir string
	mu      sync.Mutex
}

// New returns a Store rooted at baseDir. The directory layout is created
// lazily on the first save, so New itself never touches the filesystem.
func New(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// DefaultDataDir returns the store's default base dir: $LAZY_TMUX_DATA_DIR
// when set, otherwise ~/.local/share/lazy-tmux, falling back to the relative
// ".lazy-tmux" when the home directory cannot be resolved.
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

// SaveSession persists a snapshot: pane scrollback is split out into per-pane
// files (replaced by refs in the JSON), the session JSON is written atomically,
// and the index record is refreshed while preserving the session's recorded
// LastAccessed time. A zero CapturedAt is stamped with the current UTC time.
func (s *Store) SaveSession(sessionSnapshot snapshot.SessionSnapshot) error {
	if sessionSnapshot.SessionName == "" {
		return errEmptySessionName
	}

	if sessionSnapshot.CapturedAt.IsZero() {
		sessionSnapshot.CapturedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.ensureLayout()
	if err != nil {
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

	err = s.persistScrollbackUnlocked(
		sessionSnapshot.SessionName,
		safeName,
		entries,
	)
	if err != nil {
		return err
	}

	err = os.Rename(jsonTmp, path)
	if err != nil {
		return fmt.Errorf("rename tmp file: %w", err)
	}

	return s.updateIndexUnlocked(sessionSnapshot, path)
}

// DeleteSession removes a session's JSON file, its scrollback directory and
// its index entry. It is idempotent: deleting a session that has no files on
// disk still succeeds and just cleans the index.
func (s *Store) DeleteSession(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errEmptySessionName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)

	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove session file: %w", err)
	}

	safeName, err := safeScrollbackSessionName(name)
	if err != nil {
		return err
	}

	scrollRoot := filepath.Clean(filepath.Join(s.baseDir, scrollbackDir))
	sessionDir := filepath.Clean(filepath.Join(scrollRoot, safeName))

	err = ensureUnderDir(scrollRoot, sessionDir, name)
	if err != nil {
		return err
	}

	err = os.RemoveAll(sessionDir)
	if err != nil {
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

// LoadSession reads a session snapshot from disk and hydrates pane scrollback
// content from its ref files. A ref whose file has since disappeared is
// skipped, not an error, so a partially pruned store still loads.
func (s *Store) LoadSession(name string) (snapshot.SessionSnapshot, error) {
	var out snapshot.SessionSnapshot

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)

	b, err := os.ReadFile(path) // #nosec G304 -- sessionPath sanitizes the name under the data dir
	if err != nil {
		return out, fmt.Errorf("read session file: %w", err)
	}

	err = json.Unmarshal(b, &out)
	if err != nil {
		return out, fmt.Errorf("unmarshal session: %w", err)
	}

	err = s.hydrateScrollback(&out)
	if err != nil {
		return out, err
	}

	return out, nil
}

// SessionPath returns the path of the session's JSON file under the data dir,
// with the name sanitized for the filesystem. The file need not exist; only an
// empty name is an error.
func (s *Store) SessionPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errEmptySessionName
	}

	return s.sessionPath(name), nil
}

// SessionExists reports whether a snapshot file for the session is on disk. It
// stats the JSON file directly rather than consulting the index, so it stays
// truthful even when the two drift apart.
func (s *Store) SessionExists(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errEmptySessionName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(name)

	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("stat session file: %w", err)
	}

	return true, nil
}

// ListRecords returns every session record from the index, newest capture
// first, with ties broken by session name for a stable order. A store with no
// index yet yields (nil, nil).
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

// LatestRecord returns the most recently captured session record. It returns
// os.ErrNotExist when the store holds no records at all.
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

// ScrollbackExists reports whether the session has a scrollback directory on
// disk. Sessions saved without any scrollback content have none, so false is a
// normal answer for an existing session.
func (s *Store) ScrollbackExists(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errEmptySessionName
	}

	safeName, err := safeScrollbackSessionName(name)
	if err != nil {
		return false, err
	}

	scrollRoot := filepath.Clean(filepath.Join(s.baseDir, scrollbackDir))
	sessionDir := filepath.Clean(filepath.Join(scrollRoot, safeName))

	_, err = os.Stat(sessionDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("stat scrollback dir: %w", err)
	}

	return true, nil
}

// IndexEntryExists reports whether the index holds a record for the session.
// Unlike SessionExists it consults only the index, which lets callers detect
// index/file drift by comparing the two.
func (s *Store) IndexEntryExists(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errEmptySessionName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndexUnlocked()
	if err != nil {
		return false, err
	}

	_, ok := idx.Sessions[name]

	return ok, nil
}

// MarkSessionAccessed stamps the session's index record with the given access
// time (in UTC; a zero value means now) so pickers can sort by recency. It
// returns os.ErrNotExist when the session has no index record.
func (s *Store) MarkSessionAccessed(name string, accessTime time.Time) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errEmptySessionName
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

// updateIndexUnlocked refreshes the saved session's index record — preserving
// its recorded LastAccessed time — and writes the index atomically. The caller
// must hold the store mutex.
func (s *Store) updateIndexUnlocked(sessionSnapshot snapshot.SessionSnapshot, path string) error {
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

func (s *Store) ensureLayout() error {
	err := os.MkdirAll(filepath.Join(s.baseDir, sessionsDirName), defaultDirPerm)
	if err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}

	err = os.MkdirAll(filepath.Join(s.baseDir, scrollbackDir), scrollbackDirPerm)
	if err != nil {
		return fmt.Errorf("create scrollback dir: %w", err)
	}

	return nil
}

func (s *Store) loadIndexUnlocked() (snapshot.Index, error) {
	p := s.indexPath()

	fileContent, err := os.ReadFile(p) // #nosec G304 -- fixed index path under the data dir
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

	err = json.Unmarshal(fileContent, &idx)
	if err != nil {
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

	err := ensureUnderDir(scrollRoot, sessionDir, sessionName)
	if err != nil {
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

	err = os.MkdirAll(stageDir, scrollbackDirPerm)
	if err != nil {
		return fmt.Errorf("create stage dir: %w", err)
	}

	for _, ent := range entries {
		path := filepath.Join(stageDir, ent.FileName)

		err := os.WriteFile(path, []byte(ent.Content), scrollbackFilePerm)
		if err != nil {
			return fmt.Errorf("write scrollback file: %w", err)
		}
	}

	err = promoteScrollbackStage(sessionDir, stageDir)
	if err != nil {
		return err
	}

	return nil
}

func promoteScrollbackStage(sessionDir, stageDir string) error {
	backupDir := sessionDir + ".bak"
	_ = os.RemoveAll(backupDir)

	hadSessionDir := false

	_, err := os.Stat(sessionDir)
	if err == nil {
		hadSessionDir = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat session dir: %w", err)
	}

	if hadSessionDir {
		err := os.Rename(sessionDir, backupDir)
		if err != nil {
			return fmt.Errorf("backup session dir: %w", err)
		}
	}

	err = os.Rename(stageDir, sessionDir)
	if err != nil {
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

			fileContent, err := os.ReadFile(
				path,
			) // #nosec G304 -- ref validated against the base dir above
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
		return "", errEmptyScrollbackRef
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
		return "", fmt.Errorf("%w: %s", errScrollbackRefOutsideBase, ref)
	}

	return finalEval, nil
}

func safeScrollbackSessionName(sessionName string) (string, error) {
	name := sanitizeName(sessionName)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("%w: %q", errInvalidScrollbackSession, sessionName)
	}

	if strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("%w: %q", errInvalidScrollbackSession, sessionName)
	}

	if filepath.Separator != '/' && strings.Contains(name, "/") {
		return "", fmt.Errorf("%w: %q", errInvalidScrollbackSession, sessionName)
	}

	if filepath.Separator != '\\' && strings.Contains(name, "\\") {
		return "", fmt.Errorf("%w: %q", errInvalidScrollbackSession, sessionName)
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
		return fmt.Errorf("%w: %s", errPathOutsideBase, ref)
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

	err = os.Rename(tmp, path)
	if err != nil {
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

	err = os.WriteFile(tmp, append(jsonData, '\n'), perm)
	if err != nil {
		return "", fmt.Errorf("write temp file: %w", err)
	}

	return tmp, nil
}
