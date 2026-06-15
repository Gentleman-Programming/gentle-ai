package discovery

import (
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// Deduplicator merges duplicate MCP server entries by name+URL,
// keeping the entry with the highest QualityScore.
type Deduplicator struct{}

// NewDeduplicator returns a new deduplicator.
func NewDeduplicator() *Deduplicator {
	return &Deduplicator{}
}

// Deduplicate removes duplicate entries. Two entries are considered duplicates
// if they share the same Name and URL. When duplicates are found, the entry
// with the highest QualityScore is kept.
func (d *Deduplicator) Deduplicate(items []model.MCPServerRef) []model.MCPServerRef {
	if len(items) == 0 {
		return items
	}

	type keyed struct {
		item  model.MCPServerRef
		index int
	}

	seen := make(map[string]*keyed)
	var order []string

	for _, item := range items {
		key := item.Name + "\x00" + item.URL
		existing, ok := seen[key]
		if !ok {
			seen[key] = &keyed{item: item, index: len(order)}
			order = append(order, key)
			continue
		}
		// Keep the higher score
		if item.QualityScore > existing.item.QualityScore {
			existing.item = item
		}
	}

	result := make([]model.MCPServerRef, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key].item)
	}

	return result
}
