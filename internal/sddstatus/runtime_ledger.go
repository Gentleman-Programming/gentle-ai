package sddstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	RuntimeStatusSchema                      = "gentle-ai.sdd-runtime-status/v1"
	runtimeRecordSchema                      = "gentle-ai.sdd-runtime-record/v1"
	runtimeObjectiveSchema                   = "gentle-ai.sdd-runtime-objective/v1"
	runtimeObjectiveSchemaV2                 = "gentle-ai.sdd-runtime-objective/v2"
	DefaultRuntimeAttemptLimit               = 2
	DefaultRuntimeChangedLines               = 200
	maximumRuntimeAttemptLimit               = 100
	maximumRuntimeChangedLines               = 1_000_000
	maximumRuntimeRecordBytes                = 1 << 20
	maximumRuntimeChainRecords               = 10_000
	RuntimeActionBegin                       = "begin"
	RuntimeActionFinish                      = "finish"
	RuntimeActionReset                       = "reset"
	RuntimeActionComplete                    = "complete"
	runtimeOperationBegin                    = "attempt/begin"
	runtimeOperationFinish                   = "attempt/finish"
	runtimeOperationFinishRemediation        = "attempt/finish-remediation"
	runtimeOperationReset                    = "objective/reset"
	runtimeOperationRescope                  = "objective/rescope"
	runtimeOperationRepairConsecutiveRescope = "objective/repair-consecutive-rescope"
	runtimeOperationAdvance                  = "objective/advance"
	runtimeOperationHandoff                  = "attempt/handoff"
	runtimeOperationBind                     = "binding/set"
	runtimeOperationGrant                    = "authority/grant"
	maximumRuntimeGrantRoots                 = 32
	runtimeLockAcquireAttempts               = 3

	// runtimeLedgerStatusPointer suffixes every ledger refusal an ordinary
	// caller can hit (budget exhausted, active attempt, no active attempt,
	// objective already complete, no objective to reset) so the error text
	// alone — without prior knowledge of the negotiated envelope — names the
	// one command that already derives the correct continuation. The command
	// includes --cwd/--change placeholders because the bare form is rejected
	// by the CLI for missing required flags (internal/cli/sdd_attempt.go); a
	// continuation that fails when pasted is worse than none.
	runtimeLedgerStatusPointer = "run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` — its next_action names the continuation"

	// runtimeReviewIntegrationContract mirrors cli.ReviewIntegrationContractV1,
	// which owns the value. internal/cli imports this package, so the constant
	// cannot be imported back; it is duplicated only to keep a refusal from
	// naming a `review status` invocation the CLI would reject for a missing
	// contract selector.
	runtimeReviewIntegrationContract = "gentle-ai.review-integration/v1"
)

var (
	ErrRuntimeRevisionConflict = errors.New("SDD runtime ledger revision conflict")
	ErrRuntimeConcurrentUpdate = errors.New("SDD runtime ledger is concurrently updated")
	ErrRuntimeRequestConflict  = errors.New("SDD runtime request identifier was reused with different inputs")
	ErrRuntimeBudgetExhausted  = errors.New("SDD runtime objective budget is exhausted; " + runtimeLedgerStatusPointer)
	ErrRuntimeAttemptActive    = errors.New("SDD runtime objective already has an active attempt; " + runtimeLedgerStatusPointer)
	ErrRuntimeNoActiveAttempt  = errors.New("SDD runtime objective has no active attempt; " + runtimeLedgerStatusPointer)
	// ErrRuntimeObjectiveChange never travels alone either. Every return site
	// wraps it with runtimeObjectiveChangeRefusal, which names the exact
	// runnable continuation for the state the caller is actually in. The
	// sentinel text names the reset in PROSE only, and prose is not a
	// continuation: the reset it refers to needs six flags the message never
	// listed, and it is itself refused when the candidate has not drifted.
	ErrRuntimeObjectiveChange = errors.New("SDD runtime objective changed without an explicit reset")
	ErrRuntimeObjectiveDone   = errors.New("SDD runtime objective is complete; " + runtimeLedgerStatusPointer)
	ErrRuntimeNoObjective     = errors.New("SDD runtime ledger has no objective to reset; " + runtimeLedgerStatusPointer)
	// ErrRuntimeResetNotAllowed named a STATE and no continuation, which is the
	// same defect class as the sentinels above: an operator holding it knows
	// what is refused and not one command that moves. The state it named was
	// also already incomplete — a terminal failed or interrupted attempt whose
	// candidate has drifted is admitted too — so it now says which shapes are
	// admitted and routes to the ledger's own derived next action.
	ErrRuntimeResetNotAllowed = errors.New("SDD runtime objective reset requires decision-required or complete state, or a terminal attempt whose candidate has drifted; " + runtimeLedgerStatusPointer)
	// ErrRuntimeRescopeNotAllowed names exactly the complement of
	// ErrRuntimeResetNotAllowed's admitted reset shapes: rescope owns only the
	// terminal, non-decision, non-complete, zero-drift case reset refuses
	// (#2298, #2296 part 2's dead end). A decision-required or complete
	// objective, an active attempt, a non-terminal last attempt, or a
	// drifted candidate all still route to reset.
	ErrRuntimeRescopeNotAllowed = errors.New("SDD runtime objective rescope requires no active attempt, an existing objective that is not decision-required or complete, and a terminal attempt whose candidate has NOT drifted; " + runtimeLedgerStatusPointer)
	// ErrRuntimeRescopeWidened is rescope's own sentinel (never a reused
	// generic one): AUDITED NARROWING RESCOPE admits only max_attempts and
	// max_changed_lines at or below the current objective's values. The
	// maintainer decision rejected the cheap "admit reset at
	// CumulativeChangedLines==0" fallback specifically as attempt-count
	// laundering; an unguarded widen here would be the same defect wearing a
	// new operation name.
	ErrRuntimeRescopeWidened   = errors.New("SDD runtime objective rescope may only narrow or hold max_attempts and max_changed_lines, never widen them; " + runtimeLedgerStatusPointer)
	ErrBindingRevisionConflict = errors.New("SDD review binding revision conflict")
	// ErrRuntimeWorktreeMismatch never travels alone either. Every return site
	// wraps it with runtimeWorktreeMismatchRefusal, which names the exact
	// --cwd that reproduces the binding Begin actually recorded (#2296 part
	// 1). Finishing from a different linked worktree than the one Begin ran
	// under would diff a pinned base tree against an unrelated working tree —
	// changed_lines: 0 when real work happened, or a wildly inflated delta —
	// so this refuses before any candidate capture, not after.
	ErrRuntimeWorktreeMismatch = errors.New("SDD runtime attempt began in a different linked worktree than this finish is running from")
	// ErrRuntimeCandidateUnavailable classifies the one Begin/Finish failure
	// class that is NOT an authority failure: the attempt ledger loaded,
	// replayed, and stayed unmutated, but the REPOSITORY candidate the
	// mutation must capture first could not be built. Left unclassified it
	// fell through to compactMutationFailure's opaque authority_failure
	// default on the very first acquire a consumer ever issues (#2114).
	//
	// It never travels alone: every wrap keeps the snapshot builder's own
	// refusal as the cause, because that cause is the only thing that knows
	// the runnable repository exit. This sentinel classifies; the cause
	// continues.
	ErrRuntimeCandidateUnavailable    = errors.New("SDD runtime candidate could not be captured from the repository")                        // refusal:by-design world-action: the exit is a repository-state change (stage the candidate, gitignore an untracked nested checkout, restore a pruned object), which no command of this product can decide or perform; every wrap keeps the snapshot builder's own refusal as the cause and that cause names the exact action
	ErrRuntimeHandoffSource           = errors.New("SDD runtime handoff source does not equal the active attempt's effective worktree")      // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command
	ErrRuntimeHandoffDestination      = errors.New("SDD runtime handoff destination is not a registered linked worktree of this repository") // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command
	ErrRuntimeHandoffAlreadyPerformed = errors.New("SDD runtime attempt has already been handed off")                                        // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command

	runtimeRequestIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	runtimeRevisionPattern  = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	runtimeGitTreePattern   = regexp.MustCompile(`^[a-f0-9]{40}(?:[a-f0-9]{24})?$`)

	runtimePublishRecord                     = reviewtransaction.PublishFileNoReplace
	runtimeReplaceHead                       = reviewtransaction.ReplaceFileAtomic
	runtimeSyncDirectory                     = reviewtransaction.SyncReviewDirectory
	runtimeAcquireAuthorityFileLock          = reviewtransaction.AcquireAuthorityFileLock
	runtimeRemediationFinalAuthorizationHook = func() {}
)

// RuntimeRevisionConflictError is a deterministic pre-publication CAS denial.
type RuntimeRevisionConflictError struct {
	Expected string
	Current  string
}

func (err *RuntimeRevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected %q, current %q; retry with --expected-revision %q", ErrRuntimeRevisionConflict, err.Expected, err.Current, err.Current)
}

func (err *RuntimeRevisionConflictError) Unwrap() error { return ErrRuntimeRevisionConflict }

// BindingRevisionConflictError reports a deterministic binding-only CAS
// denial. Binding revisions deliberately use a separate namespace from the
// runtime ledger HEAD so callers cannot accidentally submit an authority or
// ledger revision as the expected binding token.
type BindingRevisionConflictError struct {
	Expected string
	Current  string
}

func (err *BindingRevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected %q, current %q; retry with --expected-binding-revision %q", ErrBindingRevisionConflict, err.Expected, err.Current, err.Current)
}

func (err *BindingRevisionConflictError) Unwrap() error { return ErrBindingRevisionConflict }

// RuntimePublicationError reports that HEAD was atomically replaced but its
// directory durability could not be confirmed. The exact request is safe to
// replay; replay reopens the immutable chain and repeats directory fsync.
type RuntimePublicationError struct {
	Revision  string
	Committed bool
	Cause     error
}

func (err *RuntimePublicationError) Error() string {
	return fmt.Sprintf("SDD runtime ledger publication for %s requires exact replay: %v", err.Revision, err.Cause)
}

func (err *RuntimePublicationError) Unwrap() error { return err.Cause }

type AttemptOutcome string

const (
	AttemptRunning     AttemptOutcome = "running"
	AttemptFailed      AttemptOutcome = "failed"
	AttemptInterrupted AttemptOutcome = "interrupted"
	AttemptPassed      AttemptOutcome = "passed"
)

type HarnessDisposition string

const (
	HarnessReused      HarnessDisposition = "reused"
	HarnessInvalidated HarnessDisposition = "invalidated"
)

type RuntimeObjective struct {
	ID                       string `json:"id"`
	Generation               int    `json:"generation"`
	WorkUnit                 string `json:"work_unit"`
	EvidenceGoal             string `json:"evidence_goal"`
	InitialCandidateIdentity string `json:"initial_candidate_identity"`
	InitialCandidateTree     string `json:"initial_candidate_tree"`
	MaxAttempts              int    `json:"max_attempts"`
	MaxChangedLines          int    `json:"max_changed_lines"`
}

type RuntimeAttempt struct {
	Ordinal                int    `json:"ordinal"`
	ObjectiveID            string `json:"objective_id"`
	ObjectiveGeneration    int    `json:"objective_generation"`
	WorkUnit               string `json:"work_unit"`
	BeginCandidateIdentity string `json:"begin_candidate_identity"`
	BeginCandidateTree     string `json:"begin_candidate_tree"`
	// BeginWorktree is the canonical (absolute, symlink-evaluated) --cwd Begin
	// ran under (#2296 part 1). It is empty for every chain recorded before
	// this field existed — that emptiness IS the legacy signal, so replay and
	// Finish must treat it as "no binding recorded" and enforce nothing.
	BeginWorktree              string             `json:"begin_worktree,omitempty"`
	EffectiveWorktree          string             `json:"effective_worktree,omitempty"`
	Handoff                    *RuntimeHandoff    `json:"handoff,omitempty"`
	FinishCandidateIdentity    string             `json:"finish_candidate_identity,omitempty"`
	FinishCandidateTree        string             `json:"finish_candidate_tree,omitempty"`
	Outcome                    AttemptOutcome     `json:"outcome"`
	ChangedLines               int                `json:"changed_lines"`
	EvidenceRevision           string             `json:"evidence_revision,omitempty"`
	Diagnosis                  string             `json:"diagnosis,omitempty"`
	HarnessDisposition         HarnessDisposition `json:"harness_disposition,omitempty"`
	CleanupEvidence            string             `json:"cleanup_evidence,omitempty"`
	ProcessEvidence            string             `json:"process_evidence,omitempty"`
	RemediatesEvidenceRevision string             `json:"remediates_evidence_revision,omitempty"`
	ChangedLineBudgetExceeded  bool               `json:"changed_line_budget_exceeded,omitempty"`
}

type RuntimeHandoff struct {
	Ordinal                      int    `json:"ordinal"`
	SourceWorktree               string `json:"source_worktree"`
	DestinationWorktree          string `json:"destination_worktree"`
	CommonDir                    string `json:"common_dir"`
	ExpectedRevision             string `json:"expected_revision"`
	RequestDigest                string `json:"request_digest"`
	DestinationCandidateIdentity string `json:"destination_candidate_identity"`
	DestinationCandidateTree     string `json:"destination_candidate_tree"`
}

type RuntimeReset struct {
	Revision               string `json:"revision"`
	PreviousObjectiveID    string `json:"previous_objective_id"`
	PreviousGeneration     int    `json:"previous_generation"`
	ResetCandidateIdentity string `json:"reset_candidate_identity"`
	ResetCandidateTree     string `json:"reset_candidate_tree"`
	Reason                 string `json:"reason"`
	Actor                  string `json:"actor"`
}

// RuntimeAdvance records that a passed objective handed the change to a
// distinct downstream work unit. It is deliberately not a RuntimeReset: reset
// means a maintainer abandoned or re-scoped a terminal objective, while an
// advance means the previous scope succeeded and its successor is the ordinary
// next phase. It retains the completed objective's evidence revision because
// the live per-objective field belongs to the successor from here on.
type RuntimeAdvance struct {
	Revision                 string `json:"revision"`
	PreviousObjectiveID      string `json:"previous_objective_id"`
	PreviousGeneration       int    `json:"previous_generation"`
	PreviousWorkUnit         string `json:"previous_work_unit"`
	PreviousEvidenceRevision string `json:"previous_evidence_revision"`
}

// RuntimeRescope records AUDITED NARROWING RESCOPE (#2298, #2296 part 2): a
// terminal, non-complete objective whose candidate has NOT drifted since its
// last Finish may be narrowed to a maintainer-authorized successor scope
// without losing history. It is deliberately not a RuntimeReset: reset
// zeroes CumulativeAttempts/CumulativeChangedLines (the objective's per-scope
// budget was fully consumed or the maintainer elected to abandon it), while
// rescope carries both forward UNCHANGED — the ratified decision's core
// requirement, so the new narrower ceiling is measured against the same
// consumed history, never a laundered fresh budget.
type RuntimeRescope struct {
	Revision                 string `json:"revision"`
	PreviousObjectiveID      string `json:"previous_objective_id"`
	PreviousGeneration       int    `json:"previous_generation"`
	PreviousMaxAttempts      int    `json:"previous_max_attempts"`
	PreviousMaxChangedLines  int    `json:"previous_max_changed_lines"`
	RescopeCandidateIdentity string `json:"rescope_candidate_identity"`
	RescopeCandidateTree     string `json:"rescope_candidate_tree"`
	ObjectiveID              string `json:"objective_id"`
	WorkUnit                 string `json:"work_unit"`
	EvidenceGoal             string `json:"evidence_goal"`
	MaxAttempts              int    `json:"max_attempts"`
	MaxChangedLines          int    `json:"max_changed_lines"`
	Reason                   string `json:"reason"`
	Actor                    string `json:"actor"`
}

type RuntimeRepair struct {
	Revision         string `json:"revision"`
	ReplacedRevision string `json:"replaced_revision"`
	RestoredRevision string `json:"restored_revision"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
}

type RuntimeStatus struct {
	Schema                 string            `json:"schema"`
	Change                 string            `json:"change"`
	Revision               string            `json:"revision"`
	Objective              *RuntimeObjective `json:"objective,omitempty"`
	ActiveAttempt          *RuntimeAttempt   `json:"active_attempt,omitempty"`
	Attempts               []RuntimeAttempt  `json:"attempts"`
	ObjectiveGeneration    int               `json:"objective_generation"`
	NextOrdinal            int               `json:"next_ordinal"`
	CumulativeAttempts     int               `json:"cumulative_attempts"`
	CumulativeChangedLines int               `json:"cumulative_changed_lines"`
	LifetimeAttempts       int               `json:"lifetime_attempts"`
	LifetimeChangedLines   int               `json:"lifetime_changed_lines"`
	EvidenceRevision       string            `json:"evidence_revision"`
	DecisionRequired       bool              `json:"decision_required"`
	Complete               bool              `json:"complete"`
	NextAction             string            `json:"next_action"`
	LastReset              *RuntimeReset     `json:"last_reset,omitempty"`
	LastAdvance            *RuntimeAdvance   `json:"last_advance,omitempty"`
	LastRescope            *RuntimeRescope   `json:"last_rescope,omitempty"`
	LastRepair             *RuntimeRepair    `json:"last_repair,omitempty"`
	// GrantedRoots is the per-change edit-authority projection (#2540 S2):
	// canonical absolute symlink-evaluated roots accumulated from grant
	// records in chain order. AllowedEditRoots consumption is a later slice.
	// omitempty is load-bearing: a chain without grant records serializes
	// byte-identically to every projection before this field existed.
	GrantedRoots    []string       `json:"granted_roots,omitempty"`
	BindingRevision string         `json:"binding_revision"`
	Binding         *ReviewBinding `json:"binding,omitempty"`
	// Receipt is Wave 4 S5's terminal pointer (design.md decision 1),
	// recorded additively alongside Binding/BindingRevision — see
	// runtime_receipt.go.
	Receipt *reviewtransaction.SDDReceiptRef `json:"receipt,omitempty"`
	// BlockedReason and BlockedExit carry the verdict acquire would reach for
	// the caller's own request, so the read-only surface stops answering a
	// narrower question than the one consumers were asking it (#2114). Only
	// AdmissionStatus populates them; the request-blind Status() leaves them
	// empty and every existing consumer reads exactly what it always read.
	BlockedReason CompactBlockReason `json:"blocked_reason,omitempty"`
	BlockedExit   string             `json:"blocked_exit,omitempty"`
	// SettleObligation mirrors the compact acquire field exactly, from the
	// same derivation, so the read-only surface and the operation that spends
	// the attempt cannot disagree about what the settle will owe (#2912).
	SettleObligation string `json:"settle_obligation,omitempty"`
	// Consent carries the blocking question an exhausted attempt budget asks
	// instead of dead-ending in prose (#2588, and the same dead end #2902 and
	// #2913 died in). Populated only by AdmissionStatus, and only while the
	// ledger actually requires that decision.
	Consent *BudgetConsentResult `json:"consent,omitempty"`
}

type BeginAttemptRequest struct {
	ExpectedRevision string `json:"expected_revision"`
	RequestID        string `json:"request_id"`
	WorkUnit         string `json:"work_unit"`
	EvidenceGoal     string `json:"evidence_goal"`
	MaxAttempts      int    `json:"max_attempts"`
	MaxChangedLines  int    `json:"max_changed_lines"`
}

type FinishAttemptRequest struct {
	ExpectedRevision           string             `json:"expected_revision"`
	RequestID                  string             `json:"request_id"`
	Outcome                    AttemptOutcome     `json:"outcome"`
	EvidenceRevision           string             `json:"evidence_revision"`
	Diagnosis                  string             `json:"diagnosis"`
	HarnessDisposition         HarnessDisposition `json:"harness_disposition"`
	CleanupEvidence            string             `json:"cleanup_evidence"`
	ProcessEvidence            string             `json:"process_evidence"`
	ExpectedBindingRevision    string             `json:"expected_binding_revision,omitempty"`
	SuccessorLineageID         string             `json:"successor_lineage_id,omitempty"`
	RemediatesEvidenceRevision string             `json:"remediates_evidence_revision,omitempty"`
}

type HandoffAttemptRequest struct {
	ExpectedRevision    string `json:"expected_revision"`
	RequestID           string `json:"request_id"`
	DestinationWorktree string `json:"destination_worktree"`
}

type ResetObjectiveRequest struct {
	ExpectedRevision string `json:"expected_revision"`
	RequestID        string `json:"request_id"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
}

