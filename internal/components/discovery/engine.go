package discovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// DiscoveryResult is the ranked output from discovery.
type DiscoveryResult struct {
	MCPServers     []model.MCPServerRef
	Skills         []model.SkillRef
	Recommendations RecommendationSet
	ScoredAt       string
}

// DiscoveryEngine orchestrates multi-source search with quality scoring
// and deduplication. It degrades gracefully — if some sources fail,
// results from working sources are still returned.
type DiscoveryEngine struct {
	Sources []SourceReader
	scorer  *QualityScorer
	dedup   *Deduplicator
}

// NewEngine returns a DiscoveryEngine with the given sources.
func NewEngine(sources []SourceReader) *DiscoveryEngine {
	return &DiscoveryEngine{
		Sources: sources,
		scorer:  NewQualityScorer(),
		dedup:   NewDeduplicator(),
	}
}

// Search queries all sources, deduplicates and ranks the results.
// If all sources fail, it returns an error. Partial failures are
// logged and the engine continues with available results.
func (e *DiscoveryEngine) Search(ctx context.Context, query DiscoveryQuery) (*DiscoveryResult, error) {
	var allServers []model.MCPServerRef
	var errs []error

	for _, source := range e.Sources {
		items, err := source.Search(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", source.Name(), err))
			continue
		}
		allServers = append(allServers, items...)
	}

	if len(allServers) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all discovery sources failed: %w", errors.Join(errs...))
	}

	// Deduplicate, rank, and categorize
	deduped := e.dedup.Deduplicate(allServers)
	ranked := e.scorer.Rank(deduped)
	recs := e.scorer.Categorize(ranked)

	return &DiscoveryResult{
		MCPServers:      ranked,
		Recommendations: recs,
		ScoredAt:        time.Now().UTC().Format(time.RFC3339),
	}, nil
}
