package sddstatus

import (
	"context"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// canonicalCandidateDigest is the single rule for identifying the content a
// not-applicable report describes.
//
// The report is written into the same candidate it describes, so the candidate's
// tree OID cannot serve: the value the report must contain would depend on
// containing it. Excluding the report's own canonical path yields an identity
// that survives writing the report, and that the ledger can recompute from the
// settled tree by calling this same function.
func canonicalCandidateDigest(ctx context.Context, repo, workspace, changeRoot, change, tree string) (string, error) {
	logicalPath, err := canonicalVerifyReportPaths(repo, workspace, changeRoot, change)
	if err != nil {
		return "", err
	}
	return reviewtransaction.TreeContentDigest(ctx, repo, tree, []string{logicalPath})
}
