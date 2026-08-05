// Package reviewtransaction — ReviewCore transition vocabulary (Wave 3
// Slice 3, design decision 1). This file declares the pure data shapes
// ReviewCore.Next consumes and produces; review_core.go owns the decision
// logic itself. Kept separate so the transition vocabulary — the contract a
// future caller (a facade branch, a gate, a bench journey) programs against
// — is readable without the consent/tier/replay decision tree around it.
package reviewtransaction

// CoreRequestKind names which of ReviewCore's three owned operations a
// CoreRequest describes (spec rdd-review-core-transitions, "Sole Transition
// Owner for New Lineages": only start, finalize, or validate exist).
type CoreRequestKind string

const (
	CoreRequestStart    CoreRequestKind = "start"
	CoreRequestFinalize CoreRequestKind = "finalize"
	CoreRequestValidate CoreRequestKind = "validate"
)

// CoreTransitionKind is the design's literal five-plus-one transition
// vocabulary (design decision 1): continue (freeze/advance without a
// terminal outcome yet), collect (a bounded correction is being asked for),
// approve/escalate (a terminal state was reached), repair (an escalated
// lineage has a bounded reopening path), stop (the request is refused with
// no state mutation).
type CoreTransitionKind string

const (
	CoreTransitionContinue CoreTransitionKind = "continue"
	CoreTransitionCollect  CoreTransitionKind = "collect"
	CoreTransitionApprove  CoreTransitionKind = "approve"
	CoreTransitionEscalate CoreTransitionKind = "escalate"
	CoreTransitionRepair   CoreTransitionKind = "repair"
	CoreTransitionStop     CoreTransitionKind = "stop"
)

// CoreInput names one thing a `collect` transition asks its caller for —
// today exactly the bounded correction itself; the shape is a slice so a
// future collect need never widen CoreTransition's own fields.
type CoreInput struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ReceiptRef is what a terminal (`approve`/`escalate`) transition hands back
// so the caller can bind AuthorityStore.WriteReceipt to the exact authority
// revision ReviewCore just committed — never a re-read, never re-derived.
type ReceiptRef struct {
	LineageID         string `json:"lineage_id"`
	AuthorityRevision string `json:"authority_revision"`
}

// CoreTransition is Next's exact return shape (design's literal Interfaces /
// Contracts sketch), extended with Authority: the sketch names Collect and
// Receipt for the collect/terminal cases, but a `continue` transition that
// creates or advances an authority has nothing to hand its caller without
// this field — Mutate's apply callback needs the frozen NewLineageAuthority
// verbatim, not a re-derivation of what Next just decided.
type CoreTransition struct {
	Kind       CoreTransitionKind
	ReasonCode string
	Collect    []CoreInput
	Receipt    *ReceiptRef
	Authority  *NewLineageAuthority
}

// CoreRequest is Next's exact request shape. Start-only fields freeze a new
// authority; ConsentGranted is never re-derived by ReviewCore — it is the
// caller's already-computed outcome of the reused authorizeReviewStart / v2
// consent envelope (design decision 6), and Next's only job with it is to
// enforce the ordering invariant (freeze only after it is true, unless tier
// 0's carve-out applies). Next never calls NewLineageAuthority.RecordTransition
// itself: LineageID is not yet known at start (AuthorityStore.Mutate assigns
// it from the store, after apply returns — Wave 3 Slice 2), so recording a
// transition's replay identity is the caller's job, done inside the Mutate
// apply closure once the lineage id is set.
type CoreRequest struct {
	Kind CoreRequestKind

	CandidateIdentity    CandidateIdentity
	Tier                 RiskLevel
	SelectedLenses       []string
	OriginalChangedLines int
	ConsentGranted       bool

	// LiveCandidateIdentity and Evidence are validate-only: the live
	// candidate being compared against the frozen authority, and the full
	// context relateCandidates needs beyond the two identities themselves.
	LiveCandidateIdentity CandidateIdentity
	Evidence              CoreValidateEvidence

	// AdvanceRequest is finalize-only, and deliberately opt-in (nil by
	// default). C6 remediation (verify-report CRITICAL, "ReviewCore-owned
	// transitions in finalize"): design's own finalize data flow ("admitted
	// candidate_causal? -> correcting -> one bounded correction -> validating
	// -> approved (receipt)") is finalize's decision to make, not its
	// caller's -- a caller that directly set NewLineageState on a Mutate
	// apply closure (the bug this field fixes) was deciding the transition
	// itself, contradicting "Sole Transition Owner for New Lineages". A nil
	// AdvanceRequest preserves finalize's original, still-pinned invariant
	// (TestReviewCoreFinalizeRequiresTerminalState: a bare finalize request
	// against a non-terminal authority refuses with
	// ErrFinalizeRequiresTerminalState) for every existing caller that never
	// opts in. A caller that DOES have a decision to make (currently only
	// runReviewFacadeFinalizeNewLineage) supplies it here; finalize alone
	// decides which terminal state it produces and returns the resulting
	// NewLineageAuthority as CoreTransition.Authority, mirroring start's own
	// "Next decides, Mutate's apply stores *transition.Authority verbatim"
	// contract (review_facade_new_lineage.go:73-78) -- zero state decisions
	// left in the CLI layer.
	AdvanceRequest *FinalizeAdvanceRequest
}

