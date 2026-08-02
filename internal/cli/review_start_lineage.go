package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	// reviewDerivedStartLineageDigits is how much of the target identity the
	// derived lineage name carries. It is deliberately a prefix of the target
	// identity: the SAME candidate must resume the SAME lineage, which is what
	// makes a repeated `review start` idempotent.
	reviewDerivedStartLineageDigits = 16
	// reviewStartLineageSuffixLimit bounds the search for a free name. A
	// repository that has burnt this many names for one target identity is not
	// a repository a guessed name should keep digging in.
	reviewStartLineageSuffixLimit = 64
)

// reviewDerivedStartLineage is the single derivation of the lineage name a
// `review start` uses when the operator names none. Both the start command
// itself and the negotiated status transition that tells an operator to run it
// resolve the name here, so the name that is printed and the name that is
// created can never be two different strings.
func reviewDerivedStartLineage(targetIdentity string) string {
	digest := strings.TrimPrefix(strings.TrimSpace(targetIdentity), "sha256:")
	if len(digest) < reviewDerivedStartLineageDigits {
		return ""
	}
	return "review-" + digest[:reviewDerivedStartLineageDigits]
}

// reviewAvailableStartLineage returns the lineage a `fresh_target_ready`
// transition must name explicitly so the command it prints can actually run,
// or "" when the derived name is free and the printed command already works.
//
// The defect this closes: the derived name is a function of the target
// identity alone, so a lineage that has since been SUPERSEDED still occupies
// the name its target would derive. A start that names nothing then finds that
// record, finds it is not among the leaves claiming this target, and answers
// blocked-scope-action — at exit 0, having changed nothing. Negotiated status
// is right that nothing governs the live candidate and right to route to a
// fresh start; it just has to name a lineage the store will accept.
//
// It only ever ADDS a --lineage the operator would otherwise have had to
// invent. When the derived name is free the transition keeps the exact bytes it
// already emitted, so the ordinary first review of a candidate is untouched,
// and a repeated start on that candidate still resumes rather than forking.
func reviewAvailableStartLineage(ctx context.Context, root, targetIdentity string) string {
	derived := reviewDerivedStartLineage(targetIdentity)
	if derived == "" || reviewStartLineageAvailable(ctx, root, derived) {
		return ""
	}
	for suffix := 2; suffix <= reviewStartLineageSuffixLimit; suffix++ {
		candidate := fmt.Sprintf("%s-%d", derived, suffix)
		if reviewStartLineageAvailable(ctx, root, candidate) {
			return candidate
		}
	}
	return ""
}

// reviewStartLineageAvailable reports whether no compact authority occupies
// this name. It is fail-closed on purpose: a name whose record exists but does
// not load is still taken, and a store that cannot be resolved at all answers
// taken rather than inviting a start that would collide.
func reviewStartLineageAvailable(ctx context.Context, root, lineage string) bool {
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		return false
	}
	_, loadErr := store.Load()
	return errors.Is(loadErr, os.ErrNotExist)
}
