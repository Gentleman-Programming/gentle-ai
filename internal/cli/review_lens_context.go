package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewLensContextTimeout bounds the whole assembly, not one read. The surface
// performs two discovery reads plus one patch read per changed path, and a
// reviewer that never launches is the correct outcome when the repository stops
// answering partway through.
const reviewLensContextTimeout = 120 * time.Second

// reviewLensContextByteBudget bounds the immutable candidate evidence this
// surface materializes for one lens. It is exactly the native per-command cap
// every other immutable-diff read in this product already accepts
// (reviewtransaction.MaxFrozenCandidateDiffBytes), rather than a fraction of
// it: a smaller budget would refuse candidates whose risk-tier-counted changed
// lines are small but whose manifest includes large regenerated or golden
// files, which this surface materializes in full for every manifest path.
//
// It is enforced by outright refusal. Truncating would hand a reviewer a
// partial view of the candidate while still letting it report a clean result,
// which is the one failure this surface exists to make impossible.
//
// It is also the *only* bound this surface applies. A separate 32-entry cap
// used to sit beside it, inherited unchanged from the deleted advisory-review
// adapter's request validator, where it carried no rationale of its own. A
// count is not a measure of what a reviewer's context costs: it refused 33
// one-line files whose complete evidence was a few kilobytes while admitting
// 32 files that filled this entire budget. Counting entries also made the
// refusal unfixable rather than merely large, because path count is the one
// property splitting a candidate barely changes (issue #3367).
const reviewLensContextByteBudget = reviewtransaction.MaxFrozenCandidateDiffBytes

// The two markers an installed agent definition also names are read from the
// canonical constants both halves share, never respelled here: a reviewer
// admits the block by marker name, so a second spelling on either side is the
// whole of issue #2777.
const (
	reviewLensContextBindingHeader = reviewtransaction.ReviewerBindingMarker
	reviewLensContextContextHeader = reviewtransaction.ReviewerContextMarker
	reviewLensContextTerminator    = reviewtransaction.ReviewerContextTerminator
	reviewLensContextNameStatus    = "GENTLE_AI_REVIEW_NAME_STATUS"
	reviewLensContextNumstat       = "GENTLE_AI_REVIEW_NUMSTAT"
	reviewLensContextInstruction   = "GENTLE_AI_REVIEW_INSTRUCTION"
	reviewLensContextResultSchema  = "GENTLE_AI_REVIEW_RESULT_SCHEMA"
	reviewLensContextPatch         = "GENTLE_AI_REVIEW_PATCH"
)

// reviewLensContextBinding is the machine data a relaying orchestrator used to
// be asked to author by hand. Every field is provider-derived, and `order` is
// an int here so it can never reach a reviewer as the string "0".
type reviewLensContextBinding struct {
	Lineage           string `json:"lineage"`
	Target            string `json:"target"`
	Lens              string `json:"lens"`
	Order             int    `json:"order"`
	Revision          string `json:"revision"`
	RepositoryContext string `json:"repository_context"`
	SubjectHash       string `json:"subject_hash"`
}

// reviewLensContextError is a typed, path-free refusal. Code is the first token
// so a caller branches on it without parsing prose, and Action always names
// something the caller can actually carry out.
type reviewLensContextError struct {
	Code   string
	Action string
}

func (err *reviewLensContextError) Error() string {
	return fmt.Sprintf("%s: provider-owned reviewer lens context was not produced; %s", err.Code, err.Action)
}

func reviewLensContextRefusal(code, action string) error {
	return reviewPreflightError(&reviewLensContextError{Code: code, Action: action})
}

