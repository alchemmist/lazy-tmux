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
		SessionName: ss.SessionName,
		File:        path,
		CapturedAt:  ss.CapturedAt.UTC(),
		Windows:     len(ss.Windows),
		Panes:       panes,
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

func (s *Store) ensureLayout() error {
	if err := os.MkdirAll(filepath.Join(s.baseDir, sessionsDirName), defaultDirPerm); err != nil {
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
