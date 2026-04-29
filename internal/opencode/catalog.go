package opencode

import "github.com/gentleman-programming/gentle-ai/internal/modelcatalog"

// LoadCatalog builds a normalized model picker catalog from OpenCode cache data.
func LoadCatalog(cachePath string) (modelcatalog.Catalog, error) {
	providers, err := LoadModels(cachePath)
	if err != nil {
		return modelcatalog.Catalog{}, err
	}

	available := DetectAvailableProviders(providers)
	sddModels := make(map[string][]modelcatalog.Model, len(available))
	normalizedProviders := make(map[string]modelcatalog.Provider, len(providers))

	for providerID, provider := range providers {
		normalizedModels := make(map[string]modelcatalog.Model, len(provider.Models))
		for modelID, m := range provider.Models {
			normalizedModels[modelID] = modelcatalog.Model{
				ID:        m.ID,
				Name:      m.Name,
				Reasoning: m.Reasoning,
				Cost: modelcatalog.ModelCost{
					Input:  m.Cost.Input,
					Output: m.Cost.Output,
				},
				ContextWindow: m.Limit.Context,
				MaxTokens:     m.Limit.Output,
			}
		}

		normalizedProviders[providerID] = modelcatalog.Provider{
			ID:     providerID,
			Name:   provider.Name,
			Models: normalizedModels,
		}
	}

	for _, providerID := range available {
		filtered := FilterModelsForSDD(providers[providerID])
		normalized := make([]modelcatalog.Model, 0, len(filtered))
		for _, m := range filtered {
			normalized = append(normalized, modelcatalog.Model{
				ID:        m.ID,
				Name:      m.Name,
				Reasoning: m.Reasoning,
				Cost: modelcatalog.ModelCost{
					Input:  m.Cost.Input,
					Output: m.Cost.Output,
				},
				ContextWindow: m.Limit.Context,
				MaxTokens:     m.Limit.Output,
			})
		}
		sddModels[providerID] = normalized
	}

	return modelcatalog.Catalog{
		Providers:            normalizedProviders,
		AvailableProviderIDs: available,
		SDDModels:            sddModels,
	}, nil
}