const (
	reviewLensContextRefreshAction = "ask the parent orchestrator to refresh the exact native next transition by running " +
		reviewNextTransitionRefreshCommandV21 + ", then run this operation again with the tokens it returns"
	reviewLensContextBudgetAction = "immutable candidate evidence is never truncated and retrying this candidate cannot succeed; " +
		"split the candidate into a chained sequence of smaller reviewable commits, each under the budget, then refresh the exact native next transition by running " +
		reviewNextTransitionRefreshCommandV21 + " and execute the returned transition for the reduced scope"
	reviewLensContextStartBudgetAction = "no review authority was created, so nothing has to be abandoned or repaired; " +
		"immutable candidate evidence is never truncated and retrying this exact candidate cannot succeed, so split it into a chained sequence of smaller reviewable commits, each under the budget, " +
		"then refresh the exact native next transition by running " + reviewNextTransitionRefreshCommandV21 +
		" and execute the returned transition for the reduced scope"
	reviewLensContextEmptyPatchAction = "one content-changing path produced no patch bytes at all, which no legitimate candidate does; " +
		"refresh the exact native next transition by running " + reviewNextTransitionRefreshCommandV21 +
		" and run this operation again, and if the same path keeps producing no patch treat it as a native inspection defect and stop retrying"
	reviewLensContextDeadlineAction = "refresh the exact native next transition by running " + reviewNextTransitionRefreshCommandV21 +
		", then execute the returned transition once; if the same lens slot reaches the same deadline again, an identical retry cannot change its frozen scope or deadline, so split the candidate into a chained sequence of smaller reviewable commits, then refresh the exact native next transition by running " + reviewNextTransitionRefreshCommandV21 + " and execute the returned transition for the reduced scope"
	reviewLensContextConflictAction = "this frozen lens slot already recorded a reviewer context produced by a different mechanism, and audit history is never rewritten; " +
		"produce this lens context by the same mechanism that already recorded one, or start a review for a fresh candidate by running " +
		reviewNextTransitionRefreshCommandV21
)

// reviewLensContextConflictActionFor names the mechanism the slot really
// recorded, so the exit is runnable without guessing which one it was.
//
// The generic wording is kept for the case where the record cannot be read
// back: naming a mechanism this code did not actually observe would be worse
// than staying general, because an exit that does not work sends an operator in
// circles blaming themselves.
func reviewLensContextConflictActionFor(recorded reviewtransaction.ReviewerContextLevel) string {
	if strings.TrimSpace(string(recorded)) == "" {
		return reviewLensContextConflictAction
	}
	return "this frozen lens slot already recorded a reviewer context produced by " + string(recorded) +
		", and audit history is never rewritten; re-run this operation with --delivery " + string(recorded) +
		" to produce the same context by the mechanism the slot recorded, or start a review for a fresh candidate by running " +
		reviewNextTransitionRefreshCommandV21
}

// RunReviewLensContext emits the finished reviewer lens context for one
// selected lens: the provider-authored binding, the provider-authored capture
// context, and the materialized immutable candidate evidence, budget already
// applied and refusals already resolved.
//
// It takes the two opaque tokens the collect transition already carries and
// nothing else. The repository context handle is a digest that commits to the
// lineage, target identity, revision, and repository identity, so those four
// values are recovered from the handle rather than relayed; the selected order
// is recovered from the lens. There is nothing left for a caller to assemble,
// and therefore nothing for it to author incorrectly.
//
// The command consumes no correction budget, no reviewer invocation, and no
// authority revision. Its only write is an append-only audit note recording
// that the provider produced this lens context and by which mechanism, so
// running it again with the same mechanism converges rather than conflicting.
func RunReviewLensContext(args []string, stdout io.Writer) error {
	payload, err := runReviewLensContext(args, stdout, reviewLensContextDependencies())
	if err != nil {
		return err
	}
	// Assembly completes entirely in memory before a single byte is delivered:
	// a partially written block is indistinguishable to a reviewer from a small
	// candidate, and would let a refusal read as a clean review.
	_, err = stdout.Write(payload)
	return err
}

// reviewLensContextDeps isolates the three boundaries this surface cannot
// provoke from a real repository: the aggregate deadline, authority discovery,
// and the native inspection that must never return silent emptiness for a
// content-changing path.
type reviewLensContextDeps struct {
	timeout  time.Duration
	resolve  func(context.Context, string) (string, reviewtransaction.ReviewRepositoryContextBinding, error)
	discover func(context.Context, string, string, bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error)
	prepare  func(reviewtransaction.SnapshotBuilder, context.Context, reviewtransaction.Snapshot) (reviewLensCandidateInspector, error)
	inspect  func(context.Context, reviewLensCandidateInspector, string, int, string) ([]byte, error)
	close    func(reviewLensCandidateInspector) error
	record   func(string, reviewtransaction.LensContextEmission) error
}

