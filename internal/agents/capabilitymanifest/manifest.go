// Package capabilitymanifest owns the canonical, provider-neutral capability
// facts advertised by Gentle AI agent adapters.
package capabilitymanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

type SchemaVersion string

const SchemaV1 SchemaVersion = "gentle-ai.agent-capability-manifest/v1"

type ContractID string

const ContractWorkRoutingV1 ContractID = "gentle-ai.work-routing/v1"

// ContractReviewTransportV1 is Wave 4 S4's transport capability claim
// (design.md decision 5): the adapter self-declares whether it can carry
// the receipt-driven-development review protocol at all, checked before any
// review authority, tier, lens, budget, or collection slot exists. The
// provider never probes a live runtime for this — an absent or unrecognised
// claim fails closed.
const ContractReviewTransportV1 ContractID = "gentle-ai.review-transport/v1"

// ContractImmutableReviewExecutorV1 is independent of host/orchestrator
// support. It is advertised only when a provider can launch a fresh,
// constrained reviewer and prove that boundary before review START.
const ContractImmutableReviewExecutorV1 ContractID = "gentle-ai.immutable-review-executor/v1"

type ContractExposure string

const (
	ContractExposureDormant    ContractExposure = "dormant"
	ContractExposureAdvertised ContractExposure = "advertised"
)

type SDDSelectionPolicy string

const SDDSelectionExplicitRequestOrAcceptedProposal SDDSelectionPolicy = "explicit_request_or_accepted_proposal"

var ErrUnsupportedAgent = errors.New("unsupported agent capability manifest")

// AgentCapabilityManifest is the immutable v1 capability projection for one
// adapter. It intentionally owns capability and routing semantics only: it
// contains no runtime observations, review decisions, or lifecycle state.
type AgentCapabilityManifest struct {
	SchemaVersion         SchemaVersion              `json:"schemaVersion"`
	AgentID               model.AgentID              `json:"agentId"`
	Features              AgentFeatureClaims         `json:"features"`
	ImplementationRouting ImplementationRoutingFacts `json:"implementationRouting"`
	Contracts             ContractClaims             `json:"contracts"`
}

// AgentFeatureClaims describes adapter integration mechanisms. FileSubAgents
// means the adapter consumes Gentle AI's file-based subagent projection; it
// does not infer whether the runtime can perform some other form of delegation.
type AgentFeatureClaims struct {
	OutputStyles  bool `json:"outputStyles"`
	SlashCommands bool `json:"slashCommands"`
	FileSubAgents bool `json:"fileSubAgents"`
	Skills        bool `json:"skills"`
	SystemPrompt  bool `json:"systemPrompt"`
	MCP           bool `json:"mcp"`
	Workflows     bool `json:"workflows"`
}

// ImplementationRoutingFacts is a semantic projection of the orchestrator's
// always-active delegation rules. It is data for renderers and parity checks,
// not an implementation-route selector or SDD admission authority.
type ImplementationRoutingFacts struct {
	DirectInline    DirectInlineFacts    `json:"directInline"`
	DelegatedDirect DelegatedDirectFacts `json:"delegatedDirect"`
	SDD             SDDProposalFacts     `json:"sdd"`
}

type DirectInlineFacts struct {
	MinUnderstandingFiles                    uint8 `json:"minUnderstandingFiles"`
	MaxUnderstandingFiles                    uint8 `json:"maxUnderstandingFiles"`
	MaxMechanicalWriteFiles                  uint8 `json:"maxMechanicalWriteFiles"`
	MechanicalWriteMustBeAlreadyUnderstood   bool  `json:"mechanicalWriteMustBeAlreadyUnderstood"`
	MechanicalWriteMustNotRequireResearch    bool  `json:"mechanicalWriteMustNotRequireResearch"`
	MechanicalWriteMustNotHaveOpenDesignWork bool  `json:"mechanicalWriteMustNotHaveOpenDesignWork"`
}

type DelegatedDirectFacts struct {
	MappingMinUnderstandingFiles  uint8 `json:"mappingMinUnderstandingFiles"`
	WriterMinNonTrivialFiles      uint8 `json:"writerMinNonTrivialFiles"`
	DelegateWhenReadPreparesWrite bool  `json:"delegateWhenReadPreparesWrite"`
	DelegateWhenBroadResearch     bool  `json:"delegateWhenBroadResearch"`
}