// RepairConsecutiveRescopeRequest is limited to the released consecutive-
// rescope writer defect; it is not a generic corruption recovery mechanism.
type RepairConsecutiveRescopeRequest struct {
	ExpectedRevision string `json:"expected_revision"`
	RequestID        string `json:"request_id"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
}

// RescopeObjectiveRequest carries the maintainer-authorized narrower
// successor scope for AUDITED NARROWING RESCOPE. Reason/Actor mirror
// ResetObjectiveRequest's audit fields: rescope is authorized exactly like
// reset. Unlike BeginAttemptRequest, MaxAttempts/MaxChangedLines are never
// defaulted when zero (normalizeRescopeObjectiveRequest requires >=1
// explicitly): a narrowing operation must state its narrower ceiling, not
// silently inherit DefaultRuntimeAttemptLimit/DefaultRuntimeChangedLines.
type RescopeObjectiveRequest struct {
	ExpectedRevision string `json:"expected_revision"`
	RequestID        string `json:"request_id"`
	WorkUnit         string `json:"work_unit"`
	EvidenceGoal     string `json:"evidence_goal"`
	MaxAttempts      int    `json:"max_attempts"`
	MaxChangedLines  int    `json:"max_changed_lines"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
}

// GrantRootsRequest records a per-change edit-authority grant (#2540 S2).
// Reason/Actor mirror ResetObjectiveRequest's audit fields. Roots are
// canonicalized (absolute, symlink-evaluated) before the request digest is
// computed, so the digest binds the exact identities the record carries.
type GrantRootsRequest struct {
	ExpectedRevision string   `json:"expected_revision"`
	RequestID        string   `json:"request_id"`
	Roots            []string `json:"roots"`
	Reason           string   `json:"reason"`
	Actor            string   `json:"actor"`
	// ChangeInstance is filled by Grant from the store's own ForInstance
	// identity (#2540 S5) so the request digest binds the exact instance the
	// grant belongs to. A caller-supplied value must equal the store's; Grant
	// refuses a mismatch rather than silently rebinding authority.
	ChangeInstance string `json:"change_instance"`
}

// BindReviewRequest performs a binding-only compare-and-swap. The expected
// value is the current ReviewBinding.Revision, not the runtime ledger HEAD and
// not the review authority revision.
type BindReviewRequest struct {
	ExpectedBindingRevision string `json:"expected_binding_revision"`
	RequestID               string `json:"request_id"`
	LineageID               string `json:"lineage_id"`
}

// RuntimeStore is one provider-owned immutable chain for one SDD change. Its
// directory is rooted in the repository Git common-dir, so linked worktrees
// and later processes observe the same attempt ordinals and line charges.
type RuntimeStore struct {
	Dir       string
	Repo      string
	Workspace string
	Change    string
	// ReviewDisabled records that the user's receipt-driven-development kill
	// switch is off for this clone. While it is set, the runtime ledger imposes
	// no review obligation of its own: a switched-off system has no
	// implications, so closing an attempt never demands an approved recovery
	// successor the operator could not obtain anyway (review/start is refused
	// while the switch is off).
	//
	// It removes only the IMPLICIT demand. An explicit remediation request is a
	// deliberate review operation and is still validated in full, and nothing
	// here approves, advances, or invents review authority.
	//
	// The zero value enforces, so any caller that does not resolve the switch
	// keeps today's behavior. The switch itself is read in the CLI layer, which
	// owns the single source of truth for both of its sources; an unreadable
	// switch is not a disabled switch and resolves to false.
	ReviewDisabled bool
	commonDir      string
	// instance is the change-instance identity this store session serves
	// (#2540 S5). The ledger directory is keyed by change name alone and
	// archive never touches it, so a future change reusing an archived name
	// reopens the SAME chain: without an instance boundary, the archived
	// change's grants would resurrect as workspace-permanent authority. The
	// chain itself cannot observe that boundary — the genesis revision
	// survives name reuse and a recreated change's first Begin is
	// indistinguishable from a legitimate next-work-unit advance — so the
	// identity is caller-owned and opaque here: ForInstance sets it, Grant
	// binds it into each grant record's digest, and replay projects a grant
	// into GrantedRoots only when the record's identity equals this one. The
	// zero value is the conservative containment: a store opened without an
	// instance identity projects no granted roots at all.
	instance string
}

// ForInstance derives a store bound to one change-instance identity. The
// value is opaque to the ledger; the caller (the status layer that also
// derives the consent envelope) owns its meaning, which must be stable
// across one change instance's life and distinct across recreations of the
// same change name.
func (store RuntimeStore) ForInstance(instance string) (RuntimeStore, error) {
	if instance == "" {
		return RuntimeStore{}, errors.New("change-instance identity must not be empty") // refusal:by-design operator-knowledge: only the caller knows the change instance this session serves; retry with the instance identity the status layer derived
	}
	if err := validateRuntimeText(instance, 128); err != nil {
		return RuntimeStore{}, fmt.Errorf("invalid change-instance identity: %w", err)
	}
	store.instance = instance
	return store, nil
}

type runtimeRecord struct {
	Schema           string               `json:"schema"`
	Change           string               `json:"change"`
	PreviousRevision string               `json:"previous_revision"`
	Operation        string               `json:"operation"`
	RequestID        string               `json:"request_id"`
	RequestDigest    string               `json:"request_digest"`
	Begin            *runtimeBeginEvent   `json:"begin,omitempty"`
	Finish           *runtimeFinishEvent  `json:"finish,omitempty"`
	Reset            *runtimeResetEvent   `json:"reset,omitempty"`
	Rescope          *runtimeRescopeEvent `json:"rescope,omitempty"`
	Repair           *runtimeRepairEvent  `json:"repair,omitempty"`
	Advance          *runtimeAdvanceEvent `json:"advance,omitempty"`
	Handoff          *RuntimeHandoff      `json:"handoff,omitempty"`
	Binding          *runtimeBindingEvent `json:"binding,omitempty"`
	Receipt          *runtimeReceiptEvent `json:"receipt,omitempty"`
	Grant            *runtimeGrantEvent   `json:"grant,omitempty"`
}

// runtimeGrantEvent is the persisted per-change edit-authority grant (#2540
// S2): a maintainer-authorized record that this change's apply actor may edit
// the named repository roots, recorded as canonical absolute symlink-evaluated
// paths following BeginWorktree's canonicalization precedent. GrantedAt is
// the ledger's FIRST wall-clock field: digest-safe (the content-addressed
// record revision binds it immutably) but excluded from the request digest
// and from determinism-replay expectations, which validate only that it
// parses. Like runtimeRescopeEvent, the REAL forgery guard is replay's digest
// recompute in validateRuntimeRecordShape: Roots, Actor, or Reason widened or
// altered after publication no longer match the bound RequestDigest, so
// replay refuses the chain.
type runtimeGrantEvent struct {
	Roots     []string `json:"roots"`
	Actor     string   `json:"actor"`
	Reason    string   `json:"reason"`
	GrantedAt string   `json:"granted_at"`
	// Instance is the change-instance identity this grant belongs to (#2540
	// S5), digest-bound like Roots/Actor/Reason: a record whose identity was
	// altered, stripped, or forged after publication no longer matches the
	// bound RequestDigest, so replay refuses the chain. Replay projects the
	// grant into GrantedRoots only for a store bound to the same identity,
	// which is what makes an archived name's reuse unable to resurrect the
	// archived change's authority. Required: no released writer ever emitted
	// an instance-less grant record (RuntimeStore.Grant was unreachable
	// between #2553 and this slice), so an empty value is a mutated record,
	// not a legacy one.
	Instance string `json:"instance"`
}

// runtimeAdvanceEvent accompanies the successor's begin event in one atomic
// record, so closing the passed objective and opening its successor can never
// be observed apart.
type runtimeAdvanceEvent struct {
	PreviousObjectiveID string `json:"previous_objective_id"`
	PreviousGeneration  int    `json:"previous_generation"`
	PreviousWorkUnit    string `json:"previous_work_unit"`
}

type runtimeBeginEvent struct {
	ObjectiveID            string `json:"objective_id"`
	ObjectiveGeneration    int    `json:"objective_generation,omitempty"`
	WorkUnit               string `json:"work_unit"`
	EvidenceGoal           string `json:"evidence_goal"`
	MaxAttempts            int    `json:"max_attempts"`
	MaxChangedLines        int    `json:"max_changed_lines"`
	Ordinal                int    `json:"ordinal"`
	BeginCandidateIdentity string `json:"begin_candidate_identity"`
	BeginCandidateTree     string `json:"begin_candidate_tree"`
	// BeginWorktree records store.Workspace at Begin time (#2296 part 1): the
	// resolved, symlink-evaluated absolute path of the exact --cwd this begin
	// ran under. omitempty is load-bearing — every record predating this field
	// deserializes it as "", which Finish and replay both treat as "no binding
	// recorded" rather than as a mismatch, so legacy chains replay unchanged.
	BeginWorktree     string `json:"begin_worktree,omitempty"`
	EffectiveWorktree string `json:"effective_worktree,omitempty"`
}

type runtimeResetEvent struct {
	PreviousObjectiveID    string `json:"previous_objective_id"`
	PreviousGeneration     int    `json:"previous_generation"`
	ResetCandidateIdentity string `json:"reset_candidate_identity"`
	ResetCandidateTree     string `json:"reset_candidate_tree"`
	Reason                 string `json:"reason"`
	Actor                  string `json:"actor"`
}

// runtimeRescopeEvent is the persisted record for AUDITED NARROWING RESCOPE.
// PreviousMaxAttempts/PreviousMaxChangedLines are carried for shape-level
// self-consistency (validateRuntimeRecordShape), but the REAL narrowing
// guard is applyRuntimeRescopeEvent's own recompute against replayed state —
// a forged record cannot lie about "previous" to make a widened claim look
// narrow, because replay recomputes the previous ceiling from the actual
// replayed objective, never from the record's own claim.
type runtimeRescopeEvent struct {
	PreviousObjectiveID      string `json:"previous_objective_id"`
	PreviousGeneration       int    `json:"previous_generation"`
	PreviousMaxAttempts      int    `json:"previous_max_attempts"`
	PreviousMaxChangedLines  int    `json:"previous_max_changed_lines"`
	RescopeCandidateIdentity string `json:"rescope_candidate_identity"`
	RescopeCandidateTree     string `json:"rescope_candidate_tree"`
	ObjectiveID              string `json:"objective_id"`
	ObjectiveGeneration      int    `json:"objective_generation"`
	WorkUnit                 string `json:"work_unit"`
	EvidenceGoal             string `json:"evidence_goal"`
	MaxAttempts              int    `json:"max_attempts"`
	MaxChangedLines          int    `json:"max_changed_lines"`
	Reason                   string `json:"reason"`
	Actor                    string `json:"actor"`
}

type runtimeRepairEvent struct {
	ReplacedRevision string `json:"replaced_revision"`
	RestoredRevision string `json:"restored_revision"`
	Reason           string `json:"reason"`
	Actor            string `json:"actor"`
}

type runtimeFinishEvent struct {
	Ordinal                    int                `json:"ordinal"`
	FinishCandidateIdentity    string             `json:"finish_candidate_identity"`
	FinishCandidateTree        string             `json:"finish_candidate_tree"`
	Outcome                    AttemptOutcome     `json:"outcome"`
	ChangedLines               int                `json:"changed_lines"`
	EvidenceRevision           string             `json:"evidence_revision"`
	Diagnosis                  string             `json:"diagnosis"`
	HarnessDisposition         HarnessDisposition `json:"harness_disposition"`
	CleanupEvidence            string             `json:"cleanup_evidence"`
	ProcessEvidence            string             `json:"process_evidence"`
	RemediatesEvidenceRevision string             `json:"remediates_evidence_revision,omitempty"`
	ChangedLineBudgetExceeded  bool               `json:"changed_line_budget_exceeded,omitempty"`
}

type runtimeBindingEvent struct {
	ExpectedRevision string                      `json:"expected_revision"`
	Current          ReviewBinding               `json:"current"`
	LegacyImport     *runtimeLegacyBindingImport `json:"legacy_import,omitempty"`
}

type runtimeLegacyBindingImport struct {
	SourceDigest string        `json:"source_digest"`
	Binding      ReviewBinding `json:"binding"`
}

type runtimeRequestReceipt struct {
	Digest                        string
	Revision                      string
	RemediationPredecessorLineage string
}

type runtimeReplay struct {
	Status        RuntimeStatus
	Requests      map[string]runtimeRequestReceipt
	AttemptTokens map[int]string
	// Instance carries the store's ForInstance identity into replay (#2540
	// S5): applyRuntimeGrantEvent projects a grant into GrantedRoots only
	// when the record's identity equals this one. Empty projects nothing.
	Instance string
}

func OpenRuntimeStore(ctx context.Context, repo, change string) (RuntimeStore, error) {
	if !validReviewBindingChange(change) {
		return RuntimeStore{}, fmt.Errorf("invalid SDD change name %q; want letters, digits, and single hyphens or underscores between them, at most 96 characters; run `gentle-ai sdd-status --cwd <repo> --json` to read the resolved changeName", change)
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return RuntimeStore{}, err
	}
	workspace, err := filepath.Abs(repo)
	if err != nil {
		return RuntimeStore{}, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return RuntimeStore{}, err
	}
	probe, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, "sdd-runtime-probe")
	if err != nil {
		return RuntimeStore{}, err
	}
	commonDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(probe.Dir))))
	dir := runtimeChangeLedgerDir(filepath.Join(commonDir, "gentle-ai", "sdd-runtime"), change)
	return RuntimeStore{Dir: dir, Repo: root, Workspace: workspace, Change: change, commonDir: commonDir}, nil
}

// encodedRuntimeChangeNamespace holds identities that cannot be a directory
// name verbatim. A leading underscore is unreachable for a legacy identity, so
// the namespace can never collide with a kebab-case change's own ledger.
const encodedRuntimeChangeNamespace = "_encoded"

// runtimeChangeLedgerDir derives the ledger directory for a change identity.
//
// A legacy kebab-case identity keeps its exact v1/<change> directory, so every
// attempt chain written by an earlier version stays reachable. Anything else is
// encoded, because the directory name alone cannot carry the identity: on a
// case-insensitive filesystem "DEC-X" and "dec-x" would share one directory and
// silently merge two unrelated attempt chains. Lowercasing makes the path
// stable across those filesystems and the digest of the verbatim identity keeps
// the case variants apart.
//
// The suffix keeps 128 bits rather than the whole digest. Every identity that
// shares a lowercased form differs only in case, so a name with k letters has
// 2^k variants to search: at 64 bits a birthday collision costs about 2^32
// candidates, which is reachable, while 128 bits puts it at 2^64. The remaining
// half is dropped because the leaf must stay addressable on Windows, where an
// identity at the 96-character limit plus a full 64-character digest crowds the
// 260-character path ceiling that this issue's original reporter was hitting.
const encodedRuntimeChangeDigestWidth = 32

func runtimeChangeLedgerDir(base, change string) string {
	if legacyRuntimeChangeDir(change) {
		return filepath.Join(base, "v1", change)
	}
	digest := strings.TrimPrefix(runtimeValueHash("gentle-ai.sdd-runtime-change-identity/v1", change), "sha256:")
	return filepath.Join(base, "v1", encodedRuntimeChangeNamespace, strings.ToLower(change)+"-"+digest[:encodedRuntimeChangeDigestWidth])
}

func (store RuntimeStore) Status() (RuntimeStatus, error) {
	replay, err := store.load()
	return replay.Status, err
}

// runtimeObjectiveHasRecordedAttempt reports whether ANY attempt in the
// replayed history was recorded under status.Objective's exact ID. Before
// Rescope existed, status.Objective != nil always implied at least one
// attempt existed under it (Begin and the Advance/Reset transitions that
// clear Objective are the only ways to open or close one, and Begin always
// creates the first attempt atomically with the objective). Rescope breaks
// that implication on purpose: it opens a new objective without an attempt,
// so Begin's dispatch (and its replay-time mirror in applyRuntimeBeginEvent)
// must distinguish "this objective already has a terminal Finish to chase"
// from "this objective was just opened and has none yet" instead of blindly
// inspecting the last recorded attempt, which would still belong to the
// objective THIS one just superseded.
func runtimeObjectiveHasRecordedAttempt(status RuntimeStatus) bool {
	if status.Objective == nil {
		return false
	}
	for index := len(status.Attempts) - 1; index >= 0; index-- {
		if status.Attempts[index].ObjectiveID == status.Objective.ID {
			return true
		}
	}
	return false
}

func (store RuntimeStore) Begin(ctx context.Context, request BeginAttemptRequest) (RuntimeStatus, error) {
	request, err := normalizeBeginAttemptRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request)
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(replay runtimeReplay) (runtimeRecord, error) {
		status := replay.Status
		// Every precondition, ledger-side and repository-side, is evaluated by
		// the one predicate the read-only surfaces evaluate too, under this
		// lock and against this exact replay, so a read that said "admitted"
		// a moment ago cannot let a stale verdict through here.
		admission, err := store.runtimeBeginAdmission(ctx, status, request)
		if err != nil {
			return runtimeRecord{}, err
		}
		advancing, generation, snapshot := admission.Advancing, admission.Generation, admission.Snapshot
		objectiveID := runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, snapshot.Identity, generation)
		if status.Objective != nil && !advancing {
			objectiveID = status.Objective.ID
		}
		event := &runtimeBeginEvent{
			ObjectiveID: objectiveID, ObjectiveGeneration: generation, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
			MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Ordinal: status.NextOrdinal, BeginCandidateIdentity: snapshot.Identity, BeginCandidateTree: snapshot.CandidateTree,
			BeginWorktree: store.Workspace, EffectiveWorktree: store.Workspace,
		}
		if advancing {
			return runtimeRecord{Operation: runtimeOperationAdvance, Begin: event, Advance: &runtimeAdvanceEvent{
				PreviousObjectiveID: status.Objective.ID, PreviousGeneration: status.Objective.Generation,
				PreviousWorkUnit: status.Objective.WorkUnit,
			}}, nil
		}
		return runtimeRecord{Operation: runtimeOperationBegin, Begin: event}, nil
	})
}

