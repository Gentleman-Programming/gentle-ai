package pi

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseGetAvailableModelsResponse_IgnoresUnrelatedEvents(t *testing.T) {
	jsonl := strings.NewReader(strings.Join([]string{
		`{"type":"event","name":"boot"}`,
		`{"type":"response","command":"other","success":true,"data":{}}`,
		`{"type":"response","command":"get_available_models","success":true,"data":{"models":[{"provider":"openai","id":"gpt-5","name":"GPT-5","reasoning":true,"input":["text","image"],"contextWindow":200000,"maxTokens":32000,"cost":{"input":1.25,"output":10.00,"cacheRead":0.10,"cacheWrite":1.00}}]}}`,
	}, "\n"))

	models, err := parseGetAvailableModelsResponse(jsonl)
	if err != nil {
		t.Fatalf("parseGetAvailableModelsResponse() error = %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].Provider != "openai" || models[0].ID != "gpt-5" {
		t.Fatalf("unexpected model parsed: %+v", models[0])
	}
	if len(models[0].Input) != 2 || models[0].Input[0] != "text" || models[0].Input[1] != "image" {
		t.Fatalf("unexpected input modalities: %+v", models[0].Input)
	}
}

func TestParseGetAvailableModelsResponse_CommandFailure(t *testing.T) {
	jsonl := strings.NewReader(`{"type":"response","command":"get_available_models","success":false,"error":"unauthorized"}`)

	_, err := parseGetAvailableModelsResponse(jsonl)
	if err == nil {
		t.Fatal("expected error for unsuccessful get_available_models response")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "unauthorized")
	}
}

func TestParseGetAvailableModelsResponse_ExtensionUIOnlyRealOutput(t *testing.T) {
	jsonl := strings.NewReader(strings.Join([]string{
		"mise WARN  missing: go@1.26.2 python@3.14.4",
		`{"type":"extension_ui_request","id":"84d7b971-0b9a-4659-b500-b7f0c7e4ed24","method":"setWidget","widgetKey":"btw"}`,
		`{"type":"extension_ui_request","id":"058ed9ae-be3a-4a3f-bb5a-8ea4131b2178","method":"setStatus","statusKey":"plannotator"}`,
		`{"type":"extension_ui_request","id":"d9f33feb-47be-42f2-bde0-d738a4aa0017","method":"setWidget","widgetKey":"plannotator-progress"}`,
		`{"type":"extension_ui_request","id":"54d7e574-5340-44da-856c-1906e809e931","method":"setWidget","widgetKey":"subagent-async"}`,
		`{"type":"extension_ui_request","id":"7fb5773d-d499-4b0d-b710-98314cb58111","method":"setStatus","statusKey":"engram","statusText":"..."}`,
		`{"type":"extension_ui_request","id":"633c3ee0-e596-4f99-a2e0-eb31ef01d89c","method":"setStatus","statusKey":"engram","statusText":"..."}`,
		`{"type":"extension_ui_request","id":"5538e567-185d-46a1-ac9b-cb39bee147b3","method":"notify","message":"Engram has relevant memory for this project. Use mem_context or mem_search when useful.","notifyType":"info"}`,
	}, "\n"))

	_, err := parseGetAvailableModelsResponse(jsonl)
	if err == nil {
		t.Fatal("expected error when only extension UI events are emitted")
	}
	if !strings.Contains(err.Error(), "did not return model catalog") {
		t.Fatalf("error = %q, want to contain %q", err.Error(), "did not return model catalog")
	}
}

func TestParseGetAvailableModelsResponse_ExtensionEventsBeforeCatalogResponse(t *testing.T) {
	jsonl := strings.NewReader(strings.Join([]string{
		"mise WARN  missing: go@1.26.2 python@3.14.4",
		`{"type":"extension_ui_request","id":"84d7b971-0b9a-4659-b500-b7f0c7e4ed24","method":"setWidget","widgetKey":"btw"}`,
		`{"type":"response","command":"get_available_models","success":true,"data":{"models":[{"provider":"openai","id":"gpt-5","name":"GPT-5"}]}}`,
	}, "\n"))

	models, err := parseGetAvailableModelsResponse(jsonl)
	if err != nil {
		t.Fatalf("parseGetAvailableModelsResponse() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(models))
	}
	if models[0].Provider != "openai" || models[0].ID != "gpt-5" {
		t.Fatalf("unexpected model parsed: %+v", models[0])
	}
}

func TestDefaultPiRPCModelsTimeout_AllowsExtensionStartupNoise(t *testing.T) {
	if defaultPiRPCModelsTimeout < 20*time.Second {
		t.Fatalf("defaultPiRPCModelsTimeout = %v, want >= 20s to tolerate extension startup noise", defaultPiRPCModelsTimeout)
	}
}

func TestBuildCatalogFromPIModels_NormalizesProvidersAndCosts(t *testing.T) {
	catalog := buildCatalogFromPIModels([]piRPCModel{
		{
			Provider:      "openai",
			ID:            "gpt-5",
			Name:          "GPT-5",
			Reasoning:     true,
			ContextWindow: 200000,
			MaxTokens:     32000,
			Cost: piRPCModelCost{
				Input:      1.25,
				Output:     10.00,
				CacheRead:  0.10,
				CacheWrite: 1.00,
			},
		},
		{
			Provider: "anthropic",
			ID:       "claude-sonnet-4",
			Name:     "Claude Sonnet 4",
			Cost: piRPCModelCost{
				Input:  3,
				Output: 15,
			},
		},
	})

	if len(catalog.AvailableProviderIDs) != 2 {
		t.Fatalf("len(AvailableProviderIDs) = %d, want 2", len(catalog.AvailableProviderIDs))
	}
	if _, ok := catalog.Providers["openai"]; !ok {
		t.Fatal("expected openai provider in catalog")
	}

	openAIModel := catalog.Providers["openai"].Models["gpt-5"]
	if openAIModel.Cost.CacheRead != 0.10 || openAIModel.Cost.CacheWrite != 1.00 {
		t.Fatalf("cache costs not normalized: %+v", openAIModel.Cost)
	}
	if openAIModel.ContextWindow != 200000 || openAIModel.MaxTokens != 32000 {
		t.Fatalf("limits not normalized: %+v", openAIModel)
	}
}

func TestUserFacingRPCModelLoadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "killed process is actionable",
			err:  errors.New("PI RPC command failed: signal: killed"),
			want: "terminated before it returned models",
		},
		{
			name: "deadline exceeded is actionable",
			err:  context.DeadlineExceeded,
			want: "terminated before it returned models",
		},
		{
			name: "missing pi binary guidance",
			err:  exec.ErrNotFound,
			want: "not found on PATH",
		},
		{
			name: "permission guidance",
			err:  os.ErrPermission,
			want: "not executable due to permissions",
		},
		{
			name: "extension-only rpc output guidance",
			err:  errors.New("PI RPC did not return model catalog; received extension UI events only"),
			want: "extensions emitted UI events only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UserFacingRPCModelLoadError(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("UserFacingRPCModelLoadError() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}
