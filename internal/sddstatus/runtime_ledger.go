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
	"slices"
	"strconv"
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
	runtimeOperationReset                    = "objective/reset"
	runtimeOperationRescope                  = "objective/rescope"
	runtimeOperationRepairConsecutiveRescope = "objective/repair-consecutive-rescope"
	runtimeOperationAdvance                  = "objective/advance"
	runtimeOperationHandoff                  = "attempt/handoff"
	runtimeOperationGrant                    = "authority/grant"
	maximumRuntimeGrantRoots                 = 32
	maximumRuntimeIntendedUntracked          = 32
	runtimeLockAcquireAttempts               = 3
	finalVerifyWorkUnit                      = "verify"
	finalVerifyAttestationWorkUnit           = "verify-attestation"

	// runtimeLedgerStatusPointer suffixes every ledger refusal an ordinary
	// caller can hit (budget exhausted, active attempt, no active attempt,
	// objective already complete, no objective to reset) so the error text
	// alone — without prior knowledge of the negotiated envelope — names the
	// one command that already derives the correct continuation. The command
	// includes --cwd/--change placeholders because the bare form is rejected
	// by the CLI for missing required flags (internal/cli/sdd_attempt.go); a
	// continuation that fails when pasted is worse than none.
	runtimeLedgerStatusPointer = "run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` — its next_action names the continuation"
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
	// ErrRuntimeObjectiveDone names the successor route (#3884): a complete
	// objective continues through acquire with a different --work-unit, and
	// rescope refuses it, so pointing at status alone left callers circling.
	ErrRuntimeObjectiveDone = errors.New("SDD runtime objective is complete; it continues through a successor objective, so run " +
		"`gentle-ai sdd-attempt acquire --cwd <repo> --change <change> --request-id \"<unique-request-id>\" --work-unit \"<a different label>\" " +
		"--evidence-goal \"<stable-goal>\" --max-attempts <count> --max-changed-lines <count>` with a different --work-unit (rescope applies only " +
		"to an objective that is not complete); " + runtimeLedgerStatusPointer)
	ErrRuntimeNoObjective = errors.New("SDD runtime ledger has no objective to reset; " + runtimeLedgerStatusPointer)
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
	ErrRuntimeRescopeWidened = errors.New("SDD runtime objective rescope may only narrow or hold max_attempts and max_changed_lines, never widen them; " + runtimeLedgerStatusPointer)
	// ErrRuntimeRescopeExhausted is rescope's second own sentinel (#2804):
	// rescope carries cumulative_attempts and cumulative_changed_lines
	// forward unchanged, so a successor whose ceiling the carried charge
	// already meets has no runnable ordinal. Committing it published a
	// wedge: status advertised begin while acquire refused
	// budget-exhausted, reset was refused for zero drift, and a second
	// rescope could not widen. The successor must stay runnable, which the
	// predecessor ceiling always still admits (an exhausted predecessor is
	// decision-required, where rescope is structurally refused).
	ErrRuntimeRescopeExhausted = errors.New("SDD runtime objective rescope must leave the successor runnable: max_attempts and max_changed_lines must exceed the carried cumulative charges; " + runtimeLedgerStatusPointer)
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
	ErrRuntimeCandidateUnavailable = errors.New("SDD runtime candidate could not be captured from the repository") // refusal:by-design world-action: the exit is a repository-state change (stage the candidate, gitignore an untracked nested checkout, restore a pruned object), which no command of this product can decide or perform; every wrap keeps the snapshot builder's own refusal as the cause and that cause names the exact action
	// ErrRuntimeUndeclaredUntracked classifies every
	// settlementUntrackedSelection refusal (#3881): the attempt authority is
	// intact and unmutated, and what refused is the settlement's untracked
	// ruling -- missing for files the attempt created (#3806's headline
	// case), made against a stale inventory, naming a path outside the
	// eligible inventory, narrowing a begin selection, or offered to a legacy
	// record that has no inventory to accept one against. Every exit is a
	// corrected rerun of settle/finish, so the caller can always continue;
	// left unclassified these fell through to compactMutationFailure's opaque
	// authority_failure default, which its own contract reserves for what its
	// name says. Like its siblings it never travels alone: every wrap keeps
	// the refusal text that names the runnable continuation.
	ErrRuntimeUndeclaredUntracked     = errors.New("SDD runtime settlement requires an untracked ruling this request does not carry")        // refusal:by-design operator-knowledge: only the caller can choose whether to select or exclude the eligible untracked paths, and every wrap names the exact settle/finish rerun that carries that choice
	ErrRuntimeHandoffSource           = errors.New("SDD runtime handoff source does not equal the active attempt's effective worktree")      // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command
	ErrRuntimeHandoffDestination      = errors.New("SDD runtime handoff destination is not a registered linked worktree of this repository") // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command
	ErrRuntimeHandoffAlreadyPerformed = errors.New("SDD runtime attempt has already been handed off")                                        // refusal:by-design operator-knowledge: the RuntimeStore wrapper names the active attempt's actual status command

	runtimeRequestIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	runtimeRevisionPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	runtimeGitTreePattern      = regexp.MustCompile(`^[a-f0-9]{40}(?:[a-f0-9]{24})?$`)
	runtimeChangePattern       = regexp.MustCompile(`^[A-Za-z0-9]+(?:[-_][A-Za-z0-9]+)*$`)
	legacyRuntimeChangePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

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
	Ordinal                    int      `json:"ordinal"`
	ObjectiveID                string   `json:"objective_id"`
	ObjectiveGeneration        int      `json:"objective_generation"`
	WorkUnit                   string   `json:"work_unit"`
	BeginCandidateIdentity     string   `json:"begin_candidate_identity"`
	BeginCandidateTree         string   `json:"begin_candidate_tree"`
	IntendedUntracked          []string `json:"intended_untracked,omitempty"`
	EligibleUntrackedInventory string   `json:"eligible_untracked_inventory,omitempty"`
	// BeginWorktree is the canonical (absolute, symlink-evaluated) --cwd Begin
	// ran under (#2296 part 1). It is empty for every chain recorded before
	// this field existed — that emptiness IS the legacy signal, so replay and
	// Finish must treat it as "no binding recorded" and enforce nothing.
	BeginWorktree              string             `json:"begin_worktree,omitempty"`
	EffectiveWorktree          string             `json:"effective_worktree,omitempty"`
	Handoff                    *RuntimeHandoff    `json:"handoff,omitempty"`
	FinishCandidateIdentity    string             `json:"finish_candidate_identity,omitempty"`
	FinishCandidateTree        string             `json:"finish_candidate_tree,omitempty"`
	AttestedVerifyReportDigest string             `json:"attested_verify_report_digest,omitempty"`
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
	GrantedRoots []string `json:"granted_roots,omitempty"`
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
	ExpectedRevision  string   `json:"expected_revision"`
	RequestID         string   `json:"request_id"`
	WorkUnit          string   `json:"work_unit"`
	EvidenceGoal      string   `json:"evidence_goal"`
	MaxAttempts       int      `json:"max_attempts"`
	MaxChangedLines   int      `json:"max_changed_lines"`
	IntendedUntracked []string `json:"intended_untracked"`
}

// legacyBeginAttemptRequest preserves replay of records written before
// intended_untracked became candidate provenance.
type legacyBeginAttemptRequest struct {
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
	RemediatesEvidenceRevision string             `json:"remediates_evidence_revision,omitempty"`

	// A settle-time declaration about untracked files this attempt created.
	// Both are omitempty so a request that declares nothing marshals exactly
	// as it did before the fields existed, which keeps every legacy finish
	// request digest byte-identical without a second hashing shape.
	// IntendedUntracked is nil when nothing was declared and non-nil (possibly
	// empty, from --untracked-scope=exclude) when something was.
	IntendedUntracked          *[]string `json:"intended_untracked,omitempty"`
	ExpectedUntrackedInventory string    `json:"expected_untracked_inventory,omitempty"`
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

// RuntimeStore is one provider-owned immutable chain for one SDD change. Its
// directory is rooted in the repository Git common-dir, so linked worktrees
// and later processes observe the same attempt ordinals and line charges.
type RuntimeStore struct {
	Dir       string
	Repo      string
	Workspace string
	Change    string
	// ReviewDisabled remains an in-memory test input so mode-transition tests
	// can prove it cannot affect SDD attempt admission or settlement. Runtime
	// attempt operations do not read or persist it.
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
	Grant            *runtimeGrantEvent   `json:"grant,omitempty"`
}

// runtimeGrantEvent is the persisted per-change edit-authority grant (#2540
// S2): a maintainer-authorized record that this change's apply actor may edit
// the named edit roots, recorded as canonical absolute symlink-evaluated
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
	// A nil pointer preserves records written before candidate provenance was
	// introduced; a non-nil empty slice is a modern, explicit empty selection.
	IntendedUntracked *[]string `json:"intended_untracked,omitempty"`
	// EligibleUntrackedInventory is the digest of the whole eligible untracked
	// inventory this attempt began against -- what the caller saw when they
	// declared. IntendedUntracked records only what they SELECTED, so without
	// this the finish guard cannot tell a path the caller deliberately left
	// out from one the attempt created afterwards (#3806). A nil pointer is a
	// record written before the field existed, and the guard stays silent for
	// it rather than re-asking a decision it cannot read.
	EligibleUntrackedInventory *string `json:"eligible_untracked_inventory,omitempty"`
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
	AttestedVerifyReportDigest string             `json:"attested_verify_report_digest,omitempty"`
	Outcome                    AttemptOutcome     `json:"outcome"`
	ChangedLines               int                `json:"changed_lines"`
	EvidenceRevision           string             `json:"evidence_revision"`
	Diagnosis                  string             `json:"diagnosis"`
	HarnessDisposition         HarnessDisposition `json:"harness_disposition"`
	CleanupEvidence            string             `json:"cleanup_evidence"`
	ProcessEvidence            string             `json:"process_evidence"`
	RemediatesEvidenceRevision string             `json:"remediates_evidence_revision,omitempty"`
	ChangedLineBudgetExceeded  bool               `json:"changed_line_budget_exceeded,omitempty"`

	// IntendedUntracked is the selection this settlement actually overlaid,
	// which is the begin selection unless the caller declared a new one here.
	// DeclaredUntrackedInventory is the digest they declared against; it is
	// empty exactly when they declared nothing, which is how replay knows
	// whether the request carried a selection of its own.
	IntendedUntracked          *[]string `json:"intended_untracked,omitempty"`
	DeclaredUntrackedInventory string    `json:"declared_untracked_inventory,omitempty"`
}

