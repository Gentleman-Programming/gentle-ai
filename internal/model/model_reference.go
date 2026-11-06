package model

import "strings"

// ParseModelReference reads a model selection, including OpenCode V2 expanded
// selectors and variants. SplitModelSpec remains available for bare legacy IDs.
// Catalog aliases use modelID; selectors deliberately use model instead.
func ParseModelReference(value any) (ModelAssignment, bool) {
	var result ModelAssignment
	switch value := value.(type) {
	case string:
		spec, variant, found := strings.Cut(strings.TrimSpace(value), "#")
		if found && (variant == "" || strings.Contains(variant, "#")) {
			return ModelAssignment{}, false
		}
		provider, id, ok := SplitModelSpec(spec)
		if !ok {
			return ModelAssignment{}, false
		}
		result = ModelAssignment{ProviderID: provider, ModelID: id, Effort: variant}
	case map[string]any:
		result.ProviderID, _ = value["providerID"].(string)
		result.ModelID, _ = value["model"].(string)
		if raw, exists := value["variant"]; exists {
			var ok bool
			result.Effort, ok = raw.(string)
			if !ok || result.Effort == "" {
				return ModelAssignment{}, false
			}
		}
	default:
		return ModelAssignment{}, false
	}
	if strings.TrimSpace(result.ProviderID) == "" || strings.ContainsAny(result.ProviderID, "/#") || strings.TrimSpace(result.ModelID) == "" || strings.Contains(result.ModelID, "#") || strings.Contains(result.Effort, "#") {
		return ModelAssignment{}, false
	}
	return result, true
}
