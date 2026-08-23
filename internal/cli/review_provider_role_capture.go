package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewProviderRoleCaptureSchema = "gentle-ai.review-provider-role-capture/v1"

// reviewProviderRoleCaptureArtifact is the strict submission acknowledgement
// for a captured provider role result. The immutable bytes live in the compact
// store slot; this envelope only names the binding the capture proved.
type reviewProviderRoleCaptureArtifact struct {
	Schema         string `json:"schema"`
	LineageID      string `json:"lineage_id"`
	TargetIdentity string `json:"target_identity"`
	Role           string `json:"role"`
	Captured       bool   `json:"captured"`
}

// reviewProviderRoleHostAdapter is the one seam through which role capture
// spawns the Go-owned pi process. Tests substitute a fake transport here; the
// lens path keeps its host-mediated refusal in reviewProviderAdapterFor.
var reviewProviderRoleHostAdapter = func() reviewerprovider.Adapter { return reviewerprovider.NewPiAdapter() }

// reviewProviderRoleCaptureTimeout bounds one role capture operation so a full
// Go-owned adversarial pi run fits while a stalled provider cannot hang --execute
// forever: on expiry the adapter surfaces its typed transport refusal,
// nothing is captured, and STATUS reoffers the same collection input. A var
// only as the test seam.
var reviewProviderRoleCaptureTimeout = 600 * time.Second

// reviewProviderRoleCaptureBinding is the parsed, mode-validated invocation of
// one non-lens provider role capture command. Both commands share exactly two
// modes: --materialize (read-only) prints the Go-materialized opaque role
// prompt, and --execute runs a fresh Go-owned locked-down pi process on that
// exact request and admits its raw bytes into the compact slot, so the
// adversarial verdict is never caller-authored.
type reviewProviderRoleCaptureBinding struct {
	command           string
	repositoryContext string
	lineage           string
	target            string
	revision          string
	requestHash       string
	runtime           model.AgentID
	materialize       bool
	execute           bool
	root              string
}