type runtimeRequestReceipt struct {
	Digest   string
	Revision string
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
	if !validRuntimeChange(change) {
		return RuntimeStore{}, fmt.Errorf("invalid SDD change name %q; want letters, digits, and single hyphens or underscores between them, at most 96 characters; run `gentle-ai sdd-status --cwd <repo> --json` to read the resolved changeName", change)
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		if abs, absErr := filepath.Abs(repo); absErr == nil && !workspaceHasGitMetadata(abs) {
			return RuntimeStore{}, &RuntimeRepositoryRequiredError{Workspace: abs, Cause: err}
		}
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

// RuntimeRepositoryRequiredError refuses to open the runtime attempt ledger
// outside a Git repository (#2612, #3202): its authority lives in the Git
// common directory, so there is nowhere to keep it until one exists.
type RuntimeRepositoryRequiredError struct {
	Workspace string
	Cause     error
}

func (err *RuntimeRepositoryRequiredError) Error() string {
	return fmt.Sprintf("the SDD runtime attempt ledger needs a Git repository because its authority lives in the Git common directory, and %s is not inside one; run `git init` in that workspace (or run from the repository that contains it), then rerun the same `gentle-ai sdd-attempt` command", err.Workspace)
}

func (err *RuntimeRepositoryRequiredError) Unwrap() error { return err.Cause }

func validRuntimeChange(change string) bool {
	return len(change) <= 96 && runtimeChangePattern.MatchString(change)
}

// legacyRuntimeChangeDir reports whether a change identity is one the runtime
// ledger has always stored directly at v1/<change>.
func legacyRuntimeChangeDir(change string) bool {
	return len(change) <= 96 && legacyRuntimeChangePattern.MatchString(change)
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

// FreshRescopeSuccessorInheritsIntendedUntracked reports whether the current
// fresh rescope successor can reuse its predecessor's recorded selection.
func (store RuntimeStore) FreshRescopeSuccessorInheritsIntendedUntracked() (bool, error) {
	replay, err := store.load()
	if err != nil {
		return false, err
	}
	_, inherited := runtimeRescopeSuccessorIntendedUntracked(replay.Status)
	return inherited, nil
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
	inheritIntendedUntracked := request.IntendedUntracked == nil
	legacyRequest := inheritIntendedUntracked
	request, err := normalizeBeginAttemptRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if inheritIntendedUntracked {
		replay, loadErr := store.load()
		if loadErr != nil {
			return RuntimeStatus{}, loadErr
		}
		request = runtimeRescopeSuccessorRequest(replay.Status, request, true)
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request)
	legacyDigest := ""
	if legacyRequest {
		legacyDigest = runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", legacyBeginAttemptRequest{
			ExpectedRevision: request.ExpectedRevision, RequestID: request.RequestID, WorkUnit: request.WorkUnit,
			EvidenceGoal: request.EvidenceGoal, MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
		})
	}
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
		intendedUntracked := slices.Clone(snapshot.IntendedUntracked)
		_, eligibleInventory, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).IntendedUntrackedInventory(ctx)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("%w while reading the eligible untracked inventory this attempt begins against: %w", ErrRuntimeCandidateUnavailable, err)
		}
		event := &runtimeBeginEvent{
			ObjectiveID: objectiveID, ObjectiveGeneration: generation, WorkUnit: request.WorkUnit, EvidenceGoal: request.EvidenceGoal,
			MaxAttempts: request.MaxAttempts, MaxChangedLines: request.MaxChangedLines,
			Ordinal: status.NextOrdinal, BeginCandidateIdentity: snapshot.Identity, BeginCandidateTree: snapshot.CandidateTree,
			IntendedUntracked: &intendedUntracked, EligibleUntrackedInventory: &eligibleInventory,
			BeginWorktree: store.Workspace, EffectiveWorktree: store.Workspace,
		}
		if advancing {
			return runtimeRecord{Operation: runtimeOperationAdvance, Begin: event, Advance: &runtimeAdvanceEvent{
				PreviousObjectiveID: status.Objective.ID, PreviousGeneration: status.Objective.Generation,
				PreviousWorkUnit: status.Objective.WorkUnit,
			}}, nil
		}
		return runtimeRecord{Operation: runtimeOperationBegin, Begin: event}, nil
	}, legacyDigest)
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
		// A canonical evidence revision is accepted through normalization so an
		// exact retry can reach mutate's request receipt and replay a legacy
		// interrupted record. New interrupted requests are rejected below, after
		// that idempotency check.
		if request.Outcome == AttemptInterrupted && request.EvidenceRevision != "" {
			return runtimeRecord{}, errors.New("interrupted attempts must omit evidence_revision; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --outcome interrupted and without --evidence-revision")
		}
		// Check the effective binding before candidate capture or line charging.
		// A vanished bound worktree (#2661) admits only an interrupted settle.
		bound, boundMissing := runtimeBoundWorktree(*active)
		if boundMissing && request.Outcome != AttemptInterrupted {
			return runtimeRecord{}, store.runtimeMissingWorktreeRefusal(ErrRuntimeWorktreeMismatch, *active, bound)
		}
		if !boundMissing && active.EffectiveWorktree != "" && active.EffectiveWorktree != store.Workspace {
			return runtimeRecord{}, store.runtimeEffectiveWorktreeMismatchRefusal(*active)
		}
		if !boundMissing && active.EffectiveWorktree == "" && active.BeginWorktree != "" && active.BeginWorktree != store.Workspace {
			return runtimeRecord{}, store.runtimeWorktreeMismatchRefusal(active.Ordinal, active.BeginWorktree)
		}
		// Failed-evidence remediation is an SDD-only invariant. The immutable
		// attempt chain owns both the exact failed evidence and the one passing
		// correction that may discharge it; reset and successor lineages cannot
		// change that accounting.
		evidenceRemediation := request.RemediatesEvidenceRevision != ""
		chainFailedAttempt, chainHasFailedEvidence := runtimeChainFailedAttempt(status.Attempts)
		chainFailedEvidence := chainFailedAttempt.EvidenceRevision
		if evidenceRemediation {
			if !chainHasFailedEvidence {
				if discharged, ordinal, ok := runtimeDischargedFailure(status.Attempts, request.RemediatesEvidenceRevision); ok {
					return runtimeRecord{}, runtimeDischargedFailureRefusal(discharged, ordinal)
				}
				return runtimeRecord{}, errors.New("this correction names failed verification " + request.RemediatesEvidenceRevision + ", but the attempt chain records no failed verification at all; run `gentle-ai sdd-attempt status --cwd <repo> --change <change>` to read the chain, then settle without --remediates-evidence-revision if nothing is being repaired")
			}
			if chainFailedEvidence != request.RemediatesEvidenceRevision {
				return runtimeRecord{}, errors.New("this correction names failed verification " + request.RemediatesEvidenceRevision + ", but the chain's unremediated failure is " + chainFailedEvidence + "; settle with --remediates-evidence-revision \"" + chainFailedEvidence + "\", or without the flag if this work unit repairs nothing")
			}
		}
		if request.Outcome == AttemptPassed && chainHasFailedEvidence && !evidenceRemediation {
			return runtimeRecord{}, fmt.Errorf("passing correction for failed verification %q requires --remediates-evidence-revision %q; rerun `gentle-ai sdd-attempt settle` with that flag", chainFailedEvidence, chainFailedEvidence)
		}
		// The candidate is the begin tree overlaid with tracked changes, the
		// index, and a selection of untracked paths. The selection made at
		// begin was a decision about what already existed then, so a file the
		// attempt itself created is in none of those and settling over it
		// records the attempt's own product as no change at all (#3806). This
		// resolves which selection this settlement overlays, and refuses while
		// the caller can still act when the answer is nobody's decision yet.
		if boundMissing {
			return runtimeMissingWorktreeInterruptedRecord(status, *active, request)
		}
		intendedUntracked, declaredInventory, err := store.settlementUntrackedSelection(ctx, *active, request)
		if err != nil {
			return runtimeRecord{}, err
		}
		// Issue #2394: the runtime candidate is the same declared candidate
		// review freezes -- tracked changes plus whatever the user put in the
		// index. Sweeping the worktree here would make drift detection and
		// review disagree about what the candidate even is.
		//
		// #3842: the resolved selection can still carry begin-time paths the
		// work unit committed mid-attempt (a settle-time declaration is
		// validated against the current inventory, but the undeclared branches
		// return the begin selection as recorded), so reconcile it against the
		// current index before capture. The finish record below carries the
		// reconciled list, because that is the selection this settlement's
		// overlay actually used.
		intendedUntracked, err = runtimeReplayedIntendedUntracked(ctx, store.Repo, intendedUntracked)
		if err != nil {
			return runtimeRecord{}, wrapRuntimeCandidateUnavailable("after attempt", err)
		}
		snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).Build(ctx, reviewtransaction.Target{
			Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: active.BeginCandidateTree,
			Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intendedUntracked,
		})
		if err != nil {
			return runtimeRecord{}, wrapRuntimeCandidateUnavailable("after attempt", err)
		}
		changedLines, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).ChangedLines(ctx, snapshot)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("measure native SDD runtime line charge: %w", err)
		}
		// The changed-candidate and fresh-evidence demands exist to stop an
		// unreviewed no-op from DISCHARGING a failure, so they bind only the
		// passing outcome. A truthful failed or interrupted settlement (#3422)
		// discharges nothing: it may leave the candidate unchanged (the blocker
		// can be environmental) and its evidence is the correction's own new
		// failure, not proof the named failure was repaired.
		if evidenceRemediation && request.Outcome == AttemptPassed {
			evidenceOnly := runtimeEvidenceOnlyRetryAuthorized(status.LastReset, status.LastRescope, chainFailedAttempt, snapshot.CandidateTree)
			// #3073: "changed" is judged against the failed evidence's candidate
			// snapshot, not the attempt's begin snapshot. A correction applied
			// between the audited reset and the acquire lives inside the begin
			// snapshot already, so the begin-relative comparison refused a
			// candidate that genuinely no longer matches the state that failed.
			// Records committed under this predicate require a reader deciding
			// the same predicate (applyRuntimeFinishEvent): replay compatibility
			// is forward-only, the store's standard schema-evolution discipline.
			if !evidenceOnly && runtimeRemediationCandidateUnchanged(chainFailedAttempt, *active, snapshot.Identity, snapshot.CandidateTree) {
				// refusal:by-design operator-knowledge: a remediation claim must name a candidate changed relative to the state that failed verification, or an audited reset or rescope authorizing this exact unchanged candidate.
				return runtimeRecord{}, errors.New("failed-evidence remediation requires a changed correction candidate")
			}
			if request.EvidenceRevision == request.RemediatesEvidenceRevision {
				// refusal:by-design operator-knowledge: the correction's verification evidence must be fresh and distinct from the failed evidence it repairs.
				return runtimeRecord{}, errors.New("failed-evidence remediation requires fresh corrected evidence")
			}
		}
		attestedVerifyReport := store.captureFinalVerifyReport(ctx, *active, request, snapshot.CandidateTree)
		event := &runtimeFinishEvent{
			Ordinal: active.Ordinal, FinishCandidateIdentity: snapshot.Identity, FinishCandidateTree: snapshot.CandidateTree,
			AttestedVerifyReportDigest: attestedVerifyReport,
			Outcome:                    request.Outcome, ChangedLines: changedLines, EvidenceRevision: request.EvidenceRevision,
			Diagnosis: request.Diagnosis, HarnessDisposition: request.HarnessDisposition,
			CleanupEvidence: request.CleanupEvidence, ProcessEvidence: request.ProcessEvidence,
			RemediatesEvidenceRevision: request.RemediatesEvidenceRevision,
			ChangedLineBudgetExceeded:  runtimeChangedLineBudgetExceeded(status, changedLines),
			IntendedUntracked:          &intendedUntracked,
			DeclaredUntrackedInventory: declaredInventory,
		}
		return runtimeRecord{Operation: runtimeOperationFinish, Finish: event}, nil
	})
}

