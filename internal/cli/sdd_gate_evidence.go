package cli

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// gateAncestryChecker adapts
// (reviewtransaction.SnapshotBuilder).RevisionIsAncestor (phase 19) to
// kickoff.AncestryChecker's one-method port, so internal/kickoff never
// imports internal/reviewtransaction for a single boolean question
// (design.md S4.6, S5.6).
type gateAncestryChecker struct {
	builder reviewtransaction.SnapshotBuilder
}

func (c gateAncestryChecker) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return c.builder.RevisionIsAncestor(ctx, ancestor, descendant)
}

// evidenceRefFromArgs resolves the reference kickoff.VerifyIntegrationEvidence
// judges: the commit for pr_merged, the only kind Axiom checks locally, and
// the operator-supplied free-text reference for deployment/attestation —
// neither of those two is verifiable, so whatever the operator declares is
// simply recorded, never checked (D-13, T-11).
func evidenceRefFromArgs(parsed kickoff.GateRecordArgs) string {
	if parsed.EvidenceKind == kickoff.EvidencePRMerged {
		return parsed.Commit
	}
	return parsed.Evidence
}

// verifyGateEvidence runs kickoff.VerifyIntegrationEvidence for a `gate
// record` invocation that declared --evidence-kind, injecting the real
// AncestryChecker backed by workspaceRoot's own local commit graph. This is
// the CLI adapter's only responsibility: constructing the port's real
// implementation and the Evidence value from parsed flags. The four rules
// themselves live in internal/kickoff (task 20.2); this function decides
// none of them.
func verifyGateEvidence(workspaceRoot string, parsed kickoff.GateRecordArgs) (kickoff.Evidence, error) {
	checker := gateAncestryChecker{builder: reviewtransaction.SnapshotBuilder{Repo: workspaceRoot}}
	return kickoff.VerifyIntegrationEvidence(context.Background(), kickoff.Evidence{
		Kind: parsed.EvidenceKind, Ref: evidenceRefFromArgs(parsed), BaseRef: parsed.BaseRef,
	}, checker)
}
