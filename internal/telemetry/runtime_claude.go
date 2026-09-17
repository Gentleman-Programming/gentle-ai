package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const ClaudeMaxBytes = RuntimeMaxBytes
const ClaudeTranscriptMaxBytes = 512 * 1024
const ClaudeAgentMaxBytes = 64 * 1024

var errClaude = errors.New("invalid Claude Code hook event")

// ClaudeHook retains only routing fields and the final-message correlation
// value required to find current local evidence. Identifiers, prompts, cwd,
// and permission data are discarded; the message never leaves memory.
type ClaudeHook struct {
	HookEventName       string   `json:"-"`
	AgentType           string   `json:"-"`
	TranscriptPath      string   `json:"-"`
	AgentTranscriptPath string   `json:"-"`
	LastAssistantDigest [32]byte `json:"-"`
}

// ClaudeUsage is bounded response evidence extracted from a transcript tail.
type ClaudeUsage struct {
	Evidence      bool
	Correlated    bool
	Model         string
	MessageID     string
	Input         json.RawMessage
	Output        json.RawMessage
	CacheRead     json.RawMessage
	CacheCreation json.RawMessage
}

// ClaudeObservation carries the normalized runtime row plus, for a Stop event
// with transcript evidence and a usable message id, a deterministic delivery
// id that makes re-sending the same unflushed transcript row idempotent at
// the collector.
type ClaudeObservation struct {
	Row        RuntimeRow `json:"row"`
	DeliveryID string     `json:"-"`
}

// claudeStopDeliverySalt domain-separates the Stop delivery-id hash from any
// other use of the message id, so the hash cannot be reused across contracts.
const claudeStopDeliverySalt = "gentle-ai.telemetry-runtime-claude-stop/v1\x00"

// claudeStopDeliveryID derives a stable, collector-shaped (32 lowercase hex
// chars) delivery id from a transcript's API message id. The message id
// itself never leaves the machine: only this one-way hash is transmitted.
func claudeStopDeliveryID(messageID string) string {
	if messageID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(claudeStopDeliverySalt + messageID))
	return hex.EncodeToString(sum[:16])
}

// claudeUsableID keeps a transcript identifier only if it is a short,
// printable-ASCII token. This bounds both the hash input and any risk of
// non-identifier content masquerading as a message id.
func claudeUsableID(id string) string {
	if len(id) < 1 || len(id) > 128 {
		return ""
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x20 || id[i] > 0x7e {
			return ""
		}
	}
	return id
}

// ParseClaudeHook validates one bounded hook object and retains no private
// identifiers or message content. Unknown fields remain memory-only and are
// accepted for forward compatibility with Claude's common hook envelope.
func ParseClaudeHook(input io.Reader) (ClaudeHook, error) {
	var result ClaudeHook
	data, err := io.ReadAll(io.LimitReader(input, ClaudeMaxBytes+1))
	if err != nil || len(data) > ClaudeMaxBytes {
		return result, errClaude
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if openCodeBounded(d, 0) != nil {
		return result, errClaude
	}
	if _, err = d.Token(); err != io.EOF {
		return result, errClaude
	}
	var source struct {
		HookEventName        string `json:"hook_event_name"`
		AgentType            string `json:"agent_type"`
		TranscriptPath       string `json:"transcript_path"`
		AgentTranscriptPath  string `json:"agent_transcript_path"`
		LastAssistantMessage string `json:"last_assistant_message"`
	}
	if json.Unmarshal(data, &source) != nil || source.HookEventName == "" {
		return result, errClaude
	}
	if source.HookEventName == "SubagentStop" && source.AgentType == "" {
		return result, errClaude
	}
	result = ClaudeHook{HookEventName: source.HookEventName, AgentType: source.AgentType, TranscriptPath: source.TranscriptPath, AgentTranscriptPath: source.AgentTranscriptPath}
	if source.LastAssistantMessage != "" {
		result.LastAssistantDigest = sha256.Sum256([]byte(source.LastAssistantMessage))
	}
	return result, nil
}

// ClaudeNamedAgent tests the current runtime registry as data. The generic
// aggregate classes are not installable Claude agent names.
func ClaudeNamedAgent(name string) bool {
	if !runtimeMember(name, runtimeAgentClasses) {
		return false
	}
	return !runtimeMember(name, "orchestrator|worker|explore|verify|unknown")
}

// ParseClaudeTranscriptTail returns the last valid assistant usage record in a
// subagent transcript. firstPartial means the bounded tail starts inside an
// older line. Message correlation strengthens the evidence but is not required:
// agent_transcript_path is already scoped to this single subagent run.
func ParseClaudeTranscriptTail(data []byte, firstPartial bool, expectedDigest [32]byte) (ClaudeUsage, bool) {
	if len(data) > ClaudeTranscriptMaxBytes {
		return ClaudeUsage{}, false
	}
	if firstPartial {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			return ClaudeUsage{}, false
		}
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var record struct {
			Type    string `json:"type"`
			UUID    string `json:"uuid"`
			Message struct {
				ID      string          `json:"id"`
				Model   string          `json:"model"`
				Content json.RawMessage `json:"content"`
				Usage   *struct {
					Input         json.RawMessage `json:"input_tokens"`
					Output        json.RawMessage `json:"output_tokens"`
					CacheRead     json.RawMessage `json:"cache_read_input_tokens"`
					CacheCreation json.RawMessage `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil || record.Type != "assistant" || record.Message.Usage == nil || len(record.Message.Model) > 256 {
			continue
		}
		// message.id is the primary identity: one API response can be split
		// across several transcript records sharing the same message.id (and
		// the same usage), which is why it is preferred over the per-record
		// uuid. The uuid is only a fallback when message.id is absent.
		messageID := ""
		if record.Message.ID != "" {
			messageID = claudeUsableID(record.Message.ID)
		} else {
			messageID = claudeUsableID(record.UUID)
		}
		u := ClaudeUsage{Evidence: true, Correlated: expectedDigest != ([32]byte{}) && claudeMessageMatches(record.Message.Content, expectedDigest), Model: record.Message.Model, MessageID: messageID, Input: record.Message.Usage.Input, Output: record.Message.Usage.Output, CacheRead: record.Message.Usage.CacheRead, CacheCreation: record.Message.Usage.CacheCreation}
		valid := true
		for _, raw := range []json.RawMessage{u.Input, u.Output, u.CacheRead, u.CacheCreation} {
			if raw != nil && runtimeNumber(raw) == "" {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		return u, true
	}
	return ClaudeUsage{}, false
}

func claudeMessageMatches(raw json.RawMessage, expected [32]byte) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return sha256.Sum256([]byte(text)) == expected
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil || len(blocks) == 0 {
		return false
	}
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" {
			texts = append(texts, block.Text)
		}
	}
	if len(texts) == 0 {
		return false
	}
	return sha256.Sum256([]byte(strings.Join(texts, ""))) == expected || sha256.Sum256([]byte(strings.Join(texts, "\n"))) == expected
}

// NormalizeClaude converts one completion plus optional bounded local evidence
// into the common runtime row. It performs no I/O and retains no source paths.
func NormalizeClaude(hook ClaudeHook, usage ClaudeUsage, agentDefinition []byte) *ClaudeObservation {
	if hook.HookEventName != "Stop" && hook.HookEventName != "SubagentStop" {
		return nil
	}
	if hook.HookEventName == "Stop" {
		// Stop has no agent definition: it is always the orchestrator. Usage
		// evidence is kept; a repeated final message that matches an older,
		// not-yet-flushed transcript row is made safe by the deterministic
		// delivery id derived below, not by discarding the evidence.
		agentDefinition = nil
	}
	r := RuntimeRow{Model: RuntimeModel{Provider: "unknown", ID: "unknown"}, ModelEvidence: "unknown", AgentKind: "custom", AgentClass: "unknown", SelectedEffort: "unavailable", EffectiveEffort: "unavailable", Launches: json.RawMessage("1"), Responses: json.RawMessage("null"), ErrorCategory: "none", Duration: RuntimeDuration{Kind: "unavailable", MeasuredCount: json.RawMessage("0"), SumMS: json.RawMessage("null")}}
	if hook.HookEventName == "Stop" {
		r.AgentKind, r.AgentClass = "orchestrator", "orchestrator"
	} else if ClaudeNamedAgent(hook.AgentType) {
		r.AgentKind, r.AgentClass = "built_in", hook.AgentType
	}
	selectedModel, selectedEffort := claudeFrontmatter(agentDefinition)
	selectedModel = claudeCanonicalModelID(selectedModel)
	if selectedEffort != "" && runtimeMember(selectedEffort, runtimeEfforts) {
		r.SelectedEffort = selectedEffort
	}
	model := selectedModel
	if usage.Evidence {
		r.Launches, r.Responses = json.RawMessage("null"), json.RawMessage("1")
		if usage.Model != "" {
			model, r.ModelEvidence = claudeCanonicalModelID(usage.Model), "response"
		}
	}
	if r.ModelEvidence == "unknown" && model != "" {
		r.ModelEvidence = "selected"
	}
	r.Model = claudeModel(model)
	r.Input, r.Output, r.CacheRead, r.CacheCreation = usage.Input, usage.Output, usage.CacheRead, usage.CacheCreation
	r.ReasoningTokens = json.RawMessage(`"unsupported"`)
	r.TotalTokens = json.RawMessage("null")
	r.tokenObservations()
	o := &ClaudeObservation{Row: r}
	if hook.HookEventName == "Stop" && usage.Evidence && usage.MessageID != "" {
		o.DeliveryID = claudeStopDeliveryID(usage.MessageID)
	}
	return o
}

func claudeModel(id string) RuntimeModel {
	return NormalizeRuntimeModel("anthropic", id)
}

// Claude Code frontmatter uses sonnet/opus/haiku aliases; the finite alias
// table below is the only Claude-specific folding left. Selectors that defer
// model choice carry no selected-model evidence. Anything else, including a
// dated or revisioned model id (e.g. "claude-sonnet-5-20260101"), passes
// through unfolded to the generic family-pattern normalizer.
func claudeCanonicalModelID(id string) string {
	id = strings.TrimSpace(id)
	switch id {
	case "", "inherit", "default":
		return ""
	case "sonnet":
		return "claude-sonnet-5"
	case "opus":
		return "claude-opus-5"
	case "haiku":
		return "claude-haiku-4-5"
	}
	return id
}

func claudeFrontmatter(data []byte) (string, string) {
	if len(data) == 0 || len(data) > ClaudeAgentMaxBytes {
		return "", ""
	}
	text := string(data)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", ""
	}
	text = strings.TrimPrefix(strings.TrimPrefix(text, "---\r\n"), "---\n")
	end := strings.Index(text, "\n---")
	if end < 0 {
		return "", ""
	}
	var front struct {
		Model  string `yaml:"model"`
		Effort string `yaml:"effort"`
	}
	if yaml.Unmarshal([]byte(text[:end]), &front) != nil {
		return "", ""
	}
	return strings.TrimSpace(front.Model), strings.TrimSpace(front.Effort)
}