// runtimeUndeclaredUntrackedListLimit bounds the paths a single refusal spells
// out. The eligible inventory is unbounded, and a refusal nobody can read is
// one nobody acts on.
const runtimeUndeclaredUntrackedListLimit = 10

// settlementUntrackedSelection answers which untracked paths this settlement
// overlays onto the begin tree, and returns the inventory digest the caller
// declared against (empty when they declared nothing).
//
// The question only has a new answer when the attempt created eligible
// untracked files. Everything else the caller already ruled on: a path they
// selected at begin is candidate bytes, and a path they saw in that same
// inventory and did not select, they left out on purpose. Comparing today's
// inventory against the one recorded at begin is what separates the two, and
// it is why the begin record carries that digest at all.
func (store RuntimeStore) settlementUntrackedSelection(ctx context.Context, active RuntimeAttempt, request FinishAttemptRequest) ([]string, string, error) {
	if active.EligibleUntrackedInventory == "" {
		// A record written before the begin inventory was captured cannot say
		// what the caller saw, so no decision can honestly be demanded of them
		// now and none may be accepted either.
		if request.IntendedUntracked != nil {
			return nil, "", fmt.Errorf("%w: this attempt began before settle-time untracked declarations existed, so it has no inventory to declare against; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` without --untracked-scope", ErrRuntimeUndeclaredUntracked)
		}
		return active.IntendedUntracked, "", nil
	}
	inventory, digest, err := (reviewtransaction.SnapshotBuilder{Repo: store.Repo}).IntendedUntrackedInventory(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("%w while reading the eligible untracked inventory before settling: %w", ErrRuntimeCandidateUnavailable, err)
	}
	undecided := make([]string, 0, len(inventory))
	for _, path := range inventory {
		if !slices.Contains(active.IntendedUntracked, path) {
			undecided = append(undecided, path)
		}
	}
	if request.IntendedUntracked == nil {
		// Nothing eligible is undecided, or the inventory is the very one the
		// caller declared against at begin. Either way this settlement asks
		// them nothing new.
		if len(undecided) == 0 || digest == active.EligibleUntrackedInventory {
			return active.IntendedUntracked, "", nil
		}
		return nil, "", runtimeBornDuringUntrackedRefusal(undecided, digest)
	}
	if request.ExpectedUntrackedInventory != digest {
		return nil, "", fmt.Errorf("%w: this declaration was made against untracked inventory %s but the workspace now holds %s; rerun `gentle-ai review status --next-transition` for the current inventory, then rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --expected-untracked-inventory=%s", ErrRuntimeUndeclaredUntracked, request.ExpectedUntrackedInventory, digest, digest)
	}
	selection := *request.IntendedUntracked
	for _, path := range selection {
		if !slices.Contains(inventory, path) {
			return nil, "", fmt.Errorf("%w: intended-untracked path %q is not in the current eligible inventory; rerun `gentle-ai review status --next-transition` to see what is eligible, then rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with only those paths", ErrRuntimeUndeclaredUntracked, path)
		}
	}
	// A path selected at begin is already in the begin tree. Dropping it here
	// would make the overlay subtract bytes the attempt never touched, so a
	// settlement may widen the selection but never narrow it.
	for _, path := range active.IntendedUntracked {
		if slices.Contains(inventory, path) && !slices.Contains(selection, path) {
			return nil, "", fmt.Errorf("%w: this attempt began with %q in its candidate, and a settlement cannot take it back out; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --intended-untracked=%s included", ErrRuntimeUndeclaredUntracked, path, path)
		}
	}
	return selection, digest, nil
}

// runtimeBornDuringUntrackedRefusal names the eligible untracked paths nobody
// has ruled on yet, and both ways to rule on them. Selecting one makes it
// candidate bytes and charges its lines; excluding it leaves it out, which is
// what today's settlement does silently and what this refusal exists to put on
// the record instead.
func runtimeBornDuringUntrackedRefusal(undecided []string, digest string) error {
	listed, remainder := undecided, ""
	if len(listed) > runtimeUndeclaredUntrackedListLimit {
		remainder = fmt.Sprintf(" and %d more", len(listed)-runtimeUndeclaredUntrackedListLimit)
		listed = listed[:runtimeUndeclaredUntrackedListLimit]
	}
	return fmt.Errorf("%w: this attempt left eligible untracked files its candidate does not include, so settling now would record them as no change at all: %s%s; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --untracked-scope=select --intended-untracked=<repo-relative-path> --expected-untracked-inventory=%s to account them, or --untracked-scope=exclude --expected-untracked-inventory=%s to leave them out on the record", ErrRuntimeUndeclaredUntracked, strings.Join(listed, ", "), remainder, digest, digest)
}

// captureFinalVerifyReport derives the final verification attestation from the
// candidate tree being settled. It deliberately never accepts a caller digest:
// the native finish record binds the exact report bytes it read itself.
// Attestation derivation never aborts a passing settlement: an underivable
// attestation degrades like the missing-blob branch below (empty attestation,
// archive stays fail-closed), so the derivation cannot fail and returns only
// the attested digest. Only writing the ledger itself remains fatal.
func (store RuntimeStore) captureFinalVerifyReport(ctx context.Context, active RuntimeAttempt, request FinishAttemptRequest, candidateTree string) string {
	if request.Outcome != AttemptPassed || !isFinalVerifyWorkUnit(active.WorkUnit) {
		return ""
	}
	openSpecRoot := filepath.Join(store.Workspace, "openspec")
	if _, err := os.Stat(openSpecRoot); os.IsNotExist(err) {
		// Runtime attempts are also used outside OpenSpec. Without an active
		// OpenSpec root there is no canonical verify-report to attest.
		return ""
	} else if err != nil {
		return ""
	}
	changeRoot, err := resolveBindingChangeRoot(ctx, store.Repo, store.Workspace, store.Change)
	if err != nil {
		return ""
	}
	// The canonical report path is anchored at the planning workspace (--cwd),
	// never at the Git repository root: a workspace that is a subdirectory of
	// its repository still owns exactly one canonical active-change report. The
	// settled candidate tree is built at the repository root, so the blob read
	// addresses that same report through its repository-relative path.
	logicalPath, err := canonicalVerifyReportPaths(store.Repo, store.Workspace, changeRoot, store.Change)
	if err != nil {
		return ""
	}
	artifactPaths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		return ""
	}
	specCounts, err := readSpecCounts(artifactPaths.Specs)
	if err != nil {
		return ""
	}
	payload, err := reviewtransaction.ReadTreeBlob(ctx, store.Repo, candidateTree, logicalPath, MaxVerifyReportBytes)
	if errors.Is(err, reviewtransaction.ErrTreeArtifactMissing) {
		// A report outside the settled candidate is not final verification
		// evidence. Preserve the generic runtime settlement, but it carries no
		// archive-status exception and therefore remains fail-closed later.
		return ""
	}
	if err != nil {
		return ""
	}
	admission := ValidateVerifyReportAdmission(string(payload), specCounts)
	if !admission.Valid || admission.Verdict != "pass" || admission.EvidenceRevision != request.EvidenceRevision {
		return ""
	}
	return verifyReportDigest(payload)
}

func verifyReportDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func isFinalVerifyWorkUnit(workUnit string) bool {
	return workUnit == finalVerifyWorkUnit || workUnit == finalVerifyAttestationWorkUnit
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
		// #3842: reconcile the replayed selection against the DESTINATION
		// worktree's index — that is where the capture below actually runs,
		// and a linked worktree's checked-out branch may already track paths
		// the source recorded as untracked at begin time.
		intendedUntracked, err := runtimeReplayedIntendedUntracked(ctx, request.DestinationWorktree, active.IntendedUntracked)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture delegated SDD runtime candidate before handoff: %w", err)
		}
		snapshot, err := captureRuntimeHandoffCandidate(ctx, request.DestinationWorktree, active.BeginCandidateTree, intendedUntracked)
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

// runtimeRescopeExhaustedRefusal (#2804) rejects a successor scope whose
// ceiling the carried cumulative charge already meets, BEFORE mutation.
// Committed, that successor is a published wedge: status answers begin, the
// first acquire is refused budget-exhausted, reset is refused for zero
// drift, and another rescope cannot widen from the newly accepted maximum.
// The refusal names the exact runnable range instead, which is never empty:
// rescope is only structurally reachable while the predecessor still has
// budget left, so `carried < allowed` always holds here.
func (store RuntimeStore) runtimeRescopeExhaustedRefusal(flag string, requested, carried, allowed int) error {
	return fmt.Errorf(
		"%w: received %s %d, and rescope carries the cumulative charges forward unchanged, so this successor would open already exhausted -- its status would advertise begin while every acquire is refused budget-exhausted, and no later rescope could widen it. A runnable successor needs %s greater than the carried %d and at most %d, the current objective's ceiling",
		ErrRuntimeRescopeExhausted, flag, requested, flag, carried, allowed)
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

// runtimeBoundWorktree names the worktree an active attempt is bound to and
// whether that path has vanished from disk (#2661): a removed and pruned
// linked worktree made every exit that named it unrunnable. Only a not-exist
// stat counts; any other failure keeps the ordinary binding checks.
func runtimeBoundWorktree(active RuntimeAttempt) (string, bool) {
	bound := active.EffectiveWorktree
	if bound == "" {
		bound = active.BeginWorktree
	}
	if bound == "" {
		return "", false
	}
	_, err := os.Stat(bound)
	return bound, errors.Is(err, os.ErrNotExist)
}

// runtimeMissingWorktreeExit is the one exit for a vanished bound worktree:
// passed and failed need evidence measured there, so only interrupted is
// admitted, from any worktree of the repository. cwd and change arrive
// rendered so the compact readiness surface can pass placeholders.
func runtimeMissingWorktreeExit(active RuntimeAttempt, bound, cwd, change, token string) string {
	return fmt.Sprintf("attempt %d is bound to worktree %s, which no longer exists on disk, so its passed or failed evidence cannot be measured; "+
		"settle it as interrupted from any worktree of this repository with `gentle-ai sdd-attempt settle --cwd %s --change %s --token %s "+
		"--request-id \"<unique-request-id>\" --outcome interrupted --diagnosis \"<why-the-worktree-is-gone>\" --harness-disposition invalidated "+
		"--cleanup-evidence \"<evidence>\" --process-evidence \"<evidence>\"`, then follow the ledger's next_action, or have a maintainer discard "+
		"the objective with `gentle-ai sdd-attempt reset --cwd %s --change %s --expected-revision <the revision that status prints> "+
		"--request-id \"<unique-request-id>\" --reason \"<why-the-objective-changed>\" --actor \"<actor>\"`",
		active.Ordinal, pathquote.Quote(bound), cwd, change, token, cwd, change)
}

func (store RuntimeStore) runtimeMissingWorktreeRefusal(sentinel error, active RuntimeAttempt, bound string) error {
	return fmt.Errorf("%w: %s", sentinel, runtimeMissingWorktreeExit(active, bound, pathquote.Quote(store.Workspace), fmt.Sprintf("%q", store.Change), "\"<acquire-token>\""))
}

// runtimeAttemptActiveRefusal is ErrRuntimeAttemptActive with the vanished
// worktree exit composed in when that is the state the caller is actually in.
func (store RuntimeStore) runtimeAttemptActiveRefusal(active RuntimeAttempt) error {
	if bound, missing := runtimeBoundWorktree(active); missing {
		return store.runtimeMissingWorktreeRefusal(ErrRuntimeAttemptActive, active, bound)
	}
	return ErrRuntimeAttemptActive
}

// runtimeMissingWorktreeInterruptedRecord closes an attempt whose bound
// worktree is gone on its own begin candidate with no line charge; a later
// begin or reset measures drift against the worktree it runs from, as before.
func runtimeMissingWorktreeInterruptedRecord(status RuntimeStatus, active RuntimeAttempt, request FinishAttemptRequest) (runtimeRecord, error) {
	if request.IntendedUntracked != nil {
		return runtimeRecord{}, fmt.Errorf("%w: the bound worktree no longer exists, so no untracked inventory can be declared against it; rerun `gentle-ai sdd-attempt settle` with --outcome interrupted and without --untracked-scope", ErrRuntimeUndeclaredUntracked)
	}
	intended := slices.Clone(active.IntendedUntracked)
	return runtimeRecord{Operation: runtimeOperationFinish, Finish: &runtimeFinishEvent{
		Ordinal: active.Ordinal, FinishCandidateIdentity: active.BeginCandidateIdentity, FinishCandidateTree: active.BeginCandidateTree,
		Outcome: AttemptInterrupted, Diagnosis: request.Diagnosis, HarnessDisposition: request.HarnessDisposition,
		CleanupEvidence: request.CleanupEvidence, ProcessEvidence: request.ProcessEvidence,
		RemediatesEvidenceRevision: request.RemediatesEvidenceRevision,
		ChangedLineBudgetExceeded:  runtimeChangedLineBudgetExceeded(status, 0),
		IntendedUntracked:          &intended,
	}}, nil
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
	// #3842: reconcile the replayed selection first — a selected path the user
	// has since committed would otherwise fail the capture and this probe
	// would answer false exactly when the commit made reset the right exit.
	intended, err := runtimeReplayedIntendedUntracked(ctx, store.Repo, last.IntendedUntracked)
	if err != nil {
		return false
	}
	candidate, err := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree, intended)
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
	// #3842: same replay reconciliation as the reset probe, and for the same
	// fail-closed reason — a reconcile error is a capture that could not run.
	intended, err := runtimeReplayedIntendedUntracked(ctx, store.Repo, last.IntendedUntracked)
	if err != nil {
		return false
	}
	candidate, err := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree, intended)
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
			return runtimeRecord{}, store.runtimeAttemptActiveRefusal(*status.ActiveAttempt)
		}
		if status.Objective == nil {
			return runtimeRecord{}, ErrRuntimeNoObjective
		}
		if !runtimeResetStructurallyPermitted(status) {
			return runtimeRecord{}, ErrRuntimeResetNotAllowed
		}
		last := status.Attempts[len(status.Attempts)-1]
		// #3842: the recorded selection is a historical replay by reset time —
		// the completed work unit's selected paths are ordinarily committed by
		// now — so reconcile it once against the current index and feed the
		// same reconciled list to both captures below.
		intended, err := runtimeReplayedIntendedUntracked(ctx, store.Repo, last.IntendedUntracked)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate at objective reset: %w", err)
		}
		if !status.DecisionRequired && !status.Complete {
			// The only remaining structurally-permitted scope is a terminal
			// failed/interrupted attempt with budget still available: begin
			// is the ordinary continuation here, so admit reset only when
			// begin is actually blocked by candidate drift. Otherwise an
			// elective early reset would launder the per-objective budget
			// (CumulativeAttempts resets to zero on every reset).
			candidate, driftErr := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree, intended)
			if driftErr != nil {
				return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate to check reset drift eligibility: %w", driftErr)
			}
			if candidate.Identity == last.FinishCandidateIdentity && candidate.CandidateTree == last.FinishCandidateTree {
				return runtimeRecord{}, store.runtimeZeroDriftResetRefusal(status)
			}
		}
		snapshot, err := captureRuntimeCandidate(ctx, store.Repo, intended)
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
			return runtimeRecord{}, store.runtimeAttemptActiveRefusal(*status.ActiveAttempt)
		}
		objective := status.Objective
		if objective == nil {
			return runtimeRecord{}, ErrRuntimeNoObjective
		}
		// A complete objective is refused by the sentinel that names its
		// successor (#3884), not by the generic structural refusal.
		if status.Complete {
			return runtimeRecord{}, ErrRuntimeObjectiveDone
		}
		if !runtimeObjectiveRescopeStructurallyPermitted(status) {
			return runtimeRecord{}, ErrRuntimeRescopeNotAllowed
		}
		last := status.Attempts[len(status.Attempts)-1]
		// #3842: reconcile the replayed selection once and feed the SAME
		// reconciled list to this drift capture and the fresh capture below,
		// so the CandidateTree comparability contract between them holds over
		// one selection rather than two.
		intended, err := runtimeReplayedIntendedUntracked(ctx, store.Repo, last.IntendedUntracked)
		if err != nil {
			return runtimeRecord{}, fmt.Errorf("capture SDD runtime candidate to check rescope drift eligibility: %w", err)
		}
		drift, driftErr := captureRuntimeTerminalCandidate(ctx, store, last.BeginCandidateTree, intended)
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
		// #2804: the carried charges bind at admission. A ceiling they
		// already meet would commit a successor with no runnable ordinal.
		if request.MaxAttempts <= status.CumulativeAttempts {
			return runtimeRecord{}, store.runtimeRescopeExhaustedRefusal(
				"--max-attempts", request.MaxAttempts, status.CumulativeAttempts, objective.MaxAttempts)
		}
		if request.MaxChangedLines <= status.CumulativeChangedLines {
			return runtimeRecord{}, store.runtimeRescopeExhaustedRefusal(
				"--max-changed-lines", request.MaxChangedLines, status.CumulativeChangedLines, objective.MaxChangedLines)
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
		fresh, err := captureRuntimeCandidate(ctx, store.Repo, intended)
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

func (store RuntimeStore) mutate(
	ctx context.Context,
	expected, requestID, requestDigest string,
	build func(runtimeReplay) (runtimeRecord, error),
	legacyRequestDigest ...string,
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
		if receipt.Digest != requestDigest &&
			(len(legacyRequestDigest) != 1 || receipt.Digest != legacyRequestDigest[0]) {
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
			// (1861). Both callers of acquireLock -- RepairConsecutiveRescope and
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
	// Verify BEFORE committing (#2833). Replay a candidate chain that ends at
	// this record; HEAD advances only if that replay lands on the expected
	// revision. Previously HEAD moved first and the replay could only report a
	// state it had already made permanent, so a record the store's own
	// validator rejects was on the chain and every later read walked into it.
	// The wedge class disappears by construction rather than by catching it.
	//
	// A record that fails here stays on disk, unreferenced by HEAD. Records are
	// content-addressed and immutable, so an unreachable one is inert: the next
	// attempt at the same record re-publishes identical bytes.
	if candidate, err := store.loadRevision(revision); err != nil {
		return RuntimeStatus{}, fmt.Errorf("replay candidate SDD runtime record: %w", err)
	} else if candidate.Status.Revision != revision {
		// HEAD did not move, so the chain is intact and status is actionable.
		// This deliberately does not say "restore the store": nothing was
		// committed, which is the entire point of verifying first.
		return RuntimeStatus{}, errors.New("candidate SDD runtime record did not replay to its own revision; HEAD was not advanced and the chain is unchanged; " + runtimeLedgerStatusPointer)
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
		head = ""
	}
	replay, err := store.loadRevision(head)
	if err != nil {
		return runtimeReplay{}, err
	}
	projectLegacyExhaustedSuccessor(&replay.Status)
	return replay, nil
}

// projectLegacyExhaustedSuccessor tells the truth about a successor a
// pre-#2804 writer published already exhausted: a rescope whose ceilings the
// carried cumulative charges meet, so no begin can ever be admitted under
// it. Left as replayed, that state advertised next_action: begin while
// acquire refused budget-exhausted, reset was refused for zero drift, and a
// second rescope could not widen -- the published wedge #2804's fresh
// occurrence reports. The immutable chain is never rewritten (this runs only
// on the terminal load() projection, never inside per-record replay, so the
// consecutive-rescope repair keeps validating against the exact historical
// writer state via loadRevision). It projects what
// applyRuntimeConsecutiveRescopeRepairEvent already projects for the same
// shape: an objective with no runnable ordinal is a maintainer decision, and
// reset -- admitted at decision-required -- opens the fresh budget.
//
// The scope is PINNED to that publication wedge rather than asserted in
// prose: the last transition must be the rescope that opened THIS objective,
// and the successor must have no attempt of its own yet -- exactly the only
// state the pre-guard writer could publish exhausted, because a begin under
// it was always refused. Any other exhausted shape keeps its replayed
// projection untouched, so an objective legitimately holding unused
// allowance can never be silently converted into a demanded reset.
func projectLegacyExhaustedSuccessor(status *RuntimeStatus) {
	if status.ActiveAttempt != nil || status.Objective == nil || status.Complete || status.DecisionRequired {
		return
	}
	if status.LastRescope == nil || status.LastRescope.ObjectiveID != status.Objective.ID ||
		runtimeObjectiveHasRecordedAttempt(*status) {
		return
	}
	if status.CumulativeAttempts >= status.Objective.MaxAttempts ||
		status.CumulativeChangedLines >= status.Objective.MaxChangedLines {
		status.DecisionRequired = true
		status.NextAction = RuntimeActionReset
	}
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

// RuntimeRecordRejectedError is the single refusal for a record that is not
// what the authority wrote. #2834: the ledger used to carry a bespoke message,
// category and justifying paragraph for each such case. Every one of them is an
// assertion about a record this package built, digested and CAS-chained, and
// the response to all of them is the same, so one typed refusal carries the
// same information without fifty-nine places to drift.
//
// Condition names the failed predicate; Revision names the offending record so
// an operator does not have to re-derive which one violated it by reading the
// chain; Path names its file under the Git common directory (#3938) so a
// record an older binary admitted is a maintainer decision, not a dead end.
// Expected/actual pairs are deliberately absent: that detail is where the
// bespoke messages came from (#3816).
type RuntimeRecordRejectedError struct {
	Condition string
	Revision  string
	Path      string
}

func (err *RuntimeRecordRejectedError) Error() string {
	if err.Revision == "" {
		return fmt.Sprintf("SDD runtime record rejected (condition %s); %s", err.Condition, runtimeLedgerStatusPointer)
	}
	if err.Path == "" {
		return fmt.Sprintf("SDD runtime record rejected (condition %s, revision %s); %s", err.Condition, err.Revision, runtimeLedgerStatusPointer)
	}
	// No repair command exists here by design (human authority): nothing may
	// rewrite or drop a published chain record; a maintainer inspects or
	// removes the named file, then the status pointer is the read-only re-entry.
	return fmt.Sprintf("SDD runtime record rejected (condition %s, revision %s); the record file is %s; a maintainer must inspect or remove that record, then %s", err.Condition, err.Revision, err.Path, runtimeLedgerStatusPointer)
}

// rejectRuntimeRecord refuses a record that disagrees with what the authority
// wrote. Only a record this package itself produced reaches here, so no
// runnable continuation can repair it; the status pointer in the message is
// the operator's entry point.
func rejectRuntimeRecord(condition string) error {
	return &RuntimeRecordRejectedError{Condition: condition}
}

// withRuntimeRecordRevision stamps the offending revision and its record file
// onto a rejection once the caller knows them. Unrelated errors pass through
// untouched.
func withRuntimeRecordRevision(err error, revision string, path string) error {
	var rejected *RuntimeRecordRejectedError
	if err == nil || !errors.As(err, &rejected) || rejected.Revision != "" {
		return err
	}
	return &RuntimeRecordRejectedError{Condition: rejected.Condition, Revision: revision, Path: path}
}

// applyRuntimeRecord stamps every rejection with the offending revision and
// the record file it was read from.
func applyRuntimeRecord(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	return withRuntimeRecordRevision(applyRuntimeRecordLocked(store, replay, revision, record), revision, store.recordPath(revision))
}

func applyRuntimeRecordLocked(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	if record.PreviousRevision != replay.Status.Revision {
		return rejectRuntimeRecord("predecessor_equal_replay_state")
	}
	if _, duplicate := replay.Requests[record.RequestID]; duplicate {
		return rejectRuntimeRecord("duplicate_request_identifier")
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return err
	}
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

	case runtimeOperationReset:
		event := record.Reset
		objective := replay.Status.Objective
		if replay.Status.ActiveAttempt != nil || objective == nil || !runtimeResetStructurallyPermitted(replay.Status) {
			return rejectRuntimeRecord("objective_reset_valid_successor")
		}
		if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
			event.PreviousGeneration != replay.Status.ObjectiveGeneration {
			return rejectRuntimeRecord("objective_reset_match_terminal")
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
	case runtimeOperationGrant:
		applyRuntimeGrantEvent(replay, record.Grant)
	default:
		return rejectRuntimeRecord("unsupported_operation")
	}
	replay.Status.Revision = revision
	replay.Requests[record.RequestID] = runtimeRequestReceipt{Digest: record.RequestDigest, Revision: revision}
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
		return rejectRuntimeRecord("begin_valid_successor")
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
			return rejectRuntimeRecord("initial_objective_identity_ordinal")
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
			return rejectRuntimeRecord("begin_changes_active_objective")
		}
		if runtimeObjectiveHasRecordedAttempt(replay.Status) {
			if event.BeginCandidateTree != replay.Status.Attempts[len(replay.Status.Attempts)-1].FinishCandidateTree {
				return rejectRuntimeRecord("begin_continue_terminal_candidate")
			}
		} else if event.BeginCandidateIdentity != objective.InitialCandidateIdentity || event.BeginCandidateTree != objective.InitialCandidateTree {
			// Mirrors write-time Begin's second dispatch branch: a freshly
			// rescoped objective has no attempt of its own to chase, so the
			// replayed candidate must instead match what Rescope itself
			// recorded as this objective's InitialCandidate* (#2298, #2296
			// part 2).
			return rejectRuntimeRecord("begin_continue_rescoped_objective")
		}
	}
	if replay.Status.CumulativeAttempts >= event.MaxAttempts || replay.Status.CumulativeChangedLines >= event.MaxChangedLines {
		return rejectRuntimeRecord("begin_exceeds_persisted_objective")
	}
	intendedUntracked := []string{}
	if event.IntendedUntracked != nil {
		intendedUntracked = slices.Clone(*event.IntendedUntracked)
	}
	attempt := RuntimeAttempt{
		Ordinal: event.Ordinal, ObjectiveID: event.ObjectiveID, ObjectiveGeneration: generation,
		WorkUnit: event.WorkUnit, BeginCandidateIdentity: event.BeginCandidateIdentity,
		BeginCandidateTree: event.BeginCandidateTree, IntendedUntracked: intendedUntracked, BeginWorktree: event.BeginWorktree,
		EligibleUntrackedInventory: runtimeOptionalString(event.EligibleUntrackedInventory),
		EffectiveWorktree:          event.EffectiveWorktree, Outcome: AttemptRunning,
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
		return rejectRuntimeRecord("handoff_match_active_attempt")
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
		return rejectRuntimeRecord("objective_rescope_valid_successor")
	}
	// The real narrowing guard runs FIRST and is recomputed against the
	// REPLAYED objective, never against the record's own (possibly forged)
	// PreviousMax* claim: a record cannot launder a widen by simply lying
	// about what the previous ceiling was, because this comparison never
	// reads event.PreviousMax* at all.
	if event.MaxAttempts > objective.MaxAttempts || event.MaxChangedLines > objective.MaxChangedLines {
		return rejectRuntimeRecord("objective_rescope_widens_current")
	}
	if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
		event.PreviousGeneration != replay.Status.ObjectiveGeneration ||
		event.PreviousMaxAttempts != objective.MaxAttempts || event.PreviousMaxChangedLines != objective.MaxChangedLines {
		return rejectRuntimeRecord("objective_rescope_match_terminal")
	}
	if len(replay.Status.Attempts) == 0 {
		return rejectRuntimeRecord("objective_rescope_no_terminal")
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
		return rejectRuntimeRecord("objective_rescope_candidate_match")
	}
	generation := event.ObjectiveGeneration
	expectedObjectiveID := runtimeObjectiveID(record.Change, event.WorkUnit, event.EvidenceGoal, event.RescopeCandidateIdentity, generation)
	if generation != replay.Status.ObjectiveGeneration+1 || event.ObjectiveID != expectedObjectiveID {
		return rejectRuntimeRecord("objective_rescope_identity_invalid")
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
		return rejectRuntimeRecord("consecutive_rescope_repair_restore")
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
		return rejectRuntimeRecord("objective_advance_valid_successor")
	}
	if event.PreviousObjectiveID != objective.ID || event.PreviousGeneration != objective.Generation ||
		event.PreviousGeneration != replay.Status.ObjectiveGeneration || event.PreviousWorkUnit != objective.WorkUnit {
		return rejectRuntimeRecord("objective_advance_match_terminal")
	}
	if len(replay.Status.Attempts) == 0 {
		return rejectRuntimeRecord("objective_advance_no_terminal")
	}
	last := replay.Status.Attempts[len(replay.Status.Attempts)-1]
	if last.ObjectiveID != objective.ID || last.Outcome != AttemptPassed || last.ChangedLineBudgetExceeded ||
		last.FinishCandidateIdentity == "" || last.FinishCandidateTree == "" {
		return rejectRuntimeRecord("objective_advance_follow_passed")
	}
	if record.Begin.WorkUnit == objective.WorkUnit {
		return rejectRuntimeRecord("objective_advance_select_distinct")
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

// runtimeChangedLineBudgetExceeded is the one owner of the changed-line budget
// decision. The writer stamps it onto the finish record and replay recomputes
// it to check the record did not lie about its own derived field; both must
// happen, and #2830 is what it costs when two copies of a rule disagree.
//
// Callers reach this only with an active attempt, which cannot exist without an
// objective, so the dereference matches what both inlined copies already did.
func runtimeChangedLineBudgetExceeded(status RuntimeStatus, changedLines int) bool {
	return status.CumulativeChangedLines+changedLines > status.Objective.MaxChangedLines
}

func applyRuntimeFinishEvent(replay *runtimeReplay, event *runtimeFinishEvent, unmanagedRemediation bool) error {
	active := replay.Status.ActiveAttempt
	if active == nil || active.Ordinal != event.Ordinal || len(replay.Status.Attempts) == 0 ||
		replay.Status.Attempts[len(replay.Status.Attempts)-1].Outcome != AttemptRunning {
		return rejectRuntimeRecord("finish_match_active_attempt")
	}
	budgetExceeded := runtimeChangedLineBudgetExceeded(replay.Status, event.ChangedLines)
	if event.ChangedLineBudgetExceeded != budgetExceeded {
		return rejectRuntimeRecord("finish_changed_line_budget")
	}
	if event.AttestedVerifyReportDigest != "" &&
		(event.Outcome != AttemptPassed || !isFinalVerifyWorkUnit(active.WorkUnit) || !runtimeRevisionPattern.MatchString(event.AttestedVerifyReportDigest)) {
		return rejectRuntimeRecord("finish_verify_report_attestation")
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
		// #3073 lockstep twin: replay decides the exact write-guard predicate;
		// the helper falls back to the begin comparison only for a legacy
		// failed record that carries no finish snapshot.
		unchangedCandidate := runtimeRemediationCandidateUnchanged(chainFailedAttempt, *active, event.FinishCandidateIdentity, event.FinishCandidateTree)
		// The binding must hold for every outcome; the changed-candidate and
		// fresh-evidence demands bind only the passing outcome, exactly as the
		// write-time guard in Finish decides (#3422). A truthful failed or
		// interrupted settlement discharges nothing, so it neither needs a
		// changed candidate nor fresh corrected evidence.
		bindingBroken := !chainHasFailedEvidence || chainFailedAttempt.EvidenceRevision != event.RemediatesEvidenceRevision
		passedDemandsBroken := event.Outcome == AttemptPassed &&
			((unchangedCandidate && !evidenceOnly) || event.EvidenceRevision == event.RemediatesEvidenceRevision)
		if bindingBroken || passedDemandsBroken {
			return rejectRuntimeRecord("unmanaged_remediation_finish_bind")
		}
	}
	attempt := &replay.Status.Attempts[len(replay.Status.Attempts)-1]
	attempt.FinishCandidateIdentity = event.FinishCandidateIdentity
	attempt.FinishCandidateTree = event.FinishCandidateTree
	attempt.AttestedVerifyReportDigest = event.AttestedVerifyReportDigest
	attempt.Outcome = event.Outcome
	attempt.ChangedLines = event.ChangedLines
	attempt.EvidenceRevision = event.EvidenceRevision
	attempt.Diagnosis = event.Diagnosis
	attempt.HarnessDisposition = event.HarnessDisposition
	attempt.CleanupEvidence = event.CleanupEvidence
	attempt.ProcessEvidence = event.ProcessEvidence
	attempt.RemediatesEvidenceRevision = event.RemediatesEvidenceRevision
	attempt.ChangedLineBudgetExceeded = event.ChangedLineBudgetExceeded
	// A rescope successor inherits its predecessor's recorded selection, so the
	// settled attempt must report the one it actually settled with, not the one
	// it began with (#3806).
	if event.IntendedUntracked != nil {
		attempt.IntendedUntracked = slices.Clone(*event.IntendedUntracked)
	}
	replay.Status.ActiveAttempt = nil
	replay.Status.CumulativeChangedLines += event.ChangedLines
	replay.Status.LifetimeChangedLines += event.ChangedLines
	// The objective budget refunds a call that advanced the unit, up to the
	// configured ceiling. LifetimeAttempts is never refunded, so the chain still
	// records every call that ran.
	if runtimeAttemptDeliveredIncrement(event.Outcome, event.ChangedLines) && replay.Status.CumulativeAttempts > 0 &&
		runtimeRefundedAttempts(replay.Status) <= replay.Status.Objective.MaxAttempts {
		replay.Status.CumulativeAttempts--
	}
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

// runtimeRefundedAttempts counts the settlements in the current objective that
// earned their call back, including the one being applied. It feeds the cap
// that keeps max_attempts a real bound: without it Begin's +1 and the refund's
// -1 cancel for every call that touches a line, so a partially productive
// stall runs until the changed-line cap and the human escalation max_attempts
// exists to trigger never fires. An objective earns back at most MaxAttempts
// calls, so it spends at most twice what the operator configured.
func runtimeRefundedAttempts(status RuntimeStatus) int {
	if status.Objective == nil {
		return 0
	}
	refunded := 0
	for _, attempt := range status.Attempts {
		if attempt.ObjectiveID == status.Objective.ID && attempt.ObjectiveGeneration == status.Objective.Generation &&
			runtimeAttemptDeliveredIncrement(attempt.Outcome, attempt.ChangedLines) {
			refunded++
		}
	}
	return refunded
}

// runtimeAttemptDeliveredIncrement reports whether a settlement earned back the
// call it spent. #3815: RuntimeAttempt was one provider call, one unit of
// budget and one unit of work at once, so a work unit that legitimately needs
// several calls exhausted its objective by accounting rather than by failure —
// #3808, where two calls delivered zero production and ended at
// decision_required.
//
// An interrupted call that left measurable increment advanced the unit, so it
// does not discharge an attempt against the objective. A call that delivered
// nothing is still spent, which is what keeps max_attempts bounding calls that
// produce nothing. The refund cannot run away: earning one costs delivered
// lines, and cumulative changed lines remain capped by the objective.
func runtimeAttemptDeliveredIncrement(outcome AttemptOutcome, changedLines int) bool {
	return outcome == AttemptInterrupted && changedLines > 0
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

func runtimeOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateRuntimeBeginEvent(record runtimeRecord) error {
	event := record.Begin
	if event.EligibleUntrackedInventory != nil && !runtimeRevisionPattern.MatchString(*event.EligibleUntrackedInventory) {
		return rejectRuntimeRecord("invalid_eligible_untracked_inventory")
	}
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
		return rejectRuntimeRecord("invalid_begin_event")
	}
	if event.IntendedUntracked != nil {
		canonical, err := canonicalRuntimeIntendedUntracked(*event.IntendedUntracked)
		if err != nil || !slices.Equal(canonical, *event.IntendedUntracked) {
			return rejectRuntimeRecord("invalid_intended_untracked_provenance")
		}
	}
	var intendedUntracked []string
	if event.IntendedUntracked != nil {
		intendedUntracked = *event.IntendedUntracked
	}
	request := BeginAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: event.WorkUnit,
		EvidenceGoal: event.EvidenceGoal, MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
		IntendedUntracked: intendedUntracked,
	}
	legacy := legacyBeginAttemptRequest{
		ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, WorkUnit: event.WorkUnit,
		EvidenceGoal: event.EvidenceGoal, MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
	}
	if runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", request) != record.RequestDigest &&
		(event.IntendedUntracked != nil || runtimeValueHash("gentle-ai.sdd-runtime-begin-request/v1", legacy) != record.RequestDigest) {
		return rejectRuntimeRecord("begin_request_digest_match")
	}
	return nil
}

func validateRuntimeRecordShape(record runtimeRecord) error {
	if record.Schema != runtimeRecordSchema || !validRuntimeChange(record.Change) ||
		(record.PreviousRevision != "" && !runtimeRevisionPattern.MatchString(record.PreviousRevision)) ||
		!runtimeRequestIDPattern.MatchString(record.RequestID) || !runtimeRevisionPattern.MatchString(record.RequestDigest) {
		return rejectRuntimeRecord("invalid_identity")
	}
	if record.Operation != runtimeOperationRepairConsecutiveRescope && record.Repair != nil {
		return rejectRuntimeRecord("unexpected_repair_event")
	}
	switch record.Operation {
	case runtimeOperationBegin:
		if record.Begin == nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_begin_shape")
		}
		if err := validateRuntimeBeginEvent(record); err != nil {
			return err
		}
	case runtimeOperationAdvance:
		if record.Begin == nil || record.Advance == nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_objective_advance_shape")
		}
		advance := record.Advance
		if !runtimeRevisionPattern.MatchString(advance.PreviousObjectiveID) || advance.PreviousGeneration < 1 ||
			validateRuntimeText(advance.PreviousWorkUnit, 160) != nil || advance.PreviousWorkUnit == record.Begin.WorkUnit {
			return rejectRuntimeRecord("invalid_objective_advance_event")
		}
		// The successor carries an ordinary begin request, so its digest binds
		// the same caller-visible request an ordinary begin would have bound.
		if err := validateRuntimeBeginEvent(record); err != nil {
			return err
		}
	case runtimeOperationFinish:
		if record.Finish == nil || record.Begin != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_finish_shape")
		}
		event := record.Finish
		if event.Ordinal < 1 || !validTerminalAttemptOutcome(event.Outcome) || event.ChangedLines < 0 ||
			event.ChangedLines > maximumRuntimeChangedLines ||
			((event.Outcome == AttemptInterrupted && event.EvidenceRevision != "" && !runtimeRevisionPattern.MatchString(event.EvidenceRevision)) ||
				(event.Outcome != AttemptInterrupted && !runtimeRevisionPattern.MatchString(event.EvidenceRevision))) ||
			!runtimeRevisionPattern.MatchString(event.FinishCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.FinishCandidateTree) ||
			validateRuntimeText(event.Diagnosis, 500) != nil || !validHarnessDisposition(event.HarnessDisposition) ||
			validateRuntimeText(event.CleanupEvidence, 500) != nil || validateRuntimeText(event.ProcessEvidence, 500) != nil ||
			(event.RemediatesEvidenceRevision != "" && !runtimeRevisionPattern.MatchString(event.RemediatesEvidenceRevision)) ||
			(event.AttestedVerifyReportDigest != "" && (!runtimeRevisionPattern.MatchString(event.AttestedVerifyReportDigest) || event.Outcome != AttemptPassed)) {
			return rejectRuntimeRecord("invalid_finish_event")
		}
		request := FinishAttemptRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Outcome: event.Outcome,
			EvidenceRevision: event.EvidenceRevision, Diagnosis: event.Diagnosis, HarnessDisposition: event.HarnessDisposition,
			CleanupEvidence: event.CleanupEvidence, ProcessEvidence: event.ProcessEvidence,
			RemediatesEvidenceRevision: event.RemediatesEvidenceRevision,
			ExpectedUntrackedInventory: event.DeclaredUntrackedInventory,
		}
		// The event records the selection this settlement used; the request
		// carried one only when the caller declared, which is exactly when the
		// event carries the digest they declared against.
		if event.DeclaredUntrackedInventory != "" {
			request.IntendedUntracked = event.IntendedUntracked
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-finish-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("finish_request_digest_match")
		}
	case runtimeOperationHandoff:
		if record.Handoff == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_handoff_shape")
		}
		event := record.Handoff
		if event.Ordinal < 1 || event.SourceWorktree == event.DestinationWorktree ||
			validateRuntimeText(event.SourceWorktree, 4096) != nil || !filepath.IsAbs(event.SourceWorktree) ||
			validateRuntimeText(event.DestinationWorktree, 4096) != nil || !filepath.IsAbs(event.DestinationWorktree) ||
			validateRuntimeText(event.CommonDir, 4096) != nil || !filepath.IsAbs(event.CommonDir) ||
			event.ExpectedRevision != record.PreviousRevision || event.RequestDigest != record.RequestDigest ||
			!runtimeRevisionPattern.MatchString(event.DestinationCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.DestinationCandidateTree) {
			return rejectRuntimeRecord("invalid_handoff_event")
		}
		request := HandoffAttemptRequest{ExpectedRevision: event.ExpectedRevision, RequestID: record.RequestID, DestinationWorktree: event.DestinationWorktree}
		if runtimeValueHash("gentle-ai.sdd-runtime-handoff-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("handoff_request_digest_match")
		}
	case runtimeOperationReset:
		if record.Reset == nil || record.Begin != nil || record.Finish != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_reset_shape")
		}
		event := record.Reset
		if !runtimeRevisionPattern.MatchString(event.PreviousObjectiveID) || event.PreviousGeneration < 1 ||
			!runtimeRevisionPattern.MatchString(event.ResetCandidateIdentity) || !runtimeGitTreePattern.MatchString(event.ResetCandidateTree) ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return rejectRuntimeRecord("invalid_reset_event")
		}
		request := ResetObjectiveRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID, Reason: event.Reason, Actor: event.Actor,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-reset-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("reset_request_digest_match")
		}
	case runtimeOperationRescope:
		if record.Rescope == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Advance != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_rescope_shape")
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
			return rejectRuntimeRecord("invalid_rescope_event")
		}
		request := RescopeObjectiveRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID,
			WorkUnit: event.WorkUnit, EvidenceGoal: event.EvidenceGoal,
			MaxAttempts: event.MaxAttempts, MaxChangedLines: event.MaxChangedLines,
			Reason: event.Reason, Actor: event.Actor,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-rescope-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("rescope_request_digest_match")
		}
	case runtimeOperationRepairConsecutiveRescope:
		if record.Repair == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil || record.Grant != nil {
			return rejectRuntimeRecord("invalid_consecutive_rescope_repair")
		}
		event := record.Repair
		if !runtimeRevisionPattern.MatchString(event.ReplacedRevision) || !runtimeRevisionPattern.MatchString(event.RestoredRevision) ||
			event.RestoredRevision != record.PreviousRevision || validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return rejectRuntimeRecord("invalid_consecutive_rescope_repair_2")
		}
		request := RepairConsecutiveRescopeRequest{ExpectedRevision: event.ReplacedRevision, RequestID: record.RequestID, Reason: event.Reason, Actor: event.Actor}
		if runtimeValueHash("gentle-ai.sdd-runtime-repair-consecutive-rescope-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("consecutive_rescope_repair_request")
		}
	case runtimeOperationGrant:
		if record.Grant == nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Advance != nil || record.Handoff != nil {
			return rejectRuntimeRecord("invalid_grant_shape")
		}
		event := record.Grant
		if len(event.Roots) < 1 || len(event.Roots) > maximumRuntimeGrantRoots ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return rejectRuntimeRecord("invalid_grant_event")
		}
		if event.Instance == "" || validateRuntimeText(event.Instance, 128) != nil {
			return rejectRuntimeRecord("invalid_grant_change_instance")
		}
		seen := make(map[string]struct{}, len(event.Roots))
		for _, root := range event.Roots {
			if validateRuntimeText(root, 4096) != nil || !filepath.IsAbs(root) {
				return rejectRuntimeRecord("invalid_grant_root")
			}
			if _, duplicate := seen[root]; duplicate {
				return rejectRuntimeRecord("duplicate_grant_root")
			}
			seen[root] = struct{}{}
		}
		// GrantedAt is the ledger's first wall-clock field: validated for
		// parseability only, never recomputed or compared against a clock, so
		// it stays excluded from determinism-replay expectations.
		if _, err := time.Parse(time.RFC3339Nano, event.GrantedAt); err != nil {
			return rejectRuntimeRecord("invalid_grant_timestamp")
		}
		request := GrantRootsRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID,
			Roots: event.Roots, Reason: event.Reason, Actor: event.Actor,
			ChangeInstance: event.Instance,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("grant_request_digest_match")
		}
	default:
		return rejectRuntimeRecord("invalid_operation")
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
	if request.MaxAttempts < 1 || request.MaxAttempts > maximumRuntimeAttemptLimit {
		return BeginAttemptRequest{}, fmt.Errorf("max_attempts must be within 1..%d", maximumRuntimeAttemptLimit)
	}
	if request.MaxChangedLines < 1 || request.MaxChangedLines > maximumRuntimeChangedLines {
		return BeginAttemptRequest{}, fmt.Errorf("max_changed_lines must be within 1..%d", maximumRuntimeChangedLines)
	}
	intended, err := canonicalRuntimeIntendedUntracked(request.IntendedUntracked)
	if err != nil {
		return BeginAttemptRequest{}, err
	}
	request.IntendedUntracked = intended
	return request, nil
}

