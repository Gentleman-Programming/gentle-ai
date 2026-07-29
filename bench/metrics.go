package main

import "sort"

// Derivation labels how a dimension's number was obtained. It is emitted with
// every dimension so a proxy can never be read as a measurement.
const (
	// DerivedMeasured: observed directly at the process boundary.
	DerivedMeasured = "measured"
	// DerivedProxy: a stand-in for something not observable from outside.
	DerivedProxy = "proxy"
	// DerivedUnobservable: not observable at all; Value is nil.
	DerivedUnobservable = "unobservable"
)

// Dimension is one friction number plus how it was obtained. Value is a
// pointer so an unobservable dimension serializes as null instead of a
// fabricated 0.
type Dimension struct {
	Value      *int   `json:"value"`
	Derivation string `json:"derivation"`
	Note       string `json:"note,omitempty"`
}

func measured(value int) Dimension {
	return Dimension{Value: &value, Derivation: DerivedMeasured}
}

func measuredNote(value int, note string) Dimension {
	return Dimension{Value: &value, Derivation: DerivedMeasured, Note: note}
}

func proxy(value int, note string) Dimension {
	return Dimension{Value: &value, Derivation: DerivedProxy, Note: note}
}

func unobservable(note string) Dimension {
	return Dimension{Value: nil, Derivation: DerivedUnobservable, Note: note}
}

// BlockCounts is dimension 4. Every block lands in exactly one bucket.
//
// ByDesign is a carve-out from OutOfBand, and it is a RECLASSIFICATION, never
// a subtraction: an exempted block is still a block and still in Total(). It
// sits next to OutOfBand in every table for one reason — when out_of_band gets
// smaller, the reader must be able to see where the difference went without
// leaving the row.
type BlockCounts struct {
	SelfRecovered int `json:"self_recovered"`
	InBand        int `json:"in_band"`
	OutOfBand     int `json:"out_of_band"`
	ByDesign      int `json:"by_design"`
	DeadEnd       int `json:"dead_end"`
}

func (b BlockCounts) Total() int {
	return b.SelfRecovered + b.InBand + b.OutOfBand + b.ByDesign + b.DeadEnd
}

func (b *BlockCounts) add(class string) {
	switch class {
	case BlockSelfRecovered:
		b.SelfRecovered++
	case BlockInBand:
		b.InBand++
	case BlockOutOfBand:
		b.OutOfBand++
	case BlockByDesign:
		b.ByDesign++
	case BlockDeadEnd:
		b.DeadEnd++
	}
}

func (b *BlockCounts) addAll(other BlockCounts) {
	b.SelfRecovered += other.SelfRecovered
	b.InBand += other.InBand
	b.OutOfBand += other.OutOfBand
	b.ByDesign += other.ByDesign
	b.DeadEnd += other.DeadEnd
}

// MetricSet is the seven friction dimensions, plus one informational extra.
// There is deliberately NO composite score: a single number can improve while
// dead ends increase, so the dimensions are never collapsed.
type MetricSet struct {
	HumanPrompts         Dimension   `json:"human_prompts"`
	ManualTokens         Dimension   `json:"manual_tokens"`
	CommandsToCompletion Dimension   `json:"commands_to_completion"`
	Blocks               BlockCounts `json:"blocks"`
	BlocksDerivation     string      `json:"blocks_derivation"`
	RecoveryRoundTrips   Dimension   `json:"recovery_round_trips"`
	ModelRuns            Dimension   `json:"model_runs"`
	HumanSurfaceBytes    Dimension   `json:"human_surface_bytes"`
	GitSubprocesses      Dimension   `json:"git_subprocesses"`
}

