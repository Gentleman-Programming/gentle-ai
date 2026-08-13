package advisoryreview

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// OpenCode shim (rdd-advisory-transport SKILL.md, change #3138 slice 4):
// the native replacement for the review-result-artifacts.ts review half. It
// accepts the opaque canonical reviewer task prompt and returns a provider
// block to inject, defers while the legacy plugin's review half is still
// installed (SEN-RPC-17, migration-window safety: exactly one injection
// source), or refuses with the preserved #3049 provenance text (REQ-RPC-6).
//
// The shim is a pure dispatch seam: it owns provenance admission and binding
// routing, and nothing else. ProduceBlock is where the provider assembles the
// finished lens context; the shim never renders prompt text, never applies a
// budget, and never decides admission. Raw completed task output passes
// through UnwrapReviewerTaskResult unchanged except for the runtime's
// <task_result> envelope, which Go removes -- exactly like the legacy
// plugin's reviewerResult (SEN-RPC-5/6).

// PinnedRuntime is the recorded identity of the gentle-ai binary that wrote
// the managed OpenCode reviewer assets. The wiring is production-owned: the
// state file's opencode_runtime_provenance record maps onto this DTO, and the
// running process's own identity is derived the same way the legacy adapter
// derived it (executable path, sha256 digest, version output).
type PinnedRuntime struct {
	Executable string
	SHA256     string
	Version    string
}

// DispatchResult is one shim dispatch outcome: either the review was deferred
// to the legacy plugin (Deferred, no block), or a provider block was produced
// (Block, byte-for-byte untouched). A refused dispatch is an error, never a
// DispatchResult (SEN-RPC-20: a refusal is a transport outcome, not a
// verdict -- no block, no ValidatedResult).
type DispatchResult struct {
	// Deferred reports that the legacy review plugin is installed and is the
	// sole injection source; the shim produced nothing.
	Deferred bool
	// Block is the provider's finished lens context, returned untouched.
	Block []byte
}

// OpenCodeShim is the fail-closed native OpenCode reviewer shim. Every field
// is a seam so the unit tests never touch the filesystem or spawn a process;
// the production wiring in internal/cli supplies the real implementations.
// A shim whose seams were never wired refuses with a transport error instead
// of admitting anything (NewOpenCodeShim is the zero value and is by itself
// unwired).
type OpenCodeShim struct {
	// ReadPinned reads the persisted opencode_runtime_provenance record for
	// this installation. Absence is a refusal, never an admission.
	ReadPinned func() (PinnedRuntime, error)
	// CurrentProcess derives the identity of the gentle-ai binary that owns
	// this process. An undeterminable identity is a refusal (SEN-RPC-8: a
	// PATH decoy must never be the binary that answers).
	CurrentProcess func() (PinnedRuntime, error)
	// LegacyReviewPluginInstalled reports whether the review-result-artifacts.ts
	// review half is still installed on disk. While it is, the shim defers
	// (SEN-RPC-17) so the migration window never sees two injection sources.
	LegacyReviewPluginInstalled func() (bool, error)
	// ProduceBlock assembles the finished provider lens context for one
	// repository-context handle and one selected lens. The shim hands the
	// returned bytes back untouched.
	ProduceBlock func(ctx context.Context, repositoryContext, lens string) ([]byte, error)
}

// NewOpenCodeShim returns an OpenCodeShim with no seams wired. Dispatch on
// the zero value fails closed with a "not wired" transport error; the
// production constructor in internal/cli replaces every seam before any
// dispatch happens.
func NewOpenCodeShim() *OpenCodeShim {
	return &OpenCodeShim{}
}

// ShimProvenanceRefusal is the preserved #3049 refusal text, byte for byte
// the message the managed plugin authors (runtimeProvenanceRefusal in
// review-result-artifacts.ts). The typed refusal code keeps the opaque path's
// rule that no native prose reaches the session transcript, and the recovery
// instruction is exactly the one the plugin gives so the migration window
// never shows users two different explanations (REQ-RPC-6).
const ShimProvenanceRefusal = "opencode_runtime_provenance_invalid: the synced OpenCode reviewer runtime is missing or no longer matches " +
	"the binary that installed this plugin. Run `gentle-ai sync` from the intended installation, then relaunch the reviewer."

