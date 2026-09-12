package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
)

const OpenCodeMaxBytes = 65536

var errOpenCode = errors.New("invalid OpenCode message event")

type openCodeObject map[string]json.RawMessage

// OpenCodeObservation is local-only. Positive counters are source-observed, not
// proof of provider reporting. Unverified default zeros and absent counts have unavailable coverage.
// MessageDurationMS is message elapsed time, NEVER provider call latency.
// Extra observation fields temporarily mirror the common Row.
type OpenCodeObservation struct {
	Row               RuntimeRow      `json:"row"`
	ReasoningTokens   json.RawMessage `json:"reasoning_tokens"`
	MessageDurationMS *int64          `json:"message_duration_ms"`
	ErrorCategory     string          `json:"error_category,omitempty"`
	Agent             string          `json:"-"`
}

// NormalizeOpenCode handles a single completed non-summary assistant message.
// Source: https://opencode.ai/docs/sdk/ and the parent-fetched public types at
// https://raw.githubusercontent.com/anomalyco/opencode/dev/packages/sdk/js/src/gen/types.gen.ts
// This mutable-dev source fixture does not establish released SDK compatibility.
// Caller MUST dedupe input using sessionID/id BEFORE normalization: identifiers
// are deliberately dropped, and this pure function offers no exactly-once claim.
// No hooks, transcript retrieval, installation, network, or telemetry send occurs.
func NormalizeOpenCode(input io.Reader) (*OpenCodeObservation, error) {
	data, err := io.ReadAll(io.LimitReader(input, OpenCodeMaxBytes+1))
	if err != nil || len(data) > OpenCodeMaxBytes {
		return nil, errOpenCode
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if openCodeBounded(d, 0) != nil {
		return nil, errOpenCode
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errOpenCode
	}
	root, err := openCodeMap(data)
	if err != nil {
		return nil, err
	}
	event, err := openCodeString(root["type"])
	if err != nil {
		return nil, err
	}
	if event != "message.updated" {
		return nil, nil
	}
	props, err := openCodeMap(root["properties"])
	if err != nil {
		return nil, err
	}
	info, err := openCodeMap(props["info"])
	if err != nil {
		return nil, err
	}
	role, err := openCodeString(info["role"])
	if err != nil {
		return nil, err
	}
	if role != "assistant" {
		return nil, nil
	}
	if raw, ok := info["summary"]; ok {
		var summary bool
		if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &summary) != nil {
			return nil, errOpenCode
		}
		if summary {
			return nil, nil
		}
	}
	time, err := openCodeMap(info["time"])
	if err != nil {
		return nil, err
	}
	if _, ok := time["completed"]; !ok {
		return nil, nil
	}
	completed, err := openCodeInteger(time["completed"], 16)
	if err != nil {
		return nil, err
	}
	created, err := openCodeInteger(time["created"], 16)
	if err != nil || completed < created {
		return nil, errOpenCode
	}
	duration := completed - created
	provider, err := openCodeString(info["providerID"])
	if err != nil {
		return nil, err
	}
	model, err := openCodeString(info["modelID"])
	if err != nil {
		return nil, err
	}
	agent, err := openCodeAgent(info["agent"])
	if err != nil {
		return nil, err
	}
	modelEvidence := "response"
	if provider == "" || model == "" {
		modelEvidence = "unknown"
	}
	agentKind, agentClass := openCodeAgentAttribution(agent)
	r := RuntimeRow{Model: NormalizeRuntimeModel(provider, model), ModelEvidence: modelEvidence, AgentKind: agentKind, AgentClass: agentClass, SelectedEffort: "unavailable", EffectiveEffort: "unavailable", Launches: json.RawMessage("null"), Responses: json.RawMessage("1")}
	tokens := openCodeObject{}
	if raw, ok := info["tokens"]; ok {
		tokens, err = openCodeMap(raw)
		if err != nil {
			return nil, err
		}
	}
	cache := openCodeObject{}
	if raw, ok := tokens["cache"]; ok {
		cache, err = openCodeMap(raw)
		if err != nil {
			return nil, err
		}
	}
	o := &OpenCodeObservation{MessageDurationMS: &duration, Agent: agent}
	for _, metric := range []struct {
		raw    json.RawMessage
		target *json.RawMessage
	}{
		{tokens["input"], &r.Input}, {tokens["output"], &r.Output}, {cache["read"], &r.CacheRead}, {cache["write"], &r.CacheCreation}, {tokens["reasoning"], &r.ReasoningTokens},
	} {
		*metric.target = json.RawMessage("null")
		if metric.raw == nil {
			continue
		}
		n, err := openCodeInteger(metric.raw, 12)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			*metric.target = json.RawMessage(strconv.FormatInt(n, 10))
		}
	}
	r.ErrorCategory = "none"
	r.Duration = RuntimeDuration{Kind: "message", MeasuredCount: json.RawMessage("1"), SumMS: json.RawMessage(strconv.FormatInt(duration, 10))}
	if !r.Duration.normalize() {
		return nil, errOpenCode
	}
	if raw, ok := info["error"]; ok {
		r.ErrorCategory, err = openCodeError(raw)
		if err != nil {
			return nil, err
		}
	}
	r.tokenObservations()
	o.Row = r
	o.ReasoningTokens = r.ReasoningTokens
	o.ErrorCategory = r.ErrorCategory
	return o, nil
}

