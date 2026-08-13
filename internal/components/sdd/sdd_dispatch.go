package sdd

// Native SDD phase task-result half (change #3138, slice 6, REQ-SOA-1/2):
// the SDD task-result handling that used to live in the TS plugin
// (review-result-artifacts.ts) -- isSDDPhase, the per-session
// sddDispatchLatched latch cleared on session.deleted/dispose, taskRouteModel
// route-token scrubbing, and the GENTLE_AI_SDD_FAILURE terminal handoff --
// now lives here in Go, so SDD phase execution over OpenCode works with no
// managed plugin (SEN-SOA-1). Observable latch semantics and the handoff
// bytes are identical to the TS behavior; the parity tests
// (sdd_dispatch_test.go) pin the byte equality (SEN-SOA-3).
//
// The dispatch verbs that host processes invoke live in internal/cli
// (RunSDDTaskResult / RunSDDDispatchGuard / RunSDDLatch); this file owns the
// pure contract and the latch persistence seam.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// SDDTaskFailurePrefix is the terminal handoff prefix, byte-for-byte the TS
// constant SDD_TASK_FAILURE_PREFIX (including the trailing space).
const SDDTaskFailurePrefix = "GENTLE_AI_SDD_FAILURE "

const sddTaskFailureSchema = "gentle-ai.sdd-task-result-failure/v1"

// phaseNames mirrors SDD_PHASES in review-result-artifacts.ts.
var phaseNames = []string{
	"sdd-init", "sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
	"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive", "sdd-onboard",
}

// IsSDDPhase matches the TS isSDDPhase predicate exactly: an agent is an SDD
// phase when it equals a phase name or starts with a phase name followed by a
// dash (phase-scoped subagents), so native classification cannot drift from
// the plugin's.
func IsSDDPhase(subagent string) bool {
	for _, phase := range phaseNames {
		if subagent == phase || strings.HasPrefix(subagent, phase+"-") {
			return true
		}
	}
	return false
}

// sddTaskRouteToken bounds one provider or model identifier before it may
// enter the failure handoff, mirroring the TS SDD_TASK_ROUTE_TOKEN regex: the
// first character must be alphanumeric and at most 127 characters may follow
// from the plain identifier alphabet. Anything else -- separators, whitespace,
// path shapes -- is omitted entirely rather than truncated, so hostile or
// accidental metadata never reaches the session transcript.
var sddTaskRouteToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

// TaskRouteModel extracts the child task's provider/model route from the task
// tool's result metadata ({model: {providerID, modelID}}), or "" when the
// metadata does not carry a valid route. Absence is tolerated, never
// invented: the handoff simply omits the field.
func TaskRouteModel(metadata any) string {
	meta, ok := metadata.(map[string]any)
	if !ok {
		return ""
	}
	model, ok := meta["model"].(map[string]any)
	if !ok {
		return ""
	}
	providerID, ok := model["providerID"].(string)
	if !ok {
		return ""
	}
	modelID, ok := model["modelID"].(string)
	if !ok {
		return ""
	}
	if !sddTaskRouteToken.MatchString(providerID) || !sddTaskRouteToken.MatchString(modelID) {
		return ""
	}
	return providerID + "/" + modelID
}

// sddTaskResultEnvelope mirrors the TS TASK_RESULT regex: a completed task
// wrapping exactly one <task_result> body.
var sddTaskResultEnvelope = regexp.MustCompile(`^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n</task_result>\n</task>$`)

// sddTaskTag mirrors the TS TASK_TAG regex: it recognises any task or
// task_result markup so a malformed or nested envelope is refused instead of
// being misread as raw output.
var sddTaskTag = regexp.MustCompile(`</?task(?:\s|>)|</?task_result>`)

// UnwrapSDDTaskResult mirrors taskResult(output, "SDD phase", "sddClass"): it
// hands back the model's raw final text, and on failure returns the same
// machine-readable class the TS path attached to the thrown error, so the
// terminal handoff can keep the two truths distinct (#2677): an empty result
// means the child never ran inference at all, a malformed result means it
// produced output that failed the envelope contract.
func UnwrapSDDTaskResult(output any) (body, class string, err error) {
	if text, ok := output.(string); ok {
		output = text
	} else {
		return "", "empty_result", errSDDOuputEmpty
	}
	trimmed := strings.TrimSpace(output.(string))
	if trimmed == "" {
		return "", "empty_result", errSDDOuputEmpty
	}
	if envelope := sddTaskResultEnvelope.FindStringSubmatch(trimmed); envelope != nil {
		inner := envelope[1]
		if strings.TrimSpace(inner) == "" {
			return "", "empty_result", errSDDResultEmpty
		}
		if sddTaskTag.MatchString(inner) {
			return "", "nested_envelope", errSDDNestedEnvelope
		}
		return inner, "", nil
	}
	if sddTaskTag.MatchString(trimmed) {
		return "", "malformed_result", errSDDMalformedEnvelope
	}
	return trimmed, "", nil
}

