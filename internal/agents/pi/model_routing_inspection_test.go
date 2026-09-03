package pi

import (
	"encoding/json"
	"reflect"
	"testing"
)

const richModelRoutingInspection = `{
  "contract": "gentle-pi.model-routing/v1",
  "context": {"cwd": "", "agentDir": "/agents", "target": "project", "global": {"target": "global", "configPath": "", "profilePath": ""}, "project": {"target": "project", "configPath": "/project/config", "profilePath": "/project/profile"}},
  "targets": {
    "global": {"provenance": {"target": "global", "source": "global", "status": "valid", "configPath": "", "profilePath": ""}, "assignments": {}},
    "project": {"provenance": {"target": "project", "source": "missing", "status": "missing", "configPath": "", "profilePath": ""}, "assignments": {"worker": {"model": "", "thinking": "off", "inheritModel": false, "inheritThinking": false}}}
  },
  "assignments": {"worker": {"model": "provider/model", "thinking": "xhigh", "inheritModel": true, "inheritThinking": false}, "empty": {"inheritModel": false, "inheritThinking": false}},
  "agents": [{"name": "", "source": "project", "filePath": "", "configurable": false, "assignment": {"inheritModel": false, "inheritThinking": true}}, {"name": "built-in", "source": "builtin", "configurable": true}],
  "providers": [],
  "models": [
    {"canonicalId": "", "provider": "", "modelId": "", "name": "", "api": "", "catalog": false, "configured": false, "authConfigured": false, "available": false, "authenticated": false, "operational": "unavailable", "availability": "configured", "reasoning": false, "supportedThinkingLevels": ["off", "minimal", "low", "medium", "high", "xhigh", "max"], "capabilities": {"reasoning": false, "input": [], "contextWindow": 0, "maxTokens": 0, "thinkingLevels": ["off", "minimal", "low", "medium", "high", "xhigh", "max"]}},
    {"canonicalId": "provider/model", "provider": "provider", "modelId": "model", "name": "Model", "catalog": true, "configured": true, "authConfigured": true, "available": true, "authenticated": "unknown", "operational": "authenticated", "availability": "catalog", "reasoning": true, "capabilities": {"reasoning": true, "input": ["text", "image"]}}
  ],
  "diagnostics": [{"code": "", "message": "", "severity": "error", "path": ""}, {"code": "info", "message": "informational", "severity": "info"}]
}`

func TestModelRoutingInspectionDecodesProviderData(t *testing.T) {
	var got ModelRoutingInspection
	if err := json.Unmarshal([]byte(richModelRoutingInspection), &got); err != nil {
		t.Fatal(err)
	}
	if got.Contract != "gentle-pi.model-routing/v1" || got.Context == nil || got.Context.CWD != "" || got.Context.AgentDir != "/agents" || got.Context.Target != ModelRoutingTargetProject {
		t.Fatalf("inspection/context = %#v", got)
	}
	if got.Context.Global.Target != ModelRoutingTargetGlobal || got.Context.Global.ConfigPath != "" || got.Context.Project.ProfilePath != "/project/profile" {
		t.Fatalf("context targets = %#v", got.Context)
	}
	if got.Targets[ModelRoutingTargetProject].Provenance.Source != ModelRoutingProvenanceSourceMissing || got.Targets[ModelRoutingTargetGlobal].Provenance.Status != ModelRoutingProvenanceStatusValid {
		t.Fatalf("provenance = %#v", got.Targets)
	}
	worker := got.Assignments["worker"]
	if worker.Model == nil || *worker.Model != "provider/model" || worker.Thinking == nil || *worker.Thinking != ModelRoutingThinkingXHigh || !worker.InheritModel || worker.InheritThinking {
		t.Fatalf("worker assignment = %#v", worker)
	}
	emptyAssignment := got.Assignments["empty"]
	if emptyAssignment.Model != nil || emptyAssignment.Thinking != nil || emptyAssignment.InheritModel || emptyAssignment.InheritThinking || got.Providers == nil || len(got.Providers) != 0 {
		t.Fatalf("required empty/false values were not preserved: %#v", got)
	}
	if got.Agents[0].Source != ModelRoutingAgentSourceProject || got.Agents[0].FilePath == nil || *got.Agents[0].FilePath != "" || got.Agents[0].Assignment == nil || got.Agents[0].Configurable {
		t.Fatalf("project agent = %#v", got.Agents[0])
	}
	if got.Agents[1].Source != ModelRoutingAgentSourceBuiltin || got.Agents[1].FilePath != nil || got.Agents[1].Assignment != nil {
		t.Fatalf("optional agent fields = %#v", got.Agents[1])
	}
	model := got.Models[0]
	if model.API == nil || *model.API != "" || model.Catalog || model.Configured || model.AuthConfigured || model.Available || model.Authenticated.Value || model.Authenticated.Unknown || model.Reasoning || model.Capabilities.Input == nil || len(model.Capabilities.Input) != 0 {
		t.Fatalf("false/empty model values = %#v", model)
	}
	if model.Operational != ModelRoutingOperationalUnavailable || model.Availability != ModelRoutingAvailabilityConfigured || len(model.SupportedThinkingLevels) != 7 || model.Capabilities.ContextWindow == nil || *model.Capabilities.ContextWindow != 0 || model.Capabilities.MaxTokens == nil || *model.Capabilities.MaxTokens != 0 {
		t.Fatalf("typed model fields = %#v", model)
	}
	if !got.Models[1].Authenticated.Unknown || got.Models[1].Authenticated.Value || got.Models[1].Operational != ModelRoutingOperationalAuthenticated || got.Models[1].Availability != ModelRoutingAvailabilityCatalog || got.Models[1].Capabilities.ContextWindow != nil || got.Models[1].Capabilities.MaxTokens != nil || got.Models[1].Capabilities.ThinkingLevels != nil || got.Models[1].SupportedThinkingLevels != nil {
		t.Fatalf("unknown/optional model fields = %#v", got.Models[1])
	}
	if got.Diagnostics[0].Severity != ModelRoutingDiagnosticSeverityError || got.Diagnostics[0].Path == nil || *got.Diagnostics[0].Path != "" || got.Diagnostics[1].Severity != ModelRoutingDiagnosticSeverityInfo || got.Diagnostics[1].Path != nil {
		t.Fatalf("diagnostics = %#v", got.Diagnostics)
	}
}

