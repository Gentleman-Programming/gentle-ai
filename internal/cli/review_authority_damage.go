package cli

// The delivery gate used to answer every damaged review authority store with
// one sentence — "complete review authority inventory is unavailable or
// corrupted" — identically for a forged authorization binding, a dangling
// predecessor, and a record truncated mid-write. The operator could not tell
// the damages apart while `review inspect-authority` already computed the
// per-entry diagnosis. This file derives the honest denial detail from that
// same inspection: it names the KIND of each damage and the runnable
// diagnosis, and deliberately nothing more — the gate reports damage, it does
// not guess repairs; sanctioned exits belong to the surfaces whose own
// predictions prove them (reconcile, reclaim, start).

import (
	"context"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewAuthorityCorruptionDetail names, for a gate denial over a damaged
// authority inventory, what kind of damage the store holds and the runnable
// diagnosis. It is read-only and best-effort: when the inspection itself
// cannot run, or the corruption lies outside what it describes (locks, legacy
// entries, layout), it returns "" and the denial keeps its generic sentence
// rather than claiming a diagnosis it does not have.
func reviewAuthorityCorruptionDetail(ctx context.Context, repo string) string {
	report, err := reviewtransaction.InspectCompactRecoveryEdges(ctx, repo)
	if err != nil {
		return ""
	}
	kinds := reviewtransaction.CompactAuthorityDamageKinds(report)
	if len(kinds) == 0 {
		return ""
	}
	return strings.Join(kinds, "; ") +
		". Capture the complete machine-readable diagnosis for every affected lineage with `gentle-ai review inspect-authority --cwd " +
		strconv.Quote(repo) + "`"
}
