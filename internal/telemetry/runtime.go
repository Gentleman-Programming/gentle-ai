package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const RuntimeSchema = "gentle-ai.telemetry-runtime-aggregate/v1"
const RuntimeMaxBytes = 16384

// Public package categories, not proof of runtime authority or agent-name identity.
const runtimeAgentClasses = "orchestrator|worker|explore|verify|unknown|sdd-init|sdd-explore|sdd-research|sdd-propose|sdd-spec|sdd-design|sdd-tasks|sdd-apply|sdd-verify|sdd-archive|sdd-onboard|sdd-status|sdd-sync|jd-judge-a|jd-judge-b|jd-fix-agent|review-risk|review-readability|review-reliability|review-resilience|review-refuter|review-validator"
const runtimeEfforts = "off|minimal|low|medium|high|xhigh|max|not_selected|unknown|custom|unavailable|unsupported"

// RuntimeBatch is the sanitized stdin aggregate, not a persistent batch.
// BatchID is an ignored source-compatibility field for pure legacy normalizers;
// it is never accepted in JSON, serialized, or used for delivery identity.
type RuntimeBatch struct {
	Schema   string          `json:"schema"`
	Registry json.RawMessage `json:"registry"`
	BatchID  string          `json:"-"`
	Host     string          `json:"host"`
	Rows     []RuntimeRow    `json:"rows"`
}
type RuntimeModel struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}
type RuntimeRow struct {
	Model           RuntimeModel    `json:"model"`
	ModelEvidence   string          `json:"model_evidence"`
	AgentKind       string          `json:"agent_kind"`
	AgentClass      string          `json:"agent_class"`
	SelectedEffort  string          `json:"selected_effort"`
	EffectiveEffort string          `json:"effective_effort"`
	Launches        json.RawMessage `json:"launches"`
	Responses       json.RawMessage `json:"responses"`
	Input           json.RawMessage `json:"input_tokens"`
	Output          json.RawMessage `json:"output_tokens"`
	CacheRead       json.RawMessage `json:"cache_read_tokens"`
	CacheCreation   json.RawMessage `json:"cache_creation_tokens"`
	ReasoningTokens json.RawMessage `json:"reasoning_tokens"`
	TotalTokens     json.RawMessage `json:"total_tokens"`
	ErrorCategory   string          `json:"error_category"`
	Duration        RuntimeDuration `json:"duration"`
}

// RuntimeToken preserves independent observation coverage. Counts never derive
// from launches/responses; zero observations differs from a reported zero.
type RuntimeToken struct {
	Reported    json.RawMessage `json:"reported"`
	Unavailable json.RawMessage `json:"unavailable"`
	Unsupported json.RawMessage `json:"unsupported"`
	Sum         json.RawMessage `json:"sum"`
}

func runtimeTokenNormalize(raw *json.RawMessage) bool {
	if _, ok := runtimeObject(*raw, "reported|unavailable|unsupported|sum"); !ok {
		return false
	}
	var token RuntimeToken
	if json.Unmarshal(*raw, &token) != nil {
		return false
	}
	for _, n := range []*json.RawMessage{&token.Reported, &token.Unavailable, &token.Unsupported, &token.Sum} {
		value := runtimeNumber(*n)
		if value == "" {
			return false
		}
		*n = json.RawMessage(value)
	}
	if string(token.Reported) == "0" && string(token.Sum) != "0" {
		return false
	}
	*raw, _ = json.Marshal(token)
	return true
}

// Adapter-only conversion of one source observation, never aggregate shorthand.
func runtimeTokenObservation(raw json.RawMessage) json.RawMessage {
	token := RuntimeToken{json.RawMessage("0"), json.RawMessage("0"), json.RawMessage("0"), json.RawMessage("0")}
	switch string(raw) {
	case "", "null":
		token.Unavailable = json.RawMessage("1")
	case `"unsupported"`:
		token.Unsupported = json.RawMessage("1")
	default:
		token.Reported, token.Sum = json.RawMessage("1"), raw
	}
	out, _ := json.Marshal(token)
	return out
}

func (r *RuntimeRow) tokenObservations() {
	for _, raw := range []*json.RawMessage{&r.Input, &r.Output, &r.CacheRead, &r.CacheCreation, &r.ReasoningTokens, &r.TotalTokens} {
		*raw = runtimeTokenObservation(*raw)
	}
}

// MeasuredCount counts timed attempts/messages, not successful responses.
// SumMS is an aggregate, never a per-response latency. Missing coverage is unknown.
type RuntimeDuration struct {
	Kind          string          `json:"kind"`
	MeasuredCount json.RawMessage `json:"measured_count"`
	SumMS         json.RawMessage `json:"sum_ms"`
}