func openCodeAgent(raw []byte) (string, error) {
	agent, err := openCodeString(raw)
	if err != nil || len(agent) > 64 {
		return "", errOpenCode
	}
	for i := 0; i < len(agent); i++ {
		if agent[i] < 0x20 || agent[i] > 0x7e {
			return "", errOpenCode
		}
	}
	return agent, nil
}

func openCodeAgentAttribution(agent string) (kind, class string) {
	switch agent {
	case "":
		return "unknown", "unknown"
	case "build", "plan", "gentle-orchestrator":
		return "orchestrator", "orchestrator"
	case "explore":
		return "built_in", "explore"
	case "general":
		return "built_in", "worker"
	default:
		if runtimeMember(agent, runtimeAgentClasses) && agent != "orchestrator" && agent != "worker" && agent != "explore" && agent != "verify" && agent != "unknown" {
			return "built_in", agent
		}
		return "custom", "unknown"
	}
}

func openCodeMap(raw []byte) (openCodeObject, error) {
	var obj openCodeObject
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, errOpenCode
	}
	return obj, nil
}
func openCodeString(raw []byte) (string, error) {
	if raw == nil {
		return "", nil
	}
	var s string
	if bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &s) != nil {
		return "", errOpenCode
	}
	return s, nil
}
func openCodeInteger(raw []byte, digits int) (int64, error) {
	// Counters accept JSON integer-valued forms via the common bounded helper.
	if digits == 12 {
		n := runtimeNumber(raw)
		if n == "" {
			return 0, errOpenCode
		}
		v, err := strconv.ParseInt(n, 10, 64)
		return v, err
	}
	// Millisecond timestamps are bounded integers below the safe JSON integer limit.
	v, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || v < 0 || v > 9007199254740991 {
		return 0, errOpenCode
	}
	return v, nil
}
func openCodeError(raw []byte) (string, error) {
	obj, err := openCodeMap(raw)
	if err != nil {
		return "", err
	}
	name, err := openCodeString(obj["name"])
	if err != nil {
		return "", err
	}
	category := map[string]string{"ProviderAuthError": "auth", "UnknownError": "unknown", "MessageOutputLengthError": "output_length", "MessageAbortedError": "aborted", "APIError": "api"}[name]
	if category == "" {
		category = "unknown"
	}
	if name == "APIError" {
		data, err := openCodeMap(obj["data"])
		if err != nil {
			return "", err
		}
		if status, ok := data["statusCode"]; ok {
			n, err := openCodeInteger(status, 12)
			if err != nil || n < 100 || n > 599 {
				return "", errOpenCode
			}
			switch {
			case n == 401 || n == 403:
				category = "auth"
			case n == 429:
				category = "rate_limit"
			case n >= 500:
				category = "server"
			}
		}
	}
	return category, nil
}

// Validate even discarded fields; exact map keys avoid struct case aliases.
func openCodeBounded(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errOpenCode
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	if s, ok := token.(string); ok && len(s) > 4096 {
		return errOpenCode
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || len(s) > 4096 || seen[s] {
				return errOpenCode
			}
			seen[s] = true
		}
		if err := openCodeBounded(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