type reviewLensCandidateInspector interface {
	FrozenCandidateContext() reviewtransaction.FrozenCandidateContext
	Inspect(context.Context, string, int, string) ([]byte, error)
	Close() error
}

func reviewLensContextDependencies() reviewLensContextDeps {
	return reviewLensContextDeps{
		timeout:  reviewLensContextTimeout,
		resolve:  reviewtransaction.ResolveReviewRepositoryContextBinding,
		discover: discoverCompactFacadeReview,
		prepare: func(builder reviewtransaction.SnapshotBuilder, ctx context.Context, snapshot reviewtransaction.Snapshot) (reviewLensCandidateInspector, error) {
			return builder.PrepareCandidateInspector(ctx, snapshot)
		},
		inspect: func(ctx context.Context, inspector reviewLensCandidateInspector, operation string, pathIndex int, side string) ([]byte, error) {
			return inspector.Inspect(ctx, operation, pathIndex, side)
		},
		close:  func(inspector reviewLensCandidateInspector) error { return inspector.Close() },
		record: reviewtransaction.PublishLensContextEmission,
	}
}

func runReviewLensContext(args []string, help io.Writer, deps reviewLensContextDeps) (payload []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), deps.timeout)
	defer cancel()
	flags := newReviewFlagSet("review lens-context", help,
		"Emit the finished reviewer lens context for one selected lens.\n\nForm: --repository-context <handle> --lens <lens> [--delivery provider_command|runtime_interception]\n\nBoth tokens are carried verbatim by the collect transition; nothing else is accepted, and nothing is left for the caller to assemble.")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context")
	lens := flags.String("lens", "", "exact selected lens")
	delivery := flags.String("delivery", string(reviewtransaction.ReviewerContextLevelProviderCommand),
		"how this context reaches the reviewer: provider_command when a caller relays it, runtime_interception when a runtime adapter injects it")
	if err := parseReviewFlags(flags, args); err != nil {
		return nil, err
	}
	if reviewHelpRequested(args) {
		return nil, nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*repositoryContext) == "" || strings.TrimSpace(*lens) == "" {
		return nil, reviewPreflightError(errors.New("review lens-context requires the exact provider-issued repository context and lens carried by the collect transition; run `gentle-ai review lens-context --help` for the closed command form"))
	}
	level := reviewtransaction.ReviewerContextLevel(strings.TrimSpace(*delivery))
	if level == reviewtransaction.ReviewerContextLevelProviderContract {
		return nil, reviewPreflightError(errors.New("review lens-context delivery provider_contract is reserved for Go-owned provider execution and cannot be declared by callers")) // refusal:by-design world-action: only the live Go transport may record its provider contract provenance
	}
	if !reviewtransaction.ReviewerContextLevelAccepted(level) {
		return nil, reviewPreflightError(fmt.Errorf("unknown reviewer context delivery %q; run `gentle-ai review lens-context --help` for the closed command form", *delivery))
	}

	authority, err := resolveReviewLensAuthority(ctx, deps, strings.TrimSpace(*repositoryContext), strings.TrimSpace(*lens))
	if err != nil {
		return nil, err
	}
	defer func() {
		payload, err = reviewLensContextCleanup(ctx, payload, err, func() error { return deps.close(authority.Inspector) })
	}()
	block, err := reviewLensContextBlock(ctx, deps, authority.Inspector, authority.Binding, authority.Subject, authority.Frozen)
	if err != nil {
		return nil, err
	}
	if expired := reviewLensContextDeadline(ctx, ctx.Err()); expired != nil {
		return nil, expired
	}
	// Recorded only after the context really exists, and only for the slot it
	// was produced for. This is an append-only audit note beside the captured
	// reviewer results: it costs no authority revision, so the revision the
	// caller is still holding for its capture stays valid.
	if err := deps.record(authority.Store.Dir, reviewtransaction.LensContextEmission{
		Schema: reviewtransaction.LensContextEmissionSchema, LineageID: authority.Binding.Lineage,
		TargetIdentity: authority.Binding.Target, AuthorityRevision: authority.Binding.Revision,
		Lens: authority.Binding.Lens, SelectedOrder: authority.Binding.Order, SubjectHash: authority.Binding.SubjectHash, Level: level,
	}); err != nil {
		if errors.Is(err, reviewtransaction.ErrLensContextEmissionConflict) {
			// Read back what the slot really holds so the refusal names the
			// mechanism to re-run with. The record is bound to this exact
			// revision, so a hit here is genuinely a mechanism conflict and not
			// the stale-revision collision this slot key used to produce.
			recorded, _ := reviewtransaction.ReadLensContextEmission(authority.Store.Dir, authority.Binding.Lineage,
				authority.Binding.Target, authority.Binding.Revision, authority.Binding.Lens,
				authority.Binding.Order, authority.Binding.SubjectHash)
			return nil, reviewLensContextRefusal("lens_context_emission_conflict", reviewLensContextConflictActionFor(recorded.Level))
		}
		return nil, reviewLensContextRefusal("lens_context_emission_unavailable", reviewLensContextRefreshAction)
	}
	return block, nil
}