// parseReviewProviderRoleCapture owns the complete refusal matrix shared by
// `review capture-refuter` and `review capture-validation`. Compiled runtimes
// materialize and execute their refuter and validator requests through the same
// Go-owned capture closure.
func parseReviewProviderRoleCapture(command string, args []string, stdout io.Writer, withRequestHash bool) (*reviewProviderRoleCaptureBinding, error) {
	flags := newReviewFlagSet("review "+command, stdout, "Materialize or capture one Go-issued non-lens provider role result bound to compact review authority.")
	cwd := flags.String("cwd", ".", "repository path")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context; supplied by the collect transition and mutually exclusive with --cwd")
	lineage := flags.String("lineage", "", "exact review lineage identifier")
	target := flags.String("target", "", "exact provider-issued target identity for this role")
	revision := flags.String("expected-revision", "", "exact compact authority revision")
	var requestHash *string
	if withRequestHash {
		requestHash = flags.String("request-hash", "", "provider-issued frozen targeted validation request hash")
	}
	runtimeAgent := flags.String("agent", "", "host-relay runtime identity, required for both --materialize and --execute")
	materialize := flags.Bool("materialize", false, "print the exact Go-materialized opaque provider role task without capturing anything; mutually exclusive with --execute")
	execute := flags.Bool("execute", false, "run the Go-owned locked-down pi process on the Go-materialized role request and capture its raw result")
	if err := parseReviewFlags(flags, args); err != nil {
		return nil, err
	}
	if reviewHelpRequested(args) {
		return nil, nil
	}
	binding := &reviewProviderRoleCaptureBinding{
		command: command, repositoryContext: strings.TrimSpace(*repositoryContext),
		lineage: strings.TrimSpace(*lineage), target: strings.TrimSpace(*target), revision: strings.TrimSpace(*revision),
		runtime: model.AgentID(strings.TrimSpace(*runtimeAgent)), materialize: *materialize, execute: *execute,
	}
	if requestHash != nil {
		binding.requestHash = strings.TrimSpace(*requestHash)
	}
	if binding.materialize && binding.execute {
		return nil, reviewPreflightError(fmt.Errorf("review %s --materialize only prints the Go-materialized provider task and cannot be combined with --execute", command)) // refusal:by-design world-action: materialization is read-only and never authors or admits provider role output
	}
	if flags.NArg() != 0 || binding.lineage == "" || binding.target == "" || binding.revision == "" ||
		(!binding.materialize && !binding.execute) {
		return nil, reviewPreflightError(fmt.Errorf("review %s requires --lineage, --target, --expected-revision, --agent, and either --materialize or --execute; `gentle-ai review status --contract %s --next-transition` prints the exact bindings", command, ReviewIntegrationContractV2))
	}
	if binding.runtime == "" {
		// Both modes require the identified host-relay runtime: a raw provider
		// verdict is only admissible from the runtime the negotiated
		// transition bound, never from an unidentified caller.
		return nil, reviewPreflightError(fmt.Errorf("review %s requires --agent naming the host-relay runtime", command)) // refusal:by-design operator-knowledge: only a compiled host-relay runtime identity selects the materialize and execute forms
	}
	if withRequestHash && binding.requestHash == "" {
		return nil, reviewPreflightError(fmt.Errorf("review %s requires --request-hash binding the frozen targeted validation request", command)) // refusal:by-design operator-knowledge: the validator result applies only to one exact frozen correction request
	}
	if binding.repositoryContext != "" && reviewFlagWasProvided(flags, "cwd") {
		return nil, reviewPreflightError(fmt.Errorf("review %s accepts either --repository-context or --cwd, not both", command)) // refusal:by-design operator-knowledge: only the caller knows whether the negotiated opaque context or a direct repository path names its authority
	}
	if binding.materialize && binding.repositoryContext == "" {
		return nil, reviewPreflightError(fmt.Errorf("review %s --materialize requires the provider-issued --repository-context", command)) // refusal:by-design operator-knowledge: materialization must use the negotiated opaque context
	}
	// The runtime gate is symmetric across both modes: materialization and
	// submission are two halves of the same host-relay transaction, so a
	// runtime that may not receive the prompt may not return its verdict.
	if _, err := reviewRuntimeWithImmutableTransport(string(binding.runtime)); err != nil {
		return nil, reviewPreflightError(err)
	}
	if reviewProviderCaptureRuntime(binding.runtime) && binding.materialize {
		return nil, reviewPreflightError(fmt.Errorf("review %s --materialize is unavailable for %q: its compiled Go adapter executes the provider contract directly; rerun `gentle-ai review %s` with the same binding and --execute", command, binding.runtime, command))
	}
	if !reviewProviderCaptureRuntime(binding.runtime) && !reviewProviderHostRelayMaterializeRuntime(binding.runtime) {
		return nil, reviewPreflightError(fmt.Errorf("review %s provider runtime %q has no Go-owned role capture contract", command, binding.runtime)) // refusal:by-design world-action: only compiled adapters and the Pi host relay collect non-lens provider roles
	}
	ctx := context.Background()
	var err error
	if binding.repositoryContext != "" {
		binding.root, err = resolveOpaqueReviewRepositoryRoot(ctx, binding.repositoryContext, reviewtransaction.ReviewRepositoryContextBinding{
			LineageID: binding.lineage, TargetIdentity: binding.target, Revision: binding.revision,
		})
		if err != nil {
			return nil, err
		}
	} else {
		binding.root, err = (reviewtransaction.SnapshotBuilder{Repo: *cwd}).ResolveRepositoryRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve review repository root: %w", err)
		}
	}
	// --materialize is read-only and stays reachable under a frozen switch;
	// an execution publishes provider role bytes into the store.
	if !binding.materialize {
		if err := authorizeReviewAuthorityMutation(ctx, binding.root); err != nil {
			return nil, err
		}
	}
	return binding, nil
}

func (binding *reviewProviderRoleCaptureBinding) discover(ctx context.Context) (reviewtransaction.CompactStore, reviewtransaction.CompactRecord, error) {
	store, record, err := discoverCompactFacadeReview(ctx, binding.root, binding.lineage, false)
	if err != nil {
		if binding.repositoryContext != "" {
			return store, record, reviewOpaqueContextCause("repository_context_authority_unavailable", "refresh the exact native next_transition before retrying", err)
		}
		return store, record, reviewPreflightError(fmt.Errorf("resolve review authority for lineage %q under repository %q: %w", binding.lineage, binding.root, err))
	}
	if record.State.LineageID != binding.lineage || record.Revision != binding.revision {
		return store, record, reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, fmt.Errorf("review %s binding does not match the current compact authority; refresh the binding with gentle-ai review status --cwd <repo> --contract %s --next-transition", binding.command, ReviewIntegrationContractV2))
	}
	return store, record, nil
}

