package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

// TestReviewProviderRuntimeMetadataRenderParseRoundtrip pins the canonical
// argv projection for every role that publishes metadata, and asserts the
// inverse parse recovers the exact record the renderer wrote.
func TestReviewProviderRuntimeMetadataRenderParseRoundtrip(t *testing.T) {
	for _, role := range []reviewerprovider.Role{
		reviewerprovider.RoleLens,
		reviewerprovider.RoleRefuter,
		reviewerprovider.RoleTargetedValidator,
	} {
		t.Run(string(role), func(t *testing.T) {
			meta, ok := reviewProviderRuntimeMetadataFor(role)
			if !ok {
				t.Fatalf("role %q must publish metadata", role)
			}
			if meta.Version != reviewProviderRuntimeMetadataVersion {
				t.Fatalf("role %q version = %q, want %q", role, meta.Version, reviewProviderRuntimeMetadataVersion)
			}
			if meta.ModelRunsMin < 1 {
				t.Fatalf("role %q min = %d, want >= 1", role, meta.ModelRunsMin)
			}
			if meta.ModelRunsMax < meta.ModelRunsMin {
				t.Fatalf("role %q max = %d, want >= min (%d)", role, meta.ModelRunsMax, meta.ModelRunsMin)
			}
			if len(meta.RetryReasons) == 0 {
				t.Fatalf("role %q must publish at least one retry reason", role)
			}
			rendered := reviewProviderRenderRuntimeMetadataArguments(meta)
			if len(rendered) != providerRuntimeMetadataArgumentCount {
				t.Fatalf("role %q rendered = %d args, want %d", role, len(rendered), providerRuntimeMetadataArgumentCount)
			}
			got, present, err := reviewProviderParseRuntimeMetadataArguments(rendered)
			if err != nil {
				t.Fatalf("role %q parse: %v", role, err)
			}
			if !present {
				t.Fatalf("role %q parser must treat its own render as present", role)
			}
			if !reflect.DeepEqual(got, meta) {
				t.Fatalf("role %q roundtrip lost fidelity: got %#v want %#v", role, got, meta)
			}
		})
	}
}

// TestReviewProviderRuntimeMetadataRenderUsesDistinctNames pins the four
// argument names that go on every collect binding. The Go-owned argument map
// rejects duplicates, so each name in the trailing metadata block must remain
// unique within the binding and must not collide with the binding arguments
// the provider already renders above it.
func TestReviewProviderRuntimeMetadataRenderUsesDistinctNames(t *testing.T) {
	meta, ok := reviewProviderRuntimeMetadataFor(reviewerprovider.RoleLens)
	if !ok {
		t.Fatal("lens must publish metadata")
	}
	seen := map[string]bool{}
	for _, argument := range reviewProviderRenderRuntimeMetadataArguments(meta) {
		if seen[argument.Name] {
			t.Fatalf("duplicate metadata argument name %q", argument.Name)
		}
		seen[argument.Name] = true
	}
	want := []string{"provider_cost_version", "provider_model_runs_min", "provider_model_runs_max", "provider_retry_reasons"}
	got := make([]string, 0, len(want))
	for _, argument := range reviewProviderRenderRuntimeMetadataArguments(meta) {
		got = append(got, argument.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata argument names = %v, want %v", got, want)
	}
}

// TestReviewProviderRuntimeMetadataParserRejectsUnknownVersion fails closed on
// any version the provider does not recognize: the parent and the validator
// must never silently extend the schema in-place.
func TestReviewProviderRuntimeMetadataParserRejectsUnknownVersion(t *testing.T) {
	arguments := []ReviewTransitionArgument{
		{Name: "provider_cost_version", Value: "gentle-ai.review-provider-runtime-cost/v9"},
		{Name: "provider_model_runs_min", Value: "1"},
		{Name: "provider_model_runs_max", Value: "1"},
		{Name: "provider_retry_reasons", Value: "provider_admission_refused"},
	}
	_, _, err := reviewProviderParseRuntimeMetadataArguments(arguments)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider_cost_version") {
		t.Fatalf("unknown version err = %v, want unsupported provider_cost_version", err)
	}
}

