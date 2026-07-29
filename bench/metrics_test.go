package main

import "testing"

// TestAccumulatorCountsEveryDimension walks a small synthetic flow and pins
// every dimension at once, so a change to one counter cannot silently move
// another.
func TestAccumulatorCountsEveryDimension(t *testing.T) {
	accumulator := newAccumulator()
	gitCalls := 3

	consentNotice := "Gentle AI reviewed this change without asking, because this session has no terminal to answer on.\n"
	finalizeError := "Error: review finalize requires all 4 original reviewer result(s)\n"
	stuckError := "Error: still stuck\n"
	wantSurface := len(consentNotice) + len(finalizeError) + len(stuckError)

	// 1: a high-risk start that would have asked a human.
	accumulator.observe("start", Observation{
		Args:           []string{"review", "start"},
		Stdout:         `{"state":"reviewing"}`,
		Stderr:         consentNotice,
		StdoutCaptured: true, StderrCaptured: true,
	}, &gitCalls, false)

	// 2: a reviewer capture, i.e. one model run.
	accumulator.observe("capture", Observation{
		Args:   []string{"review", "capture-result", "--lens", "review-risk", "--input", "r.json"},
		Stdout: `{"admission_decision":"completed"}`, StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	// 3: a block that names nothing runnable.
	accumulator.observe("finalize", Observation{
		Args: []string{"review", "finalize"}, ExitCode: 1,
		Stderr:         finalizeError,
		StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	// 4: a second command still stuck (part of the same recovery).
	accumulator.observe("status", Observation{
		Args: []string{"review", "status"}, ExitCode: 1, Stderr: stuckError,
		StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	// 5: the flow resumes.
	accumulator.observe("finalize", Observation{
		Args:   []string{"review", "finalize", "--captured-results=true"},
		Stdout: `{"state":"approved"}`, StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	// 6: an invocation carrying a hand-assembled token.
	accumulator.observe("abandon", Observation{
		Args:   []string{"review", "abandon", "--maintainer-authorization", "schema\nlineage=x"},
		Stdout: `{"operation":"review/abandon"}`, StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	set := accumulator.metrics("")

	assertDimension(t, "human_prompts", set.HumanPrompts, 1)
	assertDimension(t, "manual_tokens", set.ManualTokens, 1)
	assertDimension(t, "commands_to_completion", set.CommandsToCompletion, 6)
	assertDimension(t, "model_runs", set.ModelRuns, 1)
	assertDimension(t, "git_subprocesses", set.GitSubprocesses, 3)

	// Two blocks, both out of band, and the flow resumed at command 5 after
	// blocking at command 3: three commands were spent recovering.
	if set.Blocks.Total() != 2 || set.Blocks.OutOfBand != 2 {
		t.Fatalf("blocks = %+v, want 2 out_of_band", set.Blocks)
	}
	assertDimension(t, "recovery_round_trips", set.RecoveryRoundTrips, 2)

	// human_surface_bytes is every stderr byte, narration included.
	assertDimension(t, "human_surface_bytes", set.HumanSurfaceBytes, wantSurface)
}

func assertDimension(t *testing.T, name string, dimension Dimension, want int) {
	t.Helper()
	if dimension.Value == nil {
		t.Fatalf("%s is null, want %d", name, want)
	}
	if *dimension.Value != want {
		t.Fatalf("%s = %d, want %d", name, *dimension.Value, want)
	}
}

// TestUnsupportedCommandsAreNeitherBlockNorRecovery is the honesty guard at
// the accumulator level.
func TestUnsupportedCommandsAreNeitherBlockNorRecovery(t *testing.T) {
	accumulator := newAccumulator()
	record := accumulator.observe("old binary", Observation{
		Args: []string{"review", "capture-result", "--preflight"}, ExitCode: 1,
		Stderr: "Error: flag provided but not defined: -preflight\n", StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	if !record.Unsupported {
		t.Fatal("record must be marked unsupported")
	}
	if record.Block != NotABlock {
		t.Fatalf("block = %q, want none", record.Block)
	}
	set := accumulator.metrics("")
	if set.Blocks.Total() != 0 {
		t.Fatalf("blocks = %+v, want none", set.Blocks)
	}
	assertDimension(t, "recovery_round_trips", set.RecoveryRoundTrips, 0)
}

// TestUncapturedStderrMakesTheDimensionNull is the honesty contract: a
// dimension that could not be observed is null, never a guessed number.
func TestUncapturedStderrMakesTheDimensionNull(t *testing.T) {
	accumulator := newAccumulator()
	accumulator.observe("tty", Observation{
		Args: []string{"review", "start"}, Stdout: `{"state":"reviewing"}`,
		Stderr: "invisible", StdoutCaptured: true, StderrCaptured: false,
	}, nil, false)

	set := accumulator.metrics("")
	if set.HumanSurfaceBytes.Value != nil {
		t.Fatalf("human_surface_bytes = %d, want null", *set.HumanSurfaceBytes.Value)
	}
	if set.HumanSurfaceBytes.Derivation != DerivedUnobservable {
		t.Fatalf("derivation = %q, want %q", set.HumanSurfaceBytes.Derivation, DerivedUnobservable)
	}
}

func TestModelRunProxyIsLabelled(t *testing.T) {
	accumulator := newAccumulator()
	accumulator.observe("capture", Observation{
		Args:   []string{"review", "capture-result", "--input", "r.json"},
		Stdout: "{}", StdoutCaptured: true, StderrCaptured: true,
	}, nil, false)

	set := accumulator.metrics("stand-in for reviewer runs")
	if set.ModelRuns.Derivation != DerivedProxy {
		t.Fatalf("derivation = %q, want %q", set.ModelRuns.Derivation, DerivedProxy)
	}
	if set.ModelRuns.Note == "" {
		t.Fatal("a proxy must carry a note saying so")
	}
}

// TestAggregateExcludesUnsupportedJourneys is the core cross-version rule:
// "this version could not do it" must never be summed as "this version needed
// zero".
func TestAggregateExcludesUnsupportedJourneys(t *testing.T) {
	journeys := []JourneyResult{
		{ID: "a", Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(2), ManualTokens: measured(1), CommandsToCompletion: measured(9),
			Blocks: BlockCounts{InBand: 1, OutOfBand: 2}, RecoveryRoundTrips: measured(3),
			ModelRuns: measured(4), HumanSurfaceBytes: measured(500), GitSubprocesses: measured(10),
		}},
		{ID: "b", Status: StatusUnsupported, Metrics: MetricSet{
			HumanPrompts: measured(99), ManualTokens: measured(99), CommandsToCompletion: measured(99),
			Blocks: BlockCounts{DeadEnd: 99}, RecoveryRoundTrips: measured(99),
			ModelRuns: measured(99), HumanSurfaceBytes: measured(99), GitSubprocesses: measured(99),
		}},
		{ID: "c", Status: StatusFailed},
		{ID: "d", Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(1), ManualTokens: measured(0), CommandsToCompletion: measured(3),
			Blocks: BlockCounts{OutOfBand: 1}, RecoveryRoundTrips: measured(0),
			ModelRuns: measured(1), HumanSurfaceBytes: measured(100), GitSubprocesses: measured(5),
		}},
	}

	totals, counted, unsupported, failed := aggregate(journeys)
	if counted != 2 || unsupported != 1 || failed != 1 {
		t.Fatalf("counted=%d unsupported=%d failed=%d, want 2/1/1", counted, unsupported, failed)
	}
	assertDimension(t, "human_prompts", totals.HumanPrompts, 3)
	assertDimension(t, "commands_to_completion", totals.CommandsToCompletion, 12)
	assertDimension(t, "model_runs", totals.ModelRuns, 5)
	assertDimension(t, "human_surface_bytes", totals.HumanSurfaceBytes, 600)
	if totals.Blocks.Total() != 4 || totals.Blocks.DeadEnd != 0 {
		t.Fatalf("blocks = %+v, want 4 total and no dead ends from the unsupported journey", totals.Blocks)
	}
}

// TestAggregateKeepsNullNull proves an unobservable dimension poisons the
// total instead of being silently treated as zero.
func TestAggregateKeepsNullNull(t *testing.T) {
	journeys := []JourneyResult{
		{ID: "a", Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(0), ManualTokens: measured(0), CommandsToCompletion: measured(1),
			RecoveryRoundTrips: measured(0), ModelRuns: measured(0),
			HumanSurfaceBytes: unobservable("terminal stderr"), GitSubprocesses: measured(1),
		}},
	}
	totals, _, _, _ := aggregate(journeys)
	if totals.HumanSurfaceBytes.Value != nil {
		t.Fatalf("human_surface_bytes = %d, want null", *totals.HumanSurfaceBytes.Value)
	}
}

// TestAggregatePropagatesProxyLabel keeps a proxy from being laundered into a
// measurement by the summation step.
func TestAggregatePropagatesProxyLabel(t *testing.T) {
	journeys := []JourneyResult{
		{ID: "a", Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(0), ManualTokens: measured(0), CommandsToCompletion: measured(1),
			RecoveryRoundTrips: measured(0), ModelRuns: proxy(2, "counts capture-result invocations"),
			HumanSurfaceBytes: measured(0), GitSubprocesses: measured(0),
		}},
	}
	totals, _, _, _ := aggregate(journeys)
	if totals.ModelRuns.Derivation != DerivedProxy {
		t.Fatalf("derivation = %q, want %q", totals.ModelRuns.Derivation, DerivedProxy)
	}
}

// TestCompareRefusesMixedPopulations is enforced in commandCompare; this pins
// the shape the comparison builds for matching modes, including the `unsup`
// cell that must never read as 0.
func TestCompareRendersUnsupportedDistinctlyFromZero(t *testing.T) {
	before := Results{Mode: ModeDriven, Journeys: []JourneyResult{
		{ID: "j01", Status: StatusUnsupported},
	}}
	after := Results{Mode: ModeDriven, Journeys: []JourneyResult{
		{ID: "j01", Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(0), ManualTokens: measured(0), CommandsToCompletion: measured(4),
			RecoveryRoundTrips: measured(0), ModelRuns: measured(0),
			HumanSurfaceBytes: measured(0), GitSubprocesses: measured(0),
		}},
	}}
	before.Totals, before.JourneysCounted, before.JourneysUnsupported, before.JourneysFailed = aggregate(before.Journeys)
	after.Totals, after.JourneysCounted, after.JourneysUnsupported, after.JourneysFailed = aggregate(after.Journeys)

	report := buildComparison(before, after)
	cell := report.Journeys[0].Cells["3 commands_to_completion"]
	if cell != "unsup -> 4" {
		t.Fatalf("cell = %q, want %q", cell, "unsup -> 4")
	}
}

func TestAnalyzeSessionUsesTheSameClassifier(t *testing.T) {
	session := []SessionRecord{
		{Argv: []string{"review", "start"}, ExitCode: 0, Stdout: `{"state":"reviewing"}`,
			Stderr:         "Gentle AI reviewed this change without asking, because this session has no terminal to answer on.\n",
			StdoutCaptured: true, StderrCaptured: true},
		{Argv: []string{"review", "finalize"}, ExitCode: 1,
			Stderr:         "Error: capture it first with `gentle-ai review capture-evidence`\n",
			StdoutCaptured: true, StderrCaptured: true},
		{Argv: []string{"review", "capture-result", "--input", "r.json"}, ExitCode: 0,
			Stdout: `{"admission_decision":"completed"}`, StdoutCaptured: true, StderrCaptured: true},
	}
	result := analyzeSession(session)

	if result.Metrics.Blocks.InBand != 1 {
		t.Fatalf("in_band blocks = %d, want 1", result.Metrics.Blocks.InBand)
	}
	assertDimension(t, "human_prompts", result.Metrics.HumanPrompts, 1)
	assertDimension(t, "commands_to_completion", result.Metrics.CommandsToCompletion, 3)
	if result.Metrics.ModelRuns.Derivation != DerivedProxy {
		t.Fatalf("observed model_runs must be labelled a proxy, got %q", result.Metrics.ModelRuns.Derivation)
	}
	assertDimension(t, "model_runs", result.Metrics.ModelRuns, 1)
	if result.Metrics.GitSubprocesses.Value != nil {
		t.Fatal("git_subprocesses must be null when the session carried no trace counts")
	}
}

// TestCompareTotalsCoverOnlyTheComparableSubset stops the totals row from
// comparing different populations. A binary that supports nine more journeys
// would otherwise show a huge command-count "regression" that is really just a
// wider corpus.
func TestCompareTotalsCoverOnlyTheComparableSubset(t *testing.T) {
	completed := func(id string, commands int) JourneyResult {
		return JourneyResult{ID: id, Status: StatusCompleted, Metrics: MetricSet{
			HumanPrompts: measured(0), ManualTokens: measured(0), CommandsToCompletion: measured(commands),
			RecoveryRoundTrips: measured(0), ModelRuns: measured(0),
			HumanSurfaceBytes: measured(0), GitSubprocesses: measured(0),
		}}
	}
	before := Results{Mode: ModeDriven, Journeys: []JourneyResult{
		completed("j01", 5),
		{ID: "j02", Status: StatusUnsupported},
	}}
	after := Results{Mode: ModeDriven, Journeys: []JourneyResult{
		completed("j01", 4),
		completed("j02", 16),
	}}
	before.Totals, before.JourneysCounted, before.JourneysUnsupported, before.JourneysFailed = aggregate(before.Journeys)
	after.Totals, after.JourneysCounted, after.JourneysUnsupported, after.JourneysFailed = aggregate(after.Journeys)

	report := buildComparison(before, after)
	if len(report.ComparableJourneys) != 1 || report.ComparableJourneys[0] != "j01" {
		t.Fatalf("comparable = %v, want [j01]", report.ComparableJourneys)
	}
	if len(report.ExcludedJourneys) != 1 || report.ExcludedJourneys[0] != "j02" {
		t.Fatalf("excluded = %v, want [j02]", report.ExcludedJourneys)
	}
	for _, row := range report.Dimensions {
		if row.Dimension != "3 commands_to_completion" {
			continue
		}
		if row.Before == nil || *row.Before != 5 || row.After == nil || *row.After != 4 {
			t.Fatalf("commands before/after = %v/%v, want 5/4", row.Before, row.After)
		}
		if row.Delta == nil || *row.Delta != -1 {
			t.Fatalf("delta = %v, want -1", row.Delta)
		}
		return
	}
	t.Fatal("commands_to_completion row missing")
}