// RunReviewCaptureRefuter materializes or executes the one transaction-wide
// provider refuter batch. --materialize prints the Go-materialized opaque
// prompt raw; --execute runs a fresh Go-owned locked-down pi process on that
// exact request and admits its raw bytes, so adversarial provenance mirrors
// the compiled claude/codex path; admission and canonicalization stay in Go.
func RunReviewCaptureRefuter(args []string, stdout io.Writer) error {
	binding, err := parseReviewProviderRoleCapture("capture-refuter", args, stdout, false)
	if err != nil || binding == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewProviderRoleCaptureTimeout)
	defer cancel()
	store, record, err := binding.discover(ctx)
	if err != nil {
		return err
	}
	state := record.State
	if state.State != reviewtransaction.StateReviewing || state.InitialSnapshot.Identity != binding.target {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture-refuter requires the exact reviewing authority target; refresh the binding with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition"))
	}
	request, err := reviewProviderNewRefuterRequest(ctx, binding.root, store.Dir, state, record.Revision)
	if err != nil {
		return reviewPreflightError(err)
	}
	if binding.materialize {
		// Raw bytes: no JSON envelope, no trailing newline, nothing captured.
		if _, err := stdout.Write(request.Invocation.Prompt()); err != nil {
			return fmt.Errorf("write materialized provider refuter task: %w", err)
		}
		return nil
	}
	var raw []byte
	if reviewProviderCaptureRuntime(binding.runtime) {
		adapter, adapterErr := reviewProviderAdapter(reviewProviderRoleRefuter, binding.runtime)
		if adapterErr != nil {
			return reviewPreflightError(adapterErr)
		}
		raw, err = adapter.Review(ctx, request.Invocation)
	} else {
		raw, err = reviewProviderRoleHostAdapter().Review(ctx, request.Invocation)
	}
	if err != nil {
		return reviewPreflightError(fmt.Errorf("invoke provider refuter: %w", err))
	}
	if _, err := reviewProviderCaptureRefuterRaw(ctx, binding.root, store, state, record.Revision, raw); err != nil {
		return reviewPreflightError(err)
	}
	closure, err := closeReviewOnLastCapturedLens(ctx, binding.root, store, record)
	if err != nil && !reviewLastCapturedLensClosureSuperseded(store, record) {
		return reviewPreflightError(err)
	}
	if closure != nil {
		closure.Operation = reviewCaptureRefuterCaptureOperation
		return encodeReviewJSON(stdout, closure)
	}
	return encodeReviewJSON(stdout, reviewProviderRoleCaptureArtifact{
		Schema: reviewProviderRoleCaptureSchema, LineageID: state.LineageID,
		TargetIdentity: state.InitialSnapshot.Identity, Role: string(reviewerprovider.RoleRefuter), Captured: true,
	})
}

// RunReviewCaptureValidation materializes or captures the correction-bound
// provider targeted-validator result, binding the frozen validation request
// hash in both modes so the verdict can never drift to another correction.
func RunReviewCaptureValidation(args []string, stdout io.Writer) error {
	binding, err := parseReviewProviderRoleCapture("capture-validation", args, stdout, true)
	if err != nil || binding == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), reviewProviderRoleCaptureTimeout)
	defer cancel()
	store, record, err := binding.discover(ctx)
	if err != nil {
		return err
	}
	state := record.State
	correction, err := reviewProviderTargetedValidatorCorrection(ctx, binding.root, state)
	if err != nil {
		return reviewPreflightError(err)
	}
	request, err := reviewProviderNewTargetedValidatorRequest(ctx, binding.root, state, record.Revision, correction)
	if err != nil {
		return reviewPreflightError(err)
	}
	if request.ValidationRequest.CorrectionTargetIdentity != binding.target {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture-validation target does not match the frozen correction target identity; refresh the binding with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition"))
	}
	if request.ValidationRequest.RequestHash != binding.requestHash {
		return reviewPreflightRefusal(reviewPreflightCaptureBindingMismatchReason, errors.New("review capture-validation request hash does not match the frozen targeted validation request; refresh the binding with gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition"))
	}
	if binding.materialize {
		// Raw prompt bytes, exactly as for the refuter above.
		if _, err := stdout.Write(request.Invocation.Prompt()); err != nil {
			return fmt.Errorf("write materialized provider targeted validator task: %w", err)
		}
		return nil
	}
	var raw []byte
	if reviewProviderCaptureRuntime(binding.runtime) {
		adapter, adapterErr := reviewProviderAdapter(reviewProviderRoleTargetedValidator, binding.runtime)
		if adapterErr != nil {
			return reviewPreflightError(adapterErr)
		}
		raw, err = adapter.Review(ctx, request.Invocation)
	} else {
		raw, err = reviewProviderRoleHostAdapter().Review(ctx, request.Invocation)
	}
	if err != nil {
		return reviewPreflightError(fmt.Errorf("invoke provider targeted validator: %w", err))
	}
	_, _, closure, err := reviewProviderCloseTargetedValidatorRaw(ctx, binding.root, store, state, record.Revision, raw)
	if err != nil {
		return reviewPreflightError(err)
	}
	return encodeReviewJSON(stdout, closure)
}
