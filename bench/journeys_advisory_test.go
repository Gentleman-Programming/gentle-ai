package main

// journeys_advisory_test.go backs the #2777 canonical-marker unification
// (change #3138, slice 5; SEN-RPC-10): the advisory journey corpus must stay
// byte-identical across the change. The five journeys named below are the
// REAL byte-identity corpus in journeys_advisory.go, not invented rows —
// binding their exact IDs is what lets a removed or renamed journey fail
// loudly instead of silently shrinking the corpus (the same convention as the
// 98-journey count tripwire in journeys_sdd_test.go).
//
// The byte-identity argument is threefold, and each half is pinned here:
//
//  1. The corpus exists: every advisory journey ID is registered and the
//     advisory count is exactly 5 (a sixth journey must bump this test's
//     expectation deliberately, never unnoticed).
//  2. The corpus is marker-free: no REVIEW_CONTEXT marker string may ever
//     appear in journeys_advisory.go. The advisory prompt is rendered by the
//     runtime-independent advisoryreview.Prompt instruction, whose stdout is
//     exactly what j68/j76 record — a journey that started naming a
//     per-runtime lens-context marker would corrupt the byte-identical corpus
//     the moment it landed.
//  3. The live byte-identity proof survives: j68 must still drive the
//     advertised runtimes (claude-code, opencode) through the shared advisory
//     prompt steps in order, with the final step still carrying its
//     byte-identity assertion (assertAdvisoryPromptsMatch). If the corpus
//     drops or reorders a runtime, this fails. Codex's unavailable-runtime
//     refusal lives in j69 (it is wired but unadvertised since the #3138
//     slice-8 advertisement flip), never as a prompt-producing step here.
//
// STOP gate (SEN-RPC-10): if the recorded corpus ever diverges when a shared
// prompt changes, the slice stops pending re-scope. A passing corpus run
// (`gentle-ai-bench run --only j68...,j76...`) is the live half of this
// guard; these tests pin the registered half so a silent corpus shrink cannot
// make the live run pass for the wrong reason.

import (
	"os"
	"strings"
	"testing"
)

// advisoryByteIdentityJourneyIDs are the exact IDs of the advisory journeys
// in journeys_advisory.go whose recorded observations must remain
// byte-identical across the canonical-marker unification.
var advisoryByteIdentityJourneyIDs = []string{
	"j68-review-advisory-prompt-shared-across-runtimes",
	"j69-review-advisory-prompt-refuses-unselected-lens-and-unadvertised-runtime",
	"j70-review-advisory-validate-admits-clean-result",
	"j71-review-advisory-validate-refuses-mismatched-subject",
	"j76-claude-advisory-result-reaches-delivery",
}

// advisoryByteIdentityJourneySet maps each required journey ID to whether it
// was found in the corpus, mirroring portableSDDFailClosedAuthorityJourneySet.
func advisoryByteIdentityJourneySet(found bool) map[string]bool {
	journeys := make(map[string]bool, len(advisoryByteIdentityJourneyIDs))
	for _, id := range advisoryByteIdentityJourneyIDs {
		journeys[id] = found
	}
	return journeys
}

func TestAdvisoryByteIdentityJourneysAreRegistered(t *testing.T) {
	want := advisoryByteIdentityJourneySet(false)
	advisory := 0
	seen := map[string]bool{}
	for _, journey := range Journeys() {
		if seen[journey.ID] {
			t.Errorf("journey ID %q collides in the corpus", journey.ID)
		}
		seen[journey.ID] = true
		if _, ok := want[journey.ID]; ok {
			want[journey.ID] = true
		}
		for _, step := range journey.Steps {
			if strings.Contains(step.Name, "advisory") {
				advisory++
				break
			}
		}
	}
	if got := len(advisoryByteIdentityJourneyIDs); got != 5 {
		t.Fatalf("advisory byte-identity journey list has %d entries, want 5", got)
	}
	// advisory counts every distinct journey whose Steps name an advisory
	// step, mirroring the corpus-count tripwire's role: a journey cannot
	// appear or vanish unnoticed.
	for id, found := range want {
		if !found {
			t.Errorf("required advisory byte-identity journey %q is not registered", id)
		}
	}
	if advisory != 5 {
		t.Errorf("advisory journey count = %d, want 5 (bump deliberately when a journey is added or removed)", advisory)
	}
}

func TestAdvisoryByteIdentityJourneySourceNamesNoReviewContextMarker(t *testing.T) {
	source, err := os.ReadFile("journeys_advisory.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"GENTLE_AI_REVIEW_CONTEXT",
		"GENTLE_AI_CLAUDE_REVIEW_CONTEXT",
		"GENTLE_AI_REVIEW_CONTEXT_END",
		"GENTLE_AI_REVIEW_BINDING",
	} {
		if strings.Contains(string(source), marker) {
			t.Errorf("journeys_advisory.go names %q; the advisory corpus must stay outside the lens-context marker domain (SEN-RPC-10 byte-identical corpus)", marker)
		}
	}
}

func TestAdvisoryByteIdentityJourneyKeepsSharedPromptAssertion(t *testing.T) {
	journey, ok := advisoryJourneyByID("j68-review-advisory-prompt-shared-across-runtimes")
	if !ok {
		t.Fatal("j68-review-advisory-prompt-shared-across-runtimes is not registered")
	}
	var runtimes []string
	var assertionStep *Step
	for index := range journey.Steps {
		step := &journey.Steps[index]
		runtime, isPrompt := strings.CutPrefix(step.Name, "advisory prompt for ")
		if !isPrompt {
			continue
		}
		runtimes = append(runtimes, runtime)
		if step.After != nil {
			assertionStep = step
		}
	}
	if joined := strings.Join(runtimes, ","); joined != "claude-code,opencode" {
		t.Fatalf("j68 advisory prompt steps = %q, want claude-code,opencode in order", joined)
	}
	if assertionStep == nil {
		t.Fatal("j68's final runtime prompt step no longer carries the byte-identity After assertion (assertAdvisoryPromptsMatch)")
	}
	if assertionStep.Name != "advisory prompt for opencode" {
		t.Fatalf("j68 byte-identity assertion is on step %q, want the opencode step so all advertised prompts are compared", assertionStep.Name)
	}
}

func advisoryJourneyByID(id string) (Journey, bool) {
	for _, journey := range Journeys() {
		if journey.ID == id {
			return journey, true
		}
	}
	return Journey{}, false
}
