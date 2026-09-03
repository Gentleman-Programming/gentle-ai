package sdd

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

// relayAssemblyClause is the sentence that puts the complete immutable evidence
// on the parent. It is correct for a host-mediated runtime and wrong for one
// whose compiled transport captures in process.
const relayAssemblyClause = "The reviewer prompt begins with the exact literal prefix"

// TestInstalledContractNamesTheTransportEachRuntimeActuallyUses is the
// regression guard for issue #3825. The contract this product installs is the
// only thing a parent orchestrator reads before it acts, so a runtime whose
// compiled transport captures in process must not be told to assemble and relay
// a reviewer prompt: doing so makes the parent reproduce the whole candidate
// verbatim per lens, which measured roughly 145 KB per lens on a 60-file
// candidate and stops being reproducible at all as the candidate grows.
//
// The split is read from reviewerprovider rather than restated here, so this
// test fails if the contract and the dispatcher ever disagree about which
// runtimes capture in process — the drift that shipped as #2777.
func TestInstalledContractNamesTheTransportEachRuntimeActuallyUses(t *testing.T) {
	for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex, model.AgentOpenCode, model.AgentPi} {
		t.Run(string(agent), func(t *testing.T) {
			contract := boundedReviewContractFor(agent)
			if agent == model.AgentPi {
				for _, want := range []string{"`gentle_review_capture`", "`gentle_review_capture_group`", "Never run raw shell STATUS"} {
					if !strings.Contains(contract, want) {
						t.Errorf("Pi contract missing facade transport clause %q", want)
					}
				}
				if strings.Contains(contract, "gentle-ai review status") {
					t.Error("Pi contract exposes raw STATUS")
				}
				return
			}
			compiled := reviewerprovider.CapturesInProcess(agent)

			if compiled {
				if strings.Contains(contract, relayAssemblyClause) {
					t.Errorf("%s captures in process, but its installed contract still instructs the parent to assemble and relay a reviewer prompt", agent)
				}
				for _, want := range []string{"--agent", "captures in process", "Never assemble a reviewer prompt", "exactly as returned"} {
					if !strings.Contains(contract, want) {
						t.Errorf("%s contract does not tell the parent to run the returned tokens: missing %q", agent, want)
					}
				}
				return
			}

			// A host-mediated runtime has no in-process capture, so the relay is
			// its only path and must survive intact.
			if !strings.Contains(contract, relayAssemblyClause) {
				t.Errorf("%s is host-mediated and lost the relay instruction that is its only capture path", agent)
			}
		})
	}
}

// TestCompiledCaptureRuntimesComeFromOneSource keeps the predicate honest: the
// exported answer must name exactly the runtimes whose compiled adapters exist,
// so the contract cannot claim an in-process transport a runtime does not have.
func TestCompiledCaptureRuntimesComeFromOneSource(t *testing.T) {
	for agent, want := range map[model.AgentID]bool{
		model.AgentClaudeCode: true,
		model.AgentCodex:      true,
		model.AgentOpenCode:   false,
		model.AgentPi:         false,
		model.AgentKilocode:   false,
	} {
		if got := reviewerprovider.CapturesInProcess(agent); got != want {
			t.Errorf("CapturesInProcess(%s) = %v, want %v", agent, got, want)
		}
	}
}
