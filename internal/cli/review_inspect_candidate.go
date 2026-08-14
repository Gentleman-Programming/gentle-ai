package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewInspectCandidateTimeout = 25 * time.Second

type reviewInspectCandidateAuthorityError struct{ cause error }

func (err *reviewInspectCandidateAuthorityError) Error() string {
	return "repository_context_authority_unavailable: provider-issued review repository context operation failed; refresh the exact native next_transition before retrying"
}
func (err *reviewInspectCandidateAuthorityError) Unwrap() error { return err.cause }

type reviewInspectCandidateDeps struct {
	timeout          time.Duration
	operationContext func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	resolve          func(context.Context, string, reviewtransaction.ReviewRepositoryContextBinding) (string, error)
	resolveCorrected func(context.Context, string, reviewtransaction.ReviewRepositoryContextBinding, string) (reviewtransaction.SnapshotBuilder, reviewtransaction.Snapshot, error)
	discover         func(context.Context, string, string, bool) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error)
	inspect          func(reviewtransaction.SnapshotBuilder, context.Context, reviewtransaction.Snapshot, string, int, string) ([]byte, error)
}

func reviewInspectCandidateDependencies() reviewInspectCandidateDeps {
	return reviewInspectCandidateDeps{
		timeout:          reviewInspectCandidateTimeout,
		operationContext: context.WithTimeout,
		resolve:          resolveOpaqueReviewRepositoryRoot,
		resolveCorrected: reviewtransaction.ResolveCorrectedCandidateInspectionBinding,
		discover:         discoverCompactFacadeReview,
		inspect:          reviewtransaction.SnapshotBuilder.InspectCandidate,
	}
}

// RunReviewInspectCandidate exposes only provider-bound immutable candidate inspection; callers supply no repository path, tree, or literal path.
func RunReviewInspectCandidate(args []string, stdout io.Writer) error {
	payload, err := runReviewInspectCandidate(args, stdout, reviewInspectCandidateDependencies())
	if err != nil {
		return err
	}
	// The byte-bounded operational phase is complete; io.Writer has no cancellation
	// contract, so final delivery is outside the repository-operation deadline.
	_, err = stdout.Write(payload)
	return err
}

