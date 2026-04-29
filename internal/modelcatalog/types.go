package modelcatalog

// ModelCost contains per-million-token pricing metadata.
type ModelCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Model represents a model entry from any picker source.
type Model struct {
	ID            string
	Name          string
	Reasoning     bool
	ContextWindow int
	MaxTokens     int
	Cost          ModelCost
}

// Provider represents a provider and its models.
type Provider struct {
	ID     string
	Name   string
	Models map[string]Model
}

// Catalog is the normalized model picker catalog independent of source.
type Catalog struct {
	Providers            map[string]Provider
	AvailableProviderIDs []string
	SDDModels            map[string][]Model
}