func canonicalRuntimeIntendedUntracked(paths []string) ([]string, error) {
	canonical := slices.Clone(paths)
	if canonical == nil {
		return []string{}, nil
	}
	if len(canonical) > maximumRuntimeIntendedUntracked {
		return nil, fmt.Errorf("intended_untracked must name at most %d paths; rerun `gentle-ai sdd-attempt acquire` or `gentle-ai sdd-attempt begin` with an inventory-validated selection", maximumRuntimeIntendedUntracked)
	}
	slices.Sort(canonical)
	for index, path := range canonical {
		if validateRuntimeText(path, 4096) != nil || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path ||
			path == "." || path == ".." || strings.HasPrefix(path, "../") || (index > 0 && path == canonical[index-1]) {
			return nil, errors.New("intended_untracked must be canonical unique repository-relative paths; rerun `gentle-ai sdd-attempt acquire` or `gentle-ai sdd-attempt begin` with the inventory-validated --untracked-scope and --intended-untracked flags")
		}
	}
	return canonical, nil
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
	if request.Outcome == AttemptInterrupted && request.EvidenceRevision != "" && !runtimeRevisionPattern.MatchString(request.EvidenceRevision) {
		return FinishAttemptRequest{}, errors.New("interrupted evidence_revision must be empty or a canonical legacy sha256 revision; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --outcome interrupted and without --evidence-revision")
	}
	if request.Outcome != AttemptInterrupted && !runtimeRevisionPattern.MatchString(request.EvidenceRevision) {
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
	if (request.IntendedUntracked == nil) != (request.ExpectedUntrackedInventory == "") {
		return FinishAttemptRequest{}, errors.New("an untracked declaration needs both its selection and the inventory digest it was made against; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with --untracked-scope and --expected-untracked-inventory together")
	}
	if request.ExpectedUntrackedInventory != "" && !runtimeRevisionPattern.MatchString(request.ExpectedUntrackedInventory) {
		return FinishAttemptRequest{}, errors.New("expected_untracked_inventory must be sha256:<64-lowercase-hex>; rerun `gentle-ai sdd-attempt finish` or `gentle-ai sdd-attempt settle` with the digest `gentle-ai review status --next-transition` publishes")
	}
	if request.IntendedUntracked != nil {
		canonical, canonicalErr := canonicalRuntimeIntendedUntracked(*request.IntendedUntracked)
		if canonicalErr != nil {
			return FinishAttemptRequest{}, canonicalErr
		}
		request.IntendedUntracked = &canonical
	}
	if request.RemediatesEvidenceRevision != "" {
		// Every outcome is a truthful settlement of a declared correction
		// (#3422): passed discharges the failure it names, failed records the
		// correction's own new failure as the chain's bindable head, and
		// interrupted discharges nothing. Only the binding shape is validated
		// here; outcome-specific demands live in Finish and its replay twin.
		if !runtimeRevisionPattern.MatchString(request.RemediatesEvidenceRevision) {
			return FinishAttemptRequest{}, fmt.Errorf(
				"remediates_evidence_revision must be sha256:<64-lowercase-hex> (%s); rerun `gentle-ai sdd-attempt finish` with --remediates-evidence-revision sha256:<64-lowercase-hex>",
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
		return GrantRootsRequest{}, fmt.Errorf("grant requires between 1 and %d roots", maximumRuntimeGrantRoots) // refusal:by-design operator-knowledge: only the caller knows which edit roots this change needs; retry with a bounded non-empty root list
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

// runtimeReplayedIntendedUntracked reconciles a HISTORICAL intended-untracked
// selection against the repository's current index before it is replayed into
// a candidate capture (#3842). The ledger records the selection at
// acquire/begin time, but the user legitimately commits the selected paths as
// the ordinary end of a work unit — sometimes as the work unit itself — and a
// verbatim replay of the recorded list then trips the snapshot builder's
// "already tracked" refusal on every later capture: reset, rescope, settle,
// handoff, and the read-only admissibility probes all dead-end. A path that
// became tracked is already part of the ordinary candidate (its bytes live in
// HEAD/index/worktree), so dropping it from the overlay keeps the candidate
// tree byte-identical: a bare landing of the selection replays as zero drift,
// and any further edit reads as ordinary candidate drift — exactly the
// distinction the reset/rescope split already routes on. Only replayed
// history passes through here: fresh caller-supplied selections stay strict,
// and the immutable records themselves keep the selection exactly as
// acquired.
func runtimeReplayedIntendedUntracked(ctx context.Context, repo string, recorded []string) ([]string, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	return builder.StillUntrackedIntended(ctx, recorded)
}

func captureRuntimeCandidate(ctx context.Context, repo string, intendedUntracked []string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace,
		IntendedUntracked: intendedUntracked,
	})
}

// captureRuntimeTerminalCandidate rebuilds the current workspace candidate
// overlaid on the attempt's begin candidate tree, the same computation Begin
// and Reset both use to detect whether the candidate drifted out from under
// a terminal (no active attempt) objective scope.
func captureRuntimeTerminalCandidate(ctx context.Context, store RuntimeStore, beginCandidateTree string, intendedUntracked []string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: store.Repo}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: beginCandidateTree,
		Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intendedUntracked,
	})
}

