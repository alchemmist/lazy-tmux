package integration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var errAgentDisabled = errors.New("agent integration is disabled; automatic resume skipped")

type Registry struct {
	items []Integration
}

func NewRegistry(items ...Integration) *Registry {
	return &Registry{items: items}
}

func (r *Registry) Scope() *Registry {
	if r == nil {
		return NewRegistry()
	}

	items := make([]Integration, 0, len(r.items))
	for _, item := range r.items {
		if scoper, ok := item.(Scoper); ok {
			items = append(items, scoper.Scope())
		} else {
			items = append(items, item)
		}
	}

	return NewRegistry(items...)
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
	command, _ := r.ResolveChecked(pane)

	return command
}

func (r *Registry) ResolveChecked(pane snapshot.Pane) (string, error) {
	if r == nil {
		return "", nil
	}

	integ := r.match(pane)
	if integ == nil {
		if pane.Agent != nil || CommandAgent(pane.CurrentCmd) != "" ||
			CommandAgent(pane.RestoreCmd) != "" {
			return "", errAgentDisabled
		}

		return "", nil
	}

	meta := subMeta(pane.Meta, integ.Name())
	if resolver, ok := integ.(interface {
		RestoreDecision(pane snapshot.Pane, meta map[string]string) (string, error)
	}); ok {
		command, err := resolver.RestoreDecision(pane, meta)
		if err != nil {
			return "", fmt.Errorf("resolve restore: %w", err)
		}

		return command, nil
	}

	return integ.RestoreCommand(pane, meta), nil
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

func (r *Registry) StatusFor(name string, pane snapshot.Pane) (Status, bool) {
	if r == nil {
		return StatusUnknown, false
	}

	for _, integ := range r.items {
		if integ.Name() != name || !integ.Matches(pane) {
			continue
		}

		reporter, ok := integ.(StatusReporter)
		if !ok {
			return StatusUnknown, false
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
