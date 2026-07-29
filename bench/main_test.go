package main

import "testing"

// Community issue #1883: a corpus run with failed journeys exited 0, so a CI
// gate reading the exit code saw success in a run that measured nothing for
// the failed rows. runExitCode is the guard; these tests are what makes its
// deletion loud. They drive the same aggregate() path a real run takes, so a
// synthetic failing journey stands in for the real thing end to end.

func resultsWith(journeys ...JourneyResult) Results {
	results := Results{Journeys: journeys}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(journeys)
	return results
}

func TestRunExitsNonzeroWhenAnyJourneyFailed(t *testing.T) {
	results := resultsWith(
		JourneyResult{ID: "ok", Status: StatusCompleted},
		JourneyResult{ID: "broken", Status: StatusFailed, FailureReason: "synthetic"},
	)
	if runExitCode(results) == 0 {
		t.Fatal("a run with a failed journey exited 0; a gate reading that exit sees success in a run that measured nothing for the failed row (issue #1883)")
	}
}

// `unsupported` must NOT fail the run: driving an older binary is a designed
// use, and "this build lacks that surface" is a real, honestly-labelled
// measurement. Failing on it would make cross-version comparison impossible
// to script. The distinction stays visible in the summary line and the table,
// where an unsupported journey renders as `unsup`, never as a number.
func TestRunExitsZeroWhenJourneysAreOnlyUnsupported(t *testing.T) {
	results := resultsWith(
		JourneyResult{ID: "ok", Status: StatusCompleted},
		JourneyResult{ID: "older-binary", Status: StatusUnsupported},
	)
	if exit := runExitCode(results); exit != 0 {
		t.Fatalf("a run with only unsupported journeys exited %d; unsupported is a measurement about the binary, not a failure of the run", exit)
	}
}

func TestRunExitsZeroWhenEverythingCompleted(t *testing.T) {
	results := resultsWith(JourneyResult{ID: "ok", Status: StatusCompleted})
	if exit := runExitCode(results); exit != 0 {
		t.Fatalf("a clean run exited %d", exit)
	}
}
