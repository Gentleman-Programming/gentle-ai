package opencode

import (
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
)

// Normalize only the fields represented by ConfigSnapshot. Native entries are
// interpreted in their own format, never by recursively guessing legacy keys.
// A valid native value wins over the corresponding legacy value in one config.
func configuredProviders(root map[string]any) map[string]Provider {
	providers := map[string]Provider{}
	overlayConfiguredProviders(providers, root)
	return providers
}

func overlayConfiguredProviders(providers map[string]Provider, root map[string]any) {
	legacy, _ := root["provider"].(map[string]any)
	for id, p := range legacyConfiguredProviders(root) {
		def, _ := legacy[id].(map[string]any)
		existing, found := providers[id]
		if !found {
			providers[id] = p
			continue
		}
		if _, valid := def["name"].(string); valid {
			existing.Name = p.Name
		}
		if p.URL != "" {
			existing.URL = p.URL
		}
		models, _ := def["models"].(map[string]any)
		for id, m := range p.Models {
			previous, found := existing.Models[id]
			entry, _ := models[id].(map[string]any)
			if found {
				if _, exists := entry["name"]; !exists {
					m.Name = previous.Name
				}
				if _, exists := entry["family"]; !exists {
					m.Family = previous.Family
				}
				_, hasTool := entry["tool_call"]
				_, hasAlias := entry["toolcall"]
				if !hasTool && !hasAlias {
					m.ToolCall = previous.ToolCall
				}
				if _, exists := entry["reasoning"]; !exists {
					m.Reasoning = previous.Reasoning
				}
				m.Variants = previous.Variants
			}
			existing.Models[id] = m
		}
		providers[id] = existing
	}
	native, _ := root["providers"].(map[string]any)
	for id, raw := range native {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		provider, exists := providers[id]
		if !exists {
			provider = Provider{ID: id, Name: id, Models: map[string]Model{}}
		}
		if name, ok := def["name"].(string); ok {
			provider.Name = name
		}
		settings, _ := def["settings"].(map[string]any)
		if url, ok := settings["baseURL"].(string); ok {
			provider.URL = url
		}
		models, _ := def["models"].(map[string]any)
		for modelID, raw := range models {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if boolValue(entry["disabled"]) {
				delete(provider.Models, modelID)
				continue
			}
			m := provider.Models[modelID]
			m.ID = modelID // modelID inside the entry is the upstream API ID, not the selector.
			m.Name = stringValue(entry["name"], stringValue(m.Name, modelID))
			m.Family = stringValue(entry["family"], m.Family)
			capabilities, _ := entry["capabilities"].(map[string]any)
			if tools, ok := capabilities["tools"].(bool); ok {
				m.ToolCall = tools
			}
			if variants, ok := entry["variants"].([]any); ok {
				m.Variants = nil
				seen := map[string]bool{}
				for _, raw := range variants {
					v, _ := raw.(map[string]any)
					id, _ := v["id"].(string)
					if id != "" && !seen[id] {
						m.Variants = append(m.Variants, id)
						seen[id] = true
					}
				}
				sortVariants(m.Variants)
			}
			provider.Models[modelID] = m
		}
		providers[id] = provider
	}
}

func configuredAssignments(root map[string]any) map[string]AssignmentPresence {
	assignments := map[string]AssignmentPresence{}
	overlayConfiguredAssignments(assignments, root)
	return assignments
}

func overlayConfiguredAssignments(assignments map[string]AssignmentPresence, root map[string]any) {
	legacy, _ := root["agent"].(map[string]any)
	for name, raw := range legacy {
		key := name
		if name == "sdd-orchestrator" {
			key = "gentle-orchestrator"
		}
		def, _ := raw.(map[string]any)
		_, hasModel := def["model"]
		previous, exists := assignments[key]
		if exists && !hasModel {
			if effort, valid := def["variant"].(string); valid {
				previous.Assignment.Effort = effort
				assignments[key] = previous
			}
			continue
		}
		next := legacyConfiguredAssignments(map[string]any{"agent": map[string]any{name: raw}})[key]
		spec, _ := def["model"].(string)
		if _, hasVariant := def["variant"]; exists && !hasVariant && !strings.Contains(spec, "#") && !next.Cleared {
			next.Assignment.Effort = previous.Assignment.Effort
		}
		assignments[key] = next
	}
	native, _ := root["agents"].(map[string]any)
	for name, raw := range native {
		key := name
		if name == "sdd-orchestrator" {
			key = "gentle-orchestrator"
		}
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if boolValue(def["disabled"]) {
			assignments[key] = AssignmentPresence{Present: true, Cleared: true}
			continue
		}
		assignment, valid := model.ParseModelReference(def["model"])
		if valid {
			assignments[key] = AssignmentPresence{Present: true, Assignment: assignment}
			continue
		}
		// Malformed/absent native fields must not erase a valid legacy assignment.
		if _, exists := assignments[key]; !exists {
			_, hasModel := def["model"]
			assignments[key] = AssignmentPresence{Present: true, Cleared: !hasModel}
		}
	}
}