// CommandRecord is one product invocation inside a journey or session.
type CommandRecord struct {
	Sequence    int      `json:"sequence"`
	Step        string   `json:"step,omitempty"`
	Args        []string `json:"args"`
	ExitCode    int      `json:"exit_code"`
	StderrBytes int      `json:"stderr_bytes"`
	StdoutBytes int      `json:"stdout_bytes"`
	Block       string   `json:"block,omitempty"`
	// ByDesign is present whenever the corpus declared a by-design exemption
	// here, whether or not it applied. A declaration the classifier refused is
	// reported, not dropped.
	ByDesign *ByDesignOutcome `json:"by_design,omitempty"`
	// DeadEnd is present whenever the corpus declared a dead end here, whether
	// or not it applied. Same rule as ByDesign: a declaration the classifier
	// refused is reported, not dropped.
	DeadEnd     *DeadEndOutcome `json:"dead_end,omitempty"`
	Unsupported bool            `json:"unsupported,omitempty"`
	ModelRun    bool            `json:"model_run,omitempty"`
	ManualToken bool            `json:"manual_token,omitempty"`
	Prompts     int             `json:"prompts,omitempty"`
	GitCalls    *int            `json:"git_calls,omitempty"`
	Message     string          `json:"message,omitempty"`
}

// Journey statuses.
const (
	StatusCompleted   = "completed"
	StatusUnsupported = "unsupported"
	StatusFailed      = "failed"
)

// JourneyResult is one journey's outcome. Status is never conflated with the
// numbers: an `unsupported` journey reports what it managed to observe, and
// the comparison renders it as `unsup`, never as 0.
type JourneyResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Axis is empty for the black-box core corpus and carries the axis name for
	// a journey an opt-in axis contributed. It is what lets a reader of either
	// the table or the JSON tell the two populations apart without counting.
	Axis             string          `json:"axis,omitempty"`
	Source           string          `json:"source,omitempty"`
	Status           string          `json:"status"`
	UnsupportedSteps []string        `json:"unsupported_steps,omitempty"`
	FailureReason    string          `json:"failure_reason,omitempty"`
	Metrics          MetricSet       `json:"metrics"`
	Commands         []CommandRecord `json:"commands"`
}

// Results is the machine-readable output of both `run` and `analyze`.
type Results struct {
	Schema              string          `json:"schema"`
	Mode                string          `json:"mode"` // driven | observed
	Binary              string          `json:"binary"`
	BinaryVersion       string          `json:"binary_version"`
	Journeys            []JourneyResult `json:"journeys"`
	Totals              MetricSet       `json:"totals"`
	JourneysCounted     int             `json:"journeys_counted"`
	JourneysUnsupported int             `json:"journeys_unsupported"`
	JourneysFailed      int             `json:"journeys_failed"`
	// CoreJourneys and Axes are the run's provenance. A run of the black-box
	// core and a run of the core plus a coupled axis are different
	// measurements, and these two fields are what stops them looking alike.
	CoreJourneys int          `json:"core_journeys,omitempty"`
	Axes         []AxisRecord `json:"axes,omitempty"`
	Notes        []string     `json:"notes,omitempty"`
}

const (
	ModeDriven    = "driven"
	ModeObserved  = "observed"
	ResultsSchema = "gentle-ai-bench.results/v1"
)

// accumulator builds a MetricSet from a stream of observations. It is the one
// place metrics are counted, shared by `run` and `analyze`.
type accumulator struct {
	commands           int
	prompts            int
	manualTokens       int
	modelRuns          int
	stderrBytes        int
	stderrCaptured     bool
	stderrUncaptured   int
	gitCalls           int
	gitObserved        bool
	blocks             BlockCounts
	recoveryRoundTrips int

	// pendingBlock tracks the index of the last unresolved block so the
	// commands spent recovering from it can be counted.
	pendingBlock  int
	pendingActive bool
	records       []CommandRecord
}

func newAccumulator() *accumulator {
	return &accumulator{stderrCaptured: true}
}

