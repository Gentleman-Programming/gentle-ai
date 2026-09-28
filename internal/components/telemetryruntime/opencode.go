// Package telemetryruntime owns native runtime event adapters and managed hooks.
package telemetryruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/model"
	"github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

const OpenCodeSchema = "gentle-ai.telemetry-opencode/v1"
const openCodeConfigMaxBytes = 1 << 20

var errEnvelope = errors.New("invalid OpenCode runtime envelope")

// Only these source fields may cross stdin. IDs, parts, paths, error messages,
// prompts and tool data are deliberately not part of the bridge contract.
// V1 source: https://unpkg.com/@opencode-ai/sdk@1.18.30/dist/gen/types.gen.d.ts
// The plugin's published 1.18.30 entrypoint imports this V1, not /v2.
type openCodeInfo struct {
	Role string `json:"role"`
	Time struct {
		Created   *int64 `json:"created"`
		Completed *int64 `json:"completed,omitempty"`
	} `json:"time"`
	ProviderID     string          `json:"providerID"`
	ModelID        string          `json:"modelID"`
	Agent          string          `json:"agent,omitempty"`
	SelectedEffort json.RawMessage `json:"selectedEffort,omitempty"`
	Summary        bool            `json:"summary,omitempty"`
	Tokens         *struct {
		Input     *json.RawMessage `json:"input,omitempty"`
		Output    *json.RawMessage `json:"output,omitempty"`
		Reasoning *json.RawMessage `json:"reasoning,omitempty"`
		Cache     *struct {
			Read  *json.RawMessage `json:"read,omitempty"`
			Write *json.RawMessage `json:"write,omitempty"`
		} `json:"cache,omitempty"`
	} `json:"tokens,omitempty"`
	Error *struct {
		Name string `json:"name"`
		Data struct {
			StatusCode *json.RawMessage `json:"statusCode,omitempty"`
		} `json:"data"`
	} `json:"error,omitempty"`
}

func allowed(home string, getenv func(string) string) bool {
	if !telemetry.Decide(getenv, telemetry.State{Enabled: true}).Enabled {
		return false
	}
	state, err := telemetry.LoadPolicyState(home)
	return err == nil && state.NoticeShown && telemetry.Decide(getenv, state).Enabled
}

// SendOpenCode normalizes one bounded source observation, then directly sends
// once. No batch identity, local intake, persistence or retry exists. Every
// failure is discarded without exposing source content or error details.
// Like SendRuntime, context bounds HTTP, not arbitrary caller-owned input; the
// CLI supplies bytes already read under its bounded stdin deadline.
func SendOpenCode(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) string {
	decision, err := sendOpenCode(ctx, home, getenv, input, client)
	if err != nil {
		return "discarded"
	}
	return decision
}

