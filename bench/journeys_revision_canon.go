package main

// Issue #2523, the surviving CLI-boundary half of PR #2308: both unambiguous
// non-canonical spellings of a SHA-256 evidence revision (the bare uppercase
// hex PowerShell's Get-FileHash prints, and the bare lowercase form) must
// settle identically to the canonical sha256:<64-lowercase-hex> input. The
// ledger contract stays byte-for-byte strict (#2395); only the argument
// boundary respells. waveFiveJourneys appends this file's registrar because
// the central aggregator was owned by open pull requests when it landed.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// canonEvidenceDigest is the journey's content digest in canonical lowercase
// hex, unprefixed so each step can spell it its own way.
const canonEvidenceDigest = "27a44ad82d6e4f1d9f2c3b4a5d6e7f8091a2b3c4d5e6f70818293a4b5c6d7e8f"

// These stateful compact capabilities keep their explicit probes so the
// benchmark can distinguish a missing operation from a valid state refusal.
var sddAttemptAcquireCapability = &Capability{
	Verb:  []string{"sdd-attempt", "acquire"},
	Probe: []string{"sdd-attempt", "acquire", "--work-unit=probe", "--evidence-goal=probe", "--max-attempts=1", "--max-changed-lines=1"},
}
var sddAttemptSettleCapability = &Capability{
	Verb:  []string{"sdd-attempt", "settle"},
	Probe: []string{"sdd-attempt", "settle", "--outcome=failed", "--evidence-revision=probe", "--harness-disposition=reused", "--cleanup-evidence=probe", "--process-evidence=probe"},
}

// sddAcquireCompactToken spends one counted acquire and returns the token
// settle must present to continue that same attempt.
func sddAcquireCompactToken(r *journeyRun, requestID string) (string, error) {
	observation := r.run([]string{
		"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange,
		"--request-id", requestID,
		"--work-unit", "canonical evidence spelling objective",
		"--evidence-goal", "both SHA-256 spellings settle identically",
		"--max-attempts", "4", "--max-changed-lines", "20",
	}, false)
	var result struct {
		State string `json:"state"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return "", fmt.Errorf("parse sdd-attempt acquire: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if result.State != "proceed" || result.Token == "" {
		return "", fmt.Errorf("acquire = %+v, want proceed with a token", result)
	}
	return result.Token, nil
}

// sddSettleEvidenceSpelling acquires, settles with the given spelling of
// canonEvidenceDigest, and proves the runtime recorded the canonical form —
// so every spelling this journey drives lands on the identical settlement.
func sddSettleEvidenceSpelling(spelling, requestID string) func(*journeyRun) error {
	return func(r *journeyRun) error {
		token, err := sddAcquireCompactToken(r, requestID+"-acquire")
		if err != nil {
			return err
		}
		observation := r.run([]string{
			"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange,
			"--token", token, "--request-id", requestID + "-settle", "--outcome", "failed",
			"--evidence-revision", spelling,
			"--diagnosis", "the benchmark drove this attempt to a terminal outcome",
			"--harness-disposition", "reused",
			"--cleanup-evidence", "workspace clean",
			"--process-evidence", "no stray processes",
		}, false)
		if observation.ExitCode != 0 {
			return fmt.Errorf("settle with evidence revision %q blocked: %s", spelling, firstLine(observation.Stderr))
		}
		status, err := proveRuntime(r.sandbox)
		if err != nil {
			return err
		}
		canonical := "sha256:" + canonEvidenceDigest
		if status.EvidenceRevision != canonical {
			return fmt.Errorf("settle with %q recorded evidence revision %q, want the canonical %q", spelling, status.EvidenceRevision, canonical)
		}
		return nil
	}
}

func revisionCanonJourneys() []Journey {
	return []Journey{
		{
			ID:     "j62-sdd-settle-canonicalizes-unambiguous-evidence-revision",
			Title:  "Compact settle: the PowerShell uppercase digest and the canonical form settle identically",
			Source: "issue #2523 + the maintainer scoping comment on PR #2308 (idea: @decode2; uppercase report: @adgarboc on #2294)",
			// Expected: before the CLI-boundary canonicalization, the uppercase
			// settle blocks with the ledger's canonical-format refusal — an
			// in_band block, because the refusal names the runnable rerun. After
			// it, both spellings proceed and the runtime records the identical
			// canonical sha256:<64-lowercase-hex> evidence revision, which the
			// composite proves through `sdd-attempt status` after each settle.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "settle with the PowerShell-style uppercase digest", Requires: sddAttemptSettleCapability,
					Composite: sddSettleEvidenceSpelling(strings.ToUpper(canonEvidenceDigest), "bench-canon-upper")},
				{Name: "settle with the canonical lowercase prefixed digest", Requires: sddAttemptSettleCapability,
					Composite: sddSettleEvidenceSpelling("sha256:"+canonEvidenceDigest, "bench-canon-lower")},
			},
		},
	}
}
