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
			"The fixture changes only its fresh sandbox receipt and asserts the product ignores the drift pre-verify (corrective verify cycle 3: the pre-verify `compact authority changed during discovery` consultation was superseded by Wave 4's post-verify-only review consultation).",
		},
		Journeys: sourceCoupledJourneys,
	})
}

func sourceCoupledJourneys() []Journey {
	return []Journey{
		{
			ID:     "j57-sdd-authority-drift-during-discovery-fails-closed",
			Review: reviewOptedIn,
			Title:  "Authority receipt changes during discovery: not consulted before verify",
			Source: "compact authority discovery contract (superseded pre-verify half, corrective verify cycle 3 CRITICAL-C) + Wave 4's post-verify-only review consultation",
			Steps: append(sddApprovedAuthoritySteps(sddSingleAuthorityFixture),
				Step{Name: "fixture: select the sandbox-only receipt drift seam", Fixture: sddInstallDiscoveryDriftFixture},
				Step{Name: "sdd-status ignores authority drift pre-verify", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: sddSourceCoupledStatusIgnoresDriftPreVerify},
			),
		},
	}
}

// sddSourceCoupledStatusIgnoresDriftPreVerify (formerly
// sddSourceCoupledStatusFailsClosed) pinned the same deleted
// `applyPreVerifyCompactBridgeRouting` mechanism j53-j58 pinned in
// journeys_sdd.go -- see sddStatusIgnoresCorruptCompactAuthorityPreVerify's
// doc comment there for the full finding. The drift-seam proof itself (the
// fixture really did mutate the sandbox receipt between discovery reads)
// stays meaningful; only the expected product response changes.
func sddSourceCoupledStatusIgnoresDriftPreVerify(sandbox *Sandbox, observation Observation) error {
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
	return sddStatusIgnoresCorruptCompactAuthorityPreVerify("compact authority changed during discovery")(sandbox, observation)
}
