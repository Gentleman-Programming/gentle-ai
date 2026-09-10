package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

// Provider runtime cost metadata is the provider-owned, versioned extension that
// lets a parent (gentle-pi) forecast a host-relay capture binding truthfully
// without hardcoding any Go-owned retry count. A collect input carries these
// arguments alongside its existing binding only when the runtime is the Pi host
// relay, and only on the three collect capture operations that may re-invoke
// the host (lens, refuter, targeted validator).
//
// The surface is intentionally tiny and additive:
//
//   --provider-cost-version   A schema URL the parent uses to recognize the
//                             shape. The provider never silently extends this
//                             in-place; every change is an explicit, named bump.
//   --provider-model-runs-min The minimum number of underlying model runs the
//                             provider commits to per slot. Always >= 1.
//   --provider-model-runs-max The maximum number of underlying model runs the
//                             provider commits to per slot. >= min and bounded
//                             (lens/refuter/validator never exceed 2 in this
//                             version). 0 means "min-only".
//   --provider-retry-reason   The only admission class the provider will re-
//                             invoke for. Strict admission failures and Go-owned
//                             provider-role contract failures are the only
//                             reasons that may appear here, and the enum is
//                             explicit so the parent never has to infer.
//
// Admission and retry semantics stay exactly where they are: the capture
// command still runs Go's existing bounded corrective retry unchanged. This
// metadata only tells a parent how many host-relay invocations one
// collect binding may authorize; it does not author or relax admission.
const reviewProviderRuntimeMetadataVersion = "gentle-ai.review-provider-runtime-cost/v1"

// providerRuntimeMetadataArgumentCount is the fixed width of one rendered
// metadata block (one version, two run-count bounds, and one comma-joined
// retry-reason list). It pins the trailing-argument tail a valid
// host-relay-metadata collect input may carry.
const providerRuntimeMetadataArgumentCount = 4

// reviewProviderRuntimeMetadata is the parsed shape a parent reads off the
// provider-issued collect transition. Each field maps 1:1 to the argv argument
// of the same name (see reviewProviderRenderRuntimeMetadataArguments).
type reviewProviderRuntimeMetadata struct {
	Version      string
	ModelRunsMin int
	ModelRunsMax int
	// RetryReasons lists the exact admission classes that may trigger a
	// corrective re-invocation. Empty means "no metadata, no extra runs".
	RetryReasons []string
}

// reviewProviderRuntimeMetadataFor returns the metadata the provider renders on
// a host-relay collect binding, or the zero value when no metadata applies.
//
// Metadata only applies to the three collect capture operations that may run
// the corrective retry (lens, refuter, targeted validator); it never applies
// to compiled-run capture flows, which carry no host-relay relay.
func reviewProviderRuntimeMetadataFor(role reviewerprovider.Role) (reviewProviderRuntimeMetadata, bool) {
	if role != reviewerprovider.RoleLens && role != reviewerprovider.RoleRefuter && role != reviewerprovider.RoleTargetedValidator {
		return reviewProviderRuntimeMetadata{}, false
	}
	minRuns, maxRuns := 1, maxReviewerResultAdmissionAttempts
	return reviewProviderRuntimeMetadata{
		Version:      reviewProviderRuntimeMetadataVersion,
		ModelRunsMin: minRuns,
		ModelRunsMax: maxRuns,
		RetryReasons: []string{
			"provider_admission_refused",
			"provider_role_contract_violation",
		},
	}, true
}

// reviewProviderRenderRuntimeMetadataArguments converts one metadata record
// into the canonical argv arguments the parent must echo verbatim on the
// collect input. The arguments are emitted after the existing lens/order
// binding arguments and before the runtime-specific provider arguments
// (--agent, --execute) so a parent that does not recognize the schema can
// still parse the binding without crashing on unknown names.
//
// All four argument names are unique on purpose: the Go-owned
// `reviewTransitionArgumentMap` rejects duplicate names anywhere on a collect
// input, and bundling the retry reasons under a single CSV-suffixed argument
// keeps every name distinct while still letting a parent recover the full
// list through the standard argv parser.
func reviewProviderRenderRuntimeMetadataArguments(meta reviewProviderRuntimeMetadata) []ReviewTransitionArgument {
	if meta.Version == "" {
		return nil
	}
	return []ReviewTransitionArgument{
		{Name: "provider_cost_version", Value: meta.Version},
		{Name: "provider_model_runs_min", Value: strconv.Itoa(meta.ModelRunsMin)},
		{Name: "provider_model_runs_max", Value: strconv.Itoa(meta.ModelRunsMax)},
		{Name: "provider_retry_reasons", Value: strings.Join(meta.RetryReasons, ",")},
	}
}