// observe folds one invocation into the running metrics and returns the
// finished record.
func (a *accumulator) observe(step string, o Observation, gitCalls *int, modelRun bool) CommandRecord {
	a.commands++
	record := CommandRecord{
		Sequence:    a.commands,
		Step:        step,
		Args:        o.Args,
		ExitCode:    o.ExitCode,
		StdoutBytes: len(o.Stdout),
		StderrBytes: len(o.Stderr),
		GitCalls:    gitCalls,
	}

	if o.StderrCaptured {
		a.stderrBytes += len(o.Stderr)
		a.prompts += countConsentNotices(o.Stderr)
		record.Prompts = countConsentNotices(o.Stderr)
	} else {
		a.stderrCaptured = false
		a.stderrUncaptured++
		record.StderrBytes = 0
	}

	if hasManualAuthorization(o.Args) {
		a.manualTokens++
		record.ManualToken = true
	}

	if modelRun || isCaptureResult(o.Args) {
		a.modelRuns++
		record.ModelRun = true
	}

	if gitCalls != nil {
		a.gitCalls += *gitCalls
		a.gitObserved = true
	}

	if IsUnsupported(o) {
		record.Unsupported = true
		record.Message = blockMessage(o)
		if outcome := byDesignOutcome(o, NotABlock); outcome != nil {
			outcome.Reason = "this build lacks that CLI surface, so the step recorded `unsupported` rather than a block"
			record.ByDesign = outcome
		}
		if outcome := deadEndOutcome(o, NotABlock); outcome != nil {
			outcome.Reason = "this build lacks that CLI surface, so the step recorded `unsupported` rather than a block"
			record.DeadEnd = outcome
		}
		return record
	}

	class := Classify(o)
	record.Block = class
	record.ByDesign = byDesignOutcome(o, class)
	record.DeadEnd = deadEndOutcome(o, class)
	if class != NotABlock {
		record.Message = blockMessage(o)
		a.blocks.add(class)
		if class != BlockSelfRecovered {
			if !a.pendingActive {
				a.pendingActive = true
				a.pendingBlock = a.commands
			}
		}
		return record
	}

	// A non-blocked command after a block is the flow resuming. Every
	// command from the block up to and including this one was spent
	// recovering.
	if a.pendingActive {
		a.recoveryRoundTrips += a.commands - a.pendingBlock
		a.pendingActive = false
	}
	return record
}

// blockMessage is the human-readable reason a block is reported with. It
// prefers stderr prose, then the envelope's own result/reason pair, because a
// gate denial can exit 0 with an entirely silent stderr.
func blockMessage(o Observation) string {
	if line := firstLine(o.Stderr); line != "" && line != "{" {
		return line
	}
	result, hasResult := envelopeString(o.Stdout, "result")
	reason, hasReason := envelopeString(o.Stdout, "reason")
	switch {
	case hasResult && hasReason:
		return result + ": " + reason
	case hasResult:
		return result
	case hasReason:
		return reason
	}
	if line := firstLine(o.Stdout); line != "" {
		return line
	}
	if !o.StdoutCaptured || !o.StderrCaptured {
		return "(output not captured: the stream was a character device and was passed through untouched)"
	}
	return "(no message emitted)"
}

func firstLine(candidates ...string) string {
	for _, candidate := range candidates {
		for _, line := range splitLines(candidate) {
			if line != "" {
				if len(line) > 240 {
					return line[:240]
				}
				return line
			}
		}
	}
	return ""
}

func splitLines(text string) []string {
	lines := []string{}
	current := ""
	for _, char := range text {
		if char == '\n' {
			lines = append(lines, trimSpace(current))
			current = ""
			continue
		}
		current += string(char)
	}
	lines = append(lines, trimSpace(current))
	return lines
}

func trimSpace(text string) string {
	start, end := 0, len(text)
	for start < end && (text[start] == ' ' || text[start] == '\t' || text[start] == '\r') {
		start++
	}
	for end > start && (text[end-1] == ' ' || text[end-1] == '\t' || text[end-1] == '\r') {
		end--
	}
	return text[start:end]
}