func (store RuntimeStore) Finish(ctx context.Context, request FinishAttemptRequest) (RuntimeStatus, error) {
	request, err := normalizeFinishAttemptRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", request)
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(replay runtimeReplay) (runtimeRecord, error) {
		status := replay.Status
		active := status.ActiveAttempt
		if active == nil {
			return runtimeRecord{}, ErrRuntimeNoActiveAttempt
		}
		// Check the effective binding before candidate capture or line charging.
		if active.EffectiveWorktree != "" && active.EffectiveWorktree != store.Workspace {
			return runtimeRecord{}, store.runtimeEffectiveWorktreeMismatchRefusal(*active)
		}
		if active.EffectiveWorktree == "" && active.BeginWorktree != "" && active.BeginWorktree != store.Workspace {
			return runtimeRecord{}, store.runtimeWorktreeMismatchRefusal(active.Ordinal, active.BeginWorktree)
		}
		remediation := finishRequestsRemediation(request)
		unmanagedRemediation := finishRequestsUnmanagedRemediation(request)
		currentBinding := status.Binding
		var legacyBinding *ReviewBinding
		var legacyDigest string
		if request.Outcome == AttemptPassed && currentBinding == nil && (!store.ReviewDisabled || remediation) {
			legacyBinding, legacyDigest, err = store.readLegacyBinding()
			if err != nil {
				return runtimeRecord{}, fmt.Errorf("read legacy SDD review binding for remediation: %w", err)
			}
			currentBinding = legacyBinding
		}
		if remediation {
			if currentBinding == nil {
				return runtimeRecord{}, errors.New("atomic SDD remediation successor requires a populated native binding")
			}
			if currentBinding.Revision != request.ExpectedBindingRevision {
				return runtimeRecord{}, &BindingRevisionConflictError{Expected: request.ExpectedBindingRevision, Current: currentBinding.Revision}
			}
			if status.EvidenceRevision != "" && status.EvidenceRevision != request.RemediatesEvidenceRevision {
				return runtimeRecord{}, fmt.Errorf("failed evidence revision %q does not match native runtime evidence %q", request.RemediatesEvidenceRevision, status.EvidenceRevision)
			}
		}
		// #1974 slice 2: the unmanaged-remediation binding derives from the
		// immutable attempt chain, never from status.EvidenceRevision -- the
		// live pointer Reset, Rescope, and Advance wipe. An audited reset or an
		// honest interrupted settlement between the failure and its correction
		// is an audit record, not a semantic successor, so neither severs the
		// binding; the first passed settlement after the failure does.
		chainFailedAttempt, chainHasFailedEvidence := runtimeChainFailedAttempt(status.Attempts)
		chainFailedEvidence := chainFailedAttempt.EvidenceRevision
		if unmanagedRemediation {
			if !store.ReviewDisabled {
				// No by-design marker: this names a runnable continuation inline.
				return runtimeRecord{}, errors.New("unmanaged remediation requires disabled delivery; enable it or run `gentle-ai review mode disable --scope clone --cwd <repo>` for this repository only")
			}
			// A binding that predates the switch does NOT block this. The
			// contract on ReviewDisabled says that while review is off it "does
			// not exist, so it must have no implications", and a leftover
			// binding is an implication: it made the switch inert only for
			// changes that had never used review, so turning it off after the
			// fact bought nothing. The binding stays recorded and re-enabling
			// re-validates from the current state.
			if !chainHasFailedEvidence {
				// #2881: the caller named a real failure, and their own earlier
				// slice already repaired it. Blaming their input sent the
				// reporter looking for authority to guess at; the state is what
				// changed, and the exit is to stop claiming a remediation.
				if discharged, ordinal, ok := runtimeDischargedFailure(status.Attempts, request.RemediatesEvidenceRevision); ok {
					return runtimeRecord{}, runtimeDischargedFailureRefusal(discharged, ordinal)
				}
				// No by-design marker: this names a runnable continuation inline.
				return runtimeRecord{}, errors.New("this correction names failed verification " + request.RemediatesEvidenceRevision + ", but the attempt chain records no failed verification at all; run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` to read the chain, then settle without --remediates-evidence-revision if nothing is being repaired")
			}
			if chainFailedEvidence != request.RemediatesEvidenceRevision {
				// No by-design marker: this names a runnable continuation inline.
				return runtimeRecord{}, errors.New("this correction names failed verification " + request.RemediatesEvidenceRevision + ", but the chain's unremediated failure is " + chainFailedEvidence + "; settle with --remediates-evidence-revision \"" + chainFailedEvidence + "\", or without the flag if this work unit repairs nothing")
			}
		}
		// Issue #2394: the runtime candidate is the same declared candidate
		// review freezes -- tracked changes plus whatever the user put in the
		// index. Sweeping the worktree here would make drift detection and
		// review disagree about what the candidate even is.
		snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).Build(ctx, reviewtransaction.Target{
			Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: active.BeginCandidateTree,
			Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
		})
		if err != nil {
			return runtimeRecord{}, wrapRuntimeCandidateUnavailable("after attempt", err)
		}
		changedLines, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).ChangedLines(ctx, snapshot)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("measure native SDD runtime line charge: %w", err)
		}
		// Review acts AFTER implementation and verification, on the finished
		// result. This package already implements that twice: applyReviewOfferRouting
		// fires only once verify has passed, and resolveNextRecommended never
		// routes to review before verify.
		//
		// A third rule used to disagree with both. A passing IMPLEMENTATION
		// attempt was refused whenever the review binding covered the
		// pre-attempt bytes — which it always does, because changing the
		// candidate is what an attempt is for. That is review deciding whether
		// implementation may finish, before any verification has run, and it is
		// #1993: a maintainer-authorized correction passed every gate and could
		// not be closed, while the only named exit required a review the review
		// side was structurally unable to produce.
		//
		// Nothing is given up. The delivery gates (post-apply, pre-commit,
		// pre-push, pre-pr, release) re-derive their verdict from the candidate
		// actually being delivered, so an unreviewed candidate is still refused
		// there. The binding and receipt stay recorded on the ledger as
		// metadata: review stops deciding, it does not stop being tracked.
		if store.ReviewDisabled && currentBinding == nil && request.Outcome == AttemptPassed && chainHasFailedEvidence && !unmanagedRemediation {
			return runtimeRecord{}, fmt.Errorf("disabled failed verification requires --remediates-evidence-revision %q on the correction settle; rerun `gentle-ai sdd-attempt settle` with that flag", chainFailedEvidence)
		}
		if unmanagedRemediation {
			evidenceOnly := runtimeEvidenceOnlyRetryAuthorized(status.LastReset, status.LastRescope, chainFailedAttempt, snapshot.CandidateTree)
			if !evidenceOnly && (snapshot.Identity == active.BeginCandidateIdentity || snapshot.CandidateTree == active.BeginCandidateTree) {
				// refusal:by-design operator-knowledge: a remediation claim must name a candidate changed by the active correction attempt, or an audited reset or rescope authorizing this exact unchanged candidate.
				return runtimeRecord{}, errors.New("unmanaged remediation requires a changed correction candidate")
			}
			if request.EvidenceRevision == request.RemediatesEvidenceRevision {
				// refusal:by-design operator-knowledge: the correction's verification evidence must be fresh and distinct from the failed evidence it repairs.
				return runtimeRecord{}, errors.New("unmanaged remediation requires fresh corrected evidence")
			}
		}
		event := &runtimeFinishEvent{
			Ordinal: active.Ordinal, FinishCandidateIdentity: snapshot.Identity, FinishCandidateTree: snapshot.CandidateTree,
			Outcome: request.Outcome, ChangedLines: changedLines, EvidenceRevision: request.EvidenceRevision,
			Diagnosis: request.Diagnosis, HarnessDisposition: request.HarnessDisposition,
			CleanupEvidence: request.CleanupEvidence, ProcessEvidence: request.ProcessEvidence,
			RemediatesEvidenceRevision: request.RemediatesEvidenceRevision,
			ChangedLineBudgetExceeded:  status.CumulativeChangedLines+changedLines > status.Objective.MaxChangedLines,
		}
		if remediation {
			prepared, prepareErr := prepareApprovedRuntimeSuccessorBinding(ctx, store.Repo, store.Workspace, store.Change, request.SuccessorLineageID)
			if prepareErr != nil {
				return runtimeRecord{}, prepareErr
			}
			// An approved self-successor is the same lineage whose corrected,
			// re-approved authority repairs the failed evidence. It never
			// requires invalidating healthy approved authority or minting a
			// distinct recovery lineage for an unchanged scope.
			selfSuccessor := prepared.Lineage == currentBinding.Lineage
			if selfSuccessor && request.EvidenceRevision == request.RemediatesEvidenceRevision {
				return runtimeRecord{}, errors.New("approved SDD self-remediation requires distinct corrected evidence")
			}
			validateSuccessor := validateRuntimeRemediationSuccessor
			if selfSuccessor {
				validateSuccessor = validateRuntimeRemediationSelfSuccessor
			}
			if relationErr := validateSuccessor(ctx, store.Repo, *currentBinding, prepared); relationErr != nil {
				return runtimeRecord{}, relationErr
			}
			runtimeRemediationFinalAuthorizationHook()
			finalPrepared, finalPrepareErr := prepareApprovedRuntimeSuccessorBinding(ctx, store.Repo, store.Workspace, store.Change, request.SuccessorLineageID)
			if finalPrepareErr != nil {
				return runtimeRecord{}, fmt.Errorf("approved SDD remediation successor changed before native commit: %w", finalPrepareErr)
			}
			if finalPrepared.Revision != prepared.Revision {
				return runtimeRecord{}, errors.New("approved SDD remediation successor changed before native commit")
			}
			if relationErr := validateSuccessor(ctx, store.Repo, *currentBinding, finalPrepared); relationErr != nil {
				return runtimeRecord{}, relationErr
			}
			if finalPrepared.GateContext.CandidateTree != snapshot.CandidateTree {
				return runtimeRecord{}, runtimeChargedCandidateRefusal(
					finalPrepared.GateContext.CandidateTree, snapshot.CandidateTree, finalPrepared.Lineage)
			}
			prepared = finalPrepared
			bindingEvent := &runtimeBindingEvent{ExpectedRevision: request.ExpectedBindingRevision, Current: prepared}
			if legacyBinding != nil {
				finalLegacy, finalDigest, finalErr := store.readLegacyBinding()
				if finalErr != nil || finalLegacy == nil || finalDigest != legacyDigest {
					return runtimeRecord{}, errors.New("legacy SDD review binding changed before atomic remediation import")
				}
				bindingEvent.LegacyImport = &runtimeLegacyBindingImport{SourceDigest: legacyDigest, Binding: *legacyBinding}
			}
			return runtimeRecord{
				Operation: runtimeOperationFinishRemediation, Finish: event,
				Binding: bindingEvent,
			}, nil
		}
		return runtimeRecord{Operation: runtimeOperationFinish, Finish: event}, nil
	})
}

func (store RuntimeStore) Handoff(ctx context.Context, request HandoffAttemptRequest) (RuntimeStatus, error) {
	request, err := normalizeHandoffAttemptRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-handoff-request/v1", request)
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(replay runtimeReplay) (runtimeRecord, error) {
		active := replay.Status.ActiveAttempt
		if active == nil {
			return runtimeRecord{}, ErrRuntimeNoActiveAttempt
		}
		if active.EffectiveWorktree == "" || active.EffectiveWorktree != store.Workspace {
			return runtimeRecord{}, store.runtimeHandoffSourceRefusal(*active)
		}
		if active.Handoff != nil {
			return runtimeRecord{}, store.runtimeHandoffAlreadyPerformedRefusal(*active)
		}
		commonDir, err := store.validateRuntimeHandoffDestination(ctx, request.DestinationWorktree)
		if err != nil {
			return runtimeRecord{}, err
		}
		snapshot, err := captureRuntimeHandoffCandidate(ctx, request.DestinationWorktree, active.BeginCandidateTree)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture delegated SDD runtime candidate before handoff: %w", err)
		}
		return runtimeRecord{Operation: runtimeOperationHandoff, Handoff: &RuntimeHandoff{
			Ordinal: active.Ordinal, SourceWorktree: store.Workspace, DestinationWorktree: request.DestinationWorktree,
			CommonDir: commonDir, ExpectedRevision: request.ExpectedRevision, RequestDigest: digest,
			DestinationCandidateIdentity: snapshot.Identity, DestinationCandidateTree: snapshot.CandidateTree,
		}}, nil
	})
}

// runtimeRemediationExitRefusal, runtimeStrandedSuccessorRefusal and
// runtimeRemediatesArgument are deleted with the demand they served. They
// existed to tell an operator which review exit would clear a gate that no
// longer exists, and leaving them uncalled is how the coupling grows back.

// runtimeWorktreeMismatchRefusal turns a cross-worktree finish into a refusal
// that names the exact --cwd the ledger's candidate/diff math is pinned to.
//
// The demand itself is correct: Begin captured the base candidate tree
// relative to the exact --cwd it ran under, so a Finish issued from a
// DIFFERENT linked worktree would build and diff the wrong working tree
// against that pinned base — a diff between two unrelated trees, not the
// work this attempt actually did. The shared Git-common-dir ledger makes
// every linked worktree able to reach and mutate the same chain, which is
// exactly what makes the wrong --cwd able to slip through silently instead
// of failing to open the store at all.
//
// The exit is unconditional and requires no read-only eligibility probe the
// way runtimeRemediationExitRefusal's two branches do: the recorded
// begin-worktree path is itself always the one runnable continuation, so
// there is only one message to construct.
// runtimeZeroDriftResetRefusal replaces the bare ErrRuntimeResetNotAllowed at
// the one refusal site whose state has a runnable continuation the sentinel
// never named. The sentinel says which shapes reset admits and then points at
// status; status answers next_action: begin, and for a caller whose failed
// evidence proves the OBJECTIVE is wrong rather than under-attempted, re-running
// the identical objective is the one continuation that cannot help. #1974 was
// filed as a lifecycle deadlock for exactly that reason: rescope already owned
// this transition and neither surface said so.
//
// Both named exits are real for this state, which is why both appear. Rescope
// is admitted here by construction (this site is reached only when reset is
// structurally permitted, the objective is neither decision-required nor
// complete, and the candidate has not drifted, which is precisely
// runtimeObjectiveRescopeStructurallyPermitted plus zero drift), so it needs no
// second eligibility probe. It can only narrow or hold the budget, so a caller
// who needs a WIDER successor scope gets the second exit instead: spend the
// remaining attempts honestly, which is what reaches decision-required, where
// this same reset is admitted and opens a fresh budget.
func (store RuntimeStore) runtimeZeroDriftResetRefusal(status RuntimeStatus) error {
	objective := status.Objective
	return fmt.Errorf(
		"%w: this objective's candidate has not drifted and it still has attempts left, so resetting it now would launder the per-objective budget. If the failed evidence proves this OBJECTIVE is wrong rather than under-attempted, a maintainer may open a narrower successor scope instead — `gentle-ai sdd-attempt rescope --cwd %q --change %q --expected-revision %q --request-id \"<unique-request-id>\" --work-unit \"<narrower-work-unit>\" --evidence-goal \"<narrower-evidence-goal>\" --max-attempts \"<n, at most %d>\" --max-changed-lines \"<n, at most %d>\" --reason \"<why-the-objective-is-narrowing>\" --actor \"<actor>\"`; rescope carries cumulative_attempts and cumulative_changed_lines forward unchanged and never widens a budget, so if the successor needs MORE than %d changed lines, spend this objective's remaining attempts first: the run that exhausts them reaches decision-required, where this reset is admitted",
		ErrRuntimeResetNotAllowed, store.Workspace, store.Change, status.Revision,
		objective.MaxAttempts, objective.MaxChangedLines, objective.MaxChangedLines)
}

// runtimeRescopeWidenedRefusal is the complement of
// runtimeZeroDriftResetRefusal (#1974). That one caught the caller who reached
// for reset and was told nothing about rescope; this catches the caller who
// reached for rescope and is told only that a wider budget is refused. The
// state is the same one, so status answers next_action: begin, which repeats
// the objective at the same ceiling the caller just said is too small.
//
// The route that does work is not another operation, it is finishing this one:
// spending the remaining attempts reaches decision-required, and reset is
// admitted there with a budget of any size. Naming it costs nothing and does
// not weaken the narrowing rule, which exists so a successor scope cannot
// launder a consumed budget.
func (store RuntimeStore) runtimeRescopeWidenedRefusal(status RuntimeStatus, flag string, requested, allowed int) error {
	remaining := status.Objective.MaxAttempts - status.CumulativeAttempts
	return fmt.Errorf(
		"%w: received %s %d, the current objective allows %d. A wider successor scope is reached by finishing this objective rather than by rescoping it: spend its %d remaining attempt(s), and the run that exhausts them reaches decision-required, where `gentle-ai sdd-attempt reset --cwd %q --change %q --expected-revision \"<revision-from-status>\" --request-id \"<unique-request-id>\" --reason \"<why-the-objective-changed>\" --actor \"<actor>\"` opens a fresh budget of any size",
		ErrRuntimeRescopeWidened, flag, requested, allowed, remaining, store.Workspace, store.Change)
}

// runtimeObjectiveCompleteRefusal names what a completed objective's begin can
// actually do. The sentinel said only that the objective is complete and
// pointed at status, whose next_action is `complete` — true, and useless to a
// caller who has more work to do on this change.
//
// Complete is only reachable from a passed attempt within budget, so advance is
// admissible here whenever the sole failing condition is the repeated work
// unit: changing --work-unit is the entire difference. Reset is admitted too,
// for a caller who means to discard this scope rather than succeed it.
func (store RuntimeStore) runtimeObjectiveCompleteRefusal(status RuntimeStatus) error {
	return fmt.Errorf(
		"%w: it passed within budget, so this change continues through a SUCCESSOR objective, not a repeat of this one — re-run this begin with a different --work-unit (everything else may stay as it is) and it is admitted as an advance that carries this objective's evidence forward. To discard this scope instead of succeeding it, run `gentle-ai sdd-attempt reset --cwd %q --change %q --expected-revision %q --request-id \"<unique-request-id>\" --reason \"<why-the-objective-changed>\" --actor \"<actor>\"`",
		ErrRuntimeObjectiveDone, store.Workspace, store.Change, status.Revision)
}

func (store RuntimeStore) runtimeWorktreeMismatchRefusal(ordinal int, beginWorktree string) error {
	return fmt.Errorf(
		"%w: attempt %d began in %s, and its base candidate tree is pinned to that exact linked worktree, but this finish is running from %s — rerun with --cwd %s so the candidate capture and changed-line measurement use the worktree that actually did the work",
		ErrRuntimeWorktreeMismatch, ordinal, pathquote.Quote(beginWorktree), pathquote.Quote(store.Workspace), pathquote.Quote(beginWorktree))
}

func (store RuntimeStore) runtimeEffectiveWorktreeMismatchRefusal(active RuntimeAttempt) error {
	if active.EffectiveWorktree == active.BeginWorktree {
		return store.runtimeWorktreeMismatchRefusal(active.Ordinal, active.BeginWorktree)
	}
	return fmt.Errorf(
		"%w: attempt %d began in %s and was explicitly handed off to %s, but this finish is running from %s — rerun with --cwd %s so the candidate capture and changed-line measurement use the delegated worktree",
		ErrRuntimeWorktreeMismatch, active.Ordinal, pathquote.Quote(active.BeginWorktree), pathquote.Quote(active.EffectiveWorktree), pathquote.Quote(store.Workspace), pathquote.Quote(active.EffectiveWorktree))
}

// runtimeObjectiveChangeRefusal turns the changed-objective begin demand into
// a refusal that names the continuation that actually clears it.
//
// The demand itself is correct: an objective is an immutable scope, so a begin
// whose parameters or whose terminal candidate no longer match the recorded
// one would silently reopen a different piece of work under the old budget.
// What was missing is the exit. The sentinel says "without an explicit reset",
// which names an operation and never a command — the reset needs --cwd,
// --change, --expected-revision, --request-id, --reason and --actor, and a
// caller who has only the sentinel has none of them.
//
// The states are distinguished rather than merged, because a message may
// name a command only when running it resolves the block. `reset` is refused
// one layer deeper for an unchanged (non-drifted) candidate whose budget is
// intact (runtimeResetStructurallyPermitted plus the drift check in Reset),
// because an elective early reset would launder the per-objective budget.
// Naming it there would be the same defect in a new place. Since #2298 /
// #2296 part 2, that non-drifted terminal shape is no longer a dead end: it
// is exactly AUDITED NARROWING RESCOPE's admitted precondition
// (runtimeObjectiveRescopeAdmissible), so the second branch names rescope
// instead of falling straight to "begin against the scope the ledger holds"
// -- printing that message for THIS state was the dead end #2298 / #2296
// part 2 reported, because begin only ever re-offers the same objective and
// reset is refused for lack of drift. The plain "begin against the scope"
// branch survives only as the fail-closed default for a candidate-capture
// failure, where neither reset nor rescope can be verified admissible.
func (store RuntimeStore) runtimeObjectiveChangeRefusal(ctx context.Context, status RuntimeStatus) error {
	// The sentinel stays in the %w position in every branch: callers that
	// route on errors.Is must keep working no matter which exit is named.
	if store.runtimeObjectiveResetAdmissible(ctx, status) {
		return fmt.Errorf(
			"%w: reset the objective, then begin again — `gentle-ai sdd-attempt reset --cwd %s --change %q --expected-revision %q --request-id \"<unique-request-id>\" --reason \"<why-the-objective-changed>\" --actor \"<actor>\"`; the reset publishes a new ledger revision, so take the begin's --expected-revision from `gentle-ai sdd-attempt status --cwd %s --change %q` after it commits",
			ErrRuntimeObjectiveChange, pathquote.Quote(store.Workspace), store.Change, status.Revision, pathquote.Quote(store.Workspace), store.Change)
	}
	if store.runtimeObjectiveRescopeAdmissible(ctx, status) {
		objective := status.Objective
		return fmt.Errorf(
			"%w: this objective's candidate has not drifted, so resetting it is refused as an elective budget reset, but a maintainer may authorize a narrower successor scope instead — `gentle-ai sdd-attempt rescope --cwd %s --change %q --expected-revision %q --request-id \"<unique-request-id>\" --work-unit \"<narrower-work-unit>\" --evidence-goal \"<narrower-evidence-goal>\" --max-attempts \"<n, at most %d>\" --max-changed-lines \"<n, at most %d>\" --reason \"<why-the-objective-is-narrowing>\" --actor \"<actor>\"`; rescope carries cumulative_attempts and cumulative_changed_lines forward unchanged and refuses any max_attempts or max_changed_lines above the current objective's %d and %d",
			ErrRuntimeObjectiveChange, pathquote.Quote(store.Workspace), store.Change, status.Revision,
			objective.MaxAttempts, objective.MaxChangedLines, objective.MaxAttempts, objective.MaxChangedLines)
	}
	objective := status.Objective
	if objective == nil {
		// Unreachable from Begin, which only reaches this refusal with a
		// populated objective, and fail-closed if that ever stops holding:
		// inventing an objective to print would be worse than the sentinel.
		return ErrRuntimeObjectiveChange
	}
	return fmt.Errorf(
		"%w: this objective is still open on its recorded scope and its candidate has not moved, so resetting it is refused as an elective budget reset; begin against the scope the ledger holds — `gentle-ai sdd-attempt begin --cwd %s --change %q --expected-revision %q --request-id \"<unique-request-id>\" --work-unit %q --evidence-goal %q --max-attempts %d --max-changed-lines %d`; `sdd-attempt status` publishes those four as objective.work_unit, objective.evidence_goal, objective.max_attempts, and objective.max_changed_lines",
		ErrRuntimeObjectiveChange, pathquote.Quote(store.Workspace), store.Change, status.Revision,
		objective.WorkUnit, objective.EvidenceGoal, objective.MaxAttempts, objective.MaxChangedLines)
}

