package main

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// journeySource is one journeys_*.go file and the journeys it contributes.
// Journey IDs are hand-assigned across these files, so the collision check
// must be able to name the two defining files: a failure that cannot say
// where both definitions live sends the author on a repo-wide grep.
type journeySource struct {
	File     string
	Journeys []Journey
}

// journeySources maps every journeys_*.go corpus file to its constructor.
// Adding a new journeys_*.go file means registering its constructor here;
// TestJourneySourcesCoverTheWholeCorpus fails until it is registered, so a
// new file cannot bypass the collision check silently.
func journeySources() []journeySource {
	return []journeySource{
		{"journeys.go", coreJourneys()},
		{"journeys_edge.go", edgeJourneys()},
		{"journeys_sdd.go", sddJourneys()},
		{"journeys_issue_2891.go", issue2891Journeys()},
		{"journeys_issue2696.go", issue2696Journeys()},
		{"journeys_sdd_chain.go", sddChainJourneys()},
		{"journeys_issue3094.go", issue3094Journeys()},
		{"journeys_handoff.go", handoffJourneys()},
		{"journeys_capture_evidence_v5.go", captureEvidenceDescriptorJourneys()},
		{"journeys_scope_changed_fixture.go", scopeChangedFixtureJourneys()},
		{"journeys_wave1.go", waveOneJourneys()},
		{"journeys_wave3.go", waveThreeJourneys()},
		{"journeys_wave5.go", waveFiveJourneys()},
		{"journeys_zero_delta.go", zeroDeltaJourneys()},
		{"journeys_lens_context_budget.go", lensContextBudgetJourneys()},
		{"journeys_local_gate_advance.go", localGateBaseAdvanceJourneys()},
		{"journeys_intended_untracked.go", intendedUntrackedJourneys()},
		{"journeys_capture_result_dry_run.go", captureResultDryRunJourneys()},
		{"journeys_issue_2031.go", issue2031Journeys()},
		{"journeys_finding_id_prefix.go", findingIDPrefixJourneys()},
		{"journeys_rescope_write_guard.go", rescopeWriteGuardJourneys()},
		{"journeys_rescope_evidence_retry.go", rescopeEvidenceOnlyRetryJourneys()},
		{"journeys_consecutive_rescope_repair.go", consecutiveRescopeRepairJourneys()},
		{"journeys_reviewed_superset.go", reviewedSupersetJourneys()},
		{"journeys_staged_delivery.go", stagedDeliveryJourneys()},
		{"journeys_frozen_lineage_resume.go", frozenLineageResumeJourneys()},
		{"journeys_issue1800.go", issue1800Journeys()},
		{"journeys_issue2879.go", issue2879Journeys()},
		{"journeys_managed_assets.go", managedAssetJourneys()},
		{"journeys_issue2906.go", issue2906Journeys()},
		{"journeys_issue_2138.go", issue2138Journeys()},
		{"journeys_issue_3043.go", issue3043Journeys()},
		{"journeys_repository_context.go", repositoryContextJourneys()},
		{"journeys_provider_capture.go", providerCaptureRetryJourneys()},
		{"journeys_captured_provider_validator.go", capturedProviderValidatorJourneys()},
		{"journeys_sdd_shared_scaffolding.go", sddSharedScaffoldingJourneys()},
		{"journeys_sdd_post_review_verify_report.go", sddPostReviewVerifyReportJourneys()},
	}
}

// TestJourneyIDsAreUniqueAcrossSourceFiles is the focused duplicate-ID check.
// Before it existed, a colliding ID was only caught downstream by the
// registration test, whose seen[journey.ID] failure is generic and arrives
// next to an unrelated count mismatch. This failure names both defining
// files at the point of the mistake.
func TestJourneyIDsAreUniqueAcrossSourceFiles(t *testing.T) {
	owners := map[string]string{}
	for _, source := range journeySources() {
		for _, journey := range source.Journeys {
			owner, taken := owners[journey.ID]
			if !taken {
				owners[journey.ID] = source.File
				continue
			}
			t.Errorf("journey ID %q is defined in both %s and %s; pick an ID no journeys_*.go file uses yet",
				journey.ID, owner, source.File)
		}
	}
}

// TestJourneySourcesCoverTheWholeCorpus pins journeySources to Journeys().
// Without it, a new journeys_*.go file appended to Journeys() but not
// registered above would sit outside the collision check, which would put
// the corpus right back where it started: duplicates caught late, by a
// message about something else.
func TestJourneySourcesCoverTheWholeCorpus(t *testing.T) {
	counted := map[string]int{}
	for _, source := range journeySources() {
		for _, journey := range source.Journeys {
			counted[journey.ID]++
		}
	}
	for _, journey := range Journeys() {
		counted[journey.ID]--
	}
	disagreements := []string{}
	for id, count := range counted {
		if count != 0 {
			disagreements = append(disagreements, fmt.Sprintf("%s (%+d)", id, -count))
		}
	}
	sort.Strings(disagreements)
	if len(disagreements) > 0 {
		t.Fatalf("journeySources and Journeys() disagree on: %s\nEvery journeys_*.go file must be registered in journeySources so the ID-collision check covers it.",
			strings.Join(disagreements, ", "))
	}
}
