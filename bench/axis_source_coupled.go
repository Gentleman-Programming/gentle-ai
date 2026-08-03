package main

import (
	"errors"
	"fmt"
	"os"
)

const sourceCoupledAxis = "source-coupled"

const sourceCoupledReceiptContentKey = "source-coupled-receipt-content"

var errSourceCoupledFixtureUnavailable = errors.New("source-coupled receipt-drift seam unavailable")

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
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddSourceCoupledStatusFailsClosed},
			),
		},
	}
}

func sddSourceCoupledStatusFailsClosed(sandbox *Sandbox, observation Observation) error {
	before, ok := sandbox.Scratch[sourceCoupledReceiptContentKey]
	if !ok {
		return fmt.Errorf("source-coupled fixture did not record the receipt contents")
	}
	after, err := os.ReadFile(sandbox.BenchReceiptMutationPath)
	if err != nil {
		return fmt.Errorf("read source-coupled fixture receipt: %w", err)
	}
	if string(after) == before {
		return errSourceCoupledFixtureUnavailable
	}
	return sddStatusFailsClosed("compact authority changed during discovery")(sandbox, observation)
}
