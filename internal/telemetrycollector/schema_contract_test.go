package telemetrycollector

import (
	"encoding/json"
	"testing"
)

// TestEventSchema_EveryStringPropertyIsClosed walks the embedded event
// schema and fails if any node of type "string" lacks both "enum" and
// "pattern" — the two ways this schema closes off free text. This is the
// guard against a future field landing as unconstrained user-authored
// text: the dataset this collector produces is meant to be statistical
// only (see docs/telemetry-collector.md), and a plain string property with
// neither would silently reopen that door.
func TestEventSchema_EveryStringPropertyIsClosed(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(eventSchemaJSON, &doc); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}

	var violations []string
	var walk func(path string, node map[string]any)
	walk = func(path string, node map[string]any) {
		if node["type"] == "string" {
			_, hasEnum := node["enum"]
			_, hasPattern := node["pattern"]
			if !hasEnum && !hasPattern {
				violations = append(violations, path)
			}
		}
		if props, ok := node["properties"].(map[string]any); ok {
			for name, sub := range props {
				if subSchema, ok := sub.(map[string]any); ok {
					walk(path+"/properties/"+name, subSchema)
				}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			walk(path+"/items", items)
		}
	}
	walk("$", doc)

	if len(violations) > 0 {
		t.Errorf("string properties with neither enum nor pattern (free text is possible here): %v", violations)
	}
}
