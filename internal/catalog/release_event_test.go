package catalog

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestReleaseIsARepresentableReceiptValidationEvent(t *testing.T) {
	found := false
	for _, event := range SupportedTriggerEvents() {
		if event == model.EventRelease {
			found = true
		}
	}
	if !found {
		t.Fatal("SupportedTriggerEvents() does not include release")
	}
	set := model.TriggerRuleSet{Events: []model.TriggerEvent{model.EventRelease}, Bindings: []model.TriggerBinding{{
		On: model.EventRelease, When: model.TriggerWhen{Always: true}, Run: []string{"review-readability"}, Mode: model.ModeAdvisory,
	}}}
	if err := ValidateTriggerRuleSet(set); err != nil {
		t.Fatalf("ValidateTriggerRuleSet(release) error = %v", err)
	}
}