// FinalizeAdvanceRequest is finalize's decision context for a non-terminal
// authority (reviewing, correcting, or validating): Failed and
// AdmittedFindingIDs are the caller's already-computed classification
// (AdmitCandidateCausalFindings) -- finalize decides WHICH terminal state
// they produce (escalated when either is non-empty/true, approved
// otherwise) and whether AdmittedFindingIDs get appended to the resulting
// authority; it never re-derives the classification itself. Supplying this
// also reopens a `correcting` lineage's own finalize path (spec
// rdd-review-core-transitions' "In-flight new lineage still finalizes after
// rollback" scenario, whose GIVEN is literally a `correcting` lineage) --
// finalize previously refused `correcting` outright with no path forward.
// CapturedLensResults names the lenses that actually produced a captured
// result for this finalize call (absorbed N1, W3/W4 verify: "ReviewCore.
// finalize reads LensResults for the first time"). finalize refuses to
// approve when authority.SelectedLenses is non-empty and this slice does not
// cover every selected lens — closing the self-approval hole where a
// medium/high tier candidate reached `approved` with zero lens reviews ever
// performed. Escalating (Failed or AdmittedFindingIDs non-empty) never
// checks this: an escalation never claims a clean pass, so the
// self-approval precondition does not apply.
type FinalizeAdvanceRequest struct {
	Failed              bool
	AdmittedFindingIDs  []string
	CapturedLensResults []string
}

// CoreValidateEvidence carries every already-resolved input relateCandidates
// needs beyond the frozen/live CandidateIdentity pair: Snapshot boundaries,
// the delegated base-advance proof, admitted-finding scope, and ambiguity
// count. This is deliberately NOT the design's literal two-argument
// DeriveObservation(authority, live) sketch: Wave 3 Slice 2 discovered that
// signature cannot correctly call relateCandidates, because relateCandidates
// needs shadowRelationInput's full Snapshot/BaseAdvance/admitted-finding
// context, and deferred DeriveObservation to this slice rather than
// fabricate that context to satisfy the literal signature (Slice 2
// apply-progress). ReviewCore.validate is the first caller with this full
// context, so this slice corrects the signature instead of faking it.
type CoreValidateEvidence struct {
	FrozenSnapshot        Snapshot
	LiveSnapshot          Snapshot
	GenesisPaths          []string
	BaseAdvance           *BaseAdvanceCompatibility
	AdmittedFindingPaths  []string
	AdmittedPathsKnown    bool
	ApplicableAuthorities int
	LiveUnresolvable      bool
}

// DerivedObservation is the read-time-only classification a validator
// evaluates a frozen authority against. Nothing in this package ever
// persists it: NewLineageState's closed five-value vocabulary (spec
// rdd-authority-store, "Five Persisted States, Everything Else Derived")
// structurally excludes it, and this type lives outside NewLineageAuthority
// entirely.
type DerivedObservation struct {
	Relation CandidateRelation
}

// DeriveObservation computes the derived candidate relation from the frozen
// authority and the live candidate, reusing relateCandidates verbatim — no
// re-derivation of the seven-value relation algebra (design decision 6).
// The result is never persisted (design decision 3); ReviewCore.validate is
// its only caller.
func DeriveObservation(authority NewLineageAuthority, live CandidateIdentity, evidence CoreValidateEvidence) DerivedObservation {
	return DerivedObservation{
		Relation: relateCandidates(shadowRelationInput{
			Frozen: authority.CandidateIdentity, Live: live,
			FrozenSnapshot: evidence.FrozenSnapshot, LiveSnapshot: evidence.LiveSnapshot,
			GenesisPaths: evidence.GenesisPaths, BaseAdvance: evidence.BaseAdvance,
			AdmittedFindingPaths: evidence.AdmittedFindingPaths, AdmittedPathsKnown: evidence.AdmittedPathsKnown,
			ApplicableAuthorities: evidence.ApplicableAuthorities, LiveUnresolvable: evidence.LiveUnresolvable,
		}),
	}
}