// runtimeObjectiveResetAdmissible answers exactly one question: would `reset`
// be ACCEPTED right now? It re-runs the same structural rule and the same
// drift check Reset itself applies, against the same terminal candidate
// provenance, so a refusal that consults it can only name the reset when
// running that reset works. It is deliberately read-only and fail-closed: an
// active attempt, a missing objective, a non-terminal last attempt, or a
// candidate capture that fails all answer false, and the caller then names the
// begin the ledger already accepts instead of a command refused one layer in.
func (store RuntimeStore) runtimeObjectiveResetAdmissible(ctx context.Context, status RuntimeStatus) bool {
	if status.ActiveAttempt != nil || status.Objective == nil || !runtimeResetStructurallyPermitted(status) {
		return false
	}
	if status.DecisionRequired || status.Complete {
		return true
	}
	last := status.Attempts[len(status.Attempts)-1]
	candidate, err := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree)
	if err != nil {
		return false
	}
	return candidate.Identity != last.FinishCandidateIdentity || candidate.CandidateTree != last.FinishCandidateTree
}

// runtimeObjectiveRescopeAdmissible answers exactly one question: would
// `rescope` be STRUCTURALLY ACCEPTED right now (leaving only the caller's own
// narrower scope choice, which only the caller can supply, to satisfy the
// narrowing guard)? It mirrors runtimeObjectiveResetAdmissible's read-only,
// fail-closed style over the exact opposite drift outcome: an active
// attempt, a missing objective, decision-required, complete, a non-terminal
// last attempt, or a candidate capture that fails all answer false; a
// candidate that captured successfully and did NOT drift answers true.
func (store RuntimeStore) runtimeObjectiveRescopeAdmissible(ctx context.Context, status RuntimeStatus) bool {
	if status.ActiveAttempt != nil || status.Objective == nil || !runtimeObjectiveRescopeStructurallyPermitted(status) {
		return false
	}
	last := status.Attempts[len(status.Attempts)-1]
	candidate, err := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree)
	if err != nil {
		return false
	}
	return candidate.Identity == last.FinishCandidateIdentity && candidate.CandidateTree == last.FinishCandidateTree
}

// runtimeObjectiveAdvanceAdmissible answers whether a passed objective may hand
// the change to the requested successor scope. It is deliberately narrower than
// reset, which reopens any structurally terminal objective: advance requires the
// previous objective to have PASSED and the request to name a different work
// unit. Restating the settled work unit — even under a reworded evidence goal —
// stays complete, because that objective really is done and a fresh budget for
// it would be exactly the laundering the reset guard already refuses.
//
// It is read-only and fail-closed, and it applies the same rule the replay
// validator re-checks, so an admitted advance can only be one the ledger
// accepts.
func runtimeObjectiveAdvanceAdmissible(status RuntimeStatus, request BeginAttemptRequest) bool {
	if !status.Complete || status.DecisionRequired || status.ActiveAttempt != nil ||
		status.Objective == nil || len(status.Attempts) == 0 {
		return false
	}
	if request.WorkUnit == status.Objective.WorkUnit {
		return false
	}
	last := status.Attempts[len(status.Attempts)-1]
	return last.ObjectiveID == status.Objective.ID && last.Outcome == AttemptPassed &&
		!last.ChangedLineBudgetExceeded && last.FinishCandidateIdentity != "" && last.FinishCandidateTree != ""
}

// runtimeGrantClock is the ledger's only wall-clock source (#2540 S2), a
// package variable solely so tests can pin it; production records UTC.
var runtimeGrantClock = func() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// Grant commits a per-change edit-authority grant record (#2540 S2). It only
// records and projects: AllowedEditRoots expansion, the consent envelope, and
// the CLI verb are later slices. A grant has no structural precondition,
// because authorizing roots is orthogonal to attempt state.
func (store RuntimeStore) Grant(ctx context.Context, request GrantRootsRequest) (RuntimeStatus, error) {
	if store.instance == "" {
		return RuntimeStatus{}, errors.New("grant requires a change-instance identity; derive the store with ForInstance first") // refusal:by-design operator-knowledge: only the caller knows which change instance this grant authorizes; derive the store with ForInstance and retry
	}
	if request.ChangeInstance != "" && request.ChangeInstance != store.instance {
		return RuntimeStatus{}, errors.New("grant request change-instance does not equal the store's instance identity") // refusal:by-design operator-knowledge: the request and the store name different change instances; retry with one coherent instance identity
	}
	request.ChangeInstance = store.instance
	request, err := normalizeGrantRootsRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request)
	grantedAt := runtimeGrantClock()
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(runtimeReplay) (runtimeRecord, error) {
		return runtimeRecord{Operation: runtimeOperationGrant, Grant: &runtimeGrantEvent{
			Roots: request.Roots, Actor: request.Actor, Reason: request.Reason, GrantedAt: grantedAt,
			Instance: request.ChangeInstance,
		}}, nil
	})
}

// Reset closes a terminal objective scope without deleting its immutable
// attempts. The next Begin receives a new generation and budget while global
// ordinals and lifetime charges continue monotonically.
func (store RuntimeStore) Reset(ctx context.Context, request ResetObjectiveRequest) (RuntimeStatus, error) {
	request, err := normalizeResetObjectiveRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-reset-request/v1", request)
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(replay runtimeReplay) (runtimeRecord, error) {
		status := replay.Status
		if status.ActiveAttempt != nil {
			return runtimeRecord{}, ErrRuntimeAttemptActive
		}
		if status.Objective == nil {
			return runtimeRecord{}, ErrRuntimeNoObjective
		}
		if !runtimeResetStructurallyPermitted(status) {
			return runtimeRecord{}, ErrRuntimeResetNotAllowed
		}
		if !status.DecisionRequired && !status.Complete {
			// The only remaining structurally-permitted scope is a terminal
			// failed/interrupted attempt with budget still available: begin
			// is the ordinary continuation here, so admit reset only when
			// begin is actually blocked by candidate drift. Otherwise an
			// elective early reset would launder the per-objective budget
			// (CumulativeAttempts resets to zero on every reset).
			last := status.Attempts[len(status.Attempts)-1]
			candidate, driftErr := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree)
			if driftErr != nil {
				return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate to check reset drift eligibility: %w", driftErr)
			}
			if candidate.Identity == last.FinishCandidateIdentity && candidate.CandidateTree == last.FinishCandidateTree {
				return runtimeRecord{}, store.runtimeZeroDriftResetRefusal(status)
			}
		}
		snapshot, err := captureRuntimeCandidate(ctx, store.Repo)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate at objective reset: %w", err)
		}
		return runtimeRecord{Operation: runtimeOperationReset, Reset: &runtimeResetEvent{
			PreviousObjectiveID: status.Objective.ID, PreviousGeneration: status.Objective.Generation,
			ResetCandidateIdentity: snapshot.Identity, ResetCandidateTree: snapshot.CandidateTree,
			Reason: request.Reason, Actor: request.Actor,
		}}, nil
	})
}

// runtimeObjectiveRescopeStructurallyPermitted reports whether the ledger's
// terminal scope (no active attempt, which callers must verify separately)
// is the exact shape AUDITED NARROWING RESCOPE admits: NOT decision-required,
// NOT complete, and the last recorded attempt is a terminal failure or
// interruption. This is the complement of runtimeResetStructurallyPermitted's
// three admitted shapes minus its own terminal-failure/interruption branch --
// reset and rescope share that one structural precondition and are then
// split by the SAME drift check on opposite sides (Reset admits only when
// drifted; Rescope admits only when NOT drifted), so the two operations can
// never both be legal for the same replayed state. This exact predicate is
// evaluated both when writing a rescope (RuntimeStore.Rescope) and when
// replaying one from the immutable chain (applyRuntimeRecord), so a
// committed rescope always replays deterministically.
func runtimeObjectiveRescopeStructurallyPermitted(status RuntimeStatus) bool {
	if status.Objective == nil || status.DecisionRequired || status.Complete {
		return false
	}
	if len(status.Attempts) == 0 {
		return false
	}
	last := status.Attempts[len(status.Attempts)-1]
	return last.ObjectiveID == status.Objective.ID &&
		(last.Outcome == AttemptFailed || last.Outcome == AttemptInterrupted)
}

// Rescope implements AUDITED NARROWING RESCOPE (#2298, #2296 part 2): the
// maintainer-authorized exit from a terminal, non-complete, zero-drift
// objective whose recorded scope no caller may legally re-request (begin
// refuses the changed params, reset refuses the intact zero-drift budget).
// It closes the current objective and opens an immutable narrower successor
// in one record, carrying CumulativeAttempts/CumulativeChangedLines forward
// UNCHANGED so consumed history stays auditable -- the ratified decision
// explicitly rejected the cheap "admit reset at CumulativeChangedLines==0"
// fallback as attempt-count laundering, and an unzeroed carry-forward is the
// mechanism that keeps rescope from being that same defect under a new name.
func (store RuntimeStore) Rescope(ctx context.Context, request RescopeObjectiveRequest) (RuntimeStatus, error) {
	request, err := normalizeRescopeObjectiveRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request)
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(replay runtimeReplay) (runtimeRecord, error) {
		status := replay.Status
		if status.ActiveAttempt != nil {
			return runtimeRecord{}, ErrRuntimeAttemptActive
		}
		objective := status.Objective
		if objective == nil {
			return runtimeRecord{}, ErrRuntimeNoObjective
		}
		if !runtimeObjectiveRescopeStructurallyPermitted(status) {
			return runtimeRecord{}, ErrRuntimeRescopeNotAllowed
		}
		last := status.Attempts[len(status.Attempts)-1]
		drift, driftErr := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree)
		if driftErr != nil {
			return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate to check rescope drift eligibility: %w", driftErr)
		}
		if drift.Identity != last.FinishCandidateIdentity || drift.CandidateTree != last.FinishCandidateTree {
			// A drifted candidate is reset's shape, not rescope's -- naming
			// that distinction is runtimeObjectiveChangeRefusal's job, not a
			// bare mutation refusal, so Rescope itself stays fail-closed here.
			return runtimeRecord{}, ErrRuntimeRescopeNotAllowed
		}
		if request.MaxChangedLines > objective.MaxChangedLines {
			return runtimeRecord{}, store.runtimeRescopeWidenedRefusal(
				status, "--max-changed-lines", request.MaxChangedLines, objective.MaxChangedLines)
		}
		if request.MaxAttempts > objective.MaxAttempts {
			return runtimeRecord{}, store.runtimeRescopeWidenedRefusal(
				status, "--max-attempts", request.MaxAttempts, objective.MaxAttempts,
			)
		}
		// The zero-drift `drift` snapshot above was captured with
		// TargetBaseWorkspaceOverlay (same Kind/BaseRef Finish itself used),
		// so its Identity is only comparable against another
		// TargetBaseWorkspaceOverlay capture over that exact base -- never
		// against a fresh objective's ordinary TargetCurrentChanges capture.
		// The successor's InitialCandidate*/RescopeCandidate* must instead be
		// captured with captureRuntimeCandidate, exactly the Kind the very
		// next Begin will independently recompute (Begin's second dispatch
		// branch), so that later comparison is apples-to-apples. CandidateTree
		// itself (unlike Identity) is Kind-independent -- both captures agree
		// on it exactly because there is zero drift -- so this second capture
		// is provably the same underlying content the drift check just
		// verified, not a fresh unguarded read.
		fresh, err := captureRuntimeCandidate(ctx, store.Repo)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate at objective rescope: %w", err)
		}
		if fresh.CandidateTree != drift.CandidateTree {
			return runtimeRecord{}, ErrRuntimeRescopeNotAllowed
		}
		generation := status.ObjectiveGeneration + 1
		objectiveID := runtimeObjectiveID(store.Change, request.WorkUnit, request.EvidenceGoal, fresh.Identity, generation)
		return runtimeRecord{Operation: runtimeOperationRescope, Rescope: &runtimeRescopeEvent{
			PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation,
			PreviousMaxAttempts: objective.MaxAttempts, PreviousMaxChangedLines: objective.MaxChangedLines,
			RescopeCandidateIdentity: fresh.Identity, RescopeCandidateTree: fresh.CandidateTree,
			ObjectiveID: objectiveID, ObjectiveGeneration: generation,
			WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
			MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Reason: request.Reason, Actor: request.Actor,
		}}, nil
	})
}

// RepairConsecutiveRescope replaces only the poisoned HEAD reference from the
// v2.4.0-rc.1 writer defect. The invalid record stays present and invalid.
func (store RuntimeStore) RepairConsecutiveRescope(ctx context.Context, request RepairConsecutiveRescopeRequest) (RuntimeStatus, error) {
	request, err := normalizeRepairConsecutiveRescopeRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-repair-consecutive-rescope-request/v1", request)
	lock, err := store.acquireLock()
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer lock.Release()

	head, exists, err := readRuntimeHead(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return RuntimeStatus{}, err
	}
	if !exists {
		return RuntimeStatus{}, &RuntimeRevisionConflictError{Expected: request.ExpectedRevision, Current: ""}
	}
	if head != request.ExpectedRevision {
		replay, replayErr := store.load()
		if replayErr == nil {
			if receipt, ok := replay.Requests[request.RequestID]; ok {
				if receipt.Digest != digest {
					return RuntimeStatus{}, ErrRuntimeRequestConflict
				}
				record, recordErr := store.loadRecord(receipt.Revision)
				if recordErr == nil && record.Operation == runtimeOperationRepairConsecutiveRescope &&
					record.Repair.ReplacedRevision == request.ExpectedRevision {
					if err := store.syncReplay(); err != nil {
						return RuntimeStatus{}, &RuntimePublicationError{Revision: receipt.Revision, Committed: true, Cause: err}
					}
					return replay.Status, nil
				}
			}
		}
		return RuntimeStatus{}, &RuntimeRevisionConflictError{Expected: request.ExpectedRevision, Current: head}
	}

	poisoned, err := store.loadRecord(head)
	if err != nil {
		return RuntimeStatus{}, err
	}
	prefix, err := store.loadRevision(poisoned.PreviousRevision)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("replay SDD runtime repair predecessor: %w", err)
	}
	if receipt, ok := prefix.Requests[request.RequestID]; ok {
		if receipt.Digest != digest {
			return RuntimeStatus{}, ErrRuntimeRequestConflict
		}
		return RuntimeStatus{}, errors.New("SDD runtime repair request identifier is already owned by the valid predecessor") // refusal:by-design world-action: the immutable repair must bind its own request identifier
	}
	if err := validateConsecutiveRescopeRepairCandidate(prefix, poisoned); err != nil {
		return RuntimeStatus{}, fmt.Errorf("SDD runtime repair refuses non-exact consecutive-rescope authority: %w", err)
	}

	runtimeRepairBeforePublishHook()
	current, currentExists, currentErr := readRuntimeHead(filepath.Join(store.Dir, "HEAD"))
	if currentErr != nil {
		return RuntimeStatus{}, currentErr
	}
	if !currentExists || current != request.ExpectedRevision {
		return RuntimeStatus{}, &RuntimeRevisionConflictError{Expected: request.ExpectedRevision, Current: current}
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: prefix.Status.Revision,
		Operation: runtimeOperationRepairConsecutiveRescope, RequestID: request.RequestID, RequestDigest: digest,
		Repair: &runtimeRepairEvent{ReplacedRevision: head, RestoredRevision: prefix.Status.Revision, Reason: request.Reason, Actor: request.Actor},
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return RuntimeStatus{}, err
	}
	return store.commitRecordLocked(record)
}

// bindPreparedReview imports a legacy binding at most once and replaces the
// effective binding in the same immutable runtime chain. The callback is run
// while the runtime lock is held so the approved authority is revalidated
// immediately before the single HEAD compare-and-swap.
func (store RuntimeStore) bindPreparedReview(
	ctx context.Context,
	request BindReviewRequest,
	prepare func() (ReviewBinding, error),
) (RuntimeStatus, error) {
	request, err := normalizeBindReviewRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	requestDigest := runtimeValueHash("gentle-ai.sdd-runtime-bind-request/v1", request)
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.ensureDirectories(); err != nil {
		return RuntimeStatus{}, err
	}
	lock, err := store.acquireLock()
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer lock.Release()

	replay, err := store.load()
	if err != nil {
		return RuntimeStatus{}, err
	}
	if receipt, ok := replay.Requests[request.RequestID]; ok {
		if receipt.Digest != requestDigest {
			return RuntimeStatus{}, ErrRuntimeRequestConflict
		}
		if err := store.syncReplay(); err != nil {
			return RuntimeStatus{}, &RuntimePublicationError{Revision: receipt.Revision, Committed: true, Cause: err}
		}
		return replay.Status, nil
	}

	var legacy *ReviewBinding
	var legacyDigest string
	if replay.Status.Binding == nil {
		legacy, legacyDigest, err = store.readLegacyBinding()
		if err != nil {
			return RuntimeStatus{}, fmt.Errorf("read legacy SDD review binding: %w", err)
		}
	}

	prepared, err := prepare()
	if err != nil {
		return RuntimeStatus{}, err
	}
	prepared, err = validatePreparedRuntimeBinding(prepared, store.Change, request.LineageID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if replay.Status.Binding == nil {
		finalLegacy, finalDigest, finalErr := store.readLegacyBinding()
		if finalErr != nil {
			return RuntimeStatus{}, fmt.Errorf("reopen legacy SDD review binding: %w", finalErr)
		}
		if (legacy == nil) != (finalLegacy == nil) || legacyDigest != finalDigest {
			return RuntimeStatus{}, errors.New("legacy SDD review binding changed before native import")
		}
	}

	// A populated native binding is authoritative. Identical-candidate retries
	// are no-ops even when the caller repeats the original expected revision;
	// this preserves the existing idempotent bind contract without another
	// mutable request journal.
	if replay.Status.Binding != nil {
		if replay.Status.Binding.Revision == prepared.Revision {
			if err := store.syncReplay(); err != nil {
				return RuntimeStatus{}, &RuntimePublicationError{Revision: replay.Status.Revision, Committed: true, Cause: err}
			}
			return replay.Status, nil
		}
		if request.ExpectedBindingRevision != "" && !runtimeRevisionPattern.MatchString(request.ExpectedBindingRevision) {
			return RuntimeStatus{}, &BindingRevisionConflictError{Expected: request.ExpectedBindingRevision, Current: replay.Status.BindingRevision}
		}
		if replay.Status.BindingRevision != request.ExpectedBindingRevision {
			return RuntimeStatus{}, &BindingRevisionConflictError{Expected: request.ExpectedBindingRevision, Current: replay.Status.BindingRevision}
		}
	} else {
		current := ""
		if legacy != nil {
			current = legacy.Revision
		}
		if request.ExpectedBindingRevision != "" && !runtimeRevisionPattern.MatchString(request.ExpectedBindingRevision) {
			return RuntimeStatus{}, &BindingRevisionConflictError{Expected: request.ExpectedBindingRevision, Current: current}
		}
		if current != request.ExpectedBindingRevision {
			return RuntimeStatus{}, &BindingRevisionConflictError{Expected: request.ExpectedBindingRevision, Current: current}
		}
	}

	event := &runtimeBindingEvent{ExpectedRevision: request.ExpectedBindingRevision, Current: prepared}
	if replay.Status.Binding == nil {
		if legacy != nil {
			event.LegacyImport = &runtimeLegacyBindingImport{SourceDigest: legacyDigest, Binding: *legacy}
		}
	}
	record := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: replay.Status.Revision,
		Operation: runtimeOperationBind, RequestID: request.RequestID, RequestDigest: requestDigest, Binding: event,
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return RuntimeStatus{}, err
	}
	return store.commitRecordLocked(record)
}

