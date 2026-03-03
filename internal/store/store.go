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
	indexFileName   = "index.json"
	sessionsDirName = "sessions"
	scrollbackDir   = "scrollback"
	defaultDirPerm  = 0o755
	defaultFilePerm = 0o644
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

	path := s.sessionPath(ss.SessionName)
	if err := s.persistScrollbackUnlocked(&ss); err != nil {
		return err
	}
	if err := writeJSONAtomic(path, ss); err != nil {
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

func (s *Store) LoadSession(name string) (snapshot.SessionSnapshot, error) {
	var out snapshot.SessionSnapshot
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
	if err := os.MkdirAll(filepath.Join(s.baseDir, scrollbackDir), defaultDirPerm); err != nil {
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

func (s *Store) persistScrollbackUnlocked(ss *snapshot.SessionSnapshot) error {
	sessionDir := filepath.Join(s.baseDir, scrollbackDir, sanitizeName(ss.SessionName))
	hasAny := false

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
			if !hasAny {
				_ = os.RemoveAll(sessionDir)
				if err := os.MkdirAll(sessionDir, defaultDirPerm); err != nil {
					return err
				}
				hasAny = true
			}

			fileName := fmt.Sprintf("w%d_p%d.log", ss.Windows[wi].Index, pane.Index)
			path := filepath.Join(sessionDir, fileName)
			if err := os.WriteFile(path, []byte(content), defaultFilePerm); err != nil {
				return err
			}
			pane.Scrollback.Ref = filepath.Join(scrollbackDir, sanitizeName(ss.SessionName), fileName)
			pane.Scrollback.Bytes = len(content)
			pane.Scrollback.Lines = countLines(content)
			pane.Scrollback.Content = ""
		}
	}
	if !hasAny {
		_ = os.RemoveAll(sessionDir)
	}
	return nil
}

func (s *Store) hydrateScrollback(ss *snapshot.SessionSnapshot) error {
	for wi := range ss.Windows {
		for pi := range ss.Windows[wi].Panes {
			pane := &ss.Windows[wi].Panes[pi]
			if pane.Scrollback == nil || strings.TrimSpace(pane.Scrollback.Ref) == "" {
				continue
			}
			path := filepath.Join(s.baseDir, pane.Scrollback.Ref)
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

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), defaultFilePerm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
