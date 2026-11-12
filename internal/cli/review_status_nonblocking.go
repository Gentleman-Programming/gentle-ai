package cli

import (
	"context"
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// reviewStatusNonBlockingFindings reads the non-blocking findings STATUS
// returns to a client that opted in with --non-blocking-findings (issue
// #6088). It is a pure read of the same authority revision STATUS classified:
// only a current compact authority in correction_required or escalated has
// them, and every other authority yields nothing.
func reviewStatusNonBlockingFindings(ctx context.Context, root string, native reviewtransaction.TargetStatusResult) (*reviewtransaction.NonBlockingFindingSet, error) {
	if native.Applicability != reviewtransaction.TargetApplicabilityCurrent || native.AuthorityVersion != reviewtransaction.AuthorityVersionCompact ||
		(native.State != reviewtransaction.StateCorrectionRequired && native.State != reviewtransaction.StateEscalated) {
		return nil, nil
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, native.LineageID)
	if err != nil {
		return nil, fmt.Errorf("open review lineage for non-blocking findings: %w", err)
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("read review lineage for non-blocking findings: %w", err)
	}
	if record.Revision != native.Revision {
		return nil, fmt.Errorf("review lineage %s changed while STATUS read it; rerun the same gentle-ai review status command with unchanged arguments", native.LineageID)
	}
	return reviewtransaction.NonBlockingFindingSetFor(record.State), nil
}
