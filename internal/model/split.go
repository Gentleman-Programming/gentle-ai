package model

// SplitProviderModel splits a model spec string into its provider and model
// components at the FIRST occurrence of '/' or ':'. Splitting on the first
// separator correctly handles compound model identifiers such as OpenRouter
// free models — e.g. "openrouter/qwen/qwen3.6-plus:free" is split into
// providerID="openrouter" and modelID="qwen/qwen3.6-plus:free".
//
// Returns (providerID, modelID, true) when both parts are non-empty.
// Returns ("", "", false) when the input is empty, has no separator, starts
// with a separator, or has an empty model part.
func SplitProviderModel(spec string) (providerID, modelID string, ok bool) {
	sep := -1
	for i, c := range spec {
		if c == '/' || c == ':' {
			sep = i
			break
		}
	}
	if sep <= 0 {
		return "", "", false
	}
	providerID = spec[:sep]
	modelID = spec[sep+1:]
	if providerID == "" || modelID == "" {
		return "", "", false
	}
	return providerID, modelID, true
}