func runReviewInspectCandidate(args []string, help io.Writer, deps reviewInspectCandidateDeps) ([]byte, error) {
	ctx, cancel := deps.operationContext(context.Background(), deps.timeout)
	defer cancel()
	flags := newReviewFlagSet("review inspect-candidate", help, "Inspect a provider-bound frozen candidate without reading mutable repository state.\n\nGlobal form: --operation name-status|numstat\nPath form: --operation stat|patch --path-index <n>\nObject form: --operation object --path-index <n> --side base|candidate")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context")
	revision := flags.String("expected-revision", "", "exact reviewing authority revision")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact frozen target identity")
	purpose := flags.String("purpose", "reviewing-lens", "reviewing-lens or "+reviewTargetedValidationPurpose)
	requestHash := flags.String("request-hash", "", "exact targeted-validation request hash")
	lens := flags.String("lens", "", "exact selected lens")
	order := flags.Int("order", -1, "zero-based selected lens order")
	operation := flags.String("operation", "", "name-status, numstat, stat, patch, or object")
	pathIndex := flags.Int("path-index", -1, "zero-based canonical changed-path manifest index")
	side := flags.String("side", "", "object side: base or candidate")
	if err := parseReviewFlags(flags, args); err != nil || reviewHelpRequested(args) {
		return nil, err
	}
	if *purpose != reviewTargetedValidationPurpose {
		if *purpose != "reviewing-lens" {
			return nil, reviewPreflightError(errors.New("review inspect-candidate purpose must be reviewing-lens or targeted-validation")) // refusal:by-design operator-knowledge: the native inspector has exactly two disjoint authority modes
		}
		if reviewFlagWasProvided(flags, "request-hash") {
			return nil, reviewPreflightError(errors.New("review inspect-candidate request hash is valid only for targeted validation")) // refusal:by-design operator-knowledge: reviewing-lens authority never carries a correction request
		}
		if flags.NArg() != 0 || strings.TrimSpace(*repositoryContext) == "" || strings.TrimSpace(*revision) == "" ||
			strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" || strings.TrimSpace(*lens) == "" || *order < 0 {
			return nil, reviewPreflightError(errors.New("review inspect-candidate requires the exact provider-issued repository context, revision, lineage, target, lens, and order; run `gentle-ai review inspect-candidate --help` for the closed command forms"))
		}
	}
	pathProvided := reviewFlagWasProvided(flags, "path-index")
	sideProvided := reviewFlagWasProvided(flags, "side")
	switch *operation {
	case "name-status", "numstat":
		if pathProvided || sideProvided {
			return nil, reviewPreflightError(errors.New("global candidate inspection does not accept a path index or side; run `gentle-ai review inspect-candidate --help` for the closed command forms"))
		}
	case "stat", "patch":
		if !pathProvided || *pathIndex < 0 || sideProvided {
			return nil, reviewPreflightError(errors.New("path candidate inspection requires only a non-negative --path-index; run `gentle-ai review inspect-candidate --help` for the closed command forms"))
		}
	case "object":
		if !pathProvided || *pathIndex < 0 || !sideProvided || (*side != "base" && *side != "candidate") {
			return nil, reviewPreflightError(errors.New("object candidate inspection requires --path-index and --side base|candidate; run `gentle-ai review inspect-candidate --help` for the closed command forms"))
		}
	default:
		return nil, reviewPreflightError(fmt.Errorf("unknown candidate inspection operation %q; run `gentle-ai review inspect-candidate --help` for the closed command forms", *operation))
	}
	var builder reviewtransaction.SnapshotBuilder
	var snapshot reviewtransaction.Snapshot
	if *purpose == reviewTargetedValidationPurpose {
		if flags.NArg() != 0 || strings.TrimSpace(*repositoryContext) == "" || strings.TrimSpace(*revision) == "" ||
			strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*target) == "" || strings.TrimSpace(*requestHash) == "" {
			return nil, reviewPreflightError(errors.New("review inspect-candidate targeted validation requires the exact provider-issued repository context, revision, lineage, target, and request hash; run `gentle-ai review inspect-candidate --help` for the closed command forms"))
		}
		if reviewFlagWasProvided(flags, "lens") || reviewFlagWasProvided(flags, "order") {
			return nil, reviewPreflightError(errors.New("review inspect-candidate targeted validation does not accept --lens or --order")) // refusal:by-design operator-knowledge: only a fresh targeted transition names the lens-free inspector binding
		}
		var err error
		builder, snapshot, err = deps.resolveCorrected(ctx, *repositoryContext, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		}, *requestHash)
		if err != nil {
			var contextResolution *reviewtransaction.CorrectedCandidateInspectionRepositoryContextError
			if errors.As(err, &contextResolution) {
				err = reviewRepositoryContextResolutionFailure(err)
			}
			return nil, reviewInspectCandidateError(err)
		}
	} else {
		root, err := deps.resolve(ctx, *repositoryContext, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: *lineage, TargetIdentity: *target, Revision: *revision,
		})
		if err != nil {
			return nil, reviewInspectCandidateError(err)
		}
		_, record, err := deps.discover(ctx, root, *lineage, false)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, reviewInspectCandidateError(err)
			}
			return nil, reviewPreflightError(&reviewInspectCandidateAuthorityError{cause: err})
		}
		state := record.State
		if state.State != reviewtransaction.StateReviewing || state.InitialSnapshot.Identity != *target || record.Revision != *revision ||
			*order >= len(state.SelectedLenses) || state.SelectedLenses[*order] != *lens {
			return nil, reviewPreflightError(errors.New("candidate inspection binding does not match the current reviewing authority; refresh the exact native next_transition before retrying `gentle-ai review inspect-candidate`"))
		}
		builder = reviewtransaction.SnapshotBuilder{Repo: root}
		snapshot = state.InitialSnapshot
	}
	payload, err := deps.inspect(builder, ctx, snapshot, *operation, *pathIndex, *side)
	if err != nil {
		return nil, reviewInspectCandidateError(reviewPreflightError(fmt.Errorf("inspect frozen candidate: %w", err)))
	}
	if ctx.Err() != nil {
		return nil, reviewInspectCandidateError(ctx.Err())
	}
	return payload, nil
}

func reviewInspectCandidateError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return reviewPreflightError(fmt.Errorf("review inspect-candidate operation deadline exceeded: %w", context.DeadlineExceeded))
	}
	if errors.Is(err, context.Canceled) {
		return reviewPreflightError(fmt.Errorf("review inspect-candidate operation canceled: %w", context.Canceled))
	}
	return err
}
