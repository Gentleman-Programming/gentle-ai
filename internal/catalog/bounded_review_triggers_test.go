package catalog

import (
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestDefaultTriggersStartReviewOnceAndReuseReceiptAtLifecycleGates(t *testing.T) {
	set := DefaultTriggerRuleSet()
	wantValidatorEvents := []model.TriggerEvent{
		model.EventPreCommit, model.EventPrePush, model.EventPrePR, model.EventRelease,
	}
	for _, event := range wantValidatorEvents {
		bindings := bindingsForEvent(set, event)
		if len(bindings) != 1 || !slices.Equal(bindings[0].Run, []string{"review-receipt-validator"}) {
			t.Fatalf("%s bindings = %#v, want one receipt validator", event, bindings)
		}
	}

	postApply := bindingsForEvent(set, model.EventPostSDDPhase)
	if len(postApply) != 1 || !slices.Equal(postApply[0].Run, []string{"review-start"}) || !slices.Equal(postApply[0].When.Phases, []string{"apply"}) {
		t.Fatalf("post-sdd-phase bindings = %#v, want explicit post-apply review-start", postApply)
	}

	for _, binding := range set.Bindings {
		for _, run := range binding.Run {
			if run == "judgment-day" || isLegacyReviewLens(run) {
				t.Fatalf("default lifecycle binding silently launches adversarial review: %#v", binding)
			}
		}
	}
}

func TestReviewLifecycleOperationsAreKnownTriggerTargets(t *testing.T) {
	known := KnownAgents()
	for _, want := range []string{"review-start", "review-receipt-validator"} {
		if !slices.Contains(known, want) {
			t.Fatalf("KnownAgents() missing %q", want)
		}
	}
}

func bindingsForEvent(set model.TriggerRuleSet, event model.TriggerEvent) []model.TriggerBinding {
	var result []model.TriggerBinding
	for _, binding := range set.Bindings {
		if binding.On == event {
			result = append(result, binding)
		}
	}
	return result
}

func isLegacyReviewLens(value string) bool {
	switch value {
	case "review-risk", "review-readability", "review-reliability", "review-resilience":
		return true
	default:
		return false
	}
}