func captureRuntimeHandoffCandidate(ctx context.Context, worktree, beginCandidateTree string, intendedUntracked []string) (reviewtransaction.Snapshot, error) {
	builder := reviewtransaction.SnapshotBuilder{Repo: worktree}
	return builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetBaseWorkspaceOverlay, BaseRef: beginCandidateTree,
		Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intendedUntracked,
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
	if bound, missing := runtimeBoundWorktree(active); missing {
		return store.runtimeMissingWorktreeRefusal(ErrRuntimeHandoffSource, active, bound)
	}
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

// runtimeRemediationCandidateUnchanged judges whether a settling unmanaged
// remediation still presents the state that FAILED (#3073). The laundering
// baseline is the remediated failed evidence's candidate snapshot — the exact
// bytes the failure was recorded over — not the correction attempt's begin
// snapshot: a correction applied between the audited reset and the acquire is
// already inside the begin snapshot, so judging against begin refused a
// genuinely changed candidate, while a revert to the failed bytes after
// acquire counted as "changed" against begin despite re-presenting exactly
// what failed. Failed attempts recorded before the finish candidate snapshot
// existed carry no baseline of their own, so those legacy records fall back
// to the pre-#3073 begin-relative comparison.
func runtimeRemediationCandidateUnchanged(failed, active RuntimeAttempt, identity, tree string) bool {
	baselineIdentity, baselineTree := failed.FinishCandidateIdentity, failed.FinishCandidateTree
	if baselineIdentity == "" && baselineTree == "" {
		baselineIdentity, baselineTree = active.BeginCandidateIdentity, active.BeginCandidateTree
	}
	return identity == baselineIdentity || tree == baselineTree
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
	path := store.recordPath(revision)
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

// recordPath is the one derivation of a record's file under the Git common
// directory: <common>/gentle-ai/sdd-runtime/v1/<change>/records/<sha256>.json.
func (store RuntimeStore) recordPath(revision string) string {
	return filepath.Join(store.Dir, "records", strings.TrimPrefix(revision, "sha256:")+".json")
}

func (store RuntimeStore) loadRecord(revision string) (runtimeRecord, error) {
	if !runtimeRevisionPattern.MatchString(revision) {
		return runtimeRecord{}, errors.New("invalid SDD runtime record revision")
	}
	path := store.recordPath(revision)
	payload, err := readBoundedRuntimeFile(path)
	if err != nil {
		return runtimeRecord{}, fmt.Errorf("load SDD runtime revision %s: %w", revision, err)
	}
	sum := sha256.Sum256(payload)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != revision {
		return runtimeRecord{}, fmt.Errorf("SDD runtime record revision mismatch: expected %s, got %s", revision, actual)
	}
	// #2702: unknown fields are tolerated, not refused. The sha256 revision
	// above already pins the bytes, so a strict decode added no integrity; it
	// only made a record with one additive field from a newer binary
	// unreadable by every operation, reset included. The trade-off is that an
	// older binary ignores fields it does not understand. A record whose
	// schema version is newer than this binary's stays a typed refusal.
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var record runtimeRecord
	if err := decoder.Decode(&record); err != nil {
		return runtimeRecord{}, fmt.Errorf("decode SDD runtime revision %s: %w", revision, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return runtimeRecord{}, errors.New("SDD runtime record contains multiple JSON values")
	}
	if runtimeRecordSchemaIsNewer(record.Schema) {
		return runtimeRecord{}, &RuntimeRecordSchemaUnsupportedError{Revision: revision, Schema: record.Schema}
	}
	_, canonical, err := runtimeRecordRevision(record)
	if err != nil || !runtimeRecordPayloadCanonical(payload, canonical) {
		return runtimeRecord{}, errors.New("SDD runtime record is not canonical")
	}
	if record.Change != store.Change {
		return runtimeRecord{}, errors.New("SDD runtime record change does not match store")
	}
	return record, nil
}

// runtimeRecordPayloadCanonical accepts the exact canonical encoding, or a
// compact encoding whose every known top-level field is byte-identical to the
// canonical one and which carries only additive unknown fields (#2702).
func runtimeRecordPayloadCanonical(payload, canonical []byte) bool {
	if bytes.Equal(payload, canonical) {
		return true
	}
	var actual, expected map[string]json.RawMessage
	if json.Unmarshal(payload, &actual) != nil || json.Unmarshal(canonical, &expected) != nil || len(actual) <= len(expected) {
		return false
	}
	for key, value := range expected {
		if !bytes.Equal(actual[key], value) {
			return false
		}
	}
	var compact bytes.Buffer
	if json.Compact(&compact, payload) != nil {
		return false
	}
	compact.WriteByte('\n')
	return bytes.Equal(compact.Bytes(), payload)
}

// runtimeRecordSchemaIsNewer reports whether a record declares a later
// version of the runtime record schema than this binary supports.
func runtimeRecordSchemaIsNewer(schema string) bool {
	prefix := runtimeRecordSchema[:strings.LastIndex(runtimeRecordSchema, "/v")+2]
	supported, err := strconv.Atoi(strings.TrimPrefix(runtimeRecordSchema, prefix))
	if err != nil || !strings.HasPrefix(schema, prefix) {
		return false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(schema, prefix))
	return err == nil && version > supported
}

// RuntimeRecordSchemaUnsupportedError refuses a runtime record written under
// a schema version newer than this binary supports.
type RuntimeRecordSchemaUnsupportedError struct {
	Revision string
	Schema   string
}

func (err *RuntimeRecordSchemaUnsupportedError) Error() string {
	return fmt.Sprintf("SDD runtime revision %s declares \"schema\" %s, newer than this binary supports (%s); run `gentle-ai update` to install a build that reads it, then rerun the same `gentle-ai sdd-attempt` command", err.Revision, err.Schema, runtimeRecordSchema)
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
