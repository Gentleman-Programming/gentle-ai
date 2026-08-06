package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// runLegacyFacadeStartForTest is a drop-in TEST-ONLY replacement for
// RunReviewFacadeStart, same signature and same JSON-on-stdout/error-return
// contract, for any test that specifically needs LEGACY (compact-v2)
// authority.
//
// Before Wave 7 S7 (WU18), a plain `review start` (activation switch unset,
// the default) took the legacy compact-v2 branch inside runReviewFacadeStart.
// WU18 removed that branch entirely -- `review start` is now unconditionally
// v3, so there is no CLI-reachable way left to create a NEW legacy
// authority. This function replicates the exact target-building/
// risk-assessment/lens-selection/authority-creation pipeline the deleted
// legacy branch used (SnapshotBuilder.Build -> AssessSnapshotRisk ->
// facadeSelectedLenses -> facadePolicyBytes -> reviewtransaction.
// NewCompactState -> reviewtransaction.StartCompactAuthority), parses the
// identical flag surface runReviewFacadeStart itself parses (--cwd,
// --lineage, --policy, --focus, --base-ref, --projection, --committed-only,
// --workspace-overlay), and JSON-encodes the same ReviewFacadeStartResult
// shape to stdout -- so every existing call site's own decode/error-handling
// code works completely unchanged; only the function name at the call site
// needs to become this one instead of RunReviewFacadeStart.
//
// This is the fixture-construction pattern established (and individually
// verified) on finalizeApprovedFacadeReview, approveDiscoveryMarkdownProjection,
// and startFacadeReview -- generalized here to cover every flag combination
// those three narrower helpers didn't need, for the many remaining test
// files with their own ad hoc RunReviewFacadeStart call sites.
//
// Not supported (none of the migrated call sites use them; the negotiated
// contract/consent surface is switch-independent and untouched by WU18, so
// tests needing it can and do keep calling the real RunReviewFacadeStart):
// --contract, --target, --agent, --consent, --locale, --trace.
func runLegacyFacadeStartForTest(t *testing.T, args []string, stdout io.Writer) error {
	t.Helper()
	flags := flag.NewFlagSet("review start (legacy test fixture)", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", ".", "")
	lineage := flags.String("lineage", "", "")
	policySource := flags.String("policy", "", "")
	focus := flags.String("focus", "reliability", "")
	baseRef := flags.String("base-ref", "", "")
	projectionFlag := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "")
	committedOnly := flags.Bool("committed-only", false, "")
	workspaceOverlay := flags.Bool("workspace-overlay", false, "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review start argument %q", flags.Arg(0))
	}

	ctx := context.Background()
	builder := reviewtransaction.SnapshotBuilder{Repo: *cwd}
	root, err := builder.ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("resolve review repository root: %w", err)
	}
	rootBuilder := reviewtransaction.SnapshotBuilder{Repo: root}
	selectedProjection := reviewtransaction.Projection(strings.TrimSpace(*projectionFlag))
	if selectedProjection != reviewtransaction.ProjectionWorkspace && selectedProjection != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported review projection %q", *projectionFlag)
	}
	if *workspaceOverlay && (strings.TrimSpace(*baseRef) == "" || *committedOnly || selectedProjection != reviewtransaction.ProjectionWorkspace) {
		return errors.New("--workspace-overlay requires --base-ref with workspace projection and is incompatible with --committed-only")
	}
	if selectedProjection == reviewtransaction.ProjectionStaged && strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		return fmt.Errorf("review start with --projection staged and --base-ref is refused because intent is ambiguous: for a staged-index review rerun with %q; for a base-diff review rerun with %q",
			"gentle-ai review start --projection staged",
			fmt.Sprintf("gentle-ai review start --base-ref %s --committed-only", strings.TrimSpace(*baseRef)))
	}
	if strings.TrimSpace(*baseRef) != "" && !*workspaceOverlay {
		dirtyTracked, dirtyErr := rootBuilder.HasDirtyTrackedChanges(ctx)
		if dirtyErr != nil {
			return fmt.Errorf("detect dirty tracked changes for committed review: %w", dirtyErr)
		}
		if dirtyTracked && !*committedOnly {
			return errors.New("review start with --base-ref omits dirty tracked changes; rerun with --committed-only to acknowledge committed-only review scope")
		}
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetCurrentChanges, Projection: selectedProjection, IntendedUntracked: []string{}}
	if strings.TrimSpace(*baseRef) != "" {
		target.Kind = reviewtransaction.TargetBaseDiff
		target.BaseRef = strings.TrimSpace(*baseRef)
	}
	if *workspaceOverlay {
		target.Kind = reviewtransaction.TargetBaseWorkspaceOverlay
	}
	snapshot, err := rootBuilder.Build(ctx, target)
	if err != nil {
		return fmt.Errorf("build facade review target: %w", err)
	}
	assessment, err := rootBuilder.AssessSnapshotRisk(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("classify facade review target: %w", err)
	}
	lenses, err := facadeSelectedLenses(assessment, *focus)
	if err != nil {
		return err
	}
	policy, err := facadePolicyBytes(*policySource)
	if err != nil {
		return err
	}
	trimmedLineage := strings.TrimSpace(*lineage)
	explicitLineage := trimmedLineage != ""
	if !explicitLineage {
		trimmedLineage = reviewDerivedStartLineage(snapshot.Identity)
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: trimmedLineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: facadePayloadHash(policy), RiskLevel: assessment.Level,
		SelectedLenses: lenses, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		return fmt.Errorf("create compact facade review: %w", err)
	}
	started, err := reviewtransaction.StartCompactAuthority(ctx, root, reviewtransaction.CompactStartRequest{
		State: state, ExplicitLineage: explicitLineage,
	})
	if err != nil {
		return fmt.Errorf("start compact facade review: %w", err)
	}
	result := reviewFacadeStartResultFor(started.Action, started.LensesRequired, started.Record.State)
	if started.Record.State.InitialSnapshot.Identity == snapshot.Identity {
		result.RiskEvidence = reviewConsentRiskEvidence(assessment)
		switch {
		case result.ChangedFiles == 0 && target.Kind == reviewtransaction.TargetCurrentChanges:
			result.Hint = reviewStartEmptyCandidateHint
		case result.LensesRequired:
			result.Hint = "this response's selected lenses require the frozen Git trees, changed-path manifest, and artifact subjects, which only the negotiated contract form returns; rerun with `" +
				reviewNegotiatedStartCommand(started.Record.State.InitialSnapshot, "") + "` to receive them"
		}
	}
	return encodeReviewJSON(stdout, result)
}

// runLegacyFacadeStartForTestBytes is the []byte-returning convenience form
// for the (more common) call sites that decode from a bytes.Buffer.
func runLegacyFacadeStartForTestBytes(t *testing.T, args []string) ([]byte, error) {
	t.Helper()
	var output bytes.Buffer
	err := runLegacyFacadeStartForTest(t, args, &output)
	return output.Bytes(), err
}
