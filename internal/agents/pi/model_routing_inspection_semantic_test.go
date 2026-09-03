package pi

import (
	"encoding/json"
	"testing"
)

func semanticInspectionFixture(t *testing.T) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(richModelRoutingInspection), &document); err != nil {
		t.Fatal(err)
	}
	document["agents"].([]any)[0].(map[string]any)["configurable"] = true
	document["models"].([]any)[0].(map[string]any)["catalog"] = true
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func mutateSemanticNode(node any, path []any, value any, remove bool) {
	if len(path) == 1 {
		object := node.(map[string]any)
		if remove {
			delete(object, path[0].(string))
		} else {
			object[path[0].(string)] = value
		}
		return
	}
	switch key := path[0].(type) {
	case string:
		mutateSemanticNode(node.(map[string]any)[key], path[1:], value, remove)
	case int:
		mutateSemanticNode(node.([]any)[key], path[1:], value, remove)
	}
}
func mutateSemanticInspection(t *testing.T, path []any, value any, remove bool) []byte {
	t.Helper()
	var document any
	if err := json.Unmarshal(semanticInspectionFixture(t), &document); err != nil {
		t.Fatal(err)
	}
	mutateSemanticNode(document, path, value, remove)
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func rejectSemanticPaths(t *testing.T, paths [][]any, value any, remove bool) {
	t.Helper()
	for _, path := range paths {
		path := path
		t.Run("case", func(t *testing.T) {
			if _, err := decodeModelRoutingInspection(mutateSemanticInspection(t, path, value, remove)); err == nil {
				t.Fatalf("accepted semantic mutation at %v", path)
			}
		})
	}
}

func TestDecodeModelRoutingInspectionSemanticsRichSuccessFalseEmptyAndOptional(t *testing.T) {
	got, err := decodeModelRoutingInspection(semanticInspectionFixture(t))
	if err != nil || got.Contract != modelRoutingContract || got.Context == nil || got.Context.Target != ModelRoutingTargetProject || got.Assignments["worker"].Thinking == nil || *got.Assignments["worker"].Thinking != ModelRoutingThinkingXHigh {
		t.Fatalf("rich result = %#v, err=%v", got, err)
	}
	if !got.Agents[0].Configurable || !got.Models[0].Catalog || !got.Models[1].Authenticated.Unknown || got.Models[1].Authenticated.Value || got.Diagnostics[1].Severity != ModelRoutingDiagnosticSeverityInfo || got.Providers == nil || got.Models[0].Capabilities.Input == nil || got.Assignments["empty"].InheritModel {
		t.Fatalf("literal/false/empty result = %#v", got)
	}
	got, err = decodeModelRoutingInspection(mutateSemanticInspection(t, []any{"context"}, nil, true))
	if err != nil || got.Context != nil || got.Agents[1].FilePath != nil || got.Models[1].API != nil || got.Diagnostics[1].Path != nil {
		t.Fatalf("optional omission = %#v, err=%v", got, err)
	}
}

func TestDecodeModelRoutingInspectionSemanticsTables(t *testing.T) {
	rejectSemanticPaths(t, [][]any{{"contract"}, {"targets"}, {"assignments"}, {"agents"}, {"providers"}, {"models"}, {"diagnostics"}, {"context", "cwd"}, {"context", "global", "target"}, {"targets", "global", "provenance"}, {"targets", "global", "assignments"}, {"targets", "global", "provenance", "source"}, {"assignments", "worker", "inheritModel"}, {"assignments", "worker", "inheritThinking"}, {"agents", 0, "source"}, {"models", 0, "canonicalId"}, {"models", 0, "authenticated"}, {"models", 0, "capabilities"}, {"models", 0, "capabilities", "input"}, {"diagnostics", 0, "severity"}}, nil, true)
	rejectSemanticPaths(t, [][]any{{"contract"}, {"context", "target"}, {"context", "global", "target"}, {"targets", "global", "provenance", "target"}, {"targets", "global", "provenance", "source"}, {"targets", "global", "provenance", "status"}, {"assignments", "worker", "thinking"}, {"agents", 0, "source"}, {"models", 0, "operational"}, {"models", 0, "availability"}, {"models", 1, "authenticated"}, {"diagnostics", 0, "severity"}}, "bogus", false)
	rejectSemanticPaths(t, [][]any{{"agents", 0, "configurable"}, {"models", 0, "catalog"}}, false, false)
	rejectSemanticPaths(t, [][]any{{"context"}, {"assignments", "worker", "model"}, {"assignments", "worker", "thinking"}, {"agents", 0, "filePath"}, {"agents", 0, "assignment"}, {"models", 0, "api"}, {"models", 0, "supportedThinkingLevels"}, {"models", 0, "capabilities", "contextWindow"}, {"models", 0, "capabilities", "maxTokens"}, {"models", 0, "capabilities", "thinkingLevels"}, {"diagnostics", 0, "path"}, {"targets"}, {"assignments"}, {"agents"}, {"providers"}, {"models"}, {"diagnostics"}, {"targets", "global", "assignments"}, {"models", 0, "capabilities", "input"}}, nil, false)
	body := semanticInspectionFixture(t)
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	targets := document["targets"].(map[string]any)
	targets["other"], targets["global"] = targets["global"], nil
	delete(targets, "global")
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeModelRoutingInspection(body); err == nil {
		t.Fatal("accepted unknown target key")
	}
	if _, err := decodeModelRoutingInspection([]byte(`{"`)); err == nil {
		t.Fatal("accepted malformed JSON")
	}
	input := append([]byte(nil), semanticInspectionFixture(t)...)
	first, err := decodeModelRoutingInspection(input)
	if err != nil {
		t.Fatal(err)
	}
	for i := range input {
		input[i] = 'x'
	}
	second, err := decodeModelRoutingInspection(semanticInspectionFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	first.Agents[0].Name = "changed"
	first.Targets[ModelRoutingTargetGlobal].Assignments["new"] = ModelRoutingAssignment{}
	first.Models[0].Capabilities.Input = append(first.Models[0].Capabilities.Input, "changed")
	if second.Agents[0].Name != "" || len(second.Targets[ModelRoutingTargetGlobal].Assignments) != 0 || len(second.Models[0].Capabilities.Input) != 0 {
		t.Fatal("decoded results alias input or another decode")
	}
}
