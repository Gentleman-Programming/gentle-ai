package sddtaskresult

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// HandoffPrefix is the literal token every typed terminal failure starts with,
// followed by exactly one ASCII space and then the JSON payload. Consumers are
// told to preserve the payload unchanged, so the prefix and the single space
// are part of the contract.
const HandoffPrefix = "GENTLE_AI_SDD_FAILURE "

const handoffSchema = "gentle-ai.sdd-task-result-failure/v1"

const retryGuidance = "Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user."

// routeToken bounds what may be echoed back as taskModel. An unvalidated
// provider string would otherwise reach a consumer inside an envelope it is
// told to preserve verbatim.
var routeToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,255}$`)

// handoffPayload field order is the wire order. It matches the shape the
// OpenCode transport already emits so a consumer cannot tell which side
// produced it.
type handoffPayload struct {
	SchemaName   string `json:"schemaName"`
	Status       string `json:"status"`
	Code         string `json:"code"`
	Phase        string `json:"phase"`
	LatchedPhase string `json:"latchedPhase,omitempty"`
	LatchedCode  string `json:"latchedCode,omitempty"`
	TaskModel    string `json:"taskModel,omitempty"`
	Summary      string `json:"summary"`
	Continuation string `json:"continuation"`
	Exit         string `json:"exit,omitempty"`
}

// shellQuote renders one POSIX single-quoted argument. The cwd reaches the
// consumer inside a runnable continuation, so an unquoted quote character would
// hand them a broken or surprising command.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func continuationFor(cwd string) string {
	return "gentle-ai sdd-status --cwd " + shellQuote(cwd) + " --json"
}

func encode(payload handoffPayload) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		// handoffPayload is a flat struct of strings; marshalling cannot fail.
		panic("sddtaskresult: encode handoff: " + err.Error())
	}
	return HandoffPrefix + string(encoded)
}

// Handoff renders the typed terminal failure for one classified phase result.
// An admitted result has no handoff and returns the empty string.
func Handoff(class Class, phase, cwd, taskModel string) string {
	code := class.FailureCode()
	if code == "" {
		return ""
	}
	summary := fmt.Sprintf("%s returned no valid task result. %s", phase, retryGuidance)
	if class == ClassEmpty {
		summary = fmt.Sprintf("%s produced no task output at all. The child task returned nothing, which most often means the provider rejected the request before generation (authentication, region, or model access), the task was interrupted, or the phase genuinely wrote nothing. %s", phase, retryGuidance)
	}
	payload := handoffPayload{
		SchemaName: handoffSchema, Status: "blocked", Code: code, Phase: phase,
		Summary: summary, Continuation: continuationFor(cwd),
	}
	if routeToken.MatchString(taskModel) {
		payload.TaskModel = taskModel
	}
	return encode(payload)
}

// DispatchLatched renders the refusal a later launch receives after an earlier
// phase failed in the same session. That launch never dispatched, so it names
// the phase it requested alongside the phase and code that actually failed.
func DispatchLatched(requested, latchedPhase, latchedCode, cwd string) string {
	return encode(handoffPayload{
		SchemaName: handoffSchema, Status: "blocked", Code: "sdd_task_dispatch_latched",
		Phase: requested, LatchedPhase: latchedPhase, LatchedCode: latchedCode,
		Summary:      fmt.Sprintf("%s was not dispatched. Earlier in this session %s returned %s, and SDD launches stay latched afterwards so a failed phase is never silently retried and no later phase advances on top of it. No provider call, no subagent, and no artifact write happened for this launch, so it produced no new evidence about the original failure.", requested, latchedPhase, latchedCode),
		Continuation: continuationFor(cwd),
		Exit:         "Inspect the artifact state the original failure left, surface it to the user, and start a new session to launch SDD phases again. Relaunching in this session cannot dispatch.",
	})
}
