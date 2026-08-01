package main

const sourceCoupledAxis = "source-coupled"

func init() {
	RegisterAxis(Axis{
		Name:     sourceCoupledAxis,
		Title:    "Source-built receipt-drift proof",
		BlackBox: false,
		Properties: []string{
			"j57 requires the product's `bench_fixture` build tag to mutate a sandbox receipt between authority discovery reads; ordinary product binaries do not expose that seam.",
			"The portable black-box core excludes j57 and contains 57 journeys. Select this axis explicitly and build the product with `-tags bench_fixture` to run the proof.",
			"The fixture changes only its fresh sandbox receipt and asserts the product's fail-closed `compact authority changed during discovery` result.",
		},
		Journeys: sourceCoupledJourneys,
	})
}

func sourceCoupledJourneys() []Journey {
	return []Journey{
		{
			ID:     "j57-sdd-authority-drift-during-discovery-fails-closed",
			Title:  "Authority receipt changes during discovery: SDD fails closed",
			Source: "compact authority discovery contract: immutable authority reads must agree",
			Steps: append(sddApprovedAuthoritySteps(sddSingleAuthorityFixture),
				Step{Name: "fixture: select the sandbox-only receipt drift seam", Fixture: sddInstallDiscoveryDriftFixture},
				Step{Name: "sdd-status refuses authority drift", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddStatusFailsClosed("compact authority changed during discovery")},
			),
		},
	}
}