// reviewLensContextProbeOutcome is what a representability probe learned. The
// first two are verdicts about the candidate; the third is a fact about the
// probe, and the type exists so they never collapse into each other.
type reviewLensContextProbeOutcome int

const (
	reviewLensContextRepresentable reviewLensContextProbeOutcome = iota // every lens assembled in full, inside the budget
	reviewLensContextOverBudget                                         // one lens exceeded it: deterministic and permanent
	reviewLensContextUnproven                                           // never decided: the candidate may fit perfectly well
)

// reviewLensContextBudgetProbe assembles the complete immutable evidence for
// every lens a review selected and classifies the candidate. It derives the
// opaque handle without publishing it, records no emission, and writes nothing.
//
// The third outcome is the one that matters. Every stop that is not the budget
// refusal -- an unreachable tree, an expired deadline, any other typed refusal
// -- ends the probe and returns its cause, because before issue #3367 those
// failures read exactly like a candidate that fits, which let a caller be
// handed a reviewer slot nothing had verified was fillable.
func reviewLensContextBudgetProbe(
	ctx context.Context, deps reviewLensContextDeps, repo string,
	state reviewtransaction.CompactState, revision string,
) (reviewLensContextProbeOutcome, error) {
	assemblyContext, cancel := context.WithTimeout(ctx, deps.timeout)
	defer cancel()
	inspector, err := deps.prepare(reviewtransaction.SnapshotBuilder{Repo: repo}, assemblyContext, state.InitialSnapshot)
	if err != nil {
		return reviewLensContextUnproven, err
	}
	defer deps.close(inspector)
	frozen := inspector.FrozenCandidateContext()
	repositoryContext, err := reviewtransaction.DeriveReviewRepositoryContextHandle(assemblyContext, repo, reviewtransaction.ReviewRepositoryContextBinding{
		LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: revision,
	})
	if err != nil {
		return reviewLensContextUnproven, err
	}
	for order, lens := range state.SelectedLenses {
		subject, subjectErr := reviewtransaction.NewArtifactSubject(state, revision, frozen, lens, order, "")
		if subjectErr != nil {
			return reviewLensContextUnproven, subjectErr
		}
		_, assemblyErr := reviewLensContextBlock(assemblyContext, deps, inspector, reviewLensContextBinding{
			Lineage: state.LineageID, Target: state.InitialSnapshot.Identity, Lens: lens, Order: order,
			Revision: revision, RepositoryContext: repositoryContext, SubjectHash: subject.SubjectHash,
		}, subject, frozen)
		var refusal *reviewLensContextError
		if errors.As(assemblyErr, &refusal) && refusal.Code == "lens_context_budget_exceeded" {
			return reviewLensContextOverBudget, nil
		}
		if assemblyErr != nil {
			return reviewLensContextUnproven, assemblyErr
		}
	}
	return reviewLensContextRepresentable, nil
}

