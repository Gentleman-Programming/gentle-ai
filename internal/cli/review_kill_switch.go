package cli

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The kill switch freezes review authority read-only; it never destroys it.
// "Read-only" is only true if every verb that advances or consumes authority
// refuses while the switch is off, and the sole production call site used to
// authorize exactly one operation -- START -- leaving the documented
// RDDOperationMutate constant unreferenced. A lineage started before the switch
// went off could therefore still be finalized to an approved terminal receipt
// with reviews disabled, which also silently defeats re-validation on
// re-enabling: the operator would come back to approved authority.
//
// The gate is applied at one exact moment in every mutating verb: after the
// verb has validated its own request, and as it resolves the repository it is
// about to write to. Both halves of that placement are load-bearing.
//
// It cannot run earlier, at the router. A malformed request is refused on its
// own terms -- an unknown flag, a stray positional, a missing required input --
// and answering one of those with "reviews are disabled; run review mode
// enable" would name a command that does not resolve the block, because the
// request is still malformed after running it. Usage errors win, then the
// switch decides.
//
// It cannot run later either. Resolving the repository root is the last step
// every mutating verb shares before it can reach the store, so authorizing here
// means a refusal provably wrote nothing.
//
// Verbs that only read -- capabilities, status, validate, schema,
// inspect-authority, and the explicit --preflight modes of capture-result and
// repair -- never call this. That is deliberate rather than an omission:
// freezing authority is worthless if the operator can no longer inspect it, and
// `validate` in particular MUST keep answering while disabled, because
// reporting `disabled/unmanaged` at exit 0 is exactly what lets ordinary
// commit, push, and PR delivery proceed unmanaged.
//
// FINALIZE authorizes itself separately (see runReviewFacadeFinalize) because
// only it can be either half: against an in-flight lineage it advances the
// review, and against an already-terminal one it is the exact replay that
// RDDOperationRead covers, which re-emits the frozen receipt and writes nothing.

// resolveReviewMutationRoot resolves the repository a mutating review verb is
// about to write to, and refuses while the kill switch is off. It replaces the
// bare root resolution in every such verb so the two steps cannot drift apart.
func resolveReviewMutationRoot(ctx context.Context, cwd string) (string, error) {
	root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return "", err
	}
	if err := authorizeReviewAuthorityMutation(ctx, root); err != nil {
		return "", err
	}
	return root, nil
}

// authorizeReviewAuthorityMutation refuses to advance or consume existing
// review authority while review-driven development is switched off. It is
// separate from resolveReviewMutationRoot for the verbs that resolve their root
// before they know whether this run mutates anything.
func authorizeReviewAuthorityMutation(ctx context.Context, repo string) error {
	return authorizeReviewRDDOperation(ctx, repo, reviewtransaction.RDDOperationMutate)
}

func authorizeReviewRDDOperation(ctx context.Context, repo string, operation reviewtransaction.RDDOperation) error {
	global, err := readGlobalRDDMode()
	if err != nil {
		return err
	}
	_, err = reviewtransaction.AuthorizeRDDOperation(ctx, repo, global, operation)
	// An unreadable mode record still fails closed, but it names the file and
	// the command that clears it instead of surfacing a bare parse error.
	return reviewModeUnreadable(ctx, repo, global, err)
}