var (
	errSDDOuputEmpty        = errors.New("SDD phase output must not be empty")
	errSDDResultEmpty       = errors.New("SDD phase task result is empty")
	errSDDNestedEnvelope    = errors.New("SDD phase task result contains a nested task envelope")
	errSDDMalformedEnvelope = errors.New("SDD phase output contains a malformed task result envelope")
)

// shellQuote mirrors the TS shellQuote: single-quote wrapping with '\”
// escaping, used by the continuation so a hostile cwd cannot break out of
// the command shape.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// SDDTaskFailure is the terminal handoff for one failed SDD phase task. It is
// what the latch stores per session, so a later latched launch can replay the
// original failure's identity without inventing anything (#2948).
type SDDTaskFailure struct {
	Phase   string
	Code    string
	Handoff string
}

type sddTaskFailureEnvelope struct {
	SchemaName   string `json:"schemaName"`
	Status       string `json:"status"`
	Code         string `json:"code"`
	Phase        string `json:"phase"`
	TaskModel    string `json:"taskModel,omitempty"`
	Summary      string `json:"summary"`
	Continuation string `json:"continuation"`
}

type sddDispatchLatchedEnvelope struct {
	SchemaName   string `json:"schemaName"`
	Status       string `json:"status"`
	Code         string `json:"code"`
	Phase        string `json:"phase"`
	LatchedPhase string `json:"latchedPhase"`
	LatchedCode  string `json:"latchedCode"`
	Summary      string `json:"summary"`
	Continuation string `json:"continuation"`
	Exit         string `json:"exit"`
}

// compactJSON renders one envelope the way JSON.stringify rendered it in the
// TS handoff: compact, in key order, and without HTML escaping. The residual
// difference from JSON.stringify is Go's mandatory \u2028/\u2029 escaping,
// which cannot occur in a checked path or phase and is documented here rather
// than papered over.
func compactJSON(value any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(buf.String(), "\n")
}

// sddFailureGuidance is the one instruction every terminal SDD handoff
// carries, byte-for-byte the TS guidance string.
const sddFailureGuidance = "Do not retry or advance SDD; inspect the existing artifact state and surface the terminal failure to the user."

// SDDTaskResultCode maps an UnwrapSDDTaskResult classification to the
// handoff code, mirroring the TS `empty ? "sdd_task_result_empty" :
// "sdd_task_result_malformed"` choice exactly. It is exported so the CLI
// latch records the same code the envelope carries.
func SDDTaskResultCode(class string) string {
	if class == "empty_result" {
		return "sdd_task_result_empty"
	}
	return "sdd_task_result_malformed"
}

// SDDTaskFailureEnvelope builds the terminal transport handoff for one failed
// SDD phase task, byte-equal to the TS sddTaskFailure handoff for equal
// inputs (SEN-SOA-3). class is the UnwrapSDDTaskResult classification; empty
// and malformed results get two different summaries (#2677).
func SDDTaskFailureEnvelope(phase, cwd, class string, metadata any) string {
	code := SDDTaskResultCode(class)
	summary := phase + " returned no valid task result. " + sddFailureGuidance
	if class == "empty_result" {
		code = "sdd_task_result_empty"
		summary = phase + " produced no task output at all. The child task returned nothing, which most often means the " +
			"provider rejected the request before generation (authentication, region, or model access), the task was " +
			"interrupted, or the phase genuinely wrote nothing. " + sddFailureGuidance
	}
	envelope := sddTaskFailureEnvelope{
		SchemaName:   sddTaskFailureSchema,
		Status:       "blocked",
		Code:         code,
		Phase:        phase,
		TaskModel:    TaskRouteModel(metadata),
		Summary:      summary,
		Continuation: "gentle-ai sdd-status --cwd " + shellQuote(cwd) + " --json",
	}
	return SDDTaskFailurePrefix + compactJSON(envelope)
}

