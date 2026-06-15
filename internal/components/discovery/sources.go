// Package discovery provides multi-source search for MCP servers and skills.
package discovery

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// DiscoveryQuery is the input to the discovery engine.
type DiscoveryQuery struct {
	Role       model.RoleEnum
	Focus      []string
	Budget     string   // "free-only", "standard", "premium"
	KnownTools []string // existing tools to exclude from results
}

// RecommendationSet groups tool names by priority category.
type RecommendationSet struct {
	MustHave   []string // quality >= 90
	NiceToHave []string // quality 70-89
	Avoid      []string // risk_level == "high" or known CVEs
}

// SourceReader is the interface for each discovery source.
// Implementations query a single external source (MCP Awesome, SkillsMP, etc.)
// and return raw MCP server references that the engine scores and deduplicates.
type SourceReader interface {
	// Name returns a human-readable identifier for this source.
	Name() string
	// Search queries the source for tools matching the given query.
	Search(ctx context.Context, query DiscoveryQuery) ([]model.MCPServerRef, error)
	// HealthCheck verifies the source is reachable. Returns nil if healthy.
	HealthCheck(ctx context.Context) error
}