// Dispatch runs the shim contract in order: fail-closed wiring check, legacy
// deferral (SEN-RPC-17), provenance admission against the pinned record
// (REQ-RPC-6, SEN-RPC-8), the one binding route parse, then block production.
// The binding is opaque provider data the shim only routes on; everything
// after the provenance admission is untouched provider bytes.
func (shim *OpenCodeShim) Dispatch(ctx context.Context, prompt, lens string) (DispatchResult, error) {
	if shim.ReadPinned == nil || shim.CurrentProcess == nil || shim.LegacyReviewPluginInstalled == nil || shim.ProduceBlock == nil {
		return DispatchResult{}, errors.New("opencode shim not wired: dispatch requires all four seams; refusing to admit a review through an unwired shim")
	}

	// SEN-RPC-17: the deferral happens BEFORE provenance admission and BEFORE
	// any block production. An undeterminable legacy state also defers: the
	// migration window's invariant is exactly one injection, and under-injection
	// is the only direction that can never violate it.
	legacyInstalled, err := shim.LegacyReviewPluginInstalled()
	if err != nil || legacyInstalled {
		return DispatchResult{Deferred: true}, nil
	}

	pinned, err := shim.ReadPinned()
	if err != nil {
		return DispatchResult{}, errors.New(ShimProvenanceRefusal)
	}
	current, err := shim.CurrentProcess()
	if err != nil {
		return DispatchResult{}, errors.New(ShimProvenanceRefusal)
	}
	if pinned.Executable != current.Executable || pinned.SHA256 != current.SHA256 || pinned.Version != current.Version {
		return DispatchResult{}, errors.New(ShimProvenanceRefusal)
	}

	repositoryContext, ok := shimBindingRepositoryContext(prompt)
	if !ok {
		return DispatchResult{}, errors.New("immutable OpenCode candidate inspection requires a repository-context binding; the reviewer was not launched, so its exactly-once invocation is preserved")
	}

	block, err := shim.ProduceBlock(ctx, repositoryContext, lens)
	if err != nil {
		return DispatchResult{}, err
	}
	return DispatchResult{Block: block}, nil
}

// shimBindingLine matches the opaque provider binding on the first line of the
// task prompt, mirroring the legacy plugin's BINDING regex exactly (same
// marker, same single-line object capture). The prompt may carry caller prose
// after the binding line; provider injection discards it.
var shimBindingLine = regexp.MustCompile(`^GENTLE_AI_REVIEW_BINDING (\{[^\n]+\})(?:\n|$)`)

// shimBindingRepositoryContext extracts the one opaque handle the shim routes
// on: the provider-issued repository-context string inside the binding. It
// deliberately validates nothing else about the binding -- the binding is
// opaque provider data, passed through, never interpreted (this is the exact
// one-field route the legacy plugin's bindingRepositoryContext performed with
// JSON.parse). This is the shim's single sanctioned JSON decode: child output
// is never parsed here.
func shimBindingRepositoryContext(prompt string) (string, bool) {
	match := shimBindingLine.FindStringSubmatch(prompt)
	if match == nil {
		return "", false
	}
	var binding struct {
		RepositoryContext string `json:"repository_context"`
	}
	if err := json.Unmarshal([]byte(match[1]), &binding); err != nil {
		return "", false
	}
	if binding.RepositoryContext == "" {
		return "", false
	}
	return binding.RepositoryContext, true
}

// shimTaskResultEnvelope mirrors the legacy plugin's TASK_RESULT regex: a
// completed task wrapping exactly one <task_result> body. The body is the
// model's raw final text, returned back untouched once the envelope is gone.
var shimTaskResultEnvelope = regexp.MustCompile(`^<task id="[^"\r\n]+" state="completed">\n<task_result>\n([\s\S]*?)\n</task_result>\n</task>$`)

// shimTaskTag mirrors the legacy plugin's TASK_TAG regex: it recognises any
// task or task_result markup so a malformed or nested envelope is refused
// instead of being misread as raw output.
var shimTaskTag = regexp.MustCompile(`</?task(?:\s|>)|</?task_result>`)

// UnwrapReviewerTaskResult hands back the model's raw final text, mirroring
// the legacy plugin's reviewerResult/taskResult behavior exactly: raw text
// without an envelope passes through unchanged; a completed task envelope is
// removed; empty output, a malformed envelope, an empty envelope body, and a
// nested envelope are refused with the same messages the plugin used. No
// capture, no preservation, no admission: native Go decides what a malformed
// or empty result means (SEN-RPC-5/6).
func UnwrapReviewerTaskResult(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("reviewer output must not be empty")
	}
	trimmed := strings.TrimSpace(raw)
	if envelope := shimTaskResultEnvelope.FindStringSubmatch(trimmed); envelope != nil {
		body := envelope[1]
		if strings.TrimSpace(body) == "" {
			return "", errors.New("reviewer task result is empty")
		}
		if shimTaskTag.MatchString(body) {
			return "", errors.New("reviewer task result contains a nested task envelope")
		}
		return body, nil
	}
	if shimTaskTag.MatchString(trimmed) {
		return "", errors.New("reviewer output contains a malformed task result envelope")
	}
	return trimmed, nil
}