type SDDProposalFacts struct {
	ProposeWhenSubstantialOrAmbiguous     bool               `json:"proposeWhenSubstantialOrAmbiguous"`
	DurableArtifactsMustReduceUncertainty bool               `json:"durableArtifactsMustReduceUncertainty"`
	SelectionPolicy                       SDDSelectionPolicy `json:"selectionPolicy"`
}

type ContractClaims struct {
	WorkRoutingV1             ContractClaim `json:"workRoutingV1"`
	ReviewTransportV1         ContractClaim `json:"reviewTransportV1"`
	ImmutableReviewExecutorV1 ContractClaim `json:"immutableReviewExecutorV1"`
}

type ContractClaim struct {
	ID       ContractID       `json:"id"`
	Exposure ContractExposure `json:"exposure"`
}

// ForAgent returns the sole canonical manifest for a supported adapter.
func ForAgent(agent model.AgentID) (AgentCapabilityManifest, error) {
	features, ok := featureClaimsByAgent[agent]
	if !ok {
		return AgentCapabilityManifest{}, fmt.Errorf("%w: %q", ErrUnsupportedAgent, agent)
	}

	return AgentCapabilityManifest{
		SchemaVersion:         SchemaV1,
		AgentID:               agent,
		Features:              features,
		ImplementationRouting: CanonicalImplementationRouting(),
		Contracts: ContractClaims{
			WorkRoutingV1: ContractClaim{
				ID:       ContractWorkRoutingV1,
				Exposure: ContractExposureDormant,
			},
			ReviewTransportV1: ContractClaim{
				ID:       ContractReviewTransportV1,
				Exposure: reviewTransportExposureByAgent[agent],
			},
			ImmutableReviewExecutorV1: ContractClaim{
				ID:       ContractImmutableReviewExecutorV1,
				Exposure: immutableReviewExecutorExposureByAgent[agent],
			},
		},
	}, nil
}

// reviewTransportExposureByAgent is the closed set of runtimes that can
// carry the review protocol. Every supported agent is explicitly dormant
// first, then only the four runtimes with the required native transport and
// immutable reviewer boundary advertise receipt-driven review. A map miss
// (an unknown agent) remains fail-closed.
var reviewTransportExposureByAgent = func() map[model.AgentID]ContractExposure {
	exposure := make(map[model.AgentID]ContractExposure, len(featureClaimsByAgent))
	for agent := range featureClaimsByAgent {
		exposure[agent] = ContractExposureDormant
	}
	exposure[model.AgentClaudeCode] = ContractExposureAdvertised
	exposure[model.AgentOpenCode] = ContractExposureAdvertised
	exposure[model.AgentCodex] = ContractExposureAdvertised
	exposure[model.AgentPi] = ContractExposureAdvertised
	return exposure
}()

// immutableReviewExecutorExposureByAgent declares only providers with an
// enforceable fresh-reviewer boundary. Claude launches a generated subagent
// with no live tools and receives only the native prompt-carried evidence;
// OpenCode relays one host Task through a Go-native transport process, which
// materializes the bound prompt and captures matching raw output from an
// ordinary already-running session -- no restart, child process, special
// user-visible session, or `OPENCODE_DISABLE_*` variable (rdd-advisory-
// transport SKILL.md). Capability advertisement records the provider contract
// that can reach Go-owned admission; organic runtime proof is recorded by the
// provider's own execution tests. Pi advertises through gentle-pi's host
// relay: the launcher reads the negotiated collection input, spawns a
// brand-new print-mode pi subprocess in an empty scratch directory with
// every discovery surface disabled, forwards the Go-issued opaque prompt
// untouched, and returns raw final bytes (gentle-pi#311, gentle-ai#3249).
// Kilo and every other runtime remain explicitly dormant until they own an
// equivalent native boundary.
var immutableReviewExecutorExposureByAgent = func() map[model.AgentID]ContractExposure {
	exposure := make(map[model.AgentID]ContractExposure, len(featureClaimsByAgent))
	for agent := range featureClaimsByAgent {
		exposure[agent] = ContractExposureDormant
	}
	exposure[model.AgentClaudeCode] = ContractExposureAdvertised
	exposure[model.AgentOpenCode] = ContractExposureAdvertised
	exposure[model.AgentCodex] = ContractExposureAdvertised
	exposure[model.AgentPi] = ContractExposureAdvertised
	return exposure
}()