// reviewProviderParseRuntimeMetadataArguments is the symmetrical reader a
// parent (and the provider's own transition validator) uses to recover the
// metadata record off a collect transition. The retry reasons are decoded out
// of one comma-separated argument so the collect envelope stays duplicate-name
// free; an empty value is the no-metadata signal and yields an empty reasons
// slice.
func reviewProviderParseRuntimeMetadataArguments(arguments []ReviewTransitionArgument) (reviewProviderRuntimeMetadata, bool, error) {
	var (
		hasVersion bool
		version    string
		minRuns    = -1
		maxRuns    = -1
		reasons    []string
	)
	for _, argument := range arguments {
		switch argument.Name {
		case "provider_cost_version":
			hasVersion = true
			version = strings.TrimSpace(argument.Value)
		case "provider_model_runs_min":
			n, err := strconv.Atoi(argument.Value)
			if err != nil || n < 0 {
				return reviewProviderRuntimeMetadata{}, false, fmt.Errorf("provider_model_runs_min is not a non-negative integer: %s", argument.Value) // refusal:by-design operator-knowledge: the provider-issued metadata block must carry an integer min, so re-query the negotiate transition to obtain a valid block
			}
			minRuns = n
		case "provider_model_runs_max":
			n, err := strconv.Atoi(argument.Value)
			if err != nil || n < 0 {
				return reviewProviderRuntimeMetadata{}, false, fmt.Errorf("provider_model_runs_max is not a non-negative integer: %s", argument.Value) // refusal:by-design operator-knowledge: the provider-issued metadata block must carry an integer max, so re-query the negotiate transition to obtain a valid block
			}
			maxRuns = n
		case "provider_retry_reasons":
			for _, reason := range strings.Split(argument.Value, ",") {
				trimmed := strings.TrimSpace(reason)
				if trimmed != "" {
					reasons = append(reasons, trimmed)
				}
			}
		}
	}
	if !hasVersion {
		return reviewProviderRuntimeMetadata{}, false, nil
	}
	if version != reviewProviderRuntimeMetadataVersion {
		return reviewProviderRuntimeMetadata{}, false, fmt.Errorf("unsupported provider_cost_version %q", version) // refusal:by-design operator-knowledge: only the provider-issued metadata schema is admitted, so refresh gentle-ai and re-query gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --next-transition
	}
	if minRuns < 0 || maxRuns < 0 {
		return reviewProviderRuntimeMetadata{}, false, errors.New("provider_cost_version requires provider_model_runs_min and provider_model_runs_max") // refusal:by-design operator-knowledge: the metadata block must carry both bounds, so re-query the negotiate transition to obtain a valid block
	}
	if minRuns < 1 {
		return reviewProviderRuntimeMetadata{}, false, errors.New("provider_model_runs_min must be >= 1") // refusal:by-design operator-knowledge: a zero-or-negative min run bound would silently authorize no underlying model run
	}
	if maxRuns < minRuns {
		return reviewProviderRuntimeMetadata{}, false, errors.New("provider_model_runs_max must be >= provider_model_runs_min") // refusal:by-design operator-knowledge: the max bound is a truthful forecast upper, never below the committed min
	}
	return reviewProviderRuntimeMetadata{
		Version:      version,
		ModelRunsMin: minRuns,
		ModelRunsMax: maxRuns,
		RetryReasons: reasons,
	}, true, nil
}

// reviewProviderAppendRuntimeMetadataArguments appends the metadata arguments
// to an existing argv builder. Returns the same slice unchanged when the
// metadata record is empty.
func reviewProviderAppendRuntimeMetadataArguments(arguments []ReviewTransitionArgument, meta reviewProviderRuntimeMetadata) []ReviewTransitionArgument {
	return append(arguments, reviewProviderRenderRuntimeMetadataArguments(meta)...)
}