// reviewLensContextStatusBudgetExhausted proves the deterministic budget
// refusal for the frozen slots STATUS would otherwise reoffer, and only that.
//
// An unproven probe answers false rather than degrading STATUS: this runs only
// after the captured artifacts verified, so reporting its cause as that
// discovery's failure would turn a transient hiccup into a terminal
// captured-artifact verdict about something that never happened. The launch
// this keeps offering re-runs the same assembly against the same frozen trees
// and refuses with its own typed, refreshable cause.
//
// Since START refuses an unrepresentable candidate before persisting anything,
// the only lineages this can still classify as exhausted are ones an older
// build created, so it stays as the upgrade path's defence rather than the
// primary guard.
func reviewLensContextStatusBudgetExhausted(ctx context.Context, repo string, state reviewtransaction.CompactState, revision string) bool {
	outcome, _ := reviewLensContextBudgetProbe(ctx, reviewLensContextDependencies(), repo, state, revision)
	return outcome == reviewLensContextOverBudget
}

// reviewLensContextCompactAtomicStartBudgetRefusal applies the complete-evidence
// bound to the compact authority START will create or replay. It derives the
// provisional compact revision in memory, never opens or discovers authority
// storage, and therefore keeps an over-budget refusal persistence-free.
func reviewLensContextCompactAtomicStartBudgetRefusal(
	ctx context.Context, repo string, request reviewtransaction.CompactAtomicStartRequest,
) error {
	if len(request.Binding.SelectedLenses) == 0 {
		return nil
	}
	state := request.State
	binding := request.Binding
	binding.Selector.IntendedUntracked = append([]string(nil), binding.Selector.IntendedUntracked...)
	binding.Selector.LedgerIDs = append([]string(nil), binding.Selector.LedgerIDs...)
	binding.SelectedLenses = append([]string(nil), binding.SelectedLenses...)
	state.InitialAtomicStart = &binding
	revision, err := reviewtransaction.CompactRevisionForState(state)
	if err != nil {
		return reviewPreflightError(fmt.Errorf("derive compact atomic START reviewer-context binding: %w", err))
	}
	outcome, _ := reviewLensContextBudgetProbe(ctx, reviewLensContextDependencies(), repo, state, revision)
	if outcome != reviewLensContextOverBudget {
		return nil
	}
	return reviewPreflightRefusal(reviewLensContextStartBudgetReason,
		&reviewLensContextError{Code: "lens_context_budget_exceeded", Action: reviewLensContextStartBudgetAction})
}

// reviewLensContextStartBudgetReason classifies START's budget refusal. The
// default "correct the request you sent" shape would be an actively wrong
// instruction here: no flag, projection, or lineage makes an over-budget
// candidate representable, and raising the bound would only move the cliff.
// The contract message stays inside the published 240-character bound and says
// the two things a caller acts on -- that nothing was created, and that the
// change has to be reviewed as smaller candidates. The exact runnable
// continuation travels with the cause.
var reviewLensContextStartBudgetReason = reviewPreflightReason{
	Code:       "lens_context_budget_exceeded",
	Message:    "The candidate's complete reviewer evidence exceeds the native context budget, so no review authority was created; review this change as smaller candidates that each fit under it.",
	NextAction: "stop",
}

// reviewLensAuthority is the resolved provider-owned binding, subject, and
// frozen candidate context for one selected lens slot: the common prefix
// every surface that needs frozen-candidate reviewer input shares.
// resolveReviewLensAuthority is the only place that turns an opaque repository
// context and a lens name into native authority.
type reviewLensAuthority struct {
	Store     reviewtransaction.CompactStore
	Binding   reviewLensContextBinding
	Subject   reviewtransaction.ArtifactSubject
	Frozen    reviewtransaction.FrozenCandidateContext
	Inspector reviewLensCandidateInspector
}