// TestReviewProviderRuntimeMetadataParserRejectsInvalidBounds fails closed on
// any bound that violates the min/max invariants a parent would otherwise have
// to re-derive from runtime counters.
func TestReviewProviderRuntimeMetadataParserRejectsInvalidBounds(t *testing.T) {
	cases := []struct {
		name     string
		min, max string
		wantErr  string
	}{
		{name: "min zero", min: "0", max: "2", wantErr: "provider_model_runs_min must be >= 1"},
		{name: "max below min", min: "2", max: "1", wantErr: "provider_model_runs_max must be >= provider_model_runs_min"},
		{name: "min not integer", min: "abc", max: "2", wantErr: "provider_model_runs_min is not a non-negative integer"},
		{name: "max not integer", min: "1", max: "x", wantErr: "provider_model_runs_max is not a non-negative integer"},
		{name: "missing min", min: "", max: "2", wantErr: "provider_model_runs_min is not a non-negative integer"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			arguments := []ReviewTransitionArgument{
				{Name: "provider_cost_version", Value: reviewProviderRuntimeMetadataVersion},
				{Name: "provider_model_runs_min", Value: tt.min},
				{Name: "provider_model_runs_max", Value: tt.max},
				{Name: "provider_retry_reasons", Value: "provider_admission_refused"},
			}
			_, _, err := reviewProviderParseRuntimeMetadataArguments(arguments)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("invalid bounds err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

// TestReviewProviderRuntimeMetadataForUnlistedRoles confirms that roles outside
// the three-collect-operation contract surface return the zero value with no
// metadata rendered. Nothing else may project a cost block on a host binding.
func TestReviewProviderRuntimeMetadataForUnlistedRoles(t *testing.T) {
	unlisted := []reviewerprovider.Role{
		"",
		"unknown-role",
		"lens-helper",
		"refuter-fallback",
	}
	for _, role := range unlisted {
		t.Run(string(role), func(t *testing.T) {
			meta, ok := reviewProviderRuntimeMetadataFor(role)
			if ok {
				t.Fatalf("unlisted role %q must not publish metadata, got %#v", role, meta)
			}
			if meta.Version != "" || meta.ModelRunsMin != 0 || meta.ModelRunsMax != 0 || meta.RetryReasons != nil {
				t.Fatalf("unlisted role %q must return zero metadata, got %#v", role, meta)
			}
			rendered := reviewProviderRenderRuntimeMetadataArguments(meta)
			if len(rendered) != 0 {
				t.Fatalf("unlisted role %q render = %d args, want 0", role, len(rendered))
			}
		})
	}
}

// TestReviewProviderAppendRuntimeMetadataArgumentsNoOpOnEmpty ensures the
// appender is a no-op when the metadata record is empty, so callers that
// unconditionally forward the resulting slice never inflate existing argv.
func TestReviewProviderAppendRuntimeMetadataArgumentsNoOpOnEmpty(t *testing.T) {
	base := []ReviewTransitionArgument{{Name: "agent", Value: "pi"}, {Name: "execute", Value: "true"}}
	got := reviewProviderAppendRuntimeMetadataArguments(base, reviewProviderRuntimeMetadata{})
	if len(got) != len(base) {
		t.Fatalf("empty append grew the slice: got %d args, want %d", len(got), len(base))
	}
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("empty append mutated the slice: got %#v want %#v", got, base)
	}
}

// TestReviewProviderAppendRuntimeMetadataArgumentsAppendsCanonicalBlock pins
// the append order: existing arguments first, then the four canonical metadata
// arguments. The provider render path relies on this ordering so a parent
// reading the trailing tail off any collect input sees the canonical block.
func TestReviewProviderAppendRuntimeMetadataArgumentsAppendsCanonicalBlock(t *testing.T) {
	base := []ReviewTransitionArgument{{Name: "agent", Value: "pi"}, {Name: "execute", Value: "true"}}
	meta, ok := reviewProviderRuntimeMetadataFor(reviewerprovider.RoleRefuter)
	if !ok {
		t.Fatal("refuter must publish metadata")
	}
	got := reviewProviderAppendRuntimeMetadataArguments(base, meta)
	want := append([]ReviewTransitionArgument{}, base...)
	want = append(want, reviewProviderRenderRuntimeMetadataArguments(meta)...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("append ordering diverged: got %#v want %#v", got, want)
	}
	if len(got)-len(base) != providerRuntimeMetadataArgumentCount {
		t.Fatalf("appender added %d args, want %d", len(got)-len(base), providerRuntimeMetadataArgumentCount)
	}
}

// TestReviewProviderParseRuntimeMetadataArgumentsAbsentVersion is the inherited
// case: an input with no metadata block must round-trip as "not present" with
// no error, so existing transitions that do not carry the trailing block
// continue to validate.
func TestReviewProviderParseRuntimeMetadataArgumentsAbsentVersion(t *testing.T) {
	arguments := []ReviewTransitionArgument{{Name: "agent", Value: "pi"}, {Name: "execute", Value: "true"}}
	meta, present, err := reviewProviderParseRuntimeMetadataArguments(arguments)
	if err != nil {
		t.Fatalf("absent metadata parse err = %v, want nil", err)
	}
	if present {
		t.Fatalf("absent metadata present = true, want false")
	}
	if !reflect.DeepEqual(meta, reviewProviderRuntimeMetadata{}) {
		t.Fatalf("absent metadata returned non-zero %#v", meta)
	}
}

// TestReviewProviderParseRuntimeMetadataArgumentsDropsEmptyRetryReasons
// guarantees the comma-joined retry-reasons argument never produces empty
// entries when callers hand us a trailing comma or whitespace.
func TestReviewProviderParseRuntimeMetadataArgumentsDropsEmptyRetryReasons(t *testing.T) {
	arguments := []ReviewTransitionArgument{
		{Name: "provider_cost_version", Value: reviewProviderRuntimeMetadataVersion},
		{Name: "provider_model_runs_min", Value: "1"},
		{Name: "provider_model_runs_max", Value: "2"},
		{Name: "provider_retry_reasons", Value: " provider_admission_refused , , provider_role_contract_violation ,"},
	}
	meta, present, err := reviewProviderParseRuntimeMetadataArguments(arguments)
	if err != nil {
		t.Fatalf("retry-reasons parse err = %v", err)
	}
	if !present {
		t.Fatal("retry-reasons parse present = false, want true")
	}
	want := []string{"provider_admission_refused", "provider_role_contract_violation"}
	if !reflect.DeepEqual(meta.RetryReasons, want) {
		t.Fatalf("retry-reasons = %#v, want %#v", meta.RetryReasons, want)
	}
}

// TestReviewProviderParseRuntimeMetadataArgumentsAcceptsEmptyRetryReasons
// confirms the parser accepts the v1 schema with an empty retry-reasons value
// rather than treating the absence of reasons as a refusal. An empty reason
// list simply means the metadata block carries no extra runs.
func TestReviewProviderParseRuntimeMetadataArgumentsAcceptsEmptyRetryReasons(t *testing.T) {
	arguments := []ReviewTransitionArgument{
		{Name: "provider_cost_version", Value: reviewProviderRuntimeMetadataVersion},
		{Name: "provider_model_runs_min", Value: "1"},
		{Name: "provider_model_runs_max", Value: "1"},
		{Name: "provider_retry_reasons", Value: ""},
	}
	meta, present, err := reviewProviderParseRuntimeMetadataArguments(arguments)
	if err != nil {
		t.Fatalf("empty retry-reasons parse err = %v", err)
	}
	if !present || len(meta.RetryReasons) != 0 {
		t.Fatalf("empty retry-reasons metadata = %#v, present = %v", meta, present)
	}
}

