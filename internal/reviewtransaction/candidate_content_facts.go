package reviewtransaction

import (
	"context"
	"errors"
)

// CandidateContentFact is a policy-neutral immutable classification for one
// changed path. It deliberately carries no registry, evidence, or verdict, so a
// consumer can read the same candidate properties the review lifecycle reads
// without inheriting the decision the review lifecycle makes from them.
type CandidateContentFact struct {
	Path     string
	Activity VerificationContentActivity
}

// CandidateContentFacts reads immutable candidate bytes using the shared
// classifier without constructing a verification applicability decision.
//
// operationalPaths lets the caller declare the logical paths its own domain
// treats as operational. They are classified active ahead of the generic
// heuristics, which is how a consumer contributes policy it owns without the
// classifier having to know that domain. Passing none is valid and leaves only
// the generic classification in effect.
//
// excluded names changed paths the caller accounts for elsewhere. Every
// excluded path must actually be part of the candidate, so a stale exclusion
// fails instead of silently narrowing what was inspected.
func (builder SnapshotBuilder) CandidateContentFacts(
	ctx context.Context,
	snapshot Snapshot,
	operationalPaths []string,
	excluded []string,
) ([]CandidateContentFact, error) {
	if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
		return nil, err
	}
	operational, err := canonicalPaths(operationalPaths)
	if err != nil {
		return nil, err
	}
	excluded, err = canonicalPaths(excluded)
	if err != nil {
		return nil, err
	}
	for _, logicalPath := range excluded {
		if stringIndex(snapshot.Paths, logicalPath) < 0 {
			return nil, errors.New("excluded candidate content path is not changed") // refusal:by-design world-action: the exclusion list is built by the calling code, so a stale entry is fixed where it is constructed and no invocation can resolve it for the operator
		}
	}
	stats, err := builder.DiffStats(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return nil, err
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	operationalIndex := make(map[string]struct{}, len(operational))
	for _, logicalPath := range operational {
		operationalIndex[logicalPath] = struct{}{}
	}
	facts := make([]CandidateContentFact, 0, len(stats))
	for _, stat := range stats {
		if stringIndex(excluded, stat.Path) >= 0 {
			continue
		}
		activity, _, err := builder.classifyVerificationStat(ctx, snapshot, stat, operationalIndex, repo, isolation)
		if err != nil {
			return nil, err
		}
		facts = append(facts, CandidateContentFact{Path: stat.Path, Activity: activity})
	}
	return facts, nil
}