// resolveReviewLensAuthority recovers the exact lineage, target, revision,
// order, subject, and frozen candidate context from the two opaque tokens a
// caller supplies: the provider-issued repository context and the selected
// lens. Both repositoryContext and lens must already be trimmed.
func resolveReviewLensAuthority(ctx context.Context, deps reviewLensContextDeps, repositoryContext, lens string) (reviewLensAuthority, error) {
	root, binding, err := deps.resolve(ctx, repositoryContext)
	if err != nil {
		// The deadline is checked before classification: an expired aggregate
		// deadline is not evidence that the repository context is unavailable,
		// and reporting it as such would send a caller to refresh a transition
		// that was never the problem.
		if expired := reviewLensContextDeadline(ctx, err); expired != nil {
			return reviewLensAuthority{}, expired
		}
		return reviewLensAuthority{}, reviewRepositoryContextResolutionFailure(err)
	}
	store, record, err := deps.discover(ctx, root, binding.LineageID, false)
	if err != nil {
		if expired := reviewLensContextDeadline(ctx, err); expired != nil {
			return reviewLensAuthority{}, expired
		}
		return reviewLensAuthority{}, reviewLensContextRefusal("lens_context_authority_unavailable", reviewLensContextRefreshAction)
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.InitialSnapshot.Identity != binding.TargetIdentity ||
		record.Revision != binding.Revision {
		return reviewLensAuthority{}, reviewLensContextRefusal("lens_context_binding_stale", reviewLensContextRefreshAction)
	}
	order, err := reviewLensContextSelectedOrder(state.SelectedLenses, lens)
	if err != nil {
		return reviewLensAuthority{}, err
	}

	builder := reviewtransaction.SnapshotBuilder{Repo: root}
	inspector, err := deps.prepare(builder, ctx, state.InitialSnapshot)
	if err != nil {
		return reviewLensAuthority{}, reviewLensContextInspectionFailure(ctx, err)
	}
	frozen := inspector.FrozenCandidateContext()
	subject, err := reviewtransaction.NewArtifactSubject(state, record.Revision, frozen, state.SelectedLenses[order], order, "")
	if err != nil {
		if cleanupErr := inspector.Close(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		return reviewLensAuthority{}, reviewLensContextInspectionFailure(ctx, err)
	}

	return reviewLensAuthority{
		Store: store,
		Binding: reviewLensContextBinding{
			Lineage: binding.LineageID, Target: binding.TargetIdentity, Lens: state.SelectedLenses[order], Order: order,
			Revision: binding.Revision, RepositoryContext: repositoryContext, SubjectHash: subject.SubjectHash,
		},
		Subject: subject, Frozen: frozen, Inspector: inspector,
	}, nil
}

// reviewLensContextSelectedOrder recovers the frozen selected order from the
// lens alone. A review's selected lenses are always distinct — canonical 4R is
// the four supported lenses and every other tier selects at most one — so the
// lens identifies its slot without the caller relaying an index. A duplicate is
// refused rather than guessed at, because silently picking the first slot would
// bind a reviewer to a position it was not assigned.
func reviewLensContextSelectedOrder(selected []string, lens string) (int, error) {
	first := slices.Index(selected, lens)
	if first < 0 {
		return 0, reviewLensContextRefusal("lens_context_lens_not_selected", reviewLensContextRefreshAction)
	}
	if slices.Index(selected[first+1:], lens) >= 0 {
		return 0, reviewLensContextRefusal("lens_context_lens_ambiguous", reviewLensContextRefreshAction)
	}
	return first, nil
}

func reviewLensContextBlock(
	ctx context.Context, deps reviewLensContextDeps, inspector reviewLensCandidateInspector,
	binding reviewLensContextBinding, subject reviewtransaction.ArtifactSubject, frozen reviewtransaction.FrozenCandidateContext,
) ([]byte, error) {
	// RepositoryRoot stays empty: this block is produced only for an opaque
	// binding, and a reviewer transcript never carries a provider path.
	preflight := reviewCapturePreflightResult{
		Schema: reviewCapturePreflightSchema, Capability: reviewCapturePreflightCapability,
		LineageID: binding.Lineage, TargetIdentity: binding.Target, Lens: binding.Lens, SelectedOrder: binding.Order,
		ArtifactSubject: subject, BaseTree: frozen.BaseTree, CandidateTree: frozen.CandidateTree,
		ChangedPathManifest: append([]reviewtransaction.ChangedPathManifestEntry{}, frozen.ChangedPathManifest...),
	}
	var block bytes.Buffer
	if err := reviewLensContextWriteLine(&block, reviewLensContextBindingHeader, binding); err != nil {
		return nil, err
	}
	if err := reviewLensContextWriteLine(&block, reviewLensContextContextHeader, preflight); err != nil {
		return nil, err
	}

	// The budget bounds the whole delivered block, not only the evidence: at
	// this level the block IS the reviewer's prompt, so the instruction and the
	// result schema are part of what has to fit.
	budget := reviewLensContextByteBudget - block.Len()
	consume := func(header, footer string, body []byte) error {
		rendered := header + "\n" + string(bytes.TrimSpace(body)) + "\n" + footer + "\n"
		budget -= len(rendered)
		if budget < 0 {
			return reviewLensContextRefusal("lens_context_budget_exceeded", reviewLensContextBudgetAction)
		}
		block.WriteString(rendered)
		return nil
	}
	instruction, err := reviewLensContextInstructionText(binding, len(frozen.ChangedPathManifest))
	if err != nil {
		return nil, err
	}
	if err := consume(reviewLensContextInstruction, reviewLensContextInstruction+"_END", []byte(instruction)); err != nil {
		return nil, err
	}
	if err := consume(reviewLensContextResultSchema, reviewLensContextResultSchema+"_END", []byte(reviewtransaction.ReviewerResultSchema)); err != nil {
		return nil, err
	}
	for _, discovery := range []struct{ header, operation string }{
		{header: reviewLensContextNameStatus, operation: "name-status"},
		{header: reviewLensContextNumstat, operation: "numstat"},
	} {
		payload, err := deps.inspect(ctx, inspector, discovery.operation, -1, "")
		if err != nil {
			return nil, reviewLensContextInspectionFailure(ctx, err)
		}
		if err := consume(discovery.header, discovery.header+"_END", payload); err != nil {
			return nil, err
		}
	}
	for index, entry := range frozen.ChangedPathManifest {
		payload, err := deps.inspect(ctx, inspector, "patch", index, "")
		if err != nil {
			return nil, reviewLensContextInspectionFailure(ctx, err)
		}
		// An empty patch for a path that is neither mode-only nor deleted is
		// never legitimate: even a newly added empty file still renders a
		// `diff --git`/`new file mode`/`index` header with no hunk. Genuinely
		// empty bytes here mean the read produced nothing for a content-changing
		// path, which is exactly the fabricate-a-clean-review shape a budget and
		// a no-partial-evidence rule exist to prevent.
		if len(bytes.TrimSpace(payload)) == 0 && !entry.ModeOnly && !entry.Deleted {
			return nil, reviewLensContextRefusal("lens_context_empty_patch", reviewLensContextEmptyPatchAction)
		}
		if err := consume(fmt.Sprintf("%s %d %s", reviewLensContextPatch, index, entry.Path), reviewLensContextPatch+"_END", payload); err != nil {
			return nil, err
		}
	}
	block.WriteString(reviewLensContextTerminator + "\n")
	return block.Bytes(), nil
}

// reviewLensContextInstructionText is the reviewer's complete charge. At this
// level there is no runtime layer behind the block to supply anything it
// leaves out: an orchestrator with no adapter puts these exact bytes in front
// of a generic subagent, so everything the reviewer needs to know about its
// role, its permitted scope, and its return shape has to be here.
//
// The lens mandate is read from the one canonical source every installed agent
// definition also renders from, so a reviewer's charge never depends on which
// surface launched it.
func reviewLensContextInstructionText(binding reviewLensContextBinding, paths int) (string, error) {
	title, focus, found := reviewtransaction.LensMandate(binding.Lens)
	if !found {
		return "", reviewLensContextRefusal("lens_context_lens_not_selected", reviewLensContextRefreshAction)
	}
	return fmt.Sprintf(`You are the %s lens of one bounded Gentle AI review. %s

Scope. The %s sections below are the complete and only view of this candidate: all %d changed paths are present in full, in the canonical manifest order carried by %s. Do not read the working tree, the index, HEAD, or any other file, and do not run any command. Nothing outside these sections is part of this candidate, and anything you cannot see here is not evidence.

Causality. Report only what this candidate caused. Give every BLOCKER or CRITICAL finding an evidence_class and a causal_disposition, and mark what the base already contained as pre-existing or base-only rather than as a blocker.

Return. Emit exactly one JSON object and nothing else: no prose before or after it, no markdown fence, no task envelope. It must validate against the schema in %s. Set subject_hash to exactly %s. Set inspection.status to "completed" and inspection.paths to the complete unique unordered set of every manifest path. Each finding location is one path:line or path:start-end inclusive span. findings and evidence must both be present, and evidence must be non-empty.

Citations. Every finding location, and every path cited inside an evidence string or a proof_refs entry, must be a repository-relative path exactly as it appears in the changed-path manifest, optionally with :line or :start-end. Never cite absolute paths, bare file basenames without their directory, or non-path colon-number shapes such as host:port. Any token shaped like path:line is validated against the frozen repository, and one unknown path rejects the entire result. A finding whose causal_disposition is introduced, behavior-activated, or worsened must anchor its location entirely within lines this candidate changed: every line of a path:start-end span is validated as candidate-changed, one unchanged context line in the span rejects the entire result, and observations about unchanged code belong under pre-existing or base-only instead. When the candidate's own content contains a path-shaped literal that is not a real repository path (for example a traversal or fixture token inside a test), never reproduce that token in evidence or proof_refs: describe it in words and cite the manifest file and line that contain it.

Honesty. If you could not inspect the candidate, say so in evidence and do not return a clean result: an access failure is not a completed inspection, and reporting one as clean is the single outcome this review cannot recover from.`,
		title, focus, reviewLensContextPatch, paths, reviewLensContextContextHeader,
		reviewLensContextResultSchema, binding.SubjectHash), nil
}

// reviewLensContextWriteLine writes one header plus its canonical one-line
// JSON. Encoding here rather than instructing a caller to produce it is the
// entire point of this surface: a machine field can never arrive as prose.
func reviewLensContextWriteLine(block *bytes.Buffer, header string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	block.WriteString(header)
	block.WriteByte(' ')
	block.Write(payload)
	block.WriteByte('\n')
	return nil
}

func reviewLensContextInspectionFailure(ctx context.Context, err error) error {
	if expired := reviewLensContextDeadline(ctx, err); expired != nil {
		return expired
	}
	return reviewLensContextRefusal("lens_context_inspection_failed", reviewLensContextRefreshAction)
}

func reviewLensContextCleanup[T any](_ context.Context, result T, operationErr error, close func() error) (T, error) {
	cleanupErr := close()
	if cleanupErr == nil {
		return result, operationErr
	}
	var zero T
	if operationErr != nil {
		return zero, errors.Join(operationErr, cleanupErr)
	}
	// Cleanup is outside the bounded operation, so its error cannot inherit the
	// operation context's cancellation or deadline classification.
	return zero, errors.Join(
		reviewLensContextRefusal("lens_context_inspection_failed", reviewLensContextRefreshAction),
		cleanupErr,
	)
}

// reviewLensContextDeadline reports the aggregate-deadline refusal when either
// the operation context expired or the failure itself carries a cancellation,
// and nil when the failure has an unrelated cause. It never invents a
// cancellation: a nil return means the caller should classify normally.
func reviewLensContextDeadline(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return reviewLensContextRefusal("lens_context_deadline_exceeded", reviewLensContextDeadlineAction)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return reviewLensContextRefusal("lens_context_canceled", reviewLensContextRefreshAction)
	}
	return nil
}
