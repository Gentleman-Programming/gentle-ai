package main

const transitionAxis = "transition"

func init() {
	RegisterAxis(Axis{Name: transitionAxis, Title: "Mode changes during a live review", BlackBox: true, Review: reviewOptedIn, Properties: []string{"The sequence is produced through the CLI; disabling RDD leaves ordinary delivery available."}, Journeys: transitionJourneys})
}
func transitionJourneys() []Journey {
	return []Journey{
		{
			ID:     "tr09-mode-flip-while-a-review-lineage-is-open",
			Review: reviewOptedIn,
			Title:  "Turn review off while a review lineage is open",
			Source: "Standalone RDD mode changes preserve ordinary delivery",
			// The switch's own contract says delivery falls back to ordinary
			// repository policy. That is easy to honour when no review exists.
			// The interesting state is a started, unfinalized lineage: authority
			// exists, no receipt does, and the switch says stop consulting it.
			// If that combination wedges, the escape hatch fails exactly when
			// someone reaches for it.
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("transition")},
				{Name: "start a review and leave it open", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "turn review off with the lineage still open", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone")},
				{Name: "review status still answers", Requires: statusCapability,
					Args: productArgs("review", "status")},
				{Name: "and the delivery gate answers under ordinary policy",
					Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
	}
}
