package model

import "strings"

// ModelCapability returns "small" or "capable" based on the model identifier.
// This is a heuristic based on known small-model names.
func ModelCapability(modelID string) string {
	m := strings.ToLower(modelID)
	smallIndicators := []string{
		"flash",
		"mini",
		"haiku",
		"gpt-4o-mini",
		"qwen3-30b",
	}
	for _, indicator := range smallIndicators {
		if strings.Contains(m, indicator) {
			return "small"
		}
	}
	return "capable"
}