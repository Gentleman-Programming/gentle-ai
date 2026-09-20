package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	sddPreflightQuestionPrefix      = "Gentle AI SDD preflight "
	maxSDDPreflightHookPayloadBytes = 256 << 10
	maxSDDPreflightTranscriptLine   = 8 << 20
)

var sddPreflightHookSessionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var sddPreflightHookPhases = []string{"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard", "sdd-research"}

type sddPreflightHookOption struct {
	Label string `json:"label"`
}

type sddPreflightHookQuestion struct {
	Header      string                   `json:"header"`
	Question    string                   `json:"question"`
	Options     []sddPreflightHookOption `json:"options"`
	MultiSelect bool                     `json:"multiSelect,omitempty"`
}

type sddPreflightHookToolInput struct {
	SubagentType string `json:"subagent_type,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
}

type sddPreflightHookPayload struct {
	SessionID      string
	TranscriptPath string
	HookEventName  string
	ToolName       string
	AgentID        string
	AgentType      string
	ToolInput      sddPreflightHookToolInput
	ToolInputRaw   map[string]any
}

func (payload *sddPreflightHookPayload) UnmarshalJSON(raw []byte) error {
	var wire struct {
		SessionID      string          `json:"session_id"`
		TranscriptPath string          `json:"transcript_path"`
		HookEventName  string          `json:"hook_event_name"`
		ToolName       string          `json:"tool_name"`
		AgentID        string          `json:"agent_id,omitempty"`
		AgentType      string          `json:"agent_type,omitempty"`
		ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	payload.SessionID, payload.TranscriptPath = wire.SessionID, wire.TranscriptPath
	payload.HookEventName, payload.ToolName, payload.AgentID, payload.AgentType = wire.HookEventName, wire.ToolName, wire.AgentID, wire.AgentType
	if len(wire.ToolInput) > 0 {
		if err := json.Unmarshal(wire.ToolInput, &payload.ToolInput); err != nil {
			return err
		}
		if err := json.Unmarshal(wire.ToolInput, &payload.ToolInputRaw); err != nil {
			return err
		}
	}
	return nil
}

type sddPreflightHookSpecificOutput struct {
	HookEventName            string         `json:"hookEventName"`
	PermissionDecision       string         `json:"permissionDecision"`
	PermissionDecisionReason string         `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]any `json:"updatedInput,omitempty"`
}

type sddPreflightHookOutput struct {
	HookSpecificOutput sddPreflightHookSpecificOutput `json:"hookSpecificOutput"`
}

func RunSDDPreflightHook(args []string, stdout io.Writer) error {
	return runSDDPreflightHook(args, os.Stdin, stdout)
}

// runSDDPreflightHook implements `gentle-ai sdd-preflight-hook --agent
// claude-code`, installed as a Claude Code PreToolUse(Agent) hook. Authority
// is derived at dispatch time directly from the session transcript that the
// Claude Code hook runner supplies on stdin (transcript_path and
// session_id): a model-started copy of this command cannot influence the
// real dispatch decision, because Claude Code only honors the output of the
// hook invocation it started itself, and that invocation's stdin is
// runner-supplied. There is no minted, persisted record of authority.
func runSDDPreflightHook(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("sdd-preflight-hook", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agent := flags.String("agent", "", "required runtime identity")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return sddPreflightHookProtocolError("unexpected sdd-preflight-hook argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*agent) != "claude-code" {
		return sddPreflightHookProtocolError("sdd-preflight-hook requires an explicit supported agent")
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, maxSDDPreflightHookPayloadBytes+1))
	if err != nil {
		return fmt.Errorf("read hook payload: %w", err)
	}
	if len(raw) > maxSDDPreflightHookPayloadBytes {
		return sddPreflightHookProtocolError("sdd-preflight-hook payload exceeds %d bytes", maxSDDPreflightHookPayloadBytes)
	}
	var payload sddPreflightHookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("decode hook payload: %w", err)
	}

	if payload.HookEventName != "PreToolUse" || payload.ToolName != "Agent" || !isSDDPreflightHookPhase(payload.ToolInput.SubagentType) {
		return nil
	}
	if !sddPreflightHookSessionID.MatchString(payload.SessionID) {
		return sddPreflightHookProtocolError("invalid hook session id")
	}

	if payload.AgentID != "" || payload.AgentType != "" {
		return writeSDDPreflightHookDecision(stdout, "deny", "SDD child dispatch refused: only the interactive parent may carry parent-confirmed SDD preflight authority.", nil)
	}
	if strings.Contains(payload.ToolInput.Prompt, "## SDD Session Preflight") {
		return writeSDDPreflightHookDecision(stdout, "deny", "SDD child dispatch refused: model-authored preflight text cannot create parent-confirmed authority.", nil)
	}
	if !filepath.IsAbs(payload.TranscriptPath) || filepath.Base(payload.TranscriptPath) != payload.SessionID+".jsonl" {
		return writeSDDPreflightHookDecision(stdout, "deny", "SDD child dispatch refused: the hook payload does not bind the session transcript.", nil)
	}

	block, ok := resolveSDDPreflightTranscriptAuthority(payload.TranscriptPath, payload.SessionID)
	if !ok {
		return writeSDDPreflightHookDecision(stdout, "deny", "SDD child dispatch refused: parent-confirmed SDD preflight is missing, invalid, or uncorroborated. Ask the canonical grouped preflight with AskUserQuestion and stop before retrying.", nil)
	}

	updated := payload.ToolInputRaw
	if updated == nil {
		updated = map[string]any{}
	}
	updated["prompt"] = block + "\n\n" + payload.ToolInput.Prompt
	return writeSDDPreflightHookDecision(stdout, "allow", "parent-confirmed SDD preflight attached by Gentle AI", updated)
}

// sddPreflightTranscriptRecord is the subset of one JSONL transcript line
// this scan needs. Real Claude Code transcript records carry additional
// fields; anything not modeled here is ignored.
type sddPreflightTranscriptRecord struct {
	Type          string          `json:"type"`
	IsSidechain   bool            `json:"isSidechain"`
	SessionID     string          `json:"sessionId"`
	Message       json.RawMessage `json:"message"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

type sddPreflightTranscriptMessage struct {
	Content []sddPreflightTranscriptContentBlock `json:"content"`
}

type sddPreflightTranscriptContentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// resolveSDDPreflightTranscriptAuthority scans the session transcript at
// path for a corroborated canonical SDD preflight: an assistant
// AskUserQuestion tool_use asking the canonical grouped questions, matched
// by tool_use id to a later, non-error user tool_result carrying
// toolUseResult.answers for that same session (isSidechain false). The last
// corroborated pair in file order wins. Malformed or unparsable lines are
// skipped, never fatal; a line longer than maxSDDPreflightTranscriptLine has
// its remainder discarded without being buffered, and scanning continues
// with the next line.
func resolveSDDPreflightTranscriptAuthority(path, sessionID string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	pending := map[string][]sddPreflightHookQuestion{}
	block, found := "", false

	reader := bufio.NewReader(file)
	for {
		line, ok, done := readSDDPreflightTranscriptLine(reader)
		if ok && len(strings.TrimSpace(string(line))) > 0 {
			var record sddPreflightTranscriptRecord
			if err := json.Unmarshal(line, &record); err == nil && !record.IsSidechain && record.SessionID == sessionID && len(record.Message) > 0 {
				var message sddPreflightTranscriptMessage
				if err := json.Unmarshal(record.Message, &message); err == nil {
					switch record.Type {
					case "assistant":
						for _, part := range message.Content {
							if part.Type != "tool_use" || part.Name != "AskUserQuestion" || part.ID == "" {
								continue
							}
							var wrapped struct {
								Questions []sddPreflightHookQuestion `json:"questions"`
							}
							if err := json.Unmarshal(part.Input, &wrapped); err != nil {
								continue
							}
							pending[part.ID] = wrapped.Questions
						}
					case "user":
						for _, part := range message.Content {
							if part.Type != "tool_result" || part.ToolUseID == "" || part.IsError {
								continue
							}
							questions, ok := pending[part.ToolUseID]
							if !ok || len(record.ToolUseResult) == 0 {
								continue
							}
							var toolUseResult struct {
								Answers json.RawMessage `json:"answers"`
							}
							if err := json.Unmarshal(record.ToolUseResult, &toolUseResult); err != nil {
								continue
							}
							candidate, recognized, err := resolveSDDPreflightHookBlock(questions, toolUseResult.Answers)
							if err != nil || !recognized || !validSDDPreflightHookBlock(candidate) {
								continue
							}
							block, found = candidate, true
						}
					}
				}
			}
		}
		if done {
			break
		}
	}
	return block, found
}

// readSDDPreflightTranscriptLine reads one newline- or EOF-terminated line
// from r without ever buffering more than maxSDDPreflightTranscriptLine
// bytes of it. If the line exceeds that bound, ok is false and the
// remainder of the oversized line is discarded (read and dropped, never
// accumulated) so r is left positioned at the start of the next line. done
// is true once there is no further data to read; when done is true, line
// and ok may still carry the final line of the file.
func readSDDPreflightTranscriptLine(r *bufio.Reader) (line []byte, ok bool, done bool) {
	var buf []byte
	overflow := false
	for {
		chunk, err := r.ReadSlice('\n')
		if len(chunk) > 0 && !overflow {
			if len(buf)+len(chunk) > maxSDDPreflightTranscriptLine {
				overflow = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		switch err {
		case nil:
			if overflow {
				return nil, false, false
			}
			return bytes.TrimSuffix(buf, []byte("\n")), true, false
		case bufio.ErrBufferFull:
			continue
		case io.EOF:
			if overflow || len(buf) == 0 {
				return nil, false, true
			}
			return buf, true, true
		default:
			return nil, false, true
		}
	}
}

func resolveSDDPreflightHookBlock(questions []sddPreflightHookQuestion, answersRaw json.RawMessage) (string, bool, error) {
	recognized := false
	for _, question := range questions {
		if strings.HasPrefix(question.Question, sddPreflightQuestionPrefix) {
			recognized = true
			break
		}
	}
	if !recognized {
		return "", false, nil
	}
	expectedLabels := [][]string{{"Interactive", "Automatic"}, {"OpenSpec", "Engram", "Both"}, {"Ask me", "Single PR", "Auto"}}
	if len(questions) != 3 {
		return "", true, sddPreflightHookProtocolError("parent-confirmed SDD preflight requires exactly three questions")
	}
	for i, question := range questions {
		prefix := fmt.Sprintf("%s%d/3:", sddPreflightQuestionPrefix, i+1)
		if !strings.HasPrefix(question.Question, prefix) || question.MultiSelect || len(question.Options) != len(expectedLabels[i]) {
			return "", true, sddPreflightHookProtocolError("SDD preflight question %d is malformed", i+1)
		}
		for optionIndex, option := range question.Options {
			if option.Label != expectedLabels[i][optionIndex] {
				return "", true, sddPreflightHookProtocolError("SDD preflight question %d changed the canonical option semantics", i+1)
			}
		}
	}
	answers, err := sddPreflightHookAnswers(answersRaw)
	if err != nil {
		return "", true, err
	}
	indexes := make([]int, 3)
	for i, question := range questions {
		answer, ok := answers[question.Question]
		if !ok {
			return "", true, sddPreflightHookProtocolError("SDD preflight question %d has no answer", i+1)
		}
		indexes[i] = -1
		for optionIndex, option := range question.Options {
			if answer == option.Label {
				if indexes[i] >= 0 {
					return "", true, sddPreflightHookProtocolError("SDD preflight question %d answer is ambiguous", i+1)
				}
				indexes[i] = optionIndex
			}
		}
		if indexes[i] < 0 {
			return "", true, sddPreflightHookProtocolError("SDD preflight question %d answer is outside the offered domain", i+1)
		}
	}
	pace := []string{"interactive", "auto"}[indexes[0]]
	store := []string{"openspec", "engram", "hybrid"}[indexes[1]]
	strategy := []string{"ask-on-risk", "single-pr", "auto-chain"}[indexes[2]]
	block := strings.Join([]string{
		"## SDD Session Preflight",
		"Parent-confirmed by the runtime; models and child agents cannot create or modify this block.",
		"- Pace: " + pace,
		"- Artifact store: " + store,
		"- Delivery strategy: " + strategy,
		"- Review policy: 400 changed lines",
	}, "\n")
	return block, true, nil
}

// sddPreflightHookAnswers decodes a toolUseResult.answers map (question text
// -> answer). The canonical preflight forbids multi-select, but a
// single-element array is tolerated defensively.
func sddPreflightHookAnswers(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, sddPreflightHookProtocolError("SDD preflight response is empty")
	}
	var rawAnswers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawAnswers); err != nil {
		return nil, fmt.Errorf("decode SDD preflight answers: %w", err)
	}
	if len(rawAnswers) == 0 {
		return nil, sddPreflightHookProtocolError("SDD preflight response has no answers")
	}
	answers := make(map[string]string, len(rawAnswers))
	for question, rawAnswer := range rawAnswers {
		var single string
		if err := json.Unmarshal(rawAnswer, &single); err == nil {
			answers[question] = single
			continue
		}
		var multiple []string
		if err := json.Unmarshal(rawAnswer, &multiple); err != nil || len(multiple) != 1 {
			return nil, sddPreflightHookProtocolError("SDD preflight answer for %q is not single-select", question)
		}
		answers[question] = multiple[0]
	}
	return answers, nil
}

func validSDDPreflightHookBlock(block string) bool {
	lines := strings.Split(block, "\n")
	if len(lines) != 6 || lines[0] != "## SDD Session Preflight" || lines[1] != "Parent-confirmed by the runtime; models and child agents cannot create or modify this block." || lines[5] != "- Review policy: 400 changed lines" {
		return false
	}
	return sddPreflightHookOneOf(lines[2], "- Pace: interactive", "- Pace: auto") &&
		sddPreflightHookOneOf(lines[3], "- Artifact store: openspec", "- Artifact store: engram", "- Artifact store: hybrid") &&
		sddPreflightHookOneOf(lines[4], "- Delivery strategy: ask-on-risk", "- Delivery strategy: single-pr", "- Delivery strategy: auto-chain")
}

func sddPreflightHookOneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func isSDDPreflightHookPhase(agent string) bool {
	for _, phase := range sddPreflightHookPhases {
		if agent == phase || strings.HasPrefix(agent, phase+"-") {
			return true
		}
	}
	return false
}

func sddPreflightHookProtocolError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func writeSDDPreflightHookDecision(stdout io.Writer, decision, reason string, updated map[string]any) error {
	output := sddPreflightHookOutput{HookSpecificOutput: sddPreflightHookSpecificOutput{HookEventName: "PreToolUse", PermissionDecision: decision, PermissionDecisionReason: reason, UpdatedInput: updated}}
	return json.NewEncoder(stdout).Encode(output)
}
