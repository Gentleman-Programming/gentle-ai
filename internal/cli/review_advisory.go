package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/advisoryreview"
)

// RunReviewAdvisory exposes the generic prompt/text seam. It is intentionally
// read-only: validating an advisory result never creates review authority, a
// receipt, a gate result, or a delivery decision.
//
// Its binding is derived exactly like `review lens-context`: the opaque
// provider-issued repository context and the selected lens are the only
// caller-supplied tokens (resolveReviewLensAuthority). Lineage, target,
// revision, order, subject, and evidence all come from frozen native
// authority; there is no caller-authored request path.
func RunReviewAdvisory(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		// The exact flags belong to `prompt` and `validate`, not to this
		// dispatcher: naming them here would advertise a form this bare verb
		// never parses. Each subcommand's own usage documents its closed form.
		_, err := fmt.Fprintln(stdout, "Usage: gentle-ai review advisory <prompt|validate>\n\nExport or validate advisory model-review evidence derived from frozen native authority; run `gentle-ai review advisory prompt` or `gentle-ai review advisory validate` and see each subcommand's own usage for its exact closed flag form. This command never authorizes delivery or creates a receipt.")
		return err
	}
	switch args[0] {
	case "prompt":
		return runReviewAdvisoryPrompt(args[1:], stdout)
	case "validate":
		return runReviewAdvisoryValidate(args[1:], stdout)
	default:
		return fmt.Errorf("unknown review advisory command %q; run `gentle-ai review advisory prompt` or `gentle-ai review advisory validate`", args[0])
	}
}

func runReviewAdvisoryPrompt(args []string, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), reviewLensContextTimeout)
	defer cancel()
	flags := newReviewFlagSet("review advisory prompt", stdout,
		"Export the canonical advisory prompt for a runtime.\n\nForm: --repository-context <handle> --lens <lens> --runtime <runtime>\n\nBoth the repository context and the lens are carried verbatim by the collect transition, exactly as `review lens-context` consumes them; nothing else is accepted, and nothing is left for the caller to assemble.")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context")
	lens := flags.String("lens", "", "exact selected lens")
	runtime := flags.String("runtime", "", "advisory transport runtime")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*repositoryContext) == "" || strings.TrimSpace(*lens) == "" || strings.TrimSpace(*runtime) == "" {
		return reviewPreflightError(errors.New("review advisory prompt requires the exact provider-issued repository context, lens, and runtime; run `gentle-ai review advisory prompt` with no arguments to see its exact closed command form"))
	}
	request, err := resolveAdvisoryRequest(ctx, reviewLensContextDependencies(), strings.TrimSpace(*repositoryContext), strings.TrimSpace(*lens))
	if err != nil {
		return err
	}
	prompt, err := advisoryreview.PromptFor(advisoryreview.Runtime(strings.TrimSpace(*runtime)), request)
	if err != nil {
		return reviewPreflightError(err)
	}
	_, err = fmt.Fprintln(stdout, prompt)
	return err
}

func runReviewAdvisoryValidate(args []string, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), reviewLensContextTimeout)
	defer cancel()
	flags := newReviewFlagSet("review advisory validate", stdout,
		"Validate advisory model output without creating review authority.\n\nForm: --repository-context <handle> --lens <lens> --input <result.json>\n\nThe binding used to validate --input is rebuilt from the exact same repository context and lens `review advisory prompt` and `review lens-context` consume; there is no caller-authored request.")
	repositoryContext := flags.String("repository-context", "", "opaque provider-issued repository context")
	lens := flags.String("lens", "", "exact selected lens")
	inputPath := flags.String("input", "", "model result JSON")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 || strings.TrimSpace(*repositoryContext) == "" || strings.TrimSpace(*lens) == "" || strings.TrimSpace(*inputPath) == "" {
		return reviewPreflightError(errors.New("review advisory validate requires the exact provider-issued repository context, lens, and --input; run `gentle-ai review advisory validate` with no arguments to see its exact closed command form"))
	}
	request, err := resolveAdvisoryRequest(ctx, reviewLensContextDependencies(), strings.TrimSpace(*repositoryContext), strings.TrimSpace(*lens))
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(strings.TrimSpace(*inputPath))
	if err != nil {
		return reviewPreflightError(fmt.Errorf("read advisory result: %w", err))
	}
	validated, err := advisoryreview.Validate(raw, request)
	if err != nil {
		return reviewPreflightError(err)
	}
	return encodeReviewJSON(stdout, validated)
}

// resolveAdvisoryRequest derives the complete provider-owned advisory request
// from the same frozen candidate authority `review lens-context` resolves
// (resolveReviewLensAuthority): an opaque repository context plus the
// selected lens are the only inputs, and every other field -- subject,
// manifest, and per-path evidence -- comes from native authority, never from
// a caller-authored request.
//
// Evidence assembly reuses the exact aggregate budget reviewLensContextBlock
// enforces (reviewLensContextByteBudget), refusing incrementally as soon as
// the running total would exceed it, rather than materializing an unbounded
// candidate in memory before a single check. This is the same refuse-before-
// invocation, never-truncate discipline, applied to the same budget, not a
// second competing one.
func resolveAdvisoryRequest(ctx context.Context, deps reviewLensContextDeps, repositoryContext, lens string) (request advisoryreview.Request, err error) {
	authority, err := resolveReviewLensAuthority(ctx, deps, repositoryContext, lens)
	if err != nil {
		return advisoryreview.Request{}, err
	}
	defer func() {
		request, err = reviewLensContextCleanup(ctx, request, err, func() error { return deps.close(authority.Inspector) })
	}()
	if len(authority.Frozen.ChangedPathManifest) > advisoryreview.MaxEvidenceEntries {
		return advisoryreview.Request{}, reviewLensContextRefusal("lens_context_budget_exceeded", reviewLensContextCapacityAction(len(authority.Frozen.ChangedPathManifest)))
	}
	budget := reviewLensContextByteBudget
	evidence := make([]advisoryreview.Evidence, 0, len(authority.Frozen.ChangedPathManifest))
	for index, entry := range authority.Frozen.ChangedPathManifest {
		payload, err := deps.inspect(ctx, authority.Inspector, "patch", index, "")
		if err != nil {
			return advisoryreview.Request{}, reviewLensContextInspectionFailure(ctx, err)
		}
		// An empty patch for a path that is neither mode-only nor deleted is
		// never legitimate evidence -- see reviewLensContextBlock, which this
		// mirrors exactly so the two surfaces never diverge on the same
		// fabricate-a-clean-review defect.
		if len(bytes.TrimSpace(payload)) == 0 && !entry.ModeOnly && !entry.Deleted {
			return advisoryreview.Request{}, reviewLensContextRefusal("lens_context_empty_patch", reviewLensContextEmptyPatchAction)
		}
		budget -= len(entry.Path) + len(payload)
		if budget < 0 {
			return advisoryreview.Request{}, reviewLensContextRefusal("lens_context_budget_exceeded", reviewLensContextBudgetAction)
		}
		evidence = append(evidence, advisoryreview.Evidence{Path: entry.Path, Content: string(payload)})
	}
	return advisoryreview.Request{
		ArtifactSubject: authority.Subject, ChangedPathManifest: authority.Frozen.ChangedPathManifest, Evidence: evidence,
	}, nil
}
