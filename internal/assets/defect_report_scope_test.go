package assets

import (
	"strings"
	"testing"
)

// The automated defect handoff was filing other projects' failures.
//
// Its admissibility test read "when the consumer workflow appears blocked by a
// Gentle AI provider or tool defect", and from the user's seat everything that
// blocks them appears to qualify: a model refusing an image for exceeding its
// context window, a client runtime that needs its session restarted, a
// sub-agent that returns nothing. None of those are ours, and a report system
// that files other people's defects stops meaning anything when it files ours.
//
// Consent never filtered this. Asking the user authorizes a report, it does not
// classify one, and a user whose work is blocked answers yes.
//
// The discriminator is what PRODUCED the failure, not what the user happened to
// be doing when it happened. A Gentle AI workflow merely HOSTING a failure is
// not enough: an SDD phase is carried out by the client runtime, so that
// runtime failing mid-phase is that runtime's defect even though our contract
// prescribed the phase.
//
// Nothing is routed elsewhere. Naming another project as responsible is a claim
// that has to be right, it is not our work to triage, and it reintroduces the
// prompt noise this exists to remove. Out of scope means silent.

func TestDefectHandoffAdmitsOnlyFailuresGentleAIProduced(t *testing.T) {
	for _, path := range allSDDOrchestratorAssetPaths(t) {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)

			// The gate itself: production, not context.
			for _, required := range []string{
				"what produced the failure",
				"a Gentle AI invocation produced it",
				"hosting a failure is not enough",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("defect handoff does not gate on production; missing %q", required)
				}
			}

			// The excluded classes, named concretely enough to be applied.
			for _, required := range []string{
				"model provider",
				"client runtime",
				"sub-agent",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("defect handoff does not name the excluded class %q", required)
				}
			}

			// Out of scope is silent: no routing, no naming, no asking.
			for _, required := range []string{
				"there is no report and no handoff",
				"Do not name the component",
				"do not ask",
			} {
				if !strings.Contains(content, required) {
					t.Errorf("out-of-scope path is not silent; missing %q", required)
				}
			}

			// The old subjective test must be gone, or both rules ship and the
			// looser one wins whenever they disagree.
			if strings.Contains(content, "appears blocked by a Gentle AI provider or tool defect") {
				t.Error("the superseded subjective admissibility test is still present")
			}
		})
	}
}