func TestModelRoutingInspectionOptionalContextOmitted(t *testing.T) {
	var got ModelRoutingInspection
	payload := `{"contract":"","targets":{},"assignments":{},"agents":[],"providers":[],"models":[],"diagnostics":[]}`
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatal(err)
	}
	if got.Context != nil {
		t.Fatalf("context = %#v, want nil when omitted", got.Context)
	}
}

func TestModelRoutingEnumConstants(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"target", []string{string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject)}, []string{"global", "project"}},
		{"thinking", []string{string(ModelRoutingThinkingOff), string(ModelRoutingThinkingMinimal), string(ModelRoutingThinkingLow), string(ModelRoutingThinkingMedium), string(ModelRoutingThinkingHigh), string(ModelRoutingThinkingXHigh), string(ModelRoutingThinkingMax)}, []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}},
		{"diagnostic", []string{string(ModelRoutingDiagnosticSeverityError), string(ModelRoutingDiagnosticSeverityWarning), string(ModelRoutingDiagnosticSeverityInfo)}, []string{"error", "warning", "info"}},
		{"agent source", []string{string(ModelRoutingAgentSourceProject), string(ModelRoutingAgentSourceUser), string(ModelRoutingAgentSourceBuiltin)}, []string{"project", "user", "builtin"}},
		{"provenance source", []string{string(ModelRoutingProvenanceSourceGlobal), string(ModelRoutingProvenanceSourceProject), string(ModelRoutingProvenanceSourceMissing), string(ModelRoutingProvenanceSourceInvalid)}, []string{"global", "project", "missing", "invalid"}},
		{"provenance status", []string{string(ModelRoutingProvenanceStatusValid), string(ModelRoutingProvenanceStatusMissing), string(ModelRoutingProvenanceStatusInvalid)}, []string{"valid", "missing", "invalid"}},
		{"operational", []string{string(ModelRoutingOperationalAuthenticated), string(ModelRoutingOperationalUnavailable), string(ModelRoutingOperationalUnknown)}, []string{"authenticated", "unavailable", "unknown"}},
		{"availability", []string{string(ModelRoutingAvailabilityCatalog), string(ModelRoutingAvailabilityConfigured), string(ModelRoutingAvailabilityAuthenticated), string(ModelRoutingAvailabilityUnknown)}, []string{"catalog", "configured", "authenticated", "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(tt.got, tt.want) {
				t.Fatalf("constants = %#v, want %#v", tt.got, tt.want)
			}
		})
	}
}

func TestModelRoutingAuthenticatedUnmarshal(t *testing.T) {
	for _, tt := range []struct {
		name, input string
		want        ModelRoutingAuthenticated
	}{
		{"true", `true`, ModelRoutingAuthenticated{Value: true}},
		{"false", `false`, ModelRoutingAuthenticated{}},
		{"unknown", `"unknown"`, ModelRoutingAuthenticated{Unknown: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got ModelRoutingAuthenticated
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("authenticated = %#v, want %#v", got, tt.want)
			}
		})
	}
	var got ModelRoutingAuthenticated
	if err := json.Unmarshal([]byte(`"unknown"`), &got); err != nil || !got.Unknown || got.Value {
		t.Fatalf("unknown = %#v, error = %v", got, err)
	}
	if err := json.Unmarshal([]byte(`false`), &got); err != nil || got.Unknown || got.Value {
		t.Fatalf("false reset = %#v, error = %v", got, err)
	}
	if err := json.Unmarshal([]byte(`true`), &got); err != nil || !got.Value || got.Unknown {
		t.Fatalf("true reset = %#v, error = %v", got, err)
	}
	for _, input := range []string{`null`, `1`, `"true"`, `"unknown "`, `{}`} {
		if err := json.Unmarshal([]byte(input), &got); err == nil {
			t.Errorf("accepted invalid authenticated value %s", input)
		}
	}
}

func TestModelRoutingInspectionUsesDecoderOwnedCollections(t *testing.T) {
	var first, second ModelRoutingInspection
	for _, value := range []*ModelRoutingInspection{&first, &second} {
		if err := json.Unmarshal([]byte(richModelRoutingInspection), value); err != nil {
			t.Fatal(err)
		}
	}
	first.Agents[0].Name = "changed"
	first.Targets[ModelRoutingTargetGlobal].Assignments["new"] = ModelRoutingAssignment{}
	first.Models[0].Capabilities.Input = append(first.Models[0].Capabilities.Input, "changed")
	if second.Agents[0].Name != "" || len(second.Targets[ModelRoutingTargetGlobal].Assignments) != 0 || len(second.Models[0].Capabilities.Input) != 0 {
		t.Fatal("separate standard decoder allocations were shared")
	}
}
