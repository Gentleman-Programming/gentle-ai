package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const openCodeNativeFallbackAxis = "opencode-native-fallback"

func init() {
	RegisterAxis(Axis{
		Name:     openCodeNativeFallbackAxis,
		Title:    "OpenCode native fallback model persistence and delegation",
		BlackBox: false,
		Properties: []string{
			"Seeds isolated install state, drives candidate sync, then invokes gentle-orchestrator through OpenCode's native task boundary without a call-site model override.",
			"Requires a PATH-visible pinned OpenCode 1.18.4 runtime; it is unsupported rather than silently substituting another runtime.",
		},
		Journeys: openCodeNativeFallbackJourneys,
	})
}

func openCodeNativeFallbackJourneys() []Journey {
	return []Journey{{
		ID:     "oc01-native-fallback-model-persistence",
		Title:  "Sync persists and gentle-orchestrator delegates to native general and explore roles",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2104",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "sync persists and routes native fallback assignments", Skip: pinnedOpenCodeUnavailable, Composite: syncNativeFallbackAssignments},
		},
	}}
}

func pinnedOpenCodeUnavailable(*Sandbox) string {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return "opencode is unavailable"
	}
	output, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil || !strings.Contains(string(output), "1.18.4") {
		return "opencode 1.18.4 is unavailable"
	}
	return ""
}