// metrics renders the accumulated counts. modelRunNote non-empty means the
// model-run count is a proxy rather than a measurement.
func (a *accumulator) metrics(modelRunNote string) MetricSet {
	set := MetricSet{
		HumanPrompts: measuredNote(a.prompts,
			"non-TTY run: counts the consent-skipped stderr notice, i.e. times the flow WOULD have asked a human"),
		ManualTokens:         measuredNote(a.manualTokens, "invocations carrying a hand-assembled --maintainer-authorization value"),
		CommandsToCompletion: measured(a.commands),
		Blocks:               a.blocks,
		BlocksDerivation:     DerivedMeasured,
		RecoveryRoundTrips:   measured(a.recoveryRoundTrips),
	}

	if modelRunNote == "" {
		set.ModelRuns = measured(a.modelRuns)
	} else {
		set.ModelRuns = proxy(a.modelRuns, modelRunNote)
	}

	if a.stderrCaptured {
		set.HumanSurfaceBytes = measured(a.stderrBytes)
	} else {
		set.HumanSurfaceBytes = unobservable(
			"stderr was a terminal for at least one invocation and was passed through untouched rather than teed")
	}

	if a.gitObserved {
		set.GitSubprocesses = measuredNote(a.gitCalls,
			"informational, not one of the seven dimensions: GIT_TRACE built-in lines; a lower bound if the build ever uses an in-process git")
	} else {
		set.GitSubprocesses = unobservable("GIT_TRACE was not under this mode's control")
	}
	return set
}

// aggregate sums completed journeys only. Unsupported and failed journeys are
// excluded from totals and counted separately, so "this version could not do
// it" is never added up as "this version needed zero".
func aggregate(journeys []JourneyResult) (MetricSet, int, int, int) {
	total := MetricSet{
		HumanPrompts:         measured(0),
		ManualTokens:         measured(0),
		CommandsToCompletion: measured(0),
		BlocksDerivation:     DerivedMeasured,
		RecoveryRoundTrips:   measured(0),
		ModelRuns:            measured(0),
		HumanSurfaceBytes:    measured(0),
		GitSubprocesses:      measured(0),
	}
	counted, unsupported, failed := 0, 0, 0
	anyProxyModelRuns := ""
	surfaceObservable, gitObservable := true, true

	for _, journey := range journeys {
		switch journey.Status {
		case StatusUnsupported:
			unsupported++
			continue
		case StatusFailed:
			failed++
			continue
		}
		counted++
		addDimension(&total.HumanPrompts, journey.Metrics.HumanPrompts)
		addDimension(&total.ManualTokens, journey.Metrics.ManualTokens)
		addDimension(&total.CommandsToCompletion, journey.Metrics.CommandsToCompletion)
		total.Blocks.addAll(journey.Metrics.Blocks)
		addDimension(&total.RecoveryRoundTrips, journey.Metrics.RecoveryRoundTrips)
		addDimension(&total.ModelRuns, journey.Metrics.ModelRuns)
		if journey.Metrics.ModelRuns.Derivation == DerivedProxy {
			anyProxyModelRuns = journey.Metrics.ModelRuns.Note
		}
		if journey.Metrics.HumanSurfaceBytes.Value == nil {
			surfaceObservable = false
		} else {
			addDimension(&total.HumanSurfaceBytes, journey.Metrics.HumanSurfaceBytes)
		}
		if journey.Metrics.GitSubprocesses.Value == nil {
			gitObservable = false
		} else {
			addDimension(&total.GitSubprocesses, journey.Metrics.GitSubprocesses)
		}
	}

	if anyProxyModelRuns != "" {
		total.ModelRuns.Derivation = DerivedProxy
		total.ModelRuns.Note = anyProxyModelRuns
	}
	if !surfaceObservable {
		total.HumanSurfaceBytes = unobservable("at least one journey could not capture stderr")
	}
	if !gitObservable {
		total.GitSubprocesses = unobservable("at least one journey could not observe git invocations")
	}
	return total, counted, unsupported, failed
}

func addDimension(target *Dimension, source Dimension) {
	if target.Value == nil || source.Value == nil {
		return
	}
	sum := *target.Value + *source.Value
	target.Value = &sum
}

// sortJourneys keeps output stable regardless of map iteration anywhere.
// sortJourneys orders the core corpus first and then each axis, alphabetically
// within both. Interleaving them by id would mix a black-box population and a
// coupled one into a single unlabelled run of rows, which is the one thing the
// axis seam exists to prevent.
func sortJourneys(journeys []JourneyResult) {
	sort.SliceStable(journeys, func(i, j int) bool {
		if journeys[i].Axis != journeys[j].Axis {
			return journeys[i].Axis < journeys[j].Axis
		}
		return journeys[i].ID < journeys[j].ID
	})
}
