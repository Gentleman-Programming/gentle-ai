package cli

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// The kill switch freezes authority against review progress. Every operation
// that starts or advances authority refuses while it is off. Reads and the one
// sanctioned destructive cleanup, RDDOperationAbandon, remain
// available: its own storage gate proves that it can only discard a pristine
// lineage and cannot mint a terminal receipt.
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
// `review abandon` passes RDDOperationAbandon through the same mode
// resolver. Its expected revision, authorization binding, persisted
// pristineness proof, and quarantine audit record remain its own gates. Other
// cleanup-shaped verbs remain RDDOperationMutate until separately sanctioned.

// resolveReviewMutationRoot resolves the repository a mutating review verb is
// about to write to, and refuses while the kill switch is off. It replaces the
// bare root resolution in every such verb so the two steps cannot drift apart.
func resolveReviewMutationRoot(ctx context.Context, cwd string) (string, error) {
	return resolveReviewOperationRoot(ctx, cwd, reviewtransaction.RDDOperationMutate)
}

// resolveReviewOperationRoot resolves the repository and applies the single
// operation-classified RDD policy before a command can reach authority.
func resolveReviewOperationRoot(ctx context.Context, cwd string, operation reviewtransaction.RDDOperation) (string, error) {
	root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return "", err
	}
	switch operation {
	case reviewtransaction.RDDOperationMutate:
		// authorizeReviewAuthorityMutation already applies the asset gate.
		err = authorizeReviewAuthorityMutation(ctx, root)
	case reviewtransaction.RDDOperationAbandon:
		if err = authorizeReviewRDDOperation(ctx, root, operation); err == nil {
			err = authorizeManagedReviewerAssets()
		}
	default:
		err = authorizeReviewRDDOperation(ctx, root, operation)
	}
	if err != nil {
		return "", err
	}
	return root, nil
}

// authorizeReviewAuthorityMutation refuses to advance or consume existing
// review authority while receipt-driven development is switched off. It is
// separate from resolveReviewMutationRoot for the verbs that resolve their root
// before they know whether this run mutates anything.
func authorizeReviewAuthorityMutation(ctx context.Context, repo string) error {
	if err := authorizeReviewRDDOperation(ctx, repo, reviewtransaction.RDDOperationMutate); err != nil {
		return err
	}
	return authorizeManagedReviewerAssets()
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