// Canonical decimal sums without float rounding, overflow, or exponent expansion.
func runtimeDurationNumber(raw []byte) string {
	s := string(raw)
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(s), "e")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := whole + fraction
	if digits == "" {
		return ""
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return ""
		}
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return "0"
	}
	if negative {
		return ""
	}
	power := new(big.Int)
	if hasExponent {
		if _, ok := power.SetString(exponent, 10); !ok {
			return ""
		}
	}
	trimmed := strings.TrimRight(digits, "0")
	power.Add(power, big.NewInt(int64(len(digits)-len(trimmed)-len(fraction))))
	places := new(big.Int).Add(power, big.NewInt(int64(len(trimmed))))
	if places.Cmp(big.NewInt(12)) > 0 || (places.Cmp(big.NewInt(12)) == 0 && len(trimmed) > 12 && trimmed[:12] == "999999999999") {
		return ""
	}
	if power.Sign() == 0 {
		return trimmed
	}
	return trimmed + "e" + power.String()
}

func (d *RuntimeDuration) normalize() bool {
	count := runtimeNumber(d.MeasuredCount)
	if count == "" {
		return false
	}
	d.MeasuredCount = json.RawMessage(count)
	if d.Kind == "unavailable" {
		return count == "0" && string(d.SumMS) == "null"
	}
	if !runtimeMember(d.Kind, "request|message") || count == "0" {
		return false
	}
	sum := runtimeDurationNumber(d.SumMS)
	if sum == "" {
		return false
	}
	d.SumMS = json.RawMessage(sum)
	return true
}

func runtimeMember(value, choices string) bool {
	for _, choice := range strings.Split(choices, "|") {
		if value == choice {
			return true
		}
	}
	return false
}

// RuntimeEffortAllowed reports whether value belongs to the runtime telemetry
// contract's closed effort vocabulary.
func RuntimeEffortAllowed(value string) bool {
	return runtimeMember(value, runtimeEfforts)
}

// NormalizeRuntimeModel keeps model attribution inside the runtime telemetry
// contract without exposing unregistered provider or model names.
func NormalizeRuntimeModel(provider, id string) RuntimeModel {
	m := RuntimeModel{Provider: provider, ID: id}
	if provider == "" || id == "" {
		return RuntimeModel{Provider: "unknown", ID: "unknown"}
	}
	if runtimeModelOK(m) {
		return m
	}
	m = RuntimeModel{Provider: "custom", ID: "custom"}
	if provider == "opencode" {
		m.Provider = "opencode"
	}
	return m
}

const runtimeAnthropicModels = "claude-opus-5|claude-haiku-4-5|claude-haiku-4-5-20251001|claude-sonnet-5"

// Registry 1: public names verified by the parent against official catalogs.
// Namespace is a caller assertion, not endpoint/route attestation.
func runtimeModelOK(m RuntimeModel) bool {
	switch m.Provider {
	case "anthropic":
		return runtimeMember(m.ID, runtimeAnthropicModels)
	case "openai", "openai-codex":
		return runtimeMember(m.ID, "gpt-6-astra|gpt-5.6-sol|gpt-5.6-terra|gpt-5.6-luna|gpt-5.3-codex-spark|gpt-5.5|gpt-5.4|gpt-5.4-mini|gpt-5.2|gpt-5.3-codex|gpt-5.6")
	case "opencode":
		return m.ID == "custom"
	case "unknown", "custom":
		return m.ID == m.Provider
	}
	return false
}

var runtimeID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Canonicalize JSON numbers without floating-point rounding or unbounded powers.
// The caller already checked JSON syntax; integer-valued decimal/exponent forms
// have the same meaning under JSON Schema and for deduplication.
func runtimeNumber(raw []byte) string {
	s := string(raw)
	if s == "" {
		return ""
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(s), "e")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return "0"
	}
	if negative {
		return ""
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return ""
		}
	}
	power := int64(0)
	if hasExponent {
		var err error
		power, err = strconv.ParseInt(exponent, 10, 64)
		if err != nil || power > int64(len(raw)+12) || power < -int64(len(raw)) {
			return ""
		}
	}
	trimmed := strings.TrimRight(digits, "0")
	power += int64(len(digits) - len(trimmed) - len(fraction))
	if power < 0 || int64(len(trimmed))+power > 12 {
		return ""
	}
	return trimmed + strings.Repeat("0", int(power))
}