// MustForAgent is for compile-time registered adapters. A panic means the
// factory and the canonical ACI registry have drifted.
func MustForAgent(agent model.AgentID) AgentCapabilityManifest {
	manifest, err := ForAgent(agent)
	if err != nil {
		panic(err)
	}
	return manifest
}

// CanonicalImplementationRouting returns the one routing projection shared by
// every supported adapter.
func CanonicalImplementationRouting() ImplementationRoutingFacts {
	return ImplementationRoutingFacts{
		DirectInline: DirectInlineFacts{
			MinUnderstandingFiles:                    1,
			MaxUnderstandingFiles:                    3,
			MaxMechanicalWriteFiles:                  1,
			MechanicalWriteMustBeAlreadyUnderstood:   true,
			MechanicalWriteMustNotRequireResearch:    true,
			MechanicalWriteMustNotHaveOpenDesignWork: true,
		},
		DelegatedDirect: DelegatedDirectFacts{
			MappingMinUnderstandingFiles:  4,
			WriterMinNonTrivialFiles:      2,
			DelegateWhenReadPreparesWrite: true,
			DelegateWhenBroadResearch:     true,
		},
		SDD: SDDProposalFacts{
			ProposeWhenSubstantialOrAmbiguous:     true,
			DurableArtifactsMustReduceUncertainty: true,
			SelectionPolicy:                       SDDSelectionExplicitRequestOrAcceptedProposal,
		},
	}
}

func (m AgentCapabilityManifest) Validate() error {
	expected, err := ForAgent(m.AgentID)
	if err != nil {
		return err
	}
	if m != expected {
		return fmt.Errorf("agent capability manifest for %q is not canonical", m.AgentID)
	}
	return nil
}

func (m AgentCapabilityManifest) Advertises(contract ContractID) bool {
	switch contract {
	case ContractWorkRoutingV1:
		return m.Contracts.WorkRoutingV1.ID == contract &&
			m.Contracts.WorkRoutingV1.Exposure == ContractExposureAdvertised
	case ContractReviewTransportV1:
		return m.Contracts.ReviewTransportV1.ID == contract &&
			m.Contracts.ReviewTransportV1.Exposure == ContractExposureAdvertised
	case ContractImmutableReviewExecutorV1:
		return m.Contracts.ImmutableReviewExecutorV1.ID == contract &&
			m.Contracts.ImmutableReviewExecutorV1.Exposure == ContractExposureAdvertised
	default:
		return false
	}
}

func (m AgentCapabilityManifest) CanonicalJSON() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal agent capability manifest: %w", err)
	}
	return payload, nil
}

func (m AgentCapabilityManifest) Digest() (string, error) {
	payload, err := m.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return digest("gentle-ai.agent-capability-manifest/v1", payload), nil
}

func (m AgentCapabilityManifest) RoutingDigest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(m.ImplementationRouting)
	if err != nil {
		return "", fmt.Errorf("marshal implementation routing facts: %w", err)
	}
	return digest("gentle-ai.implementation-routing/v1", payload), nil
}

func digest(domain string, payload []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

var featureClaimsByAgent = map[model.AgentID]AgentFeatureClaims{
	model.AgentAntigravity: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentClaudeCode: {
		OutputStyles: true, SlashCommands: true,
		FileSubAgents: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentCodex: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentCursor: {
		FileSubAgents: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentGeminiCLI: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentHermes: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentKilocode: {
		SlashCommands: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentKimi: {
		FileSubAgents: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentKiroIDE: {
		FileSubAgents: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentOpenClaw: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentOpenCode: {
		SlashCommands: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentPi: {
		SystemPrompt: false, MCP: true,
	},
	model.AgentQwenCode: {
		SlashCommands: true, Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentTrae: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentVSCodeCopilot: {
		Skills: true, SystemPrompt: true, MCP: true,
	},
	model.AgentWindsurf: {
		Skills: true, SystemPrompt: true, MCP: true, Workflows: true,
	},
}
