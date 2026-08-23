package main

import "testing"

// TestTransitionCorpusRequiresRescopeCapability keeps unsupported
// classification tied to the command transitionRescope actually invokes. A
// reset probe can succeed while a binary lacks rescope, silently turning this
// axis's rescope coverage into a false completed result.
func TestTransitionCorpusRequiresRescopeCapability(t *testing.T) {
	want := map[string]map[string]bool{
		"tr01-consecutive-rescope-keeps-the-ledger-readable": {
			"narrow the objective once":                        false,
			"narrow it again, which is where the report lands": false,
		},
		"tr02-reset-after-rescope-keeps-the-ledger-readable": {
			"narrow the objective": false,
		},
		"tr04-reset-then-rescope-keeps-the-ledger-readable": {
			"then narrow the fresh objective": false,
		},
		"tr08-scope-change-after-a-delegated-handoff": {
			"now change the scope of the handed-off objective": false,
		},
		"tr10-scope-change-after-the-review-is-bound": {
			"now move the unbound objective": false,
		},
	}

	journeys := transitionJourneys()
	if got := len(journeys); got != 11 {
		t.Fatalf("transition journey count = %d, want 11", got)
	}
	for _, journey := range journeys {
		steps, tracked := want[journey.ID]
		if !tracked {
			continue
		}
		for _, step := range journey.Steps {
			found, tracked := steps[step.Name]
			if !tracked {
				continue
			}
			if step.Requires != sddAttemptRescopeCapability {
				t.Errorf("%s / %s capability = %#v, want sddAttemptRescopeCapability", journey.ID, step.Name, step.Requires)
			}
			if found {
				t.Errorf("%s / %s is listed more than once", journey.ID, step.Name)
			}
			steps[step.Name] = true
		}
	}
	for journeyID, steps := range want {
		for stepName, found := range steps {
			if !found {
				t.Errorf("%s / %s is missing from the transition corpus", journeyID, stepName)
			}
		}
	}
}