func syncNativeFallbackAssignments(r *journeyRun) error {
	home := r.sandbox.Home
	configDir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	fixture := &nativeFallbackFixture{}
	server := httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	defer server.Close()
	config := map[string]any{
		"model": "fixture/root",
		"provider": map[string]any{"fixture": map[string]any{
			"npm":     "@ai-sdk/openai-compatible",
			"options": map[string]any{"baseURL": server.URL + "/v1", "apiKey": "fixture"},
			"models":  map[string]any{"root": map[string]any{"name": "Root", "tool_call": true}, "general": map[string]any{"name": "General", "tool_call": true}, "explore": map[string]any{"name": "Explore", "tool_call": true}},
		}},
		"plugin": []any{},
	}
	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), encodedConfig, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(configDir, "AGENTS.md"), []byte("# Isolated benchmark runtime\n"), 0o644); err != nil {
		return err
	}
	state := map[string]any{
		"installed_agents":     []string{"opencode"},
		"selection_configured": true,
		"components":           []string{"sdd"},
		"preset":               "full-gentleman",
		"sdd_mode":             "multi",
		"persona":              "neutral",
		"model_assignments": map[string]any{
			"sdd-mid":     map[string]string{"provider_id": "fixture", "model_id": "general", "effort": "high"},
			"sdd-explore": map[string]string{"provider_id": "fixture", "model_id": "explore", "effort": "low"},
		},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	stateDir := filepath.Join(home, ".gentle-ai")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	observation := r.run([]string{"sync"}, false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("candidate sync failed: %s", firstLine(observation.Stderr))
	}
	settings, err := os.ReadFile(filepath.Join(configDir, "opencode.json"))
	if err != nil {
		return err
	}
	var decoded struct {
		Agent map[string]struct {
			Model   string `json:"model"`
			Variant string `json:"variant"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(settings, &decoded); err != nil {
		return err
	}
	for role, want := range map[string]struct{ model, variant string }{
		"general": {"fixture/general", "high"},
		"explore": {"fixture/explore", "low"},
	} {
		got := decoded.Agent[role]
		if got.Model != want.model || got.Variant != want.variant {
			return fmt.Errorf("%s persisted as %#v, want model %q variant %q", role, got, want.model, want.variant)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "opencode", "run", "--pure", "--format", "json", "--agent", "gentle-orchestrator", "--dir", r.sandbox.Repo, "delegate both native fallback roles")
	command.Env = append(r.sandbox.env(),
		"OPENCODE_CONFIG_DIR="+configDir,
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
		"OPENCODE_DISABLE_EXTERNAL_SKILLS=1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
		"OPENCODE_FAST_BOOT=1",
		"OPENCODE_PURE=1",
	)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	err = command.Run()
	observation = Observation{Args: []string{"opencode", "run", "--agent", "gentle-orchestrator"}, ExitCode: 0, Stdout: output.String(), StdoutCaptured: true, StderrCaptured: true}
	if err != nil {
		observation.ExitCode = 1
		observation.Stderr = output.String()
	}
	record := r.accumulator.observe(r.step, observation, nil, true)
	r.accumulator.records = append(r.accumulator.records, record)
	if err != nil {
		return fmt.Errorf("gentle-orchestrator native fallback delegation: %w: %s", err, output.String())
	}
	return fixture.assertComplete()
}

type nativeFallbackFixture struct {
	next      int
	models    []string
	tasks     []string
	roles     []string
	responses []string
}

type nativeFallbackRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
}

func (f *nativeFallbackFixture) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	var input nativeFallbackRequest
	if err := json.NewDecoder(io.LimitReader(request.Body, 1<<20)).Decode(&input); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	f.models = append(f.models, input.Model)
	if input.Model == "root" && strings.Contains(nativeFallbackRequestText(input), "You are a title generator") {
		writeNativeFallbackText(writer, "fixture title")
		return
	}
	for _, role := range []string{"general", "explore"} {
		if input.Model == role || input.Model == "fixture/"+role {
			f.roles = append(f.roles, role)
			f.responses = append(f.responses, "ROLE_RESPONSE "+role)
			writeNativeFallbackText(writer, "ROLE_RESPONSE "+role)
			return
		}
	}
	if f.next == 0 {
		f.next = 1
		f.tasks = append(f.tasks, "general")
		writeNativeFallbackTask(writer, "general")
		return
	}
	if f.next == 1 {
		f.next = 2
		f.tasks = append(f.tasks, "explore")
		writeNativeFallbackTask(writer, "explore")
		return
	}
	writeNativeFallbackText(writer, "native fallback delegation complete")
}

func (f *nativeFallbackFixture) assertComplete() error {
	if strings.Join(f.tasks, ",") != "general,explore" {
		return fmt.Errorf("native task metadata = %v, want [general explore]", f.tasks)
	}
	if strings.Join(f.roles, ",") != "general,explore" {
		return fmt.Errorf("native role request models = %v (all requests %v), want [general explore]", f.roles, f.models)
	}
	if strings.Join(f.responses, ",") != "ROLE_RESPONSE general,ROLE_RESPONSE explore" {
		return fmt.Errorf("native role responses = %v", f.responses)
	}
	return nil
}

func nativeFallbackRequestText(request nativeFallbackRequest) string {
	var text strings.Builder
	for _, message := range request.Messages {
		text.WriteString(fmt.Sprint(message.Content))
	}
	return text.String()
}

func writeNativeFallbackTask(writer http.ResponseWriter, role string) {
	arguments, _ := json.Marshal(map[string]string{"description": "native fallback proof", "subagent_type": role, "prompt": "return ROLE_RESPONSE " + role})
	toolCall := map[string]any{"index": 0, "id": "call-" + role, "type": "function", "function": map[string]any{"name": "task", "arguments": string(arguments)}}
	writeNativeFallbackChunks(writer, []any{map[string]any{"id": "native", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{toolCall}}, "finish_reason": nil}}}, map[string]any{"id": "native", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}}})
}

func writeNativeFallbackText(writer http.ResponseWriter, text string) {
	writeNativeFallbackChunks(writer, []any{map[string]any{"id": "native", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": text}, "finish_reason": nil}}}, map[string]any{"id": "native", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}}})
}

func writeNativeFallbackChunks(writer http.ResponseWriter, chunks []any) {
	writer.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		encoded, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(writer, "data: %s\n\n", encoded)
	}
	_, _ = io.WriteString(writer, "data: [DONE]\n\n")
}
