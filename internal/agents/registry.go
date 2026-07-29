package agents

import (
	"fmt"
	"slices"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type Registry struct {
	adapters  map[model.AgentID]Adapter
	manifests map[model.AgentID]AgentCapabilityManifest
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	r := &Registry{
		adapters:  map[model.AgentID]Adapter{},
		manifests: map[model.AgentID]AgentCapabilityManifest{},
	}
	for _, adapter := range adapters {
		if err := r.Register(adapter); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func (r *Registry) Register(adapter Adapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}

	agent := adapter.Agent()
	if _, exists := r.adapters[agent]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateAdapter, agent)
	}

	manifest, err := ResolveCapabilityManifest(adapter)
	if err != nil {
		return fmt.Errorf("register adapter %q: %w", agent, err)
	}

	r.adapters[agent] = adapter
	r.manifests[agent] = manifest
	return nil
}

func (r *Registry) Get(agent model.AgentID) (Adapter, bool) {
	adapter, ok := r.adapters[agent]
	return adapter, ok
}

func (r *Registry) CapabilityManifest(agent model.AgentID) (AgentCapabilityManifest, bool) {
	manifest, ok := r.manifests[agent]
	return manifest, ok
}

func (r *Registry) SupportedAgents() []model.AgentID {
	ids := make([]model.AgentID, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
