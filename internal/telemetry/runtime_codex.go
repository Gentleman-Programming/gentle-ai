package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
)

const CodexMaxBytes = RuntimeMaxBytes
const CodexTranscriptHeadMaxBytes = 65536
const CodexTranscriptMaxBytes = 262144

var errCodex = errors.New("invalid Codex runtime event")

// CodexHook is local-only parsed source metadata. TranscriptPath is used only
// for the immediate bounded read and must never be serialized or logged.
type CodexHook struct {
	Event          string `json:"-"`
	AgentType      string `json:"-"`
	TranscriptPath string `json:"-"`
}

// CodexAssignment is a previously persisted Gentle AI selection. Missing or
// invalid fields remain unavailable; runtime evidence never invents defaults.
type CodexAssignment struct {
	Model  string
	Effort string
}

type CodexObservation struct {
	Row RuntimeRow `json:"row"`
}

// DecodeCodexHook admits only the documented Stop/SubagentStop input shape.
// Other hook events are ignored. Private identifiers, paths, and messages are
// validated but retained only in this local-only value.
func DecodeCodexHook(input io.Reader) (*CodexHook, error) {
	data, err := io.ReadAll(io.LimitReader(input, CodexMaxBytes+1))
	if err != nil || len(data) > CodexMaxBytes {
		return nil, errCodex
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := runtimeUnique(d); err != nil {
		return nil, errCodex
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errCodex
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil || root == nil {
		return nil, errCodex
	}
	event, ok := codexString(root["hook_event_name"], false)
	if !ok {
		return nil, errCodex
	}
	if event != "SubagentStop" && event != "Stop" {
		return nil, nil
	}
	allowed := map[string]bool{
		"session_id": true, "transcript_path": true, "cwd": true,
		"hook_event_name": true, "model": true, "permission_mode": true, "turn_id": true,
		"agent_id": true, "agent_type": true, "agent_transcript_path": true,
		"stop_hook_active": true, "last_assistant_message": true,
	}
	for key := range root {
		if !allowed[key] {
			return nil, errCodex
		}
	}
	for _, key := range []string{"session_id", "cwd", "model", "permission_mode", "turn_id"} {
		if _, ok := codexString(root[key], false); !ok {
			return nil, errCodex
		}
	}
	transcriptPath, ok := codexString(root["transcript_path"], true)
	if !ok {
		return nil, errCodex
	}
	if raw := root["stop_hook_active"]; raw != nil {
		var active bool
		if json.Unmarshal(raw, &active) != nil {
			return nil, errCodex
		}
	}
	if raw := root["last_assistant_message"]; raw != nil {
		if _, ok := codexString(raw, true); !ok {
			return nil, errCodex
		}
	}
	hook := &CodexHook{Event: event}
	if event == "SubagentStop" {
		for _, key := range []string{"agent_id", "agent_type"} {
			if _, ok := codexString(root[key], false); !ok {
				return nil, errCodex
			}
		}
		hook.AgentType, _ = codexString(root["agent_type"], false)
		agentTranscriptPath, ok := codexString(root["agent_transcript_path"], true)
		if !ok {
			return nil, errCodex
		}
		hook.TranscriptPath = agentTranscriptPath
	} else {
		hook.TranscriptPath = transcriptPath
	}
	return hook, nil
}

func codexString(raw json.RawMessage, nullable bool) (string, bool) {
	if raw == nil {
		return "", false
	}
	if bytes.Equal(raw, []byte("null")) {
		return "", nullable
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

// ResolveCodexAgentType derives a canonical class from the first transcript
// record only when the hook's agent_type is not already canonical. Source
// paths and all other session metadata remain local and are discarded.
func ResolveCodexAgentType(source CodexHook, transcriptHead io.Reader) (CodexHook, error) {
	if source.Event != "SubagentStop" || runtimeMember(source.AgentType, runtimeAgentClasses) {
		return source, nil
	}
	data, err := io.ReadAll(io.LimitReader(transcriptHead, CodexTranscriptHeadMaxBytes+1))
	if err != nil || len(data) > CodexTranscriptHeadMaxBytes {
		return source, errCodex
	}
	if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
		data = data[:newline]
	}
	var record map[string]json.RawMessage
	if json.Unmarshal(data, &record) != nil {
		return source, nil
	}
	kind, ok := codexString(record["type"], false)
	if !ok || kind != "session_meta" {
		return source, nil
	}
	var payload, sourceValue, subagent, threadSpawn map[string]json.RawMessage
	if json.Unmarshal(record["payload"], &payload) != nil ||
		json.Unmarshal(payload["source"], &sourceValue) != nil ||
		json.Unmarshal(sourceValue["subagent"], &subagent) != nil ||
		json.Unmarshal(subagent["thread_spawn"], &threadSpawn) != nil {
		return source, nil
	}
	agentPath, ok := codexString(threadSpawn["agent_path"], false)
	if !ok || agentPath == "" {
		return source, nil
	}
	candidate := strings.ReplaceAll(path.Base(agentPath), "_", "-")
	if runtimeMember(candidate, runtimeAgentClasses) {
		source.AgentType = candidate
	}
	return source, nil
}

// NormalizeCodex converts one decoded hook plus a bounded transcript tail into
// one sanitized observation. It performs no filesystem, network, persistence,
// logging, or lifetime deduplication.
func NormalizeCodex(source CodexHook, transcript io.Reader, selected CodexAssignment) (*CodexObservation, error) {
	data, err := io.ReadAll(io.LimitReader(transcript, CodexTranscriptMaxBytes+1))
	if err != nil || len(data) > CodexTranscriptMaxBytes {
		return nil, errCodex
	}
	model, effort, usage := codexTranscriptEvidence(data)
	row := RuntimeRow{
		Model:           RuntimeModel{Provider: "unknown", ID: "unknown"},
		ModelEvidence:   "unknown",
		AgentKind:       "custom",
		AgentClass:      "unknown",
		SelectedEffort:  codexEffort(selected.Effort),
		EffectiveEffort: effort,
		Launches:        json.RawMessage("1"),
		Responses:       json.RawMessage("1"),
		Input:           usage.Input,
		Output:          usage.Output,
		CacheRead:       usage.CacheRead,
		CacheCreation:   usage.CacheCreation,
		ReasoningTokens: usage.Reasoning,
		TotalTokens:     usage.Total,
		ErrorCategory:   "none",
		Duration: RuntimeDuration{
			Kind:          "unavailable",
			MeasuredCount: json.RawMessage("0"),
			SumMS:         json.RawMessage("null"),
		},
	}
	if source.Event == "Stop" {
		row.AgentKind, row.AgentClass = "orchestrator", "orchestrator"
		row.Launches = json.RawMessage("null")
	} else if runtimeMember(source.AgentType, runtimeAgentClasses) {
		row.AgentKind, row.AgentClass = "built_in", source.AgentType
	}
	if model.ID != "" {
		row.Model, row.ModelEvidence = model, "response"
	} else if chosen := codexModel(selected.Model); chosen.ID != "" {
		row.Model, row.ModelEvidence = chosen, "selected"
	}
	row.tokenObservations()
	return &CodexObservation{Row: row}, nil
}

func codexEffort(value string) string {
	if value == "none" {
		return "off"
	}
	if runtimeMember(value, runtimeEfforts) && value != "custom" && value != "unavailable" && value != "unsupported" {
		return value
	}
	return "unavailable"
}

func codexModel(value string) RuntimeModel {
	if value == "" {
		return RuntimeModel{}
	}
	model := RuntimeModel{Provider: "openai-codex", ID: value}
	if runtimeModelOK(model) {
		return model
	}
	return RuntimeModel{Provider: "custom", ID: "custom"}
}

type codexUsage struct {
	Input, Output, CacheRead, CacheCreation, Reasoning, Total json.RawMessage
}

func codexTranscriptEvidence(data []byte) (RuntimeModel, string, codexUsage) {
	model := RuntimeModel{}
	effort := "unavailable"
	usage := codexUsage{}
	hasContext := false
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record map[string]json.RawMessage
		if json.Unmarshal(line, &record) != nil {
			continue
		}
		kind, ok := codexString(record["type"], false)
		if !ok {
			continue
		}
		switch kind {
		case "turn_context":
			// A turn_context starts a new evidence segment even when its
			// payload is malformed. Never combine an older turn's model or
			// effort with token usage observed after a newer context marker.
			hasContext = true
			model = RuntimeModel{}
			effort = "unavailable"
			usage = codexUsage{}
			var payload map[string]json.RawMessage
			if json.Unmarshal(record["payload"], &payload) != nil {
				continue
			}
			if value, ok := codexString(payload["model"], false); ok && value != "" {
				model = codexModel(value)
			}
			effort = codexContextEffort(payload)
		case "event_msg":
			if !hasContext {
				continue
			}
			var payload map[string]json.RawMessage
			if json.Unmarshal(record["payload"], &payload) != nil {
				continue
			}
			payloadType, ok := codexString(payload["type"], false)
			if !ok || payloadType != "token_count" {
				continue
			}
			var info, last map[string]json.RawMessage
			if json.Unmarshal(payload["info"], &info) != nil || json.Unmarshal(info["last_token_usage"], &last) != nil {
				continue
			}
			usage = codexUsage{
				Input: codexCounter(last["input_tokens"]), Output: codexCounter(last["output_tokens"]),
				CacheRead: codexCounter(last["cached_input_tokens"]), CacheCreation: codexCounter(last["cache_write_input_tokens"]),
				Reasoning: codexCounter(last["reasoning_output_tokens"]), Total: codexCounter(last["total_tokens"]),
			}
		}
	}
	return model, effort, usage
}

func codexContextEffort(payload map[string]json.RawMessage) string {
	if value, ok := codexString(payload["effort"], false); ok {
		if normalized := codexEffort(value); normalized != "unavailable" {
			return normalized
		}
	}
	var collaborationMode, settings map[string]json.RawMessage
	if json.Unmarshal(payload["collaboration_mode"], &collaborationMode) != nil ||
		json.Unmarshal(collaborationMode["settings"], &settings) != nil {
		return "unavailable"
	}
	value, ok := codexString(settings["reasoning_effort"], false)
	if !ok {
		return "unavailable"
	}
	return codexEffort(value)
}

func codexCounter(raw json.RawMessage) json.RawMessage {
	if number := runtimeNumber(raw); number != "" {
		return json.RawMessage(number)
	}
	return json.RawMessage("null")
}
