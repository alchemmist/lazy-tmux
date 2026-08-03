package integration

import (
	"errors"
	"testing"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errBoom = errors.New("boom")

type fakeIntegration struct {
	name       string
	matchCmd   string
	meta       map[string]string
	captureErr error
}

func (f fakeIntegration) Name() string                 { return f.name }
func (f fakeIntegration) Matches(p snapshot.Pane) bool { return p.CurrentCmd == f.matchCmd }
func (f fakeIntegration) Capture(snapshot.Pane) (map[string]string, error) {
	return f.meta, f.captureErr
}

func (f fakeIntegration) RestoreCommand(_ snapshot.Pane, meta map[string]string) string {
	if meta["session_id"] == "" {
		return ""
	}

	return "resume " + meta["session_id"]
}

func paneSnap(panes ...snapshot.Pane) *snapshot.SessionSnapshot {
	return &snapshot.SessionSnapshot{Windows: []snapshot.Window{{Panes: panes}}}
}

func TestRegistryEnrichNamespacesMeta(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeIntegration{
		name:     "fake",
		matchCmd: "myprog",
		meta:     map[string]string{"session_id": "abc"},
	})

	snap := paneSnap(
		snapshot.Pane{Index: 0, CurrentCmd: "myprog"},
		snapshot.Pane{Index: 1, CurrentCmd: "zsh"},
	)
	reg.Enrich(snap)

	if got := snap.Windows[0].Panes[0].Meta["fake.session_id"]; got != "abc" {
		t.Fatalf("matched pane should carry namespaced meta, got %q", got)
	}

	if snap.Windows[0].Panes[1].Meta != nil {
		t.Fatalf("non-matching pane must stay un-enriched, got %v", snap.Windows[0].Panes[1].Meta)
	}
}

func TestRegistryEnrichSwallowsCaptureFailures(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeIntegration{
		name:       "fake",
		matchCmd:   "myprog",
		captureErr: errBoom,
	})

	snap := paneSnap(snapshot.Pane{CurrentCmd: "myprog"})
	reg.Enrich(snap)

	if snap.Windows[0].Panes[0].Meta != nil {
		t.Fatal("a failing Capture must record nothing")
	}
}

func TestRegistryEnrichesAgainOnEveryCall(t *testing.T) {
	t.Parallel()

	var calls int
	reg := NewRegistry(dynamicIntegration{calls: &calls})
	snap := paneSnap(snapshot.Pane{CurrentCmd: "dynamic"})
	reg.Enrich(snap)
	reg.Enrich(snap)

	if calls != 2 {
		t.Fatalf("integration capture called %d times, want 2", calls)
	}
}

type dynamicIntegration struct{ calls *int }

func (d dynamicIntegration) Name() string                 { return "dynamic" }
func (d dynamicIntegration) Matches(p snapshot.Pane) bool { return p.CurrentCmd == "dynamic" }
func (d dynamicIntegration) Capture(snapshot.Pane) (map[string]string, error) {
	(*d.calls)++

	return map[string]string{"session_id": "current"}, nil
}

func (d dynamicIntegration) RestoreCommand(
	_ snapshot.Pane,
	_ map[string]string,
) string {
	return ""
}

func TestRegistryResolveDeNamespaces(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeIntegration{name: "fake", matchCmd: "myprog"})

	pane := snapshot.Pane{
		CurrentCmd: "myprog",
		Meta:       map[string]string{"fake.session_id": "xyz", "other.k": "v"},
	}

	if got := reg.Resolve(pane); got != "resume xyz" {
		t.Fatalf("Resolve should pass de-namespaced meta, got %q", got)
	}
}

func TestRegistryResolveNoMatch(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeIntegration{name: "fake", matchCmd: "myprog"})

	if got := reg.Resolve(snapshot.Pane{CurrentCmd: "vim"}); got != "" {
		t.Fatalf("no match should resolve to empty, got %q", got)
	}
}

type fakeStatusIntegration struct {
	fakeIntegration

	status Status
}

func (f fakeStatusIntegration) Status(snapshot.Pane) (Status, bool) {
	return f.status, true
}

func TestRegistryStatus(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeStatusIntegration{
		fakeIntegration: fakeIntegration{name: "fake", matchCmd: "myprog"},
		status:          StatusAwaitingDecision,
	})

	got, ok := reg.Status(snapshot.Pane{CurrentCmd: "myprog"})
	if !ok || got != StatusAwaitingDecision {
		t.Fatalf("expected awaiting-decision, got %v ok=%v", got, ok)
	}

	if _, ok := reg.Status(snapshot.Pane{CurrentCmd: "other"}); ok {
		t.Fatal("non-matching pane should have no status")
	}
}

func TestRegistryStatusNonReporter(t *testing.T) {
	t.Parallel()

	reg := NewRegistry(fakeIntegration{name: "fake", matchCmd: "myprog"})

	if _, ok := reg.Status(snapshot.Pane{CurrentCmd: "myprog"}); ok {
		t.Fatal("integration without StatusReporter should yield no status")
	}
}

func TestRegistryNilSafe(t *testing.T) {
	t.Parallel()

	var reg *Registry

	reg.Enrich(paneSnap(snapshot.Pane{CurrentCmd: "x"}))

	if got := reg.Resolve(snapshot.Pane{CurrentCmd: "x"}); got != "" {
		t.Fatalf("nil registry resolve should be empty, got %q", got)
	}
}