func (store RuntimeStore) mutate(
	ctx context.Context,
	expected, requestID, requestDigest string,
	build func(runtimeReplay) (runtimeRecord, error),
) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.ensureDirectories(); err != nil {
		return RuntimeStatus{}, err
	}
	// acquireLock already maps contention to ErrRuntimeConcurrentUpdate; the
	// re-wrap that used to live here duplicated the prefix and flattened the
	// underlying contention proof back out of the chain (1861).
	lock, err := store.acquireLock()
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer lock.Release()

	replay, err := store.load()
	if err != nil {
		return RuntimeStatus{}, err
	}
	if receipt, ok := replay.Requests[requestID]; ok {
		if receipt.Digest != requestDigest {
			return RuntimeStatus{}, ErrRuntimeRequestConflict
		}
		if err := store.syncReplay(); err != nil {
			return RuntimeStatus{}, &RuntimePublicationError{Revision: receipt.Revision, Committed: true, Cause: err}
		}
		return replay.Status, nil
	}
	if replay.Status.Revision != expected {
		return RuntimeStatus{}, &RuntimeRevisionConflictError{Expected: expected, Current: replay.Status.Revision}
	}
	record, err := build(replay)
	if err != nil {
		return RuntimeStatus{}, err
	}
	record.Schema = runtimeRecordSchema
	record.Change = store.Change
	record.PreviousRevision = expected
	record.RequestID = requestID
	record.RequestDigest = requestDigest
	if err := validateRuntimeRecordShape(record); err != nil {
		return RuntimeStatus{}, err
	}
	return store.commitRecordLocked(record)
}

func (store RuntimeStore) acquireLock() (*reviewtransaction.AuthorityFileLock, error) {
	lockPath := filepath.Join(store.Dir, "LOCK")
	var lastErr error
	for attempt := 0; attempt < runtimeLockAcquireAttempts; attempt++ {
		lock, err := runtimeAcquireAuthorityFileLock(lockPath)
		if err == nil {
			return lock, nil
		}
		if errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
			// Wrapped rather than flattened so the contention proof survives
			// (1861). Both callers of acquireLock -- bindPreparedReview and
			// mutate -- refuse here strictly before commitRecordLocked, so a
			// refusal at acquisition wrote nothing, and a caller must be told
			// that instead of being handed an unknown mutation outcome. The
			// rendered text is identical to the previous %v form.
			return nil, fmt.Errorf("%w: %w", ErrRuntimeConcurrentUpdate, err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
		if attempt == runtimeLockAcquireAttempts-1 {
			break
		}
		if err := store.ensureDirectories(); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("acquire SDD runtime ledger lock %s after %d attempts: %w", pathquote.Quote(lockPath), runtimeLockAcquireAttempts, lastErr)
}

func (store RuntimeStore) commitRecordLocked(record runtimeRecord) (RuntimeStatus, error) {
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.publishRecord(revision, payload); err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.publishHead(revision); err != nil {
		return RuntimeStatus{}, err
	}
	if err := runtimeSyncDirectory(store.Dir); err != nil {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: fmt.Errorf("sync SDD runtime HEAD directory: %w", err)}
	}

	committed, err := store.load()
	if err != nil {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: fmt.Errorf("replay committed SDD runtime HEAD: %w", err)}
	}
	if committed.Status.Revision != revision {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: errors.New("committed SDD runtime HEAD did not replay to candidate revision")}
	}
	return committed.Status, nil
}

func (store RuntimeStore) load() (runtimeReplay, error) {
	head, exists, err := readRuntimeHead(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return runtimeReplay{}, err
	}
	if !exists {
		return store.loadRevision("")
	}
	return store.loadRevision(head)
}

func (store RuntimeStore) loadRevision(head string) (runtimeReplay, error) {
	replay := runtimeReplay{
		Status: RuntimeStatus{
			Schema: RuntimeStatusSchema, Change: store.Change, Attempts: []RuntimeAttempt{},
			NextOrdinal: 1, NextAction: RuntimeActionBegin,
		},
		Requests:      map[string]runtimeRequestReceipt{},
		AttemptTokens: map[int]string{},
		Instance:      store.instance,
	}
	type revisionRecord struct {
		revision string
		record   runtimeRecord
	}
	reverse := make([]revisionRecord, 0, 16)
	seen := map[string]struct{}{}
	for revision := head; revision != ""; {
		if len(reverse) >= maximumRuntimeChainRecords {
			return runtimeReplay{}, errors.New("SDD runtime chain exceeds the bounded record count")
		}
		if _, duplicate := seen[revision]; duplicate {
			return runtimeReplay{}, errors.New("SDD runtime record predecessor cycle detected")
		}
		seen[revision] = struct{}{}
		record, err := store.loadRecord(revision)
		if err != nil {
			return runtimeReplay{}, err
		}
		reverse = append(reverse, revisionRecord{revision: revision, record: record})
		revision = record.PreviousRevision
	}
	for index := len(reverse) - 1; index >= 0; index-- {
		entry := reverse[index]
		if err := applyRuntimeRecord(store, &replay, entry.revision, entry.record); err != nil {
			if entry.revision == head && validateConsecutiveRescopeRepairCandidate(replay, entry.record) == nil {
				return runtimeReplay{}, &runtimeConsecutiveRescopeRepairRequiredError{Revision: head, Continuation: store.consecutiveRescopeRepairContinuation(head)}
			}
			return runtimeReplay{}, fmt.Errorf("replay SDD runtime revision %s: %w", entry.revision, err)
		}
	}
	if head != "" && replay.Status.Revision != head {
		return runtimeReplay{}, errors.New("SDD runtime HEAD does not equal replayed revision")
	}
	return replay, nil
}

func applyRuntimeRecord(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	if record.PreviousRevision != replay.Status.Revision {
		return errors.New("record predecessor does not equal replay state")
	}
	if _, duplicate := replay.Requests[record.RequestID]; duplicate {
		return errors.New("duplicate runtime request identifier")
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return err
	}
	remediationPredecessorLineage := ""
	switch record.Operation {
	case runtimeOperationBegin:
		if err := applyRuntimeBeginEvent(replay, revision, record); err != nil {
			return err
		}

	case runtimeOperationAdvance:
		if err := applyRuntimeAdvanceEvent(replay, revision, record); err != nil {
			return err
		}

	case runtimeOperationFinish:
		if err := applyRuntimeFinishEvent(replay, record.Finish, record.Finish.RemediatesEvidenceRevision != ""); err != nil {
			return err
		}
	case runtimeOperationHandoff:
		if err := applyRuntimeHandoffEvent(replay, record.Handoff); err != nil {
			return err
		}

	case runtimeOperationFinishRemediation:
		currentBinding := replay.Status.Binding
		if currentBinding == nil && record.Binding.LegacyImport != nil {
			legacy := record.Binding.LegacyImport.Binding
			currentBinding = &legacy
		}
		if currentBinding == nil || currentBinding.Revision != record.Binding.ExpectedRevision {
			return errors.New("atomic remediation binding does not match replay state")
		}
		remediationPredecessorLineage = currentBinding.Lineage
		if replay.Status.EvidenceRevision != "" && replay.Status.EvidenceRevision != record.Finish.RemediatesEvidenceRevision {
			return errors.New("atomic remediation failed evidence does not match replay state")
		}
		if record.Binding.Current.Lineage == currentBinding.Lineage &&
			record.Finish.EvidenceRevision == record.Finish.RemediatesEvidenceRevision {
			// A same-lineage record is a legal approved self-successor only when
			// its corrected evidence differs from the failed evidence it repairs.
			return errors.New("atomic remediation binding does not select a distinct successor or corrected self-successor")
		}
		if err := applyRuntimeFinishEvent(replay, record.Finish, false); err != nil {
			return err
		}
		if !record.Finish.ChangedLineBudgetExceeded {
			if err := applyRuntimeBindingEvent(replay, record.Binding); err != nil {
				return err
			}
		}

	case runtimeOperationReset:
		event := record.Reset
		objective := replay.Status.Objective
		if replay.Status.ActiveAttempt != nil || objective == nil || !runtimeResetStructurallyPermitted(replay.Status) {
			return errors.New("objective reset is not a valid successor")
		}
		if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
			event.PreviousGeneration != replay.Status.ObjectiveGeneration {
			return errors.New("objective reset does not match the terminal objective")
		}
		replay.Status.Objective = nil
		replay.Status.CumulativeAttempts = 0
		replay.Status.CumulativeChangedLines = 0
		replay.Status.EvidenceRevision = ""
		replay.Status.DecisionRequired = false
		replay.Status.Complete = false
		replay.Status.NextAction = RuntimeActionBegin
		replay.Status.LastReset = &RuntimeReset{
			Revision: revision, PreviousObjectiveID: event.PreviousObjectiveID, PreviousGeneration: event.PreviousGeneration,
			ResetCandidateIdentity: event.ResetCandidateIdentity, ResetCandidateTree: event.ResetCandidateTree,
			Reason: event.Reason, Actor: event.Actor,
		}

	case runtimeOperationRescope:
		if err := applyRuntimeRescopeEvent(replay, revision, record); err != nil {
			return err
		}
	case runtimeOperationRepairConsecutiveRescope:
		if err := applyRuntimeConsecutiveRescopeRepairEvent(store, replay, revision, record); err != nil {
			return err
		}

	case runtimeOperationBind:
		if err := applyRuntimeBindingEvent(replay, record.Binding); err != nil {
			return err
		}
	case runtimeOperationReceipt:
		if err := applyRuntimeReceiptEvent(replay, record.Receipt); err != nil {
			return err
		}
	case runtimeOperationGrant:
		applyRuntimeGrantEvent(replay, record.Grant)
	default:
		return errors.New("unsupported SDD runtime record operation")
	}
	replay.Status.Revision = revision
	replay.Requests[record.RequestID] = runtimeRequestReceipt{
		Digest: record.RequestDigest, Revision: revision,
		RemediationPredecessorLineage: remediationPredecessorLineage,
	}
	return nil
}

func applyRuntimeBeginEvent(replay *runtimeReplay, revision string, record runtimeRecord) error {
	event := record.Begin
	generation := event.ObjectiveGeneration
	if generation == 0 {
		generation = replay.Status.ObjectiveGeneration + 1
		if replay.Status.Objective != nil {
			generation = replay.Status.Objective.Generation
		}
	}
	if replay.Status.ActiveAttempt != nil || replay.Status.Complete || replay.Status.DecisionRequired {
		return errors.New("begin record is not a valid successor")
	}
	if replay.Status.Objective == nil {
		expectedObjectiveID := runtimeObjectiveID(record.Change, event.WorkUnit, event.EvidenceGoal, event.BeginCandidateIdentity, generation)
		if event.ObjectiveGeneration == 0 {
			expectedObjectiveID = legacyRuntimeObjectiveID(record.Change, event.EvidenceGoal)
		}
		legacyGeneratedID := runtimeObjectiveIDV1(record.Change, event.EvidenceGoal, event.BeginCandidateIdentity, generation)
		validObjectiveID := event.ObjectiveID == expectedObjectiveID ||
			event.ObjectiveGeneration != 0 && event.ObjectiveID == legacyGeneratedID
		if event.Ordinal != replay.Status.NextOrdinal || generation != replay.Status.ObjectiveGeneration+1 || !validObjectiveID {
			return errors.New("initial objective identity or ordinal is invalid")
		}
		replay.Status.Objective = &RuntimeObjective{
			ID: event.ObjectiveID, Generation: generation, WorkUnit: event.WorkUnit, EvidenceGoal: event.EvidenceGoal,
			InitialCandidateIdentity: event.BeginCandidateIdentity, InitialCandidateTree: event.BeginCandidateTree,
			MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
		}
		replay.Status.ObjectiveGeneration = generation
	} else {
		objective := replay.Status.Objective
		if event.ObjectiveID != objective.ID || generation != objective.Generation || event.EvidenceGoal != objective.EvidenceGoal ||
			event.WorkUnit != objective.WorkUnit ||
			event.MaxAttempts != objective.MaxAttempts || event.MaxChangedLines != objective.MaxChangedLines ||
			event.Ordinal != replay.Status.NextOrdinal {
			return errors.New("begin record changes the active objective or ordinal")
		}
		if runtimeObjectiveHasRecordedAttempt(replay.Status) {
			if event.BeginCandidateTree != replay.Status.Attempts[len(replay.Status.Attempts)-1].FinishCandidateTree {
				return errors.New("begin record does not continue the terminal candidate")
			}
		} else if event.BeginCandidateIdentity != objective.InitialCandidateIdentity || event.BeginCandidateTree != objective.InitialCandidateTree {
			// Mirrors write-time Begin's second dispatch branch: a freshly
			// rescoped objective has no attempt of its own to chase, so the
			// replayed candidate must instead match what Rescope itself
			// recorded as this objective's InitialCandidate* (#2298, #2296
			// part 2).
			return errors.New("begin record does not continue the rescoped objective's recorded candidate") // refusal:by-design world-action: this shape is constructed by the authority itself from Rescope's own recorded InitialCandidate*, so a mismatch is a mutated record and the exit is restoring the store
		}
	}
	if replay.Status.CumulativeAttempts >= event.MaxAttempts || replay.Status.CumulativeChangedLines >= event.MaxChangedLines {
		return errors.New("begin record exceeds the persisted objective budget")
	}
	attempt := RuntimeAttempt{
		Ordinal: event.Ordinal, ObjectiveID: event.ObjectiveID, ObjectiveGeneration: generation,
		WorkUnit: event.WorkUnit, BeginCandidateIdentity: event.BeginCandidateIdentity,
		BeginCandidateTree: event.BeginCandidateTree, BeginWorktree: event.BeginWorktree,
		EffectiveWorktree: event.EffectiveWorktree, Outcome: AttemptRunning,
	}
	replay.Status.Attempts = append(replay.Status.Attempts, attempt)
	replay.AttemptTokens[event.Ordinal] = revision
	active := attempt
	replay.Status.ActiveAttempt = &active
	replay.Status.CumulativeAttempts++
	replay.Status.LifetimeAttempts++
	replay.Status.NextOrdinal = event.Ordinal + 1
	replay.Status.NextAction = RuntimeActionFinish
	return nil
}

func applyRuntimeHandoffEvent(replay *runtimeReplay, event *RuntimeHandoff) error {
	active := replay.Status.ActiveAttempt
	if active == nil || active.Ordinal != event.Ordinal || active.EffectiveWorktree == "" ||
		active.EffectiveWorktree != event.SourceWorktree || active.Handoff != nil ||
		len(replay.Status.Attempts) == 0 || replay.Status.Attempts[len(replay.Status.Attempts)-1].Outcome != AttemptRunning {
		return errors.New("handoff record does not match the active attempt") // refusal:by-design world-action: a contradictory immutable record requires restoring the authority store
	}
	attempt := &replay.Status.Attempts[len(replay.Status.Attempts)-1]
	handoff := *event
	attempt.EffectiveWorktree = event.DestinationWorktree
	attempt.Handoff = &handoff
	activeCopy := *attempt
	replay.Status.ActiveAttempt = &activeCopy
	return nil
}

// applyRuntimeRescopeEvent is the REPLAY-TIME validator for AUDITED NARROWING
// RESCOPE: it recomputes and verifies narrowing purely from replayed state,
// never trusting the record's own claimed "previous" values, so a
// corrupted/forged record claiming a widened ceiling (or lying about the
// previous ceiling to make a widen look like a narrow) is rejected on
// replay, not merely refused at write time. `objective` below is always the
// REPLAYED objective -- the one authority-verified transitions actually
// produced -- so `event.MaxAttempts > objective.MaxAttempts` cannot be
// fooled by a forged event.PreviousMaxAttempts.
func applyRuntimeRescopeEvent(replay *runtimeReplay, revision string, record runtimeRecord) error {
	event := record.Rescope
	objective := replay.Status.Objective
	if replay.Status.ActiveAttempt != nil || objective == nil || !runtimeObjectiveRescopeStructurallyPermitted(replay.Status) {
		return errors.New("objective rescope is not a valid successor") // refusal:by-design world-action: a replayed chain that contradicts its own write-time state is damaged authority; the exit is restoring the Git-common-dir store, not a command
	}
	// The real narrowing guard runs FIRST and is recomputed against the
	// REPLAYED objective, never against the record's own (possibly forged)
	// PreviousMax* claim: a record cannot launder a widen by simply lying
	// about what the previous ceiling was, because this comparison never
	// reads event.PreviousMax* at all.
	if event.MaxAttempts > objective.MaxAttempts || event.MaxChangedLines > objective.MaxChangedLines {
		return errors.New("objective rescope widens the current objective's budget") // refusal:by-design world-action: narrowing was enforced before publication, so a widened replayed record is a forged or corrupted chain and the exit is restoring the store
	}
	if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
		event.PreviousGeneration != replay.Status.ObjectiveGeneration ||
		event.PreviousMaxAttempts != objective.MaxAttempts || event.PreviousMaxChangedLines != objective.MaxChangedLines {
		return errors.New("objective rescope does not match the terminal objective") // refusal:by-design world-action: the predecessor scope was frozen at publication, so a mismatch is a mutated record and the exit is restoring the store
	}
	if len(replay.Status.Attempts) == 0 {
		return errors.New("objective rescope has no terminal candidate provenance") // refusal:by-design world-action: a rescope can only follow a settled attempt, so an empty history is a truncated chain and the exit is restoring the store
	}
	last := replay.Status.Attempts[len(replay.Status.Attempts)-1]
	// RescopeCandidateTree is captured with Kind=TargetCurrentChanges (the
	// same Kind the successor's very next Begin will independently
	// recompute), while FinishCandidateTree was captured with
	// Kind=TargetBaseWorkspaceOverlay -- different Kinds, so only
	// CandidateTree (Kind-independent: a pure function of workspace content)
	// is comparable across them. RescopeCandidateIdentity is NOT compared
	// here for the same reason; it is still shape-validated and reused
	// verbatim to derive ObjectiveID below.
	if (last.Outcome != AttemptFailed && last.Outcome != AttemptInterrupted) ||
		last.FinishCandidateTree != event.RescopeCandidateTree {
		return errors.New("objective rescope candidate does not match the terminal zero-drift finish") // refusal:by-design world-action: the zero-drift candidate was verified before publication, so a mismatch is a mutated record and the exit is restoring the store
	}
	generation := event.ObjectiveGeneration
	expectedObjectiveID := runtimeObjectiveID(record.Change, event.WorkUnit, event.EvidenceGoal, event.RescopeCandidateIdentity, generation)
	if generation != replay.Status.ObjectiveGeneration+1 || event.ObjectiveID != expectedObjectiveID {
		return errors.New("objective rescope identity is invalid") // refusal:by-design world-action: the successor identity is derived deterministically at publication, so a mismatch is a mutated record and the exit is restoring the store
	}
	replay.Status.Objective = &RuntimeObjective{
		ID: event.ObjectiveID, Generation: generation, WorkUnit: event.WorkUnit, EvidenceGoal: event.EvidenceGoal,
		InitialCandidateIdentity: event.RescopeCandidateIdentity, InitialCandidateTree: event.RescopeCandidateTree,
		MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
	}
	replay.Status.ObjectiveGeneration = generation
	replay.Status.EvidenceRevision = ""
	replay.Status.DecisionRequired = false
	replay.Status.Complete = false
	replay.Status.NextAction = RuntimeActionBegin
	replay.Status.LastRescope = &RuntimeRescope{
		Revision: revision, PreviousObjectiveID: event.PreviousObjectiveID, PreviousGeneration: event.PreviousGeneration,
		PreviousMaxAttempts: event.PreviousMaxAttempts, PreviousMaxChangedLines: event.PreviousMaxChangedLines,
		RescopeCandidateIdentity: event.RescopeCandidateIdentity, RescopeCandidateTree: event.RescopeCandidateTree,
		ObjectiveID: event.ObjectiveID, WorkUnit: event.WorkUnit, EvidenceGoal: event.EvidenceGoal,
		MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
		Reason: event.Reason, Actor: event.Actor,
	}
	// NextOrdinal, CumulativeAttempts, CumulativeChangedLines,
	// LifetimeAttempts, and LifetimeChangedLines are deliberately left
	// untouched: rescope carries history forward and never zeroes it (the
	// ratified decision's core requirement) -- the one difference from
	// runtimeOperationReset, which zeroes CumulativeAttempts/
	// CumulativeChangedLines above.
	return nil
}