// SDDDispatchLatchedEnvelope builds the refusal for a launch that never
// reached the provider, byte-equal to the TS sddDispatchLatched handoff
// (SEN-SOA-3). It names three things the replay of the old latch never did:
// which phase THIS launch asked for, which earlier phase actually failed and
// how, and the exit -- a new session, because the latch is per-session state
// cleared on session.deleted and dispose (#2948).
func SDDDispatchLatchedEnvelope(requested string, failure SDDTaskFailure, cwd string) string {
	envelope := sddDispatchLatchedEnvelope{
		SchemaName:   sddTaskFailureSchema,
		Status:       "blocked",
		Code:         "sdd_task_dispatch_latched",
		Phase:        requested,
		LatchedPhase: failure.Phase,
		LatchedCode:  failure.Code,
		Summary: requested + " was not dispatched. Earlier in this session " + failure.Phase + " returned " +
			failure.Code + ", and SDD launches stay latched afterwards so a failed phase is never silently " +
			"retried and no later phase advances on top of it. No provider call, no subagent, and no artifact " +
			"write happened for this launch, so it produced no new evidence about the original failure.",
		Continuation: "gentle-ai sdd-status --cwd " + shellQuote(cwd) + " --json",
		Exit: "Inspect the artifact state the original failure left, surface it to the user, and start a " +
			"new session to launch SDD phases again. Relaunching in this session cannot dispatch.",
	}
	return SDDTaskFailurePrefix + compactJSON(envelope)
}

// SDDLatchStore is the persistence seam for the per-session dispatch latch.
// The dispatch verbs are spawned per hook call, so production wiring uses
// FileSDDLatchStore; unit tests use NewInMemorySDDLatchStore.
type SDDLatchStore interface {
	Record(sessionID string, failure SDDTaskFailure) error
	Recall(sessionID string) (SDDTaskFailure, bool, error)
	Clear(sessionID string) error
	ClearAll() error
}

// InMemorySDDLatchStore keeps the latch in process memory. It is the unit
// seam; the CLI never wires it because the verbs run one-shot.
type InMemorySDDLatchStore struct {
	mu    sync.Mutex
	items map[string]SDDTaskFailure
}

func NewInMemorySDDLatchStore() *InMemorySDDLatchStore {
	return &InMemorySDDLatchStore{items: map[string]SDDTaskFailure{}}
}

func (s *InMemorySDDLatchStore) Record(sessionID string, failure SDDTaskFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sessionID] = failure
	return nil
}

func (s *InMemorySDDLatchStore) Recall(sessionID string) (SDDTaskFailure, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	failure, ok := s.items[sessionID]
	return failure, ok, nil
}

func (s *InMemorySDDLatchStore) Clear(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, sessionID)
	return nil
}

func (s *InMemorySDDLatchStore) ClearAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = map[string]SDDTaskFailure{}
	return nil
}

// SDDLatchStorePath returns the host-scoped latch file for a home directory.
// The latch is per-session host state (not per-repo), so it lives beside the
// managed state file, never inside a repository.
func SDDLatchStorePath(homeDir string) string {
	return filepath.Join(homeDir, ".gentle-ai", "sdd-dispatch-latch.json")
}

// FileSDDLatchStore persists the latch as a JSON map keyed by session ID,
// replaced atomically so a crash cannot leave a torn latch. It survives the
// one-shot verb processes exact like the per-session semantics require.
type FileSDDLatchStore struct {
	path string
	mu   sync.Mutex
}

func NewFileSDDLatchStore(path string) *FileSDDLatchStore {
	return &FileSDDLatchStore{path: path}
}

func (s *FileSDDLatchStore) load() (map[string]SDDTaskFailure, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]SDDTaskFailure{}, nil
		}
		return nil, err
	}
	var items map[string]SDDTaskFailure
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *FileSDDLatchStore) save(items map[string]SDDTaskFailure) error {
	if len(items) == 0 {
		// An empty latch is an absent latch; removing the file keeps
		// ClearAll observable across processes.
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *FileSDDLatchStore) Record(sessionID string, failure SDDTaskFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return err
	}
	items[sessionID] = failure
	return s.save(items)
}

func (s *FileSDDLatchStore) Recall(sessionID string) (SDDTaskFailure, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return SDDTaskFailure{}, false, err
	}
	failure, ok := items[sessionID]
	return failure, ok, nil
}

func (s *FileSDDLatchStore) Clear(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.load()
	if err != nil {
		return err
	}
	delete(items, sessionID)
	return s.save(items)
}

func (s *FileSDDLatchStore) ClearAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(map[string]SDDTaskFailure{})
}
