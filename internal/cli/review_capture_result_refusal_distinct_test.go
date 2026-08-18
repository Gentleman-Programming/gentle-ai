package cli

import (
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// TestCaptureResultHostMediatedRefusalsAreDistinguishable is #3441.
//
// `review capture-result` has two opposite host-mediated refusals: one fires
// inside `--materialize` when the runtime has no host-relay transport, the
// other fires without `--materialize` when the runtime is not a compiled
// capture runtime. They used to emit the byte-identical sentence
//
//	review capture-result provider runtime %q is host-mediated; use its live transport collection
//
// so a report that quoted it (Gentleman-Programming/gentle-pi#367, defect C)
// could not be traced to either branch -- and the two branches send the
// follow-up to different repositories. Each refusal now names the flag the
// caller passed, the runtime's compiled transport, and the one accepted form.
func TestCaptureResultHostMediatedRefusalsAreDistinguishable(t *testing.T) {
	binding := []string{
		"--repository-context", "rctx1_" + strings.Repeat("0", 64),
		"--lineage", "distinct-host-mediated-refusals", "--target", "sha256:" + strings.Repeat("0", 64),
		"--expected-revision", "sha256:" + strings.Repeat("1", 64), "--lens", "review-reliability", "--order", "0",
	}

	for _, test := range []struct {
		name string
		argv []string
		// want is the complete refusal, asserted exactly: a substring match
		// is what let two opposite causes share one sentence unnoticed.
		want string
	}{
		{
			// The materialize branch, reached by a runtime whose live host
			// transport never prints the Go-materialized task.
			name: "materialize requested for a non-host-relay transport",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentOpenCode), "--materialize=true"),
			want: `review capture-result --materialize is unavailable for "opencode": ` +
				`printing the Go-materialized provider task is the host-relay form, ` +
				`and this runtime's compiled transport is "opencode_provider_injected"; ` +
				`collect its reviewer result through that live host transport instead`,
		},
		{
			// The opposite branch: --materialize did not take effect, and the
			// host relay owns the reviewer subprocess this process cannot
			// launch. Naming --materialize=true here is what turns the
			// gentle-pi report into a checkable question.
			name: "materialize omitted for the pi host relay",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentPi)),
			want: `review capture-result --agent "pi" without --materialize has no in-process reviewer to run: ` +
				`its compiled transport is "pi_host_relay", whose host owns the reviewer subprocess; ` +
				`print the provider task with --materialize=true, run it in the host, ` +
				`then submit the raw result with --input and the same binding`,
		},
		{
			// Same branch as above, different runtime: OpenCode has no
			// materialize form to point at, so it gets its own sentence
			// rather than the pi one.
			name: "materialize omitted for a runtime with no materialize form",
			argv: append(slices.Clone(binding), "--agent", string(model.AgentOpenCode)),
			want: `review capture-result --agent "opencode" has no compiled in-process reviewer adapter: ` +
				`its compiled transport is "opencode_provider_injected"; ` +
				`collect its reviewer result through that live host transport instead`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			err := RunReviewCaptureResult(test.argv, io.Discard)
			if err == nil {
				t.Fatal("want a refusal")
			}
			if err.Error() != test.want {
				t.Fatalf("refusal =\n\t%q\nwant\n\t%q", err.Error(), test.want)
			}
		})
	}

	// The property the shared sentence violated: no two of these refusals may
	// read the same, whatever their wording becomes.
	t.Run("no two refusals share their text", func(t *testing.T) {
		seen := map[string]string{}
		for _, argv := range [][]string{
			append(slices.Clone(binding), "--agent", string(model.AgentOpenCode), "--materialize=true"),
			append(slices.Clone(binding), "--agent", string(model.AgentPi)),
			append(slices.Clone(binding), "--agent", string(model.AgentOpenCode)),
			append(slices.Clone(binding), "--agent", string(model.AgentClaudeCode), "--materialize=true"),
		} {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			err := RunReviewCaptureResult(argv, io.Discard)
			if err == nil {
				t.Fatalf("%v: want a refusal", argv)
			}
			if first, duplicate := seen[err.Error()]; duplicate {
				t.Fatalf("two refusals share one sentence:\n\t%v\n\t%v\n\t%q", first, argv, err.Error())
			}
			seen[err.Error()] = strings.Join(argv, " ")
		}
	})
}