func validateConsecutiveRescopeRepairCandidate(replay runtimeReplay, poisoned runtimeRecord) error {
	if poisoned.Operation != runtimeOperationRescope || poisoned.Rescope == nil || poisoned.PreviousRevision != replay.Status.Revision {
		return errors.New("record is not a rescope directly following the valid prefix") // refusal:by-design world-action: a repair cannot safely bypass a different immutable chain edge
	}
	objective := replay.Status.Objective
	if replay.Status.ActiveAttempt != nil || objective == nil || replay.Status.DecisionRequired || replay.Status.Complete || len(replay.Status.Attempts) == 0 {
		return errors.New("record does not meet the historical writer preconditions") // refusal:by-design world-action: a non-exact damaged record has no safe self-service repair
	}
	last := replay.Status.Attempts[len(replay.Status.Attempts)-1]
	if last.ObjectiveID == objective.ID || (last.Outcome != AttemptFailed && last.Outcome != AttemptInterrupted) {
		return errors.New("record is not missing only current-objective attempt ownership") // refusal:by-design world-action: normal replay remains authoritative for every other rescope shape
	}
	event := poisoned.Rescope
	if event.MaxAttempts > objective.MaxAttempts || event.MaxChangedLines > objective.MaxChangedLines ||
		event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
		event.PreviousGeneration != replay.Status.ObjectiveGeneration ||
		event.PreviousMaxAttempts != objective.MaxAttempts || event.PreviousMaxChangedLines != objective.MaxChangedLines ||
		last.FinishCandidateTree != event.RescopeCandidateTree {
		return errors.New("record does not match the historical writer preconditions") // refusal:by-design world-action: mismatched immutable evidence is not the released writer defect
	}
	expectedObjectiveID := runtimeObjectiveID(poisoned.Change, event.WorkUnit, event.EvidenceGoal, event.RescopeCandidateIdentity, event.ObjectiveGeneration)
	if event.ObjectiveGeneration != replay.Status.ObjectiveGeneration+1 || event.ObjectiveID != expectedObjectiveID {
		return errors.New("record has an invalid successor objective identity") // refusal:by-design world-action: a repair cannot reconstruct a forged successor identity
	}
	return nil
}

func applyRuntimeConsecutiveRescopeRepairEvent(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	event := record.Repair
	if record.PreviousRevision != event.RestoredRevision || replay.Status.Revision != event.RestoredRevision {
		return errors.New("consecutive-rescope repair does not restore its recorded predecessor") // refusal:by-design world-action: the repair record must chain directly from the valid predecessor it names
	}
	poisoned, err := store.loadRecord(event.ReplacedRevision)
	if err != nil {
		return fmt.Errorf("reopen preserved consecutive-rescope record: %w", err)
	}
	if err := validateConsecutiveRescopeRepairCandidate(*replay, poisoned); err != nil {
		return fmt.Errorf("preserved consecutive-rescope record is not the historical publication defect: %w", err)
	}
	replay.Status.LastRepair = &RuntimeRepair{
		Revision: revision, ReplacedRevision: event.ReplacedRevision, RestoredRevision: event.RestoredRevision,
		Reason: event.Reason, Actor: event.Actor,
	}
	if replay.Status.CumulativeAttempts >= replay.Status.Objective.MaxAttempts ||
		replay.Status.CumulativeChangedLines >= replay.Status.Objective.MaxChangedLines {
		replay.Status.DecisionRequired = true
		replay.Status.NextAction = RuntimeActionReset
	}
	return nil
}

// applyRuntimeAdvanceEvent closes a passed objective and opens its distinct
// successor inside one record. It re-derives the same rule
// runtimeObjectiveAdvanceAdmissible applied at write time, so a replayed chain
// can never admit an advance the authority would have refused.
func applyRuntimeAdvanceEvent(replay *runtimeReplay, revision string, record runtimeRecord) error {
	event := record.Advance
	objective := replay.Status.Objective
	// Every refusal below re-checks an invariant the authority already enforced
	// before publishing this record, so reaching one means the persisted chain
	// was damaged or forged after the fact.
	if replay.Status.ActiveAttempt != nil || objective == nil || !replay.Status.Complete || replay.Status.DecisionRequired {
		return errors.New("objective advance is not a valid successor") // refusal:by-design world-action: a replayed chain that contradicts its own write-time state is damaged authority; the exit is restoring the Git-common-dir store, not a command
	}
	if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
		event.PreviousGeneration != replay.Status.ObjectiveGeneration || event.PreviousWorkUnit != objective.WorkUnit {
		return errors.New("objective advance does not match the terminal objective") // refusal:by-design world-action: the predecessor identity was frozen at publication, so a mismatch is a mutated record and the exit is restoring the store
	}
	if len(replay.Status.Attempts) == 0 {
		return errors.New("objective advance has no terminal candidate provenance") // refusal:by-design world-action: an advance can only follow a settled attempt, so an empty history is a truncated chain and the exit is restoring the store
	}
	last := replay.Status.Attempts[len(replay.Status.Attempts)-1]
	if last.ObjectiveID != objective.ID || last.Outcome != AttemptPassed || last.ChangedLineBudgetExceeded ||
		last.FinishCandidateIdentity == "" || last.FinishCandidateTree == "" {
		return errors.New("objective advance does not follow a passed terminal objective") // refusal:by-design world-action: the passed predecessor was verified before publication, so this is a mutated record and the exit is restoring the store
	}
	if record.Begin.WorkUnit == objective.WorkUnit {
		return errors.New("objective advance does not select a distinct work unit") // refusal:by-design world-action: same-scope advance is refused at write time, so observing one on replay is a forged record and the exit is restoring the store
	}
	replay.Status.LastAdvance = &RuntimeAdvance{
		Revision: revision, PreviousObjectiveID: objective.ID, PreviousGeneration: objective.Generation,
		PreviousWorkUnit: objective.WorkUnit, PreviousEvidenceRevision: replay.Status.EvidenceRevision,
	}
	replay.Status.Objective = nil
	replay.Status.CumulativeAttempts = 0
	replay.Status.CumulativeChangedLines = 0
	replay.Status.EvidenceRevision = ""
	replay.Status.Complete = false
	return applyRuntimeBeginEvent(replay, revision, record)
}

func applyRuntimeFinishEvent(replay *runtimeReplay, event *runtimeFinishEvent, unmanagedRemediation bool) error {
	active := replay.Status.ActiveAttempt
	if active == nil || active.Ordinal != event.Ordinal || len(replay.Status.Attempts) == 0 ||
		replay.Status.Attempts[len(replay.Status.Attempts)-1].Outcome != AttemptRunning {
		return errors.New("finish record does not match the active attempt")
	}
	budgetExceeded := replay.Status.CumulativeChangedLines+event.ChangedLines > replay.Status.Objective.MaxChangedLines
	if event.ChangedLineBudgetExceeded != budgetExceeded {
		return errors.New("finish record changed-line budget decision does not match replay state")
	}
	if unmanagedRemediation {
		// Lockstep twin of the write-time guard in Finish: the binding derives
		// from the immutable chain, so replayed corrections recorded across an
		// audited reset stay valid. The chain walk requires a settled failed
		// predecessor, which subsumes the former len(Attempts) < 2 conjunct.
		chainFailedAttempt, chainHasFailedEvidence := runtimeChainFailedAttempt(replay.Status.Attempts)
		// #2621 lockstep twin: an audited reset or rescope that terminated the
		// failing objective against these exact bytes authorizes one evidence-only
		// retry, so a replayed correction may leave the candidate unchanged.
		evidenceOnly := runtimeEvidenceOnlyRetryAuthorized(replay.Status.LastReset, replay.Status.LastRescope, chainFailedAttempt, event.FinishCandidateTree)
		unchangedCandidate := event.FinishCandidateIdentity == active.BeginCandidateIdentity ||
			event.FinishCandidateTree == active.BeginCandidateTree
		// A binding is deliberately NOT checked here. The write path stopped
		// treating a leftover binding as a blocker (the kill switch must have
		// no implications while it is off), and this replay mirror has to agree
		// with it or a legitimately committed record makes the whole chain
		// unreplayable.
		if event.Outcome != AttemptPassed ||
			!chainHasFailedEvidence || chainFailedAttempt.EvidenceRevision != event.RemediatesEvidenceRevision ||
			(unchangedCandidate && !evidenceOnly) ||
			event.EvidenceRevision == event.RemediatesEvidenceRevision {
			// refusal:by-design world-action: a replayed event that breaks immutable evidence/candidate binding can only be repaired by restoring the authority.
			return errors.New("unmanaged remediation finish does not bind the final failed-evidence correction")
		}
	}
	attempt := &replay.Status.Attempts[len(replay.Status.Attempts)-1]
	attempt.FinishCandidateIdentity = event.FinishCandidateIdentity
	attempt.FinishCandidateTree = event.FinishCandidateTree
	attempt.Outcome = event.Outcome
	attempt.ChangedLines = event.ChangedLines
	attempt.EvidenceRevision = event.EvidenceRevision
	attempt.Diagnosis = event.Diagnosis
	attempt.HarnessDisposition = event.HarnessDisposition
	attempt.CleanupEvidence = event.CleanupEvidence
	attempt.ProcessEvidence = event.ProcessEvidence
	attempt.RemediatesEvidenceRevision = event.RemediatesEvidenceRevision
	attempt.ChangedLineBudgetExceeded = event.ChangedLineBudgetExceeded
	replay.Status.ActiveAttempt = nil
	replay.Status.CumulativeChangedLines += event.ChangedLines
	replay.Status.LifetimeChangedLines += event.ChangedLines
	replay.Status.EvidenceRevision = event.EvidenceRevision
	if event.Outcome == AttemptPassed && !event.ChangedLineBudgetExceeded {
		replay.Status.Complete = true
		replay.Status.NextAction = RuntimeActionComplete
	} else if event.ChangedLineBudgetExceeded || replay.Status.CumulativeAttempts >= replay.Status.Objective.MaxAttempts ||
		replay.Status.CumulativeChangedLines >= replay.Status.Objective.MaxChangedLines {
		replay.Status.DecisionRequired = true
		replay.Status.NextAction = RuntimeActionReset
	} else {
		replay.Status.NextAction = RuntimeActionBegin
	}
	return nil
}

// applyRuntimeGrantEvent accumulates a grant's canonical roots into the
// GrantedRoots projection in chain order, deduplicating already-granted
// identities. It has no structural precondition and returns no error: every
// integrity guard a grant needs already ran in validateRuntimeRecordShape,
// and a chain with no grant records never reaches it, so legacy chains
// replay unchanged. A grant projects only into a replay bound to its own
// change-instance identity (#2540 S5): the chain outlives the change it
// served, so a recreated change reusing an archived name replays the same
// records under a different identity and inherits none of its authority. A
// replay without an identity — every reader that has not called ForInstance,
// including the status.go embedding until S4b threads one — projects no
// granted roots at all.
func applyRuntimeGrantEvent(replay *runtimeReplay, event *runtimeGrantEvent) {
	if replay.Instance == "" || event.Instance != replay.Instance {
		return
	}
	for _, root := range event.Roots {
		duplicate := false
		for _, granted := range replay.Status.GrantedRoots {
			if granted == root {
				duplicate = true
				break
			}
		}
		if !duplicate {
			replay.Status.GrantedRoots = append(replay.Status.GrantedRoots, root)
		}
	}
}

func applyRuntimeBindingEvent(replay *runtimeReplay, event *runtimeBindingEvent) error {
	current := ""
	if replay.Status.Binding != nil {
		if event.LegacyImport != nil {
			return errors.New("native binding successor cannot import legacy authority again")
		}
		current = replay.Status.BindingRevision
	} else if event.LegacyImport != nil {
		current = event.LegacyImport.Binding.Revision
	}
	if current != event.ExpectedRevision {
		return errors.New("binding record expected revision does not equal replay state")
	}
	binding := event.Current
	replay.Status.Binding = &binding
	replay.Status.BindingRevision = binding.Revision
	return nil
}

func validateRuntimeBeginEvent(record runtimeRecord) error {
	event := record.Begin
	if !runtimeRevisionPattern.MatchString(event.ObjectiveID) || event.ObjectiveGeneration < 0 || validateRuntimeText(event.WorkUnit, 160) != nil ||
		validateRuntimeText(event.EvidenceGoal, 240) != nil || event.MaxAttempts < 1 || event.MaxAttempts > maximumRuntimeAttemptLimit ||
		event.MaxChangedLines < 1 || event.MaxChangedLines > maximumRuntimeChangedLines || event.Ordinal < 1 ||
		!runtimeRevisionPattern.MatchString(event.BeginCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.BeginCandidateTree) ||
		// BeginWorktree is optional (empty means legacy/pre-field), but a
		// PRESENT value is still an identity string, not free user input: it
		// must be a bounded, trimmed, single-line value like every other
		// recorded text field, not raw garbage.
		(event.BeginWorktree != "" && validateRuntimeText(event.BeginWorktree, 4096) != nil) ||
		(event.EffectiveWorktree != "" && (validateRuntimeText(event.EffectiveWorktree, 4096) != nil || event.EffectiveWorktree != event.BeginWorktree)) {
		return errors.New("invalid SDD runtime begin event")
	}
	request := BeginAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: event.WorkUnit,
		EvidenceGoal: event.EvidenceGoal, MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
	}
	if runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request) != record.RequestDigest {
		return errors.New("SDD runtime begin request digest does not match record")
	}
	return nil
}

