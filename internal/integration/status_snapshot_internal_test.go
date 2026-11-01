package integration

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

func TestStatusSnapshotSharesConcurrentCaptureAndFailure(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			loader := func() ([]snapshot.Pane, error) {
				calls.Add(1)
				if fail {
					return nil, os.ErrNotExist
				}

				return []snapshot.Pane{{CurrentCmd: "claude"}}, nil
			}
			for range 2 {
				cache := &StatusSnapshot{}
				var workers sync.WaitGroup
				for range 32 {
					workers.Go(func() {
						panes := cache.Load(loader)
						if (len(panes) == 0) != fail {
							t.Errorf("unexpected panes: %v", panes)
						}
					})
				}
				workers.Wait()
			}
			if calls.Load() != 2 {
				t.Fatalf("captures = %d, want one per fresh scope", calls.Load())
			}
		})
	}
}