var errRuntimeInput = errors.New("invalid runtime aggregate")

func decodeRuntime(data []byte) (RuntimeBatch, error) {
	var b RuntimeBatch
	// encoding/json alone accepts duplicate keys: reject them before typed decoding.
	d := json.NewDecoder(bytes.NewReader(data))
	if err := runtimeUnique(d); err != nil {
		return b, errRuntimeInput
	}
	if _, err := d.Token(); err != io.EOF {
		return b, errRuntimeInput
	}
	if !runtimeExactKeys(data) {
		return b, errRuntimeInput
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&b); err != nil {
		return b, errRuntimeInput
	}
	if b.Schema != RuntimeSchema || runtimeNumber(b.Registry) != "1" || !runtimeMember(b.Host, "claude-code|opencode|codex|pi") || len(b.Rows) < 1 || len(b.Rows) > 32 {
		return b, errRuntimeInput
	}
	b.Registry = json.RawMessage("1")
	if err := normalizeRuntimeRows(b.Rows); err != nil {
		return b, err
	}
	return b, nil
}

func normalizeRuntimeRows(rows []RuntimeRow) error {
	for i := range rows {
		r := &rows[i]
		// Deprecated compatibility alias emitted by older registry-1 clients.
		// Keep it out of the canonical vocabulary and normalize it before storage.
		if r.AgentClass == "sdd-proposal" {
			r.AgentClass = "sdd-propose"
		}
		if !runtimeMember(r.ModelEvidence, "selected|response|unknown") || !runtimeModelOK(r.Model) || !runtimeMember(r.AgentKind, "orchestrator|built_in|custom|unknown") || !runtimeMember(r.AgentClass, runtimeAgentClasses) {
			return errRuntimeInput
		}
		for _, effort := range []string{r.SelectedEffort, r.EffectiveEffort} {
			if !RuntimeEffortAllowed(effort) {
				return errRuntimeInput
			}
		}
		if !runtimeMember(r.ErrorCategory, "none|unknown|auth|output_length|aborted|api|rate_limit|server") || !r.Duration.normalize() {
			return errRuntimeInput
		}
		for _, metric := range []*json.RawMessage{&r.Input, &r.Output, &r.CacheRead, &r.CacheCreation, &r.ReasoningTokens, &r.TotalTokens} {
			if !runtimeTokenNormalize(metric) {
				return errRuntimeInput
			}
		}
		for _, metric := range []*json.RawMessage{&r.Launches, &r.Responses} {
			if string(*metric) == "null" {
				continue
			}
			var availability string
			if json.Unmarshal(*metric, &availability) == nil && availability == "unsupported" {
				*metric = json.RawMessage(`"unsupported"`)
				continue
			}
			number := runtimeNumber(*metric)
			if number == "" {
				return errRuntimeInput
			}
			*metric = json.RawMessage(number)
		}
	}
	return nil
}

// Struct decoding is case-insensitive. Check exact required key sets first,
// using encoding/json for objects rather than accepting struct field aliases.
func runtimeObject(data []byte, keys string) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return nil, false
	}
	required := strings.Split(keys, "|")
	if len(object) != len(required) {
		return nil, false
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return nil, false
		}
	}
	return object, true
}

func runtimeExactKeys(data []byte) bool {
	batch, ok := runtimeObject(data, "schema|registry|host|rows")
	if !ok {
		return false
	}
	return runtimeExactRows(batch["rows"])
}

func runtimeExactRows(data []byte) bool {
	var rows []json.RawMessage
	if json.Unmarshal(data, &rows) != nil {
		return false
	}
	for _, raw := range rows {
		row, ok := runtimeObject(raw, "model|model_evidence|agent_kind|agent_class|selected_effort|effective_effort|launches|responses|input_tokens|output_tokens|cache_read_tokens|cache_creation_tokens|reasoning_tokens|total_tokens|error_category|duration")
		if !ok {
			return false
		}
		if _, ok := runtimeObject(row["duration"], "kind|measured_count|sum_ms"); !ok {
			return false
		}
		if _, ok := runtimeObject(row["model"], "provider|id"); !ok {
			return false
		}
	}
	return true
}

func runtimeUnique(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return errRuntimeInput
			}
			keys[name] = true
		}
		if err := runtimeUnique(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func runtimeAllowed(home string, getenv func(string) string) bool {
	if !Decide(getenv, State{Enabled: true}).Enabled {
		return false
	}
	policy, err := LoadPolicyState(home)
	return err == nil && policy.NoticeShown && Decide(getenv, policy).Enabled
}
