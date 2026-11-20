package integration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alchemmist/lazy-tmux/internal/snapshot"
)

var (
	errAgentDisabled   = errors.New("agent integration is disabled; automatic resume skipped")
	errNoResumeCommand = errors.New("agent has no verified resume command")
)

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
	status := &StatusSnapshot{}
	for _, item := range r.items {
		if scoper, ok := item.(StatusScoper); ok {
			items = append(items, scoper.ScopeWithStatus(status))
		} else if scoper, ok := item.(Scoper); ok {
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
			_ = r.enrichPane(&snap.Windows[wi].Panes[pi])
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

func (r *Registry) HasVerifiedNativeSession(pane snapshot.Pane) bool {
	if r == nil {
		return false
	}
	native, ok := r.match(pane).(interface {
		SessionID(pane snapshot.Pane) (snapshot.AgentSession, bool)
	})
	if !ok {
		return false
	}
	_, verified := native.SessionID(pane)

	return verified
}

// EnrichForSave refuses incomplete agent captures before storage is mutated.
// Unlike picker enrichment, saving must surface failures: Sleep closes the live
// session only after this operation succeeds. Disabled integrations remain opt-outs.
func (r *Registry) EnrichForSave(snap *snapshot.SessionSnapshot) error {
	if r == nil || snap == nil {
		return nil
	}
	var failures []error
	for wi := range snap.Windows {
		window := &snap.Windows[wi]
		for pi := range window.Panes {
			pane := &window.Panes[pi]
			err := r.enrichSavePane(pane)
			if err != nil {
				failures = append(failures, fmt.Errorf(
					"session %q window %d (%q) pane %d: %w",
					snap.SessionName, window.Index, window.Name, pane.Index, err,
				))
			}
		}
	}

	return errors.Join(failures...)
}

func (r *Registry) enrichSavePane(pane *snapshot.Pane) error {
	integ := r.match(*pane)
	if integ == nil {
		return nil
	}
	err := r.enrichPane(pane)
	if err != nil {
		return fmt.Errorf("capture %s: %w", integ.Name(), err)
	}
	if CommandAgent(integ.Name()) == "" {
		return nil
	}
	command, err := r.ResolveChecked(*pane)
	if err != nil {
		return err
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("%s: %w", integ.Name(), errNoResumeCommand)
	}

	return nil
}

func (r *Registry) enrichPane(pane *snapshot.Pane) error {
	integ := r.match(*pane)
	if integ == nil {
		return nil
	}

	meta, err := integ.Capture(*pane)
	if err != nil {
		return fmt.Errorf("capture integration %s: %w", integ.Name(), err)
	}
	if len(meta) == 0 {
		return nil
	}

	if pane.Meta == nil {
		pane.Meta = make(map[string]string, len(meta))
	}

	for key, value := range meta {
		pane.Meta[integ.Name()+"."+key] = value
	}

	return nil
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
