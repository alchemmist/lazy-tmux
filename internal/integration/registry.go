package integration

import (
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

type Registry struct {
	items []Integration
}

func NewRegistry(items ...Integration) *Registry {
	return &Registry{items: items}
}

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

func (r *Registry) Status(pane snapshot.Pane) (Status, bool) {
	if r == nil {
		return StatusUnknown, false
	}

	for _, integ := range r.items {
		if !integ.Matches(pane) {
			continue
		}

		reporter, ok := integ.(StatusReporter)
		if !ok {
			continue
		}

		return reporter.Status(pane)
	}

	return StatusUnknown, false
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
