package agents

import (
	"errors"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestDefaultRegistryManifestsMatchEveryAdapterProjection(t *testing.T) {
	t.Parallel()

	registry, err := NewDefaultRegistry()
	if err != nil {
		t.Fatalf("NewDefaultRegistry() error = %v", err)
	}

	var routingDigest string
	for _, agent := range registry.SupportedAgents() {
		adapter, ok := registry.Get(agent)
		if !ok {
			t.Fatalf("registry missing adapter %q", agent)
		}

		resolved, err := ResolveCapabilityManifest(adapter)
		if err != nil {
			t.Fatalf("ResolveCapabilityManifest(%q) error = %v", agent, err)
		}
		stored, ok := registry.CapabilityManifest(agent)
		if !ok {
			t.Fatalf("registry missing capability manifest %q", agent)
		}
		if stored != resolved {
			t.Fatalf("stored manifest for %q differs from resolved manifest", agent)
		}

		gotRoutingDigest, err := stored.RoutingDigest()
		if err != nil {
			t.Fatalf("RoutingDigest(%q) error = %v", agent, err)
		}
		if routingDigest == "" {
			routingDigest = gotRoutingDigest
		} else if gotRoutingDigest != routingDigest {
			t.Fatalf("adapter %q weakened or changed shared routing semantics: %q != %q", agent, gotRoutingDigest, routingDigest)
		}

		if stored.Advertises(ContractWorkRoutingV1) {
			t.Fatalf("adapter %q advertises dormant %q", agent, ContractWorkRoutingV1)
		}
	}
}

type manifestlessAdapter struct {
	Adapter
}

func TestRegistryRejectsAdapterWithoutTypedManifest(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapter(model.AgentClaudeCode)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	_, err = NewRegistry(manifestlessAdapter{Adapter: adapter})
	if !errors.Is(err, ErrCapabilityManifestUnavailable) {
		t.Fatalf("NewRegistry() error = %v, want ErrCapabilityManifestUnavailable", err)
	}
}

type tamperedManifestAdapter struct {
	mockAdapter
	manifest capabilitymanifest.AgentCapabilityManifest
}

func (a tamperedManifestAdapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return a.manifest
}

func TestRegistryRejectsWeakenedRoutingManifest(t *testing.T) {
	t.Parallel()

	manifest := capabilitymanifest.MustForAgent(model.AgentClaudeCode)
	manifest.ImplementationRouting.DelegatedDirect.MappingMinUnderstandingFiles = 5
	adapter := tamperedManifestAdapter{
		mockAdapter: mockAdapter{agent: model.AgentClaudeCode},
		manifest:    manifest,
	}

	_, err := NewRegistry(adapter)
	if !errors.Is(err, ErrCapabilityManifestMismatch) {
		t.Fatalf("NewRegistry() error = %v, want ErrCapabilityManifestMismatch", err)
	}
}

type projectionMismatchAdapter struct {
	mockAdapter
}

func (a projectionMismatchAdapter) SupportsAutoInstall() bool {
	return !a.CapabilityManifest().Features.AutoInstall
}

func TestRegistryRejectsLegacyProjectionMismatch(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry(projectionMismatchAdapter{
		mockAdapter: mockAdapter{agent: model.AgentClaudeCode},
	})
	if !errors.Is(err, ErrCapabilityManifestMismatch) {
		t.Fatalf("NewRegistry() error = %v, want ErrCapabilityManifestMismatch", err)
	}
}
