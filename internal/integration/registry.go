package integration

import (
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

// Registry holds the enabled integrations and applies them at save and restore
// time. The zero value (and a nil *Registry) is safe and inert.
type Registry struct {
	items []Integration
}

// NewRegistry builds a registry from the given integrations, consulted in order
// (the first match wins per pane).
func NewRegistry(items ...Integration) *Registry {
	return &Registry{items: items}
}

// Enrich runs the first matching integration's Capture on every pane and stores
// the result on the pane, namespaced as "<name>.<key>". Capture failures are
// swallowed so saving can never break on integration state.
func (r *Registry) Enrich(snap *snapshot.SessionSnapshot) {
	if r == nil || snap == nil || len(r.items) == 0 {
		return
	}

	for wi := range snap.Windows {
		for pi := range snap.Windows[wi].Panes {
			r.enrichPane(&snap.Windows[wi].Panes[pi])
		}
	}
}

// Resolve returns the restore command from the first matching integration, or ""
// to fall back to the default restore. It satisfies tmux's
// RestoreCommandResolver and is pure (reads only the pane's stored metadata).
func (r *Registry) Resolve(pane snapshot.Pane) string {
	if r == nil {
		return ""
	}

	integ := r.match(pane)
	if integ == nil {
		return ""
	}

	return integ.RestoreCommand(pane, subMeta(pane.Meta, integ.Name()))
}

func (r *Registry) enrichPane(pane *snapshot.Pane) {
	integ := r.match(*pane)
	if integ == nil {
		return
	}

	meta, err := integ.Capture(*pane)
	if err != nil || len(meta) == 0 {
		return
	}

	if pane.Meta == nil {
		pane.Meta = make(map[string]string, len(meta))
	}

	for key, value := range meta {
		pane.Meta[integ.Name()+"."+key] = value
	}
}

func (r *Registry) match(pane snapshot.Pane) Integration {
	for _, integ := range r.items {
		if integ.Matches(pane) {
			return integ
		}
	}

	return nil
}

// subMeta extracts the keys belonging to one integration, stripping the
// "<name>." namespace prefix.
func subMeta(meta map[string]string, name string) map[string]string {
	prefix := name + "."
	out := make(map[string]string)

	for key, value := range meta {
		if after, ok := strings.CutPrefix(key, prefix); ok {
			out[after] = value
		}
	}

	return out
}
