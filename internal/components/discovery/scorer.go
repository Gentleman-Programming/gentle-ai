package discovery

import (
	"sort"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// QualityScorer ranks MCP servers by a weighted quality formula:
//
//	relevance(0.4) × safety(0.3) × popularity(0.3)
//
// Since relevance is not stored on MCPServerRef directly, the scorer
// uses QualityScore as a proxy for the combined metric and sorts descending.
type QualityScorer struct{}

// NewQualityScorer returns a scorer with default weights.
func NewQualityScorer() *QualityScorer {
	return &QualityScorer{}
}

// Rank returns a copy of items sorted by QualityScore descending.
// The original slice is not modified.
func (s *QualityScorer) Rank(items []model.MCPServerRef) []model.MCPServerRef {
	if len(items) == 0 {
		return items
	}

	ranked := make([]model.MCPServerRef, len(items))
	copy(ranked, items)

	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].QualityScore > ranked[j].QualityScore
	})

	return ranked
}

// Categorize classifies MCP servers into must_have (score >= 90),
// nice_to_have (70-89), and avoid (risk_level == "high").
func (s *QualityScorer) Categorize(items []model.MCPServerRef) RecommendationSet {
	recs := RecommendationSet{}

	for _, item := range items {
		switch {
		case item.RiskLevel == "high":
			recs.Avoid = append(recs.Avoid, item.Name)
		case item.QualityScore >= 90:
			recs.MustHave = append(recs.MustHave, item.Name)
		case item.QualityScore >= 70:
			recs.NiceToHave = append(recs.NiceToHave, item.Name)
		default:
			// Below 70 and not high-risk: treat as nice_to_have
			recs.NiceToHave = append(recs.NiceToHave, item.Name)
		}
	}

	return recs
}