func sendOpenCode(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) (string, error) {
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	data, err := io.ReadAll(io.LimitReader(input, telemetry.RuntimeMaxBytes+1))
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	if err != nil || len(data) > telemetry.RuntimeMaxBytes {
		return "", errEnvelope
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if strictObjectKeys(d, 0) != nil {
		return "", errEnvelope
	}
	if _, err = d.Token(); err != io.EOF {
		return "", errEnvelope
	}
	var envelope struct {
		Schema string       `json:"schema"`
		Info   openCodeInfo `json:"info"`
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&envelope) != nil || (envelope.Schema != OpenCodeSchema && envelope.Schema != "gentle-ai.telemetry-opencode/v2") {
		return "", errEnvelope
	}
	if envelope.Schema == OpenCodeSchema && envelope.Info.SelectedEffort != nil {
		return "", errEnvelope
	}
	if envelope.Info.Time.Completed != nil && envelope.Info.Time.Created == nil {
		return "", errEnvelope
	}
	event, _ := json.Marshal(struct {
		Type       string `json:"type"`
		Properties struct {
			Info openCodeInfo `json:"info"`
		} `json:"properties"`
	}{Type: "message.updated", Properties: struct {
		Info openCodeInfo `json:"info"`
	}{envelope.Info}})
	observation, err := telemetry.NormalizeOpenCode(bytes.NewReader(event))
	if err != nil {
		return "", errEnvelope
	}
	if observation == nil {
		return "ignored", nil
	}
	if envelope.Schema == OpenCodeSchema {
		root := readOpenCodeConfig(home)
		assignment := openCodeAssignment(root, observation.Agent)
		if assignment.Effort == "" {
			provider, modelID := envelope.Info.ProviderID, envelope.Info.ModelID
			if provider == "" || modelID == "" {
				provider, modelID = assignment.ProviderID, assignment.ModelID
			}
			assignment.Effort = openCodeOptionEffort(root, observation.Agent, provider, modelID)
		}
		applyOpenCodeAssignment(&observation.Row, assignment)
	} else {
		// V2 start observes the runtime selection, not provider response identity.
		if observation.Row.ModelEvidence != "unknown" {
			observation.Row.ModelEvidence = "selected"
		}
		var effort string
		if len(envelope.Info.SelectedEffort) > 0 && json.Unmarshal(envelope.Info.SelectedEffort, &effort) != nil {
			return "", errEnvelope
		}
		if telemetry.RuntimeEffortAllowed(effort) {
			observation.Row.SelectedEffort = effort
		}
	}
	batch, err := json.Marshal(telemetry.RuntimeBatch{Schema: telemetry.RuntimeSchema, Registry: json.RawMessage("1"), Host: "opencode", Rows: []telemetry.RuntimeRow{observation.Row}})
	if err != nil {
		return "", errEnvelope
	}
	return telemetry.SendRuntime(ctx, home, getenv, bytes.NewReader(batch), client), nil
}

func applyOpenCodeAssignment(row *telemetry.RuntimeRow, assignment model.ModelAssignment) {
	if telemetry.RuntimeEffortAllowed(assignment.Effort) {
		row.SelectedEffort = assignment.Effort
	}
	if row.ModelEvidence == "unknown" && assignment.ProviderID != "" && assignment.ModelID != "" {
		row.Model = telemetry.NormalizeRuntimeModel(assignment.ProviderID, assignment.ModelID)
		row.ModelEvidence = "selected"
	}
}

func openCodeAssignment(root map[string]any, agent string) model.ModelAssignment {
	if agent == "" || root == nil {
		return model.ModelAssignment{}
	}
	return opencode.ConfigAssignments(root)[agent].Assignment
}

// readOpenCodeConfig returns nil for a missing, oversized, or invalid file so
// configuration never blocks the send.
func readOpenCodeConfig(home string) map[string]any {
	path := opencode.DefaultSettingsPathForHome(home)
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, openCodeConfigMaxBytes+1))
	if err != nil || len(data) > openCodeConfigMaxBytes {
		return nil
	}
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		return nil
	}
	return root
}

// openCodeOptionEfforts are the concrete levels also accepted from the V2
// plugin's selected variant; contract meta values are never configuration.
var openCodeOptionEfforts = map[string]bool{"off": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}

// openCodeOptionEffort resolves the documented reasoningEffort option used when
// no variant is configured: agent options (https://opencode.ai/docs/agents/)
// override the model's global options (https://opencode.ai/docs/models/).
// Only a concrete level is returned; anything else stays unavailable. Any
// configured agent variant, even one without a model, can override these
// options, so its presence withholds the fallback.
func openCodeOptionEffort(root map[string]any, agent, provider, modelID string) string {
	agents, _ := root["agent"].(map[string]any)
	definition, _ := agents[agent].(map[string]any)
	if _, hasVariant := definition["variant"]; hasVariant {
		return ""
	}
	if effort, _ := definition["reasoningEffort"].(string); agent != "" && openCodeOptionEfforts[effort] {
		return effort
	}
	providers, _ := root["provider"].(map[string]any)
	providerConfig, _ := providers[provider].(map[string]any)
	models, _ := providerConfig["models"].(map[string]any)
	modelConfig, _ := models[modelID].(map[string]any)
	options, _ := modelConfig["options"].(map[string]any)
	if effort, _ := options["reasoningEffort"].(string); provider != "" && modelID != "" && openCodeOptionEfforts[effort] {
		return effort
	}
	return ""
}

// Reject duplicate keys and case aliases before struct decoding. All accepted
// envelope keys use their exact documented spelling, even in discarded inputs.
func strictObjectKeys(d *json.Decoder, depth int) error {
	if depth > 8 {
		return errEnvelope
	}
	token, err := d.Token()
	if err != nil {
		return errEnvelope
	}
	if s, ok := token.(string); ok && len(s) > 256 {
		return errEnvelope
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' {
		return errEnvelope
	}
	keys := map[string]bool{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return errEnvelope
		}
		s, ok := key.(string)
		if !ok || keys[s] {
			return errEnvelope
		}
		switch s {
		case "selectedEffort", "schema", "info", "role", "time", "created", "completed", "providerID", "modelID", "agent", "summary", "tokens", "input", "output", "reasoning", "cache", "read", "write", "error", "name", "data", "statusCode":
		default:
			return errEnvelope
		}
		keys[s] = true
		if err := strictObjectKeys(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