func validateRuntimeRecordShape(record runtimeRecord) error {
	if record.Schema != runtimeRecordSchema || !validReviewBindingChange(record.Change) ||
		(record.PreviousRevision != "" && !runtimeRevisionPattern.MatchString(record.PreviousRevision)) ||
		!runtimeRequestIDPattern.MatchString(record.RequestID) || !runtimeRevisionPattern.MatchString(record.RequestDigest) {
		return errors.New("invalid SDD runtime record identity")
	}
	if record.Operation != runtimeOperationRepairConsecutiveRescope && record.Repair != nil {
		return errors.New("unexpected SDD runtime repair event") // refusal:by-design world-action: an immutable record may carry only the event its operation names
	}
	switch record.Operation {
	case runtimeOperationBegin:
		if record.Begin == nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime begin record shape")
		}
		if err := validateRuntimeBeginEvent(record); err != nil {
			return err
		}
	case runtimeOperationAdvance:
		if record.Begin == nil || record.Advance == nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime objective advance record shape") // refusal:by-design world-action: this shape is constructed by the authority itself, so a violation is a mutated record and the exit is restoring the store
		}
		advance := record.Advance
		if !runtimeRevisionPattern.MatchString(advance.PreviousObjectiveID) || advance.PreviousGeneration < 1 ||
			validateRuntimeText(advance.PreviousWorkUnit, 160) != nil || advance.PreviousWorkUnit == record.Begin.WorkUnit {
			return errors.New("invalid SDD runtime objective advance event") // refusal:by-design world-action: the advance event is derived from validated status, so a violation is a mutated record and the exit is restoring the store
		}
		// The successor carries an ordinary begin request, so its digest binds
		// the same caller-visible request an ordinary begin would have bound.
		if err := validateRuntimeBeginEvent(record); err != nil {
			return err
		}
	case runtimeOperationFinish:
		if record.Finish == nil || record.Begin != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime finish record shape")
		}
		event := record.Finish
		if event.Ordinal < 1 || !validTerminalAttemptOutcome(event.Outcome) || event.ChangedLines < 0 ||
			event.ChangedLines > maximumRuntimeChangedLines || !runtimeRevisionPattern.MatchString(event.EvidenceRevision) ||
			!runtimeRevisionPattern.MatchString(event.FinishCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.FinishCandidateTree) ||
			validateRuntimeText(event.Diagnosis, 500) != nil || !validHarnessDisposition(event.HarnessDisposition) ||
			validateRuntimeText(event.CleanupEvidence, 500) != nil || validateRuntimeText(event.ProcessEvidence, 500) != nil ||
			(event.RemediatesEvidenceRevision != "" && (!runtimeRevisionPattern.MatchString(event.RemediatesEvidenceRevision) || event.Outcome != AttemptPassed)) {
			return errors.New("invalid SDD runtime finish event")
		}
		request := FinishAttemptRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: event.Outcome,
			EvidenceRevision: event.EvidenceRevision, Diagnosis: event.Diagnosis, HarnessDisposition: event.HarnessDisposition,
			CleanupEvidence: event.CleanupEvidence, ProcessEvidence: event.ProcessEvidence,
			RemediatesEvidenceRevision: event.RemediatesEvidenceRevision,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime finish request digest does not match record")
		}
	case runtimeOperationHandoff:
		if record.Handoff == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime handoff record shape") // refusal:by-design world-action: a malformed immutable record requires restoring the authority store
		}
		event := record.Handoff
		if event.Ordinal < 1 || event.SourceWorktree == event.DestinationWorktree ||
			validateRuntimeText(event.SourceWorktree, 4096) != nil || !filepath.IsAbs(event.SourceWorktree) ||
			validateRuntimeText(event.DestinationWorktree, 4096) != nil || !filepath.IsAbs(event.DestinationWorktree) ||
			validateRuntimeText(event.CommonDir, 4096) != nil || !filepath.IsAbs(event.CommonDir) ||
			event.ExpectedRevision != record.PreviousRevision || event.RequestDigest != record.RequestDigest ||
			!runtimeRevisionPattern.MatchString(event.DestinationCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.DestinationCandidateTree) {
			return errors.New("invalid SDD runtime handoff event") // refusal:by-design world-action: a malformed immutable event requires restoring the authority store
		}
		request := HandoffAttemptRequest{ExpectedRevision: event.ExpectedRevision, RequestID: record.RequestID, DestinationWorktree: event.DestinationWorktree}
		if runtimeValueHash("gentle-ai.sdd-runtime-handoff-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime handoff request digest does not match record") // refusal:by-design world-action: a forged immutable record requires restoring the authority store
		}
	case runtimeOperationFinishRemediation:
		if record.Finish == nil || record.Binding == nil || record.Begin != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid atomic SDD runtime remediation record shape")
		}
		finish, binding := record.Finish, record.Binding
		if finish.Ordinal < 1 || finish.Outcome != AttemptPassed || finish.ChangedLines < 0 ||
			finish.ChangedLines > maximumRuntimeChangedLines || !runtimeRevisionPattern.MatchString(finish.EvidenceRevision) ||
			!runtimeRevisionPattern.MatchString(finish.RemediatesEvidenceRevision) ||
			!runtimeRevisionPattern.MatchString(finish.FinishCandidateIdentity) || !runtimeGitTreePattern.MatchString(finish.FinishCandidateTree) ||
			validateRuntimeText(finish.Diagnosis, 500) != nil || !validHarnessDisposition(finish.HarnessDisposition) ||
			validateRuntimeText(finish.CleanupEvidence, 500) != nil || validateRuntimeText(finish.ProcessEvidence, 500) != nil {
			return errors.New("invalid atomic SDD runtime remediation finish event")
		}
		if !runtimeRevisionPattern.MatchString(binding.ExpectedRevision) {
			return errors.New("invalid atomic SDD runtime remediation binding event")
		}
		if _, err := validatePreparedRuntimeBinding(binding.Current, record.Change, binding.Current.Lineage); err != nil {
			return fmt.Errorf("invalid atomic SDD runtime remediation successor: %w", err)
		}
		if binding.LegacyImport != nil {
			legacy, err := validatePreparedRuntimeBinding(binding.LegacyImport.Binding, record.Change, binding.LegacyImport.Binding.Lineage)
			if err != nil {
				return fmt.Errorf("invalid atomic remediation legacy binding import: %w", err)
			}
			payload, _ := bindingBytes(legacy)
			if binding.LegacyImport.SourceDigest != bindingHash(payload) || binding.ExpectedRevision != legacy.Revision {
				return errors.New("atomic remediation legacy binding import does not match its source or expected revision")
			}
		}
		request := FinishAttemptRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: finish.Outcome,
			EvidenceRevision: finish.EvidenceRevision, Diagnosis: finish.Diagnosis, HarnessDisposition: finish.HarnessDisposition,
			CleanupEvidence: finish.CleanupEvidence, ProcessEvidence: finish.ProcessEvidence,
			ExpectedBindingRevision: binding.ExpectedRevision, SuccessorLineageID: binding.Current.Lineage,
			RemediatesEvidenceRevision: finish.RemediatesEvidenceRevision,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", request) != record.RequestDigest {
			return errors.New("atomic SDD runtime remediation request digest does not match record")
		}
	case runtimeOperationReset:
		if record.Reset == nil || record.Begin != nil || record.Finish != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime reset record shape")
		}
		event := record.Reset
		if !runtimeRevisionPattern.MatchString(event.PreviousObjectiveID) || event.PreviousGeneration < 1 ||
			!runtimeRevisionPattern.MatchString(event.ResetCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.ResetCandidateTree) ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return errors.New("invalid SDD runtime reset event")
		}
		request := ResetObjectiveRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Reason: event.Reason, Actor: event.Actor,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-reset-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime reset request digest does not match record")
		}
	case runtimeOperationRescope:
		if record.Rescope == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime rescope record shape") // refusal:by-design world-action: this shape is constructed by the authority itself, so a violation is a mutated record and the exit is restoring the store
		}
		event := record.Rescope
		if !runtimeRevisionPattern.MatchString(event.PreviousObjectiveID) || event.PreviousGeneration < 1 ||
			event.PreviousMaxAttempts < 1 || event.PreviousMaxAttempts > maximumRuntimeAttemptLimit ||
			event.PreviousMaxChangedLines < 1 || event.PreviousMaxChangedLines > maximumRuntimeChangedLines ||
			!runtimeRevisionPattern.MatchString(event.RescopeCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.RescopeCandidateTree) ||
			!runtimeRevisionPattern.MatchString(event.ObjectiveID) || event.ObjectiveGeneration < 1 ||
			validateRuntimeText(event.WorkUnit, 160) != nil || validateRuntimeText(event.EvidenceGoal, 240) != nil ||
			event.MaxAttempts < 1 || event.MaxAttempts > maximumRuntimeAttemptLimit ||
			event.MaxChangedLines < 1 || event.MaxChangedLines > maximumRuntimeChangedLines ||
			// The shape-level narrowing check below defends against a record
			// whose OWN fields are internally inconsistent; it is not the
			// real narrowing guard. applyRuntimeRescopeEvent independently
			// recomputes narrowing against the REPLAYED objective, which a
			// forged PreviousMax* cannot fool (see its doc comment).
			event.MaxAttempts > event.PreviousMaxAttempts || event.MaxChangedLines > event.PreviousMaxChangedLines ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return errors.New("invalid SDD runtime rescope event") // refusal:by-design world-action: this shape (including narrowing) is enforced before publication, so a violation is a mutated record and the exit is restoring the store
		}
		request := RescopeObjectiveRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID,
			WorkUnit: event.WorkUnit, EvidenceGoal: event.EvidenceGoal,
			MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
			Reason: event.Reason, Actor: event.Actor,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime rescope request digest does not match record") // refusal:by-design world-action: the digest is computed from the same request at write time, so a mismatch is a mutated record and the exit is restoring the store
		}
	case runtimeOperationRepairConsecutiveRescope:
		if record.Repair == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime consecutive-rescope repair record shape") // refusal:by-design world-action: repair has one immutable event and cannot carry a parallel authority mutation
		}
		event := record.Repair
		if !runtimeRevisionPattern.MatchString(event.ReplacedRevision) || !runtimeRevisionPattern.MatchString(event.RestoredRevision) ||
			event.RestoredRevision != record.PreviousRevision || validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return errors.New("invalid SDD runtime consecutive-rescope repair event") // refusal:by-design world-action: repair binds exact revisions and audited actor and reason
		}
		request := RepairConsecutiveRescopeRequest{ExpectedRevision: event.ReplacedRevision, RequestID: record.RequestID, Reason: event.Reason, Actor: event.Actor}
		if runtimeValueHash("gentle-ai.sdd-runtime-repair-consecutive-rescope-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime consecutive-rescope repair request digest does not match record") // refusal:by-design world-action: altered repair authority must fail replay rather than be reinterpreted
		}
	case runtimeOperationBind:
		if record.Binding == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Receipt != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime binding record shape")
		}
		event := record.Binding
		if event.ExpectedRevision != "" && !runtimeRevisionPattern.MatchString(event.ExpectedRevision) {
			return errors.New("invalid expected SDD review binding revision")
		}
		if _, err := validatePreparedRuntimeBinding(event.Current, record.Change, event.Current.Lineage); err != nil {
			return fmt.Errorf("invalid current SDD review binding: %w", err)
		}
		if event.LegacyImport != nil {
			legacy, err := validatePreparedRuntimeBinding(event.LegacyImport.Binding, record.Change, event.LegacyImport.Binding.Lineage)
			if err != nil {
				return fmt.Errorf("invalid imported legacy SDD review binding: %w", err)
			}
			payload, _ := bindingBytes(legacy)
			if event.LegacyImport.SourceDigest != bindingHash(payload) || event.ExpectedRevision != legacy.Revision {
				return errors.New("legacy SDD review binding import does not match its source or expected revision")
			}
		}
		request := BindReviewRequest{
			ExpectedBindingRevision: event.ExpectedRevision, RequestID: record.RequestID, LineageID: event.Current.Lineage,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-bind-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime binding request digest does not match record")
		}
	case runtimeOperationReceipt:
		if record.Receipt == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Grant != nil {
			return errors.New("invalid SDD runtime receipt record shape") // refusal:by-design world-action: this shape is constructed by the authority itself, so a violation is a mutated record and the exit is restoring the store
		}
		event := record.Receipt
		if event.ExpectedRevision != "" && !runtimeRevisionPattern.MatchString(event.ExpectedRevision) {
			return errors.New("invalid expected SDD runtime receipt revision") // refusal:by-design world-action: this field is written only by commitRecordLocked itself, so a violation is a mutated record and the exit is restoring the store
		}
		if event.Current.Lineage != record.Change && event.Current.Lineage == "" {
			return errors.New("invalid current SDD runtime receipt lineage") // refusal:by-design world-action: the lineage is frozen at request normalization, so a violation is a mutated record and the exit is restoring the store
		}
		if !validReviewBindingLineage(event.Current.Lineage) || !reviewBindingHash.MatchString(event.Current.ReceiptHash) {
			return errors.New("invalid current SDD runtime receipt") // refusal:by-design world-action: the receipt shape is validated before every commit, so a violation is a mutated record and the exit is restoring the store
		}
		if event.LegacyImport != nil {
			if !validReviewBindingLineage(event.LegacyImport.Receipt.Lineage) || !reviewBindingHash.MatchString(event.LegacyImport.Receipt.ReceiptHash) {
				return errors.New("invalid imported legacy SDD runtime receipt") // refusal:by-design world-action: the legacy import is projected once at commit time, so a violation is a mutated record and the exit is restoring the store
			}
			if event.LegacyImport.SourceDigest == "" || event.ExpectedRevision != receiptRefDigest(event.LegacyImport.Receipt) {
				return errors.New("legacy SDD runtime receipt import does not match its source or expected revision") // refusal:by-design world-action: the import binding is derived from the legacy artifact at commit time, so a mismatch is a mutated record and the exit is restoring the store
			}
		}
		request := RecordReceiptRequest{
			ExpectedReceiptRevision: event.ExpectedRevision, RequestID: record.RequestID, Lineage: event.Current.Lineage,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-receipt-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime receipt request digest does not match record") // refusal:by-design world-action: the digest is computed from the same request at write time, so a mismatch is a mutated record and the exit is restoring the store
		}
	case runtimeOperationGrant:
		if record.Grant == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Binding != nil || record.Receipt != nil {
			return errors.New("invalid SDD runtime grant record shape") // refusal:by-design world-action: this shape is constructed by the authority itself, so a violation is a mutated record and the exit is restoring the store
		}
		event := record.Grant
		if len(event.Roots) < 1 || len(event.Roots) > maximumRuntimeGrantRoots ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return errors.New("invalid SDD runtime grant event") // refusal:by-design world-action: bounds and audit fields are enforced before publication, so a violation is a mutated record and the exit is restoring the store
		}
		if event.Instance == "" || validateRuntimeText(event.Instance, 128) != nil {
			return errors.New("invalid SDD runtime grant change-instance identity") // refusal:by-design world-action: every writer binds the store's ForInstance identity before publication and no released writer ever emitted an instance-less grant, so a violation is a mutated record and the exit is restoring the store
		}
		seen := make(map[string]struct{}, len(event.Roots))
		for _, root := range event.Roots {
			if validateRuntimeText(root, 4096) != nil || !filepath.IsAbs(root) {
				return errors.New("invalid SDD runtime grant root") // refusal:by-design world-action: roots are canonicalized before publication, so a violation is a mutated record and the exit is restoring the store
			}
			if _, duplicate := seen[root]; duplicate {
				return errors.New("duplicate SDD runtime grant root") // refusal:by-design world-action: canonical duplicates collapse before publication, so a violation is a mutated record and the exit is restoring the store
			}
			seen[root] = struct{}{}
		}
		// GrantedAt is the ledger's first wall-clock field: validated for
		// parseability only, never recomputed or compared against a clock, so
		// it stays excluded from determinism-replay expectations.
		if _, err := time.Parse(time.RFC3339Nano, event.GrantedAt); err != nil {
			return errors.New("invalid SDD runtime grant timestamp") // refusal:by-design world-action: the timestamp is rendered by the authority's own clock at publication, so a violation is a mutated record and the exit is restoring the store
		}
		request := GrantRootsRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID,
			Roots: event.Roots, Reason: event.Reason, Actor: event.Actor,
			ChangeInstance: event.Instance,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request) != record.RequestDigest {
			return errors.New("SDD runtime grant request digest does not match record") // refusal:by-design world-action: the digest binds the granted roots at write time, so a widened or altered record fails this recompute and the exit is restoring the store
		}
	default:
		return errors.New("invalid SDD runtime record operation")
	}
	return nil
}

func normalizeBeginAttemptRequest(request BeginAttemptRequest) (BeginAttemptRequest, error) {
	if request.ExpectedRevision != "" && !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return BeginAttemptRequest{}, errors.New("expected runtime revision must be empty or sha256")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return BeginAttemptRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if err := validateRuntimeText(request.WorkUnit, 160); err != nil {
		return BeginAttemptRequest{}, fmt.Errorf("invalid work_unit: %w", err)
	}
	if err := validateRuntimeText(request.EvidenceGoal, 240); err != nil {
		return BeginAttemptRequest{}, fmt.Errorf("invalid evidence_goal: %w", err)
	}
	if request.MaxAttempts == 0 {
		request.MaxAttempts = DefaultRuntimeAttemptLimit
	}
	if request.MaxChangedLines == 0 {
		request.MaxChangedLines = DefaultRuntimeChangedLines
	}
	if request.MaxAttempts < 1 || request.MaxAttempts > maximumRuntimeAttemptLimit {
		return BeginAttemptRequest{}, fmt.Errorf("max_attempts must be within 1..%d", maximumRuntimeAttemptLimit)
	}
	if request.MaxChangedLines < 1 || request.MaxChangedLines > maximumRuntimeChangedLines {
		return BeginAttemptRequest{}, fmt.Errorf("max_changed_lines must be within 1..%d", maximumRuntimeChangedLines)
	}
	return request, nil
}

// runtimeRevisionShapeObservation describes a rejected sha256:<64-lowercase-hex>
// candidate without ever echoing it verbatim: the value may be long or, for a
// caller that pasted the wrong secret, sensitive. It reports only what the
// exact-match failure cannot otherwise tell an operator — length, whether the
// sha256: prefix is present, and whether the remainder contains characters
// outside 0-9a-f (which catches the common case of uppercase hex from
// PowerShell `Get-FileHash` or `shasum -a 256` output).
func runtimeRevisionShapeObservation(value string) string {
	const prefix = "sha256:"
	hasPrefix := strings.HasPrefix(value, prefix)
	body := strings.TrimPrefix(value, prefix)
	nonLowercaseHex := false
	for _, r := range body {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			nonLowercaseHex = true
			break
		}
	}
	return fmt.Sprintf("received length=%d, sha256: prefix=%t, non-lowercase-hex characters=%t", len(value), hasPrefix, nonLowercaseHex)
}

func normalizeFinishAttemptRequest(request FinishAttemptRequest) (FinishAttemptRequest, error) {
	if request.ExpectedRevision == "" || !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return FinishAttemptRequest{}, errors.New("finish requires an exact expected runtime revision")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return FinishAttemptRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if !validTerminalAttemptOutcome(request.Outcome) {
		return FinishAttemptRequest{}, errors.New("outcome must be failed, interrupted, or passed")
	}
	if !runtimeRevisionPattern.MatchString(request.EvidenceRevision) {
		return FinishAttemptRequest{}, fmt.Errorf(
			"evidence_revision must be sha256:<64-lowercase-hex> (%s); rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --evidence-revision sha256:<64-lowercase-hex>",
			runtimeRevisionShapeObservation(request.EvidenceRevision),
		)
	}
	if err := validateRuntimeText(request.Diagnosis, 500); err != nil {
		return FinishAttemptRequest{}, fmt.Errorf("invalid diagnosis: %w", err)
	}
	if !validHarnessDisposition(request.HarnessDisposition) {
		return FinishAttemptRequest{}, errors.New("harness_disposition must be reused or invalidated")
	}
	if err := validateRuntimeText(request.CleanupEvidence, 500); err != nil {
		return FinishAttemptRequest{}, fmt.Errorf("invalid cleanup_evidence: %w", err)
	}
	if err := validateRuntimeText(request.ProcessEvidence, 500); err != nil {
		return FinishAttemptRequest{}, fmt.Errorf("invalid process_evidence: %w", err)
	}
	managedRemediationFields := 0
	for _, value := range []string{request.ExpectedBindingRevision, request.SuccessorLineageID} {
		if value != "" {
			managedRemediationFields++
		}
	}
	if managedRemediationFields != 0 && (managedRemediationFields != 2 || request.RemediatesEvidenceRevision == "") {
		return FinishAttemptRequest{}, errors.New("remediation successor requires expected_binding_revision, successor_lineage_id, and remediates_evidence_revision together")
	}
	if managedRemediationFields == 2 {
		if request.Outcome != AttemptPassed {
			return FinishAttemptRequest{}, errors.New("an atomic remediation successor is valid only for a passed attempt")
		}
		if !runtimeRevisionPattern.MatchString(request.ExpectedBindingRevision) {
			return FinishAttemptRequest{}, fmt.Errorf(
				"expected_binding_revision must be sha256:<64-lowercase-hex> for atomic remediation (%s); rerun `gentle-ai sdd-attempt finish` with --expected-binding-revision sha256:<64-lowercase-hex> --successor-lineage <lineage> --remediates-evidence-revision sha256:<64-lowercase-hex>",
				runtimeRevisionShapeObservation(request.ExpectedBindingRevision),
			)
		}
		if !validReviewBindingLineage(request.SuccessorLineageID) {
			return FinishAttemptRequest{}, errors.New("successor_lineage_id must be a canonical lowercase lineage")
		}
		if !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
			return FinishAttemptRequest{}, fmt.Errorf(
				"remediates_evidence_revision must be sha256:<64-lowercase-hex> (%s); rerun `gentle-ai sdd-attempt finish` with --expected-binding-revision sha256:<64-lowercase-hex> --successor-lineage <lineage> --remediates-evidence-revision sha256:<64-lowercase-hex>",
				runtimeRevisionShapeObservation(request.RemediatesEvidenceRevision),
			)
		}
	} else if request.RemediatesEvidenceRevision != "" {
		if request.Outcome != AttemptPassed {
			// refusal:by-design operator-knowledge: the caller alone knows whether its correction passed and must supply that outcome truthfully.
			return FinishAttemptRequest{}, errors.New("unmanaged remediation is valid only for a passed attempt")
		}
		if !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
			return FinishAttemptRequest{}, fmt.Errorf(
				"remediates_evidence_revision must be sha256:<64-lowercase-hex> for unmanaged remediation (%s); rerun `gentle-ai sdd-attempt finish` with --remediates-evidence-revision sha256:<64-lowercase-hex>",
				runtimeRevisionShapeObservation(request.RemediatesEvidenceRevision),
			)
		}
	}
	return request, nil
}

func normalizeHandoffAttemptRequest(request HandoffAttemptRequest) (HandoffAttemptRequest, error) {
	if !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return HandoffAttemptRequest{}, errors.New("handoff requires an exact expected runtime revision") // refusal:by-design operator-knowledge: the caller must supply the current runtime revision
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return HandoffAttemptRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if err := validateRuntimeText(request.DestinationWorktree, 4096); err != nil {
		return HandoffAttemptRequest{}, fmt.Errorf("invalid handoff destination worktree: %w", err)
	}
	destination, err := canonicalRuntimeHandoffPath(request.DestinationWorktree)
	if err != nil {
		return HandoffAttemptRequest{}, fmt.Errorf("resolve handoff destination worktree: %w", err)
	}
	request.DestinationWorktree = destination
	return request, nil
}

func finishRequestsRemediation(request FinishAttemptRequest) bool {
	return request.ExpectedBindingRevision != "" || request.SuccessorLineageID != ""
}

func finishRequestsUnmanagedRemediation(request FinishAttemptRequest) bool {
	return request.ExpectedBindingRevision == "" && request.SuccessorLineageID == "" && request.RemediatesEvidenceRevision != ""
}

func normalizeResetObjectiveRequest(request ResetObjectiveRequest) (ResetObjectiveRequest, error) {
	if request.ExpectedRevision == "" || !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return ResetObjectiveRequest{}, errors.New("reset requires an exact expected runtime revision")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return ResetObjectiveRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if err := validateRuntimeText(request.Reason, 500); err != nil {
		return ResetObjectiveRequest{}, fmt.Errorf("invalid reset reason: %w", err)
	}
	if err := validateRuntimeText(request.Actor, 128); err != nil {
		return ResetObjectiveRequest{}, fmt.Errorf("invalid reset actor: %w", err)
	}
	return request, nil
}

func normalizeRepairConsecutiveRescopeRequest(request RepairConsecutiveRescopeRequest) (RepairConsecutiveRescopeRequest, error) {
	if request.ExpectedRevision == "" || !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return RepairConsecutiveRescopeRequest{}, errors.New("repair requires an exact expected runtime revision") // refusal:by-design operator-knowledge: the unreadable HEAD names the only revision this repair may replace
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return RepairConsecutiveRescopeRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if err := validateRuntimeText(request.Reason, 500); err != nil {
		return RepairConsecutiveRescopeRequest{}, fmt.Errorf("invalid repair reason: %w", err)
	}
	if err := validateRuntimeText(request.Actor, 128); err != nil {
		return RepairConsecutiveRescopeRequest{}, fmt.Errorf("invalid repair actor: %w", err)
	}
	return request, nil
}

// normalizeGrantRootsRequest mirrors normalizeResetObjectiveRequest's
// CAS/audit-field validation and canonicalizes every requested root the way
// OpenRuntimeStore canonicalizes the workspace for BeginWorktree: absolute,
// then symlink-evaluated, so a link and its target record one identity.
// Canonical duplicates collapse, keeping the digest identical to the event.
func normalizeGrantRootsRequest(request GrantRootsRequest) (GrantRootsRequest, error) {
	if request.ExpectedRevision != "" && !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return GrantRootsRequest{}, errors.New("expected runtime revision must be empty or sha256")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return GrantRootsRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if len(request.Roots) < 1 || len(request.Roots) > maximumRuntimeGrantRoots {
		return GrantRootsRequest{}, fmt.Errorf("grant requires between 1 and %d roots", maximumRuntimeGrantRoots) // refusal:by-design operator-knowledge: only the caller knows which repository roots this change needs; retry with a bounded non-empty root list
	}
	canonical := make([]string, 0, len(request.Roots))
	seen := make(map[string]struct{}, len(request.Roots))
	for _, root := range request.Roots {
		if err := validateRuntimeText(root, 4096); err != nil {
			return GrantRootsRequest{}, fmt.Errorf("invalid grant root: %w", err)
		}
		resolved, err := filepath.Abs(root)
		if err == nil {
			resolved, err = filepath.EvalSymlinks(resolved)
		}
		if err != nil {
			return GrantRootsRequest{}, fmt.Errorf("resolve grant root %s: %w", pathquote.Quote(root), err)
		}
		if err := validateRuntimeText(resolved, 4096); err != nil {
			return GrantRootsRequest{}, fmt.Errorf("invalid canonical grant root: %w", err)
		}
		if _, duplicate := seen[resolved]; duplicate {
			continue
		}
		seen[resolved] = struct{}{}
		canonical = append(canonical, resolved)
	}
	request.Roots = canonical
	if err := validateRuntimeText(request.Reason, 500); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant reason: %w", err)
	}
	if err := validateRuntimeText(request.Actor, 128); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant actor: %w", err)
	}
	if request.ChangeInstance == "" {
		return GrantRootsRequest{}, errors.New("grant requires a change-instance identity") // refusal:by-design operator-knowledge: only the caller knows which change instance this grant authorizes; derive the store with ForInstance and retry
	}
	if err := validateRuntimeText(request.ChangeInstance, 128); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant change-instance identity: %w", err)
	}
	return request, nil
}

