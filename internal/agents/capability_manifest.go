package agents

import (
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
)

var (
	ErrCapabilityManifestUnavailable = errors.New("agent capability manifest unavailable")
	ErrCapabilityManifestMismatch    = errors.New("agent capability manifest mismatch")
)

type AgentCapabilityManifest = capabilitymanifest.AgentCapabilityManifest
type AgentFeatureClaims = capabilitymanifest.AgentFeatureClaims
type ImplementationRoutingFacts = capabilitymanifest.ImplementationRoutingFacts
type ContractID = capabilitymanifest.ContractID
type ContractExposure = capabilitymanifest.ContractExposure

const (
	AgentCapabilityManifestV1 = capabilitymanifest.SchemaV1
	ContractWorkRoutingV1     = capabilitymanifest.ContractWorkRoutingV1

	ContractExposureDormant    = capabilitymanifest.ContractExposureDormant
	ContractExposureAdvertised = capabilitymanifest.ContractExposureAdvertised
)

// CapabilityManifestProvider is an additive compatibility seam: existing
// Adapter consumers remain source-compatible, while ACI-aware registries fail
// closed when an adapter has no typed manifest.
type CapabilityManifestProvider interface {
	CapabilityManifest() capabilitymanifest.AgentCapabilityManifest
}

type workflowCapabilityProjection interface {
	SupportsWorkflows() bool
}

// ResolveCapabilityManifest validates both the canonical manifest and every
// legacy Supports* projection. Callers never need to infer capabilities from
// paths, assets, or adapter identity.
func ResolveCapabilityManifest(adapter Adapter) (AgentCapabilityManifest, error) {
	if adapter == nil {
		return AgentCapabilityManifest{}, ErrCapabilityManifestUnavailable
	}

	provider, ok := adapter.(CapabilityManifestProvider)
	if !ok {
		return AgentCapabilityManifest{}, fmt.Errorf("%w: %q", ErrCapabilityManifestUnavailable, adapter.Agent())
	}

	manifest := provider.CapabilityManifest()
	if err := manifest.Validate(); err != nil {
		return AgentCapabilityManifest{}, fmt.Errorf("%w for %q: %v", ErrCapabilityManifestMismatch, adapter.Agent(), err)
	}
	if manifest.AgentID != adapter.Agent() {
		return AgentCapabilityManifest{}, fmt.Errorf(
			"%w: adapter %q returned manifest for %q",
			ErrCapabilityManifestMismatch,
			adapter.Agent(),
			manifest.AgentID,
		)
	}

	features := manifest.Features
	projections := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsAutoInstall", got: adapter.SupportsAutoInstall(), want: features.AutoInstall},
		{name: "SupportsOutputStyles", got: adapter.SupportsOutputStyles(), want: features.OutputStyles},
		{name: "SupportsSlashCommands", got: adapter.SupportsSlashCommands(), want: features.SlashCommands},
		{name: "SupportsSubAgents", got: adapter.SupportsSubAgents(), want: features.FileSubAgents},
		{name: "SupportsSkills", got: adapter.SupportsSkills(), want: features.Skills},
		{name: "SupportsSystemPrompt", got: adapter.SupportsSystemPrompt(), want: features.SystemPrompt},
		{name: "SupportsMCP", got: adapter.SupportsMCP(), want: features.MCP},
	}
	for _, projection := range projections {
		if projection.got != projection.want {
			return AgentCapabilityManifest{}, fmt.Errorf(
				"%w for %q: %s=%t, manifest=%t",
				ErrCapabilityManifestMismatch,
				adapter.Agent(),
				projection.name,
				projection.got,
				projection.want,
			)
		}
	}

	workflowProjection, projectsWorkflows := adapter.(workflowCapabilityProjection)
	if projectsWorkflows {
		if got := workflowProjection.SupportsWorkflows(); got != features.Workflows {
			return AgentCapabilityManifest{}, fmt.Errorf(
				"%w for %q: SupportsWorkflows=%t, manifest=%t",
				ErrCapabilityManifestMismatch,
				adapter.Agent(),
				got,
				features.Workflows,
			)
		}
	} else if features.Workflows {
		return AgentCapabilityManifest{}, fmt.Errorf(
			"%w for %q: workflows claimed without a compatibility projection",
			ErrCapabilityManifestMismatch,
			adapter.Agent(),
		)
	}

	return manifest, nil
}
