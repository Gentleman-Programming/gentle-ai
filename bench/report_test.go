package main

import (
	"bytes"
	"strings"
	"testing"
)

// byDesignResults is one journey holding one exempted block and one ordinary
// out-of-band block, which is the exact situation the report has to render
// without letting the reader mistake the first for the second.
func byDesignResults() Results {
	journey := JourneyResult{
		ID:     "j17-bare-repository",
		Title:  "Bare repository",
		Status: StatusCompleted,
		Metrics: MetricSet{
			HumanPrompts: measured(0), ManualTokens: measured(0), CommandsToCompletion: measured(2),
			Blocks:             BlockCounts{OutOfBand: 1, ByDesign: 1},
			BlocksDerivation:   DerivedMeasured,
			RecoveryRoundTrips: measured(0), ModelRuns: measured(0),
			HumanSurfaceBytes: measured(10), GitSubprocesses: measured(1),
		},
		Commands: []CommandRecord{
			{
				Sequence: 1, Step: "review start in a bare repository", ExitCode: 1,
				Block:   BlockByDesign,
				Message: "Error: review needs a working tree",
				ByDesign: &ByDesignOutcome{
					Shape: ByDesignOperatorKnowledge, Applied: true,
					NextAction: "run the same command again from a checkout",
				},
			},
			{Sequence: 2, Step: "an ordinary defect", ExitCode: 1, Block: BlockOutOfBand, Message: "Error: no."},
		},
	}
	results := Results{Schema: ResultsSchema, Mode: ModeDriven, Binary: "/bin/gentle-ai", Journeys: []JourneyResult{journey}}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)
	return results
}

// TestReportShowsTheByDesignSplit is the guard on the whole point of the
// class. out_of_band getting smaller is only honest if the reader can see
// where the difference went, in the same output, without opening the JSON.
func TestReportShowsTheByDesignSplit(t *testing.T) {
	buffer := &bytes.Buffer{}
	writeRunReport(buffer, byDesignResults())
	report := buffer.String()

	for _, want := range []string{
		"design",             // its own table column
		"4d   by_design",     // its own legend row, next to out_of_band
		"4c   out_of_band",   // which is still printed separately
		"By-design blocks",   // its own section
		"operator-knowledge", // showing the declared shape
		"run the same command again from a checkout", // and the verified quote
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report does not contain %q; the split is invisible to a reader\n%s", want, report)
		}
	}

	// The exempted block must still be listed as a block, not quietly removed
	// from the per-journey detail.
	if !strings.Contains(report, "by_design     review start in a bare repository") {
		t.Fatalf("the exempted block vanished from the block detail\n%s", report)
	}

	// And it must still be inside the block total: an exemption is a
	// reclassification, never a subtraction.
	if total := byDesignResults().Totals.Blocks.Total(); total != 2 {
		t.Fatalf("blocks total = %d, want 2: by_design is still a block", total)
	}
}

// TestReportShowsAStaleDeclaration proves a declaration the classifier
// refused is reported rather than silently ignored.
func TestReportShowsAStaleDeclaration(t *testing.T) {
	results := byDesignResults()
	results.Journeys[0].Commands[1].ByDesign = &ByDesignOutcome{
		Shape: ByDesignWorldAction, Applied: false,
		NextAction: "free some disk space",
		Reason:     "the declared next action is not in the emitted output",
	}
	buffer := &bytes.Buffer{}
	writeRunReport(buffer, results)
	report := buffer.String()

	for _, want := range []string{
		"stale",
		"the declared next action is not in the emitted output",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report does not contain %q; a stale declaration must be visible\n%s", want, report)
		}
	}
}

// TestReportOmitsTheByDesignSectionWhenNothingIsDeclared keeps the ordinary
// report unchanged for a corpus that declares no exemptions.
func TestReportOmitsTheByDesignSectionWhenNothingIsDeclared(t *testing.T) {
	results := byDesignResults()
	results.Journeys[0].Commands[0].Block = BlockOutOfBand
	results.Journeys[0].Commands[0].ByDesign = nil
	buffer := &bytes.Buffer{}
	writeRunReport(buffer, results)
	if strings.Contains(buffer.String(), "By-design blocks") {
		t.Fatalf("the section printed with nothing to report\n%s", buffer.String())
	}
}