// normalizeRescopeObjectiveRequest mirrors normalizeResetObjectiveRequest's
// CAS/audit-field validation, plus BeginAttemptRequest's work-unit-scope
// validation for the successor scope -- except MaxAttempts/MaxChangedLines
// are never defaulted when zero: an audited narrowing rescope must state its
// narrower ceiling explicitly, not silently inherit
// DefaultRuntimeAttemptLimit/DefaultRuntimeChangedLines.
func normalizeRescopeObjectiveRequest(request RescopeObjectiveRequest) (RescopeObjectiveRequest, error) {
	if request.ExpectedRevision == "" || !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return RescopeObjectiveRequest{}, errors.New("rescope requires an exact expected runtime revision") // refusal:by-design operator-knowledge: only the caller knows the intended expected-revision token; retry with the exact `sdd-attempt status` revision
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return RescopeObjectiveRequest{}, errors.New("request_id must be a canonical lowercase identifier") // refusal:by-design operator-knowledge: only the caller can supply a canonical lowercase request identifier
	}
	if err := validateRuntimeText(request.WorkUnit, 160); err != nil {
		return RescopeObjectiveRequest{}, fmt.Errorf("invalid work_unit: %w", err)
	}
	if err := validateRuntimeText(request.EvidenceGoal, 240); err != nil {
		return RescopeObjectiveRequest{}, fmt.Errorf("invalid evidence_goal: %w", err)
	}
	if request.MaxAttempts < 1 || request.MaxAttempts > maximumRuntimeAttemptLimit {
		return RescopeObjectiveRequest{}, fmt.Errorf("max_attempts must be within 1..%d", maximumRuntimeAttemptLimit)
	}
	if request.MaxChangedLines < 1 || request.MaxChangedLines > maximumRuntimeChangedLines {
		return RescopeObjectiveRequest{}, fmt.Errorf("max_changed_lines must be within 1..%d", maximumRuntimeChangedLines)
	}
	if err := validateRuntimeText(request.Reason, 500); err != nil {
		return RescopeObjectiveRequest{}, fmt.Errorf("invalid rescope reason: %w", err)
	}
	if err := validateRuntimeText(request.Actor, 128); err != nil {
		return RescopeObjectiveRequest{}, fmt.Errorf("invalid rescope actor: %w", err)
	}
	return request, nil
}

func normalizeBindReviewRequest(request BindReviewRequest) (BindReviewRequest, error) {
	// Expected revision syntax is checked only after candidate preparation so
	// an identical-candidate retry remains idempotent even when an old caller
	// repeats a malformed token. A non-idempotent request can never publish it.
	if len(request.ExpectedBindingRevision) > 128 || strings.ContainsAny(request.ExpectedBindingRevision, "\r\n\x00") {
		return BindReviewRequest{}, errors.New("expected binding revision is not a bounded single-line value")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return BindReviewRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if !validReviewBindingLineage(request.LineageID) {
		return BindReviewRequest{}, errors.New("lineage_id must be a canonical lowercase lineage")
	}
	return request, nil
}

func validatePreparedRuntimeBinding(binding ReviewBinding, change, lineage string) (ReviewBinding, error) {
	payload, err := bindingBytes(binding)
	if err != nil {
		return ReviewBinding{}, err
	}
	parsed, err := parseBinding(payload)
	if err != nil {
		return ReviewBinding{}, err
	}
	if parsed.Change != change || parsed.Lineage != lineage {
		return ReviewBinding{}, errors.New("prepared SDD review binding does not match selected change and lineage")
	}
	return parsed, nil
}

// readLegacyBinding is the only compatibility read of mutable binding.json.
// Callers invoke it only while the native runtime binding is absent; replay of
// a native import never consults the legacy artifact again.
func (store RuntimeStore) readLegacyBinding() (*ReviewBinding, string, error) {
	path := filepath.Join(store.commonDir, "gentle-ai", "sdd-review-bindings", "v1", store.Change, "binding.json")
	payload, err := readBoundedRuntimeFile(path)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	binding, err := parseBinding(payload)
	if err != nil {
		return nil, "", err
	}
	if binding.Change != store.Change {
		return nil, "", errors.New("legacy SDD review binding change does not match store")
	}
	return &binding, bindingHash(payload), nil
}

func validateRuntimeText(value string, maximum int) error {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("value must be non-empty, trimmed, single-line, and bounded")
	}
	return nil
}

func validTerminalAttemptOutcome(outcome AttemptOutcome) bool {
	return outcome == AttemptFailed || outcome == AttemptInterrupted || outcome == AttemptPassed
}

func validHarnessDisposition(disposition HarnessDisposition) bool {
	return disposition == HarnessReused || disposition == HarnessInvalidated
}

// wrapRuntimeCandidateUnavailable is the single wrap for every candidate
// capture the attempt ledger performs, so no capture site can reach the
// compact boundary unclassified the way both of them did before #2114.
func wrapRuntimeCandidateUnavailable(stage string, cause error) error {
	return fmt.Errorf("%w %s: %w", ErrRuntimeCandidateUnavailable, stage, cause)
}

func captureRuntimeCandidate(ctx context.Context, repo string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: []string{},
	})
}

// captureRuntimeTerminalCandidate rebuilds the current workspace candidate
// overlaid on the attempt's begin candidate tree, the same computation Begin
// and Reset both use to detect whether the candidate drifted out from under
// a terminal (no active attempt) objective scope.
func captureRuntimeTerminalCandidate(ctx context.Context, store RuntimeStore, beginCandidateTree string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: store.Repo}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: beginCandidateTree,
		Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
}

func captureRuntimeHandoffCandidate(ctx context.Context, worktree, beginCandidateTree string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: worktree}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: beginCandidateTree,
		Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: []string{},
	})
}

func canonicalRuntimeHandoffPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if err := validateRuntimeText(resolved, 4096); err != nil || !filepath.IsAbs(resolved) {
		return "", errors.New("canonical worktree path is not an absolute bounded single-line directory") // refusal:by-design operator-knowledge: only the caller can choose an existing canonical destination
	}
	return filepath.Clean(resolved), nil
}

func (store RuntimeStore) runtimeHandoffStatusExit() string {
	return fmt.Sprintf("run `gentle-ai sdd-attempt status --cwd %s --change %q` to read the active attempt and its current execution worktree", pathquote.Quote(store.Workspace), store.Change)
}

func (store RuntimeStore) runtimeHandoffSourceRefusal(active RuntimeAttempt) error {
	return fmt.Errorf("%w: attempt %d has effective worktree %s, but handoff ran from %s; %s",
		ErrRuntimeHandoffSource, active.Ordinal, pathquote.Quote(active.EffectiveWorktree), pathquote.Quote(store.Workspace), store.runtimeHandoffStatusExit())
}

func (store RuntimeStore) runtimeHandoffAlreadyPerformedRefusal(active RuntimeAttempt) error {
	return fmt.Errorf("%w: attempt %d already moved from %s to %s; %s",
		ErrRuntimeHandoffAlreadyPerformed, active.Ordinal, pathquote.Quote(active.Handoff.SourceWorktree), pathquote.Quote(active.Handoff.DestinationWorktree), store.runtimeHandoffStatusExit())
}

func (store RuntimeStore) validateRuntimeHandoffDestination(ctx context.Context, destination string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", store.Repo, "worktree", "list", "--porcelain")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("%w: cannot read the Git worktree registry: %v; %s", ErrRuntimeHandoffDestination, err, store.runtimeHandoffStatusExit())
	}
	registered := false
	for _, line := range strings.Split(string(output), "\n") {
		path, ok := strings.CutPrefix(line, "worktree ")
		if !ok || path == "" {
			continue
		}
		canonical, resolveErr := canonicalRuntimeHandoffPath(path)
		if resolveErr == nil && canonical == destination {
			registered = true
			break
		}
	}
	if !registered || destination == store.Workspace {
		return "", fmt.Errorf("%w: %s is not a distinct registered linked worktree; %s", ErrRuntimeHandoffDestination, pathquote.Quote(destination), store.runtimeHandoffStatusExit())
	}
	command = exec.CommandContext(ctx, "git", "-C", destination, "rev-parse", "--git-common-dir")
	output, err = command.Output()
	if err != nil {
		return "", fmt.Errorf("%w: cannot resolve destination Git common directory: %v; %s", ErrRuntimeHandoffDestination, err, store.runtimeHandoffStatusExit())
	}
	commonDir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(destination, commonDir)
	}
	commonDir, err = canonicalRuntimeHandoffPath(commonDir)
	if err != nil {
		return "", fmt.Errorf("%w: resolve destination Git common directory: %v; %s", ErrRuntimeHandoffDestination, err, store.runtimeHandoffStatusExit())
	}
	storeCommonDir, err := canonicalRuntimeHandoffPath(store.commonDir)
	if err != nil || commonDir != storeCommonDir {
		return "", fmt.Errorf("%w: %s does not share this ledger's Git common directory; %s", ErrRuntimeHandoffDestination, pathquote.Quote(destination), store.runtimeHandoffStatusExit())
	}
	return commonDir, nil
}

// runtimeResetStructurallyPermitted reports whether the ledger's terminal
// scope (no active attempt, which callers must verify separately) is one
// from which a reset record is a structurally valid successor: the
// objective already requires a decision, is complete, or its last recorded
// attempt is a terminal failure/interruption. This exact predicate is
// evaluated both when writing a reset (RuntimeStore.Reset) and when
// replaying one from the immutable chain (applyRuntimeRecord), so a
// committed reset always replays deterministically.
func runtimeResetStructurallyPermitted(status RuntimeStatus) bool {
	if status.DecisionRequired || status.Complete {
		return true
	}
	if len(status.Attempts) == 0 {
		return false
	}
	last := status.Attempts[len(status.Attempts)-1]
	return last.Outcome == AttemptFailed || last.Outcome == AttemptInterrupted
}

// runtimeChainFailedEvidence derives the unmanaged-remediation binding from
// the immutable attempt chain (#1974 slice 2): the most recent settled
// AttemptFailed attempt's EvidenceRevision, provided no AttemptPassed
// settlement follows it. Running and interrupted attempts between the failure
// and its correction are honest audit records, not semantic successors, and
// audited resets, rescopes, and advances never appear in the chain at all, so
// none of them sever the binding. The first passed settlement after the
// failure DOES sever it: that pass is the one correction the failed evidence
// admits, so a later correction claiming the same revision finds no failed
// evidence in the chain and is refused -- the same anti-laundering budget the
// live evidence pointer used to enforce, now immune to that pointer being
// wiped. Evaluated by RuntimeStore.Finish and applyRuntimeFinishEvent in
// lockstep, so a committed correction always replays deterministically.
func runtimeChainFailedEvidence(attempts []RuntimeAttempt) (string, bool) {
	failed, ok := runtimeChainFailedAttempt(attempts)
	if !ok {
		return "", false
	}
	return failed.EvidenceRevision, true
}

// runtimeChainFailedAttempt is runtimeChainFailedEvidence's whole record. The
// evidence revision alone answers "which failure does this correction repair";
// #2621 also has to answer "which objective was that failure recorded under",
// so the authority a reset carries can be matched against the exact failure it
// terminated rather than against any failure that happens to precede it.
func runtimeChainFailedAttempt(attempts []RuntimeAttempt) (RuntimeAttempt, bool) {
	for index := len(attempts) - 1; index >= 0; index-- {
		switch attempts[index].Outcome {
		case AttemptPassed:
			return RuntimeAttempt{}, false
		case AttemptFailed:
			return attempts[index], true
		}
	}
	return RuntimeAttempt{}, false
}

// runtimeEvidenceOnlyRetryAuthorized reports whether an audited reset or rescope
// already authorized one evidence-only correction of this exact candidate (#2621).
//
// A verification failure is not always candidate-caused: when the authoritative
// suite fails on a transient defect the candidate never introduced, the honest
// correction reruns the verification instead of editing bytes that were never
// wrong. The ordinary changed-candidate demand exists to stop unreviewed
// content from being laundered into a passing record, and it must not be
// weakened for candidates nobody vouched for. So the waiver rests entirely on
// authority the ledger ALREADY records rather than on a new operator claim:
//
//   - The reset or rescope names a maintainer and a reason. It is an audited
//     human act.
//   - It terminated the exact objective+generation the failure was recorded
//     under, so an authority transition taken before that failure can never
//     authorize it.
//   - Its candidate tree is these bytes, so the maintainer authorized the
//     retry while looking at precisely the content that is about to pass.
//
// Everything else the correction owes stays enforced by the caller: review must
// be disabled with no binding, the chain must still hold the failed evidence
// being repaired, and the corrected evidence must be fresh and distinct. No
// approval is fabricated -- one that a human already gave is honored.
func runtimeEvidenceOnlyRetryAuthorized(reset *RuntimeReset, rescope *RuntimeRescope, failed RuntimeAttempt, candidateTree string) bool {
	if candidateTree == "" {
		return false
	}
	if reset != nil && reset.Actor != "" && reset.Reason != "" &&
		reset.PreviousObjectiveID == failed.ObjectiveID && reset.PreviousGeneration == failed.ObjectiveGeneration &&
		reset.ResetCandidateTree == candidateTree {
		return true
	}
	return rescope != nil && rescope.Actor != "" && rescope.Reason != "" &&
		rescope.PreviousObjectiveID == failed.ObjectiveID && rescope.PreviousGeneration == failed.ObjectiveGeneration &&
		rescope.RescopeCandidateTree == candidateTree
}

func runtimeObjectiveID(change, workUnit, evidenceGoal, candidateIdentity string, generation int) string {
	return runtimeValueHash(runtimeObjectiveSchemaV2, struct {
		Change            string `json:"change"`
		WorkUnit          string `json:"work_unit"`
		EvidenceGoal      string `json:"evidence_goal"`
		CandidateIdentity string `json:"candidate_identity"`
		Generation        int    `json:"generation"`
	}{Change: change, WorkUnit: workUnit, EvidenceGoal: evidenceGoal, CandidateIdentity: candidateIdentity, Generation: generation})
}

func runtimeObjectiveIDV1(change, evidenceGoal, candidateIdentity string, generation int) string {
	return runtimeValueHash(runtimeObjectiveSchema, struct {
		Change            string `json:"change"`
		EvidenceGoal      string `json:"evidence_goal"`
		CandidateIdentity string `json:"candidate_identity"`
		Generation        int    `json:"generation"`
	}{Change: change, EvidenceGoal: evidenceGoal, CandidateIdentity: candidateIdentity, Generation: generation})
}

func legacyRuntimeObjectiveID(change, evidenceGoal string) string {
	return runtimeValueHash(runtimeObjectiveSchema, struct {
		Change       string `json:"change"`
		EvidenceGoal string `json:"evidence_goal"`
	}{Change: change, EvidenceGoal: evidenceGoal})
}

func runtimeValueHash(domain string, value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(append(append([]byte(domain), '\n'), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func runtimeRecordRevision(record runtimeRecord) (string, []byte, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", nil, err
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), payload, nil
}

type runtimeConsecutiveRescopeRepairRequiredError struct {
	Revision     string
	Continuation string
}

func (err *runtimeConsecutiveRescopeRepairRequiredError) Error() string {
	return fmt.Sprintf("published consecutive-rescope record %s is unreadable under normal replay; run this repair command:\n%s", err.Revision, err.Continuation)
}

func (store RuntimeStore) consecutiveRescopeRepairContinuation(revision string) string {
	requestID := "repair-" + strings.TrimPrefix(revision, "sha256:")[:16]
	reason := "repair historical consecutive-rescope publication " + revision
	return strings.Join([]string{
		"gentle-ai", "sdd-attempt", "repair",
		"--cwd", pathquote.ShellWord(store.Workspace),
		"--change", pathquote.ShellWord(store.Change),
		"--expected-revision", pathquote.ShellWord(revision),
		"--request-id", pathquote.ShellWord(requestID),
		"--actor", pathquote.ShellWord("sdd-runtime-repair"),
		"--reason", pathquote.ShellWord(reason),
	}, " ")
}

var runtimeRepairBeforePublishHook = func() {}

func (store RuntimeStore) ensureDirectories() error {
	if filepath.Clean(store.commonDir) == "." || !filepath.IsAbs(store.commonDir) {
		return errors.New("SDD runtime common directory is invalid")
	}
	relative, err := filepath.Rel(store.commonDir, filepath.Join(store.Dir, "records"))
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("SDD runtime authority escapes the Git common directory")
	}
	current := store.commonDir
	segments := strings.Split(relative, string(filepath.Separator))
	created := make([]string, 0, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("SDD runtime authority contains an invalid path segment")
		}
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			mode := os.FileMode(0o700)
			// The shared gentle-ai container predates this private store and may
			// also hold review authority. New SDD runtime descendants remain 0700.
			if index == 0 && segment == "gentle-ai" {
				mode = 0o755
			}
			if err := os.Mkdir(current, mode); err != nil {
				if !os.IsExist(err) {
					return err
				}
			} else {
				created = append(created, current)
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("SDD runtime authority path is not a private directory")
		}
	}
	if filepath.Clean(current) != filepath.Clean(filepath.Join(store.Dir, "records")) {
		return errors.New("SDD runtime authority path resolution is inconsistent")
	}
	for _, path := range created {
		if err := runtimeSyncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync parent of SDD runtime authority directory: %w", err)
		}
	}
	if err := os.Chmod(store.Dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(store.Dir, "records"), 0o700); err != nil {
		return err
	}
	return nil
}

func (store RuntimeStore) publishRecord(revision string, payload []byte) error {
	recordsDir := filepath.Join(store.Dir, "records")
	path := filepath.Join(recordsDir, strings.TrimPrefix(revision, "sha256:")+".json")
	temp, err := os.CreateTemp(recordsDir, ".record-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.Write(payload)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := runtimePublishRecord(tempPath, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := readBoundedRuntimeFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, payload) {
			return errors.New("existing immutable SDD runtime record differs from its revision")
		}
	}
	if err := runtimeSyncDirectory(recordsDir); err != nil {
		return fmt.Errorf("sync immutable SDD runtime record directory: %w", err)
	}
	return nil
}

func (store RuntimeStore) publishHead(revision string) error {
	temp, err := os.CreateTemp(store.Dir, ".head-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.WriteString(revision + "\n")
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := runtimeReplaceHead(tempPath, filepath.Join(store.Dir, "HEAD")); err != nil {
		return fmt.Errorf("publish SDD runtime HEAD: %w", err)
	}
	return nil
}

func (store RuntimeStore) syncReplay() error {
	if err := runtimeSyncDirectory(filepath.Join(store.Dir, "records")); err != nil {
		return fmt.Errorf("sync immutable SDD runtime record directory: %w", err)
	}
	if err := runtimeSyncDirectory(store.Dir); err != nil {
		return fmt.Errorf("sync SDD runtime HEAD directory: %w", err)
	}
	return nil
}

func readRuntimeHead(path string) (string, bool, error) {
	payload, err := readBoundedRuntimeFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if len(payload) != len("sha256:")+64+1 || payload[len(payload)-1] != '\n' {
		return "", true, errors.New("invalid SDD runtime HEAD encoding")
	}
	revision := strings.TrimSuffix(string(payload), "\n")
	if !runtimeRevisionPattern.MatchString(revision) {
		return "", true, errors.New("invalid SDD runtime HEAD revision")
	}
	return revision, true, nil
}

func (store RuntimeStore) loadRecord(revision string) (runtimeRecord, error) {
	if !runtimeRevisionPattern.MatchString(revision) {
		return runtimeRecord{}, errors.New("invalid SDD runtime record revision")
	}
	path := filepath.Join(store.Dir, "records", strings.TrimPrefix(revision, "sha256:")+".json")
	payload, err := readBoundedRuntimeFile(path)
	if err != nil {
		return runtimeRecord{}, fmt.Errorf("load SDD runtime revision %s: %w", revision, err)
	}
	sum := sha256.Sum256(payload)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != revision {
		return runtimeRecord{}, fmt.Errorf("SDD runtime record revision mismatch: expected %s, got %s", revision, actual)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record runtimeRecord
	if err := decoder.Decode(&record); err != nil {
		return runtimeRecord{}, fmt.Errorf("decode SDD runtime revision %s: %w", revision, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return runtimeRecord{}, errors.New("SDD runtime record contains multiple JSON values")
	}
	_, canonical, err := runtimeRecordRevision(record)
	if err != nil || !bytes.Equal(payload, canonical) {
		return runtimeRecord{}, errors.New("SDD runtime record is not canonical")
	}
	if record.Change != store.Change {
		return runtimeRecord{}, errors.New("SDD runtime record change does not match store")
	}
	return record, nil
}

func readBoundedRuntimeFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumRuntimeRecordBytes {
		return nil, errors.New("SDD runtime authority artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maximumRuntimeRecordBytes+1))
}
