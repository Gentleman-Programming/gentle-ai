package recoverytrace

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnprovenDeletion      = errors.New("deletion is not proven by a destination or an explicit no-retained-invariant declaration")
	ErrOrphanedInvariant     = errors.New("retained invariant has no owning context or exact proof reference")
	ErrMissingCredit         = errors.New("row does not credit a contributor")
	ErrCountMismatch         = errors.New("reconciliation counts do not match the recovered backlog")
	ErrUnauthorizedDeviation = errors.New("early removal deviation is not authorized by publication evidence")
	ErrUnknownDisposition    = errors.New("row carries an unknown disposition")
	ErrDuplicatePath         = errors.New("path carries more than one disposition")
	ErrBacklogMismatch       = errors.New("reconciliation counts do not match the backlog items behind them")
	ErrUnknownReleaseClass   = errors.New("backlog item carries a release classification outside the five")
)

// Expected totals for the recovered backlog. They are frozen here so a ledger
// rebuilt from a partial scan cannot pass as a complete reconciliation.
const (
	expectedIssues         = 241
	expectedPullRequests   = 92
	expectedCollisionPRs   = 74
	expectedOverlaps       = 499
	expectedDecompositions = 16
)

// Refs that must all report the path as absent before an early removal is
// authorized. The release tag is matched positionally because the tag name moves
// with each release while the branch refs do not.
const (
	remoteMainRef = "origin/main"
	localMainRef  = "main"
	requiredRefs  = 3
)

func ValidateLedgers(ledgers Ledgers) error {
	if err := validateReconciliation(ledgers.Reconciliation); err != nil {
		return err
	}
	// Both checks must hold. The frozen totals prove the ledger is complete; the
	// backlog cross-check proves those totals have real items behind them, which
	// counts alone could always claim without carrying a single one.
	if err := validateBacklog(ledgers.Reconciliation, ledgers.Backlog); err != nil {
		return err
	}
	// One item, one disposition. Without this, a contradictory pair of rows can
	// authorize a deletion that neither row could authorize alone.
	seen := make(map[string]struct{}, len(ledgers.Rows))
	for _, row := range ledgers.Rows {
		if err := validateRow(row); err != nil {
			return err
		}
		if _, duplicate := seen[row.Path]; duplicate {
			return fmt.Errorf("%w: %s", ErrDuplicatePath, row.Path)
		}
		seen[row.Path] = struct{}{}
	}
	return nil
}

func validateReconciliation(reconciliation Reconciliation) error {
	if reconciliation.Issues != expectedIssues ||
		reconciliation.PullRequests != expectedPullRequests ||
		reconciliation.CollisionPRs != expectedCollisionPRs ||
		reconciliation.Overlaps != expectedOverlaps ||
		reconciliation.Decompositions != expectedDecompositions {
		return fmt.Errorf("%w: %+v", ErrCountMismatch, reconciliation)
	}
	return nil
}

// validateBacklog recounts the items and compares them against what the
// reconciliation declares. Items are tallied per kind and number so a repeated
// item cannot stand in for a missing one, and an item of an unrecognized kind
// counts toward neither total rather than being absorbed into one.
func validateBacklog(reconciliation Reconciliation, backlog []BacklogEntry) error {
	counted := make(map[BacklogKind]map[int]struct{}, len(appendixBlocks))
	for _, item := range backlog {
		if item.Release != "" && !knownReleaseClass(item.Release) {
			return fmt.Errorf("%w: %s %d is %q", ErrUnknownReleaseClass,
				item.Kind, item.Number, item.Release)
		}
		byNumber, known := counted[item.Kind]
		if !known {
			byNumber = make(map[int]struct{})
			counted[item.Kind] = byNumber
		}
		byNumber[item.Number] = struct{}{}
	}

	issues := len(counted[BacklogIssue])
	pullRequests := len(counted[BacklogPullRequest])
	if issues != reconciliation.Issues || pullRequests != reconciliation.PullRequests {
		return fmt.Errorf("%w: declared %d issues and %d pull requests, carried %d and %d",
			ErrBacklogMismatch, reconciliation.Issues, reconciliation.PullRequests,
			issues, pullRequests)
	}
	return nil
}

func knownReleaseClass(class ReleaseClass) bool {
	switch class {
	case ReleaseClose, ReleaseSuperseded, ReleasePartiallyCovered,
		ReleaseStillValid, ReleaseNeedsReproduction:
		return true
	default:
		return false
	}
}

func validateRow(row Row) error {
	if !knownDisposition(row.Disposition) {
		return fmt.Errorf("%w: %s", ErrUnknownDisposition, row.Path)
	}
	if row.Contributor == "" {
		return fmt.Errorf("%w: %s", ErrMissingCredit, row.Path)
	}
	if retainsInvariant(row) && (row.Context == "" || len(row.Proof) == 0) {
		return fmt.Errorf("%w: %s", ErrOrphanedInvariant, row.Path)
	}
	if row.Disposition == DispositionDelete && !provenDeletion(row) {
		return fmt.Errorf("%w: %s", ErrUnprovenDeletion, row.Path)
	}
	if row.EarlyDeviation && !unpublishedEverywhere(row.Publication) {
		return fmt.Errorf("%w: %s", ErrUnauthorizedDeviation, row.Path)
	}
	return nil
}

func knownDisposition(disposition Disposition) bool {
	switch disposition {
	case DispositionKeep, DispositionTransplant, DispositionRewrite,
		DispositionDelete, DispositionRegenerate, DispositionDefer:
		return true
	default:
		return false
	}
}

func retainsInvariant(row Row) bool {
	if row.Invariant == "" {
		return false
	}
	switch row.Disposition {
	case DispositionKeep, DispositionTransplant, DispositionRewrite, DispositionRegenerate:
		return true
	default:
		return false
	}
}

// provenDeletion accepts either a transplant that names where the invariant went
// and the test proving it landed, or an explicit declaration that nothing was
// retained. Claiming the latter while still naming an invariant is contradictory,
// so it fails closed rather than trusting one field over the other.
func provenDeletion(row Row) bool {
	if row.DestinationPath != "" && len(row.DestinationProof) > 0 {
		return true
	}
	return row.NoRetainedInvariant && row.Invariant == ""
}

func unpublishedEverywhere(publication []PublicationRef) bool {
	if len(publication) != requiredRefs {
		return false
	}
	var sawRemoteMain, sawLocalMain, sawReleaseTag bool
	for _, ref := range publication {
		if ref.Present {
			return false
		}
		switch ref.Ref {
		case remoteMainRef:
			sawRemoteMain = true
		case localMainRef:
			sawLocalMain = true
		default:
			sawReleaseTag = sawReleaseTag || releaseTagRef(ref.Ref)
		}
	}
	return sawRemoteMain && sawLocalMain && sawReleaseTag
}

// releaseTagRef reports whether a ref names a release tag rather than a branch.
// Both the qualified and bare forms are accepted because callers may capture
// either, but a branch ref must never stand in for the tag: absence from a tag is
// the only evidence proving the path was never shipped to a user.
func releaseTagRef(ref string) bool {
	if name, qualified := strings.CutPrefix(ref, "refs/tags/"); qualified {
		return name != ""
	}
	return len(ref) > 1 && ref[0] == 'v' && ref[1] >= '0' && ref[1] <= '9'
}
