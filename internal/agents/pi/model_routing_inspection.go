package pi

import (
	"bytes"
	"encoding/json"
	"errors"
)

type ModelRoutingTarget string

const (
	ModelRoutingTargetGlobal  ModelRoutingTarget = "global"
	ModelRoutingTargetProject ModelRoutingTarget = "project"
)

type ModelRoutingThinking string

const (
	ModelRoutingThinkingOff     ModelRoutingThinking = "off"
	ModelRoutingThinkingMinimal ModelRoutingThinking = "minimal"
	ModelRoutingThinkingLow     ModelRoutingThinking = "low"
	ModelRoutingThinkingMedium  ModelRoutingThinking = "medium"
	ModelRoutingThinkingHigh    ModelRoutingThinking = "high"
	ModelRoutingThinkingXHigh   ModelRoutingThinking = "xhigh"
	ModelRoutingThinkingMax     ModelRoutingThinking = "max"
)

type ModelRoutingDiagnosticSeverity string

const (
	ModelRoutingDiagnosticSeverityError   ModelRoutingDiagnosticSeverity = "error"
	ModelRoutingDiagnosticSeverityWarning ModelRoutingDiagnosticSeverity = "warning"
	ModelRoutingDiagnosticSeverityInfo    ModelRoutingDiagnosticSeverity = "info"
)

type ModelRoutingAgentSource string

const (
	ModelRoutingAgentSourceProject ModelRoutingAgentSource = "project"
	ModelRoutingAgentSourceUser    ModelRoutingAgentSource = "user"
	ModelRoutingAgentSourceBuiltin ModelRoutingAgentSource = "builtin"
)

type ModelRoutingProvenanceSource string

const (
	ModelRoutingProvenanceSourceGlobal  ModelRoutingProvenanceSource = "global"
	ModelRoutingProvenanceSourceProject ModelRoutingProvenanceSource = "project"
	ModelRoutingProvenanceSourceMissing ModelRoutingProvenanceSource = "missing"
	ModelRoutingProvenanceSourceInvalid ModelRoutingProvenanceSource = "invalid"
)

type ModelRoutingProvenanceStatus string

const (
	ModelRoutingProvenanceStatusValid   ModelRoutingProvenanceStatus = "valid"
	ModelRoutingProvenanceStatusMissing ModelRoutingProvenanceStatus = "missing"
	ModelRoutingProvenanceStatusInvalid ModelRoutingProvenanceStatus = "invalid"
)

type ModelRoutingOperational string

const (
	ModelRoutingOperationalAuthenticated ModelRoutingOperational = "authenticated"
	ModelRoutingOperationalUnavailable   ModelRoutingOperational = "unavailable"
	ModelRoutingOperationalUnknown       ModelRoutingOperational = "unknown"
)

type ModelRoutingAvailability string

const (
	ModelRoutingAvailabilityCatalog       ModelRoutingAvailability = "catalog"
	ModelRoutingAvailabilityConfigured    ModelRoutingAvailability = "configured"
	ModelRoutingAvailabilityAuthenticated ModelRoutingAvailability = "authenticated"
	ModelRoutingAvailabilityUnknown       ModelRoutingAvailability = "unknown"
)

type ModelRoutingDiagnostic struct {
	Code     string                         `json:"code"`
	Message  string                         `json:"message"`
	Severity ModelRoutingDiagnosticSeverity `json:"severity"`
	Path     *string                        `json:"path,omitempty"`
}

type ModelRoutingAssignment struct {
	Model           *string               `json:"model,omitempty"`
	Thinking        *ModelRoutingThinking `json:"thinking,omitempty"`
	InheritModel    bool                  `json:"inheritModel"`
	InheritThinking bool                  `json:"inheritThinking"`
}

type ModelRoutingProvenance struct {
	Target      ModelRoutingTarget           `json:"target"`
	Source      ModelRoutingProvenanceSource `json:"source"`
	Status      ModelRoutingProvenanceStatus `json:"status"`
	ConfigPath  string                       `json:"configPath"`
	ProfilePath string                       `json:"profilePath"`
}

type ModelRoutingResolvedTarget struct {
	Target      ModelRoutingTarget `json:"target"`
	ConfigPath  string             `json:"configPath"`
	ProfilePath string             `json:"profilePath"`
}

type ModelRoutingTargetInspection struct {
	Provenance  ModelRoutingProvenance            `json:"provenance"`
	Assignments map[string]ModelRoutingAssignment `json:"assignments"`
}

type ModelRoutingContext struct {
	CWD      string                     `json:"cwd"`
	AgentDir string                     `json:"agentDir"`
	Target   ModelRoutingTarget         `json:"target"`
	Global   ModelRoutingResolvedTarget `json:"global"`
	Project  ModelRoutingResolvedTarget `json:"project"`
}

type ModelRoutingAgent struct {
	Name         string                  `json:"name"`
	Source       ModelRoutingAgentSource `json:"source"`
	FilePath     *string                 `json:"filePath,omitempty"`
	Configurable bool                    `json:"configurable"`
	Assignment   *ModelRoutingAssignment `json:"assignment,omitempty"`
}

type ModelRoutingModelCapabilities struct {
	Reasoning      bool     `json:"reasoning"`
	Input          []string `json:"input"`
	ContextWindow  *int     `json:"contextWindow,omitempty"`
	MaxTokens      *int     `json:"maxTokens,omitempty"`
	ThinkingLevels []string `json:"thinkingLevels,omitempty"`
}

type ModelRoutingAuthenticated struct {
	Value   bool `json:"-"`
	Unknown bool `json:"-"`
}

func (a *ModelRoutingAuthenticated) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("true")):
		*a = ModelRoutingAuthenticated{Value: true}
		return nil
	case bytes.Equal(data, []byte("false")):
		*a = ModelRoutingAuthenticated{}
		return nil
	}

	var value string
	if len(data) > 0 && data[0] == '"' && json.Unmarshal(data, &value) == nil && value == "unknown" {
		*a = ModelRoutingAuthenticated{Unknown: true}
		return nil
	}
	return errors.New(`authenticated must be a boolean or "unknown"`)
}

type ModelRoutingModel struct {
	CanonicalID             string                        `json:"canonicalId"`
	Provider                string                        `json:"provider"`
	ModelID                 string                        `json:"modelId"`
	Name                    string                        `json:"name"`
	API                     *string                       `json:"api,omitempty"`
	Catalog                 bool                          `json:"catalog"`
	Configured              bool                          `json:"configured"`
	AuthConfigured          bool                          `json:"authConfigured"`
	Available               bool                          `json:"available"`
	Authenticated           ModelRoutingAuthenticated     `json:"authenticated"`
	Operational             ModelRoutingOperational       `json:"operational"`
	Availability            ModelRoutingAvailability      `json:"availability"`
	Reasoning               bool                          `json:"reasoning"`
	SupportedThinkingLevels []string                      `json:"supportedThinkingLevels,omitempty"`
	Capabilities            ModelRoutingModelCapabilities `json:"capabilities"`
}

type ModelRoutingInspection struct {
	Contract    string                                              `json:"contract"`
	Context     *ModelRoutingContext                                `json:"context,omitempty"`
	Targets     map[ModelRoutingTarget]ModelRoutingTargetInspection `json:"targets"`
	Assignments map[string]ModelRoutingAssignment                   `json:"assignments"`
	Agents      []ModelRoutingAgent                                 `json:"agents"`
	Providers   []string                                            `json:"providers"`
	Models      []ModelRoutingModel                                 `json:"models"`
	Diagnostics []ModelRoutingDiagnostic                            `json:"diagnostics"`
}
