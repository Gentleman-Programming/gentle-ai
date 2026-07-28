package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// dimensionRow describes one printable dimension. Blocks are printed as five
// separate rows because collapsing them would hide the two things that must
// never be hidden: dead ends going up while everything else goes down, and
// out_of_band going down because blocks moved into by_design.
type dimensionRow struct {
	Label string
	Value func(MetricSet) (int, bool, string) // value, present, derivation
}

func dimensionRows() []dimensionRow {
	pick := func(get func(MetricSet) Dimension) func(MetricSet) (int, bool, string) {
		return func(set MetricSet) (int, bool, string) {
			dimension := get(set)
			if dimension.Value == nil {
				return 0, false, dimension.Derivation
			}
			return *dimension.Value, true, dimension.Derivation
		}
	}
	blocks := func(get func(BlockCounts) int) func(MetricSet) (int, bool, string) {
		return func(set MetricSet) (int, bool, string) { return get(set.Blocks), true, set.BlocksDerivation }
	}
	return []dimensionRow{
		{"1 human_prompts", pick(func(s MetricSet) Dimension { return s.HumanPrompts })},
		{"2 manual_tokens", pick(func(s MetricSet) Dimension { return s.ManualTokens })},
		{"3 commands_to_completion", pick(func(s MetricSet) Dimension { return s.CommandsToCompletion })},
		{"4 blocks (total)", blocks(func(b BlockCounts) int { return b.Total() })},
		{"4a   self_recovered", blocks(func(b BlockCounts) int { return b.SelfRecovered })},
		{"4b   in_band", blocks(func(b BlockCounts) int { return b.InBand })},
		{"4c   out_of_band", blocks(func(b BlockCounts) int { return b.OutOfBand })},
		{"4d   by_design", blocks(func(b BlockCounts) int { return b.ByDesign })},
		{"4e   dead_end", blocks(func(b BlockCounts) int { return b.DeadEnd })},
		{"5 recovery_round_trips", pick(func(s MetricSet) Dimension { return s.RecoveryRoundTrips })},
		{"6 model_runs", pick(func(s MetricSet) Dimension { return s.ModelRuns })},
		{"7 human_surface_bytes", pick(func(s MetricSet) Dimension { return s.HumanSurfaceBytes })},
		{"- git_subprocesses (info)", pick(func(s MetricSet) Dimension { return s.GitSubprocesses })},
	}
}

func writeRunReport(out io.Writer, results Results) {
	fmt.Fprintf(out, "gentle-ai-bench — %s mode\n", results.Mode)
	fmt.Fprintf(out, "binary : %s\n", results.Binary)
	if results.BinaryVersion != "" {
		fmt.Fprintf(out, "version: %s\n", results.BinaryVersion)
	}
	fmt.Fprintf(out, "journeys: %d completed, %d unsupported, %d failed\n",
		results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed)
	writeCorpusProvenance(out, results)
	fmt.Fprintln(out)

	rows := dimensionRows()
	// The axis column appears only when a run selected one, so a core-only
	// report is byte-for-byte the report it has always been.
	withAxis := len(results.Axes) > 0
	headers := []string{"journey", "status"}
	for _, row := range rows {
		headers = append(headers, shortLabel(row.Label))
	}
	if withAxis {
		headers = append(headers, "axis")
	}
	table := [][]string{headers}
	for _, journey := range results.Journeys {
		line := []string{journey.ID, journey.Status}
		for _, row := range rows {
			line = append(line, cell(journey, row))
		}
		if withAxis {
			line = append(line, axisColumn(journey.Axis))
		}
		table = append(table, line)
	}
	total := []string{"TOTAL (completed only)", ""}
	for _, row := range rows {
		value, present, _ := row.Value(results.Totals)
		total = append(total, format(value, present))
	}
	if withAxis {
		total = append(total, "")
	}
	table = append(table, total)
	renderTable(out, table)

	fmt.Fprintf(out, "\nDimension legend\n")
	for _, row := range rows {
		value, present, derivation := row.Value(results.Totals)
		_ = value
		_ = present
		fmt.Fprintf(out, "  %-26s %s\n", row.Label, derivation)
	}

	blocked := []JourneyResult{}
	for _, journey := range results.Journeys {
		if journey.Status != StatusCompleted || journey.Metrics.Blocks.Total() > 0 {
			blocked = append(blocked, journey)
		}
	}
	if len(blocked) > 0 {
		fmt.Fprintf(out, "\nBlocks and unsupported steps\n")
		for _, journey := range blocked {
			fmt.Fprintf(out, "  %s (%s)\n", journey.ID, journey.Status)
			for _, step := range journey.UnsupportedSteps {
				fmt.Fprintf(out, "      unsupported: %s\n", step)
			}
			if journey.FailureReason != "" {
				fmt.Fprintf(out, "      failed: %s\n", journey.FailureReason)
			}
			for _, command := range journey.Commands {
				if command.Block == NotABlock {
					continue
				}
				fmt.Fprintf(out, "      %-13s %s\n", command.Block, command.Step)
				fmt.Fprintf(out, "                    %s\n", command.Message)
			}
		}
	}
	writeByDesignSection(out, results)
	writeDeadEndSection(out, results)
	writeAxisSection(out, results)

	for _, note := range results.Notes {
		fmt.Fprintf(out, "\nnote: %s\n", note)
	}
}

func axisColumn(name string) string {
	if name == "" {
		return "core"
	}
	return name
}

// writeCorpusProvenance states, in the first six lines of the report, which
// populations produced the numbers below it.
//
// "43 journeys" and "43 journeys plus a damaged-store axis" are different
// measurements. They must never render alike, and the difference must be
// visible without opening the JSON or counting rows — so this prints even when
// no axis was selected, and says so by name.
func writeCorpusProvenance(out io.Writer, results Results) {
	if results.Mode != ModeDriven {
		return
	}
	fmt.Fprintf(out, "corpus  : %d core journeys, black-box (the harness drives the binary and nothing else)\n",
		results.CoreJourneys)
	if len(results.Axes) == 0 {
		available := "none registered in this build"
		if names := axisNames(); len(names) > 0 {
			available = "available: " + strings.Join(names, ", ")
		}
		fmt.Fprintf(out, "axes    : none selected (%s; add one with --axis)\n", available)
		return
	}
	for _, axis := range results.Axes {
		property := "black-box"
		if !axis.BlackBox {
			property = "NOT black-box"
		}
		fmt.Fprintf(out, "axis    : %s — %d journeys, %s\n", axis.Name, len(axis.JourneyIDs), property)
	}
}

// writeAxisSection prints each selected axis's own honesty contract next to the
// journeys it contributed.
//
// It is here rather than in the README because a property of the measurement
// belongs with the measurement. Someone reading a run's output has to be able
// to see which journeys were black-box, which were not, and what the ones that
// were not will do when the coupling they depend on moves — without going and
// finding a document.
func writeAxisSection(out io.Writer, results Results) {
	if len(results.Axes) == 0 {
		return
	}
	fmt.Fprintf(out, "\nOpt-in axes in this run\n")
	fmt.Fprintf(out, "  The core corpus above is black-box: it drives the binary through the process\n")
	fmt.Fprintf(out, "  boundary and touches nothing else, which is what lets it measure any build.\n")
	fmt.Fprintf(out, "  An axis is opt-in because it does not. Each one declares what it costs.\n")
	for _, axis := range results.Axes {
		property := "black-box"
		if !axis.BlackBox {
			property = "NOT black-box"
		}
		fmt.Fprintf(out, "\n    %s — %s\n", axis.Name, axis.Title)
		fmt.Fprintf(out, "      property: %s\n", property)
		for _, line := range axis.Properties {
			fmt.Fprintf(out, "      - %s\n", line)
		}
		fmt.Fprintf(out, "      journeys: %s\n", strings.Join(axis.JourneyIDs, ", "))
	}
}

// writeByDesignSection prints every by-design declaration in the run and what
// became of it. It exists because `4c out_of_band` getting smaller is only
// honest if the reader can see, in the same output, which blocks moved and on
// what grounds — and because a declaration the classifier REFUSED is the first
// sign the product's message changed under the corpus, so it is printed too.
//
// The section is omitted entirely when the corpus declared nothing, which
// keeps the ordinary report unchanged.
func writeByDesignSection(out io.Writer, results Results) {
	type entry struct {
		journey string
		command CommandRecord
	}
	applied, stale := []entry{}, []entry{}
	for _, journey := range results.Journeys {
		for _, command := range journey.Commands {
			if command.ByDesign == nil {
				continue
			}
			if command.ByDesign.Applied {
				applied = append(applied, entry{journey.ID, command})
				continue
			}
			stale = append(stale, entry{journey.ID, command})
		}
	}
	if len(applied)+len(stale) == 0 {
		return
	}

	fmt.Fprintf(out, "\nBy-design blocks (declared exemptions from out_of_band)\n")
	fmt.Fprintf(out, "  Each one is a block the corpus declares CORRECT: a refusal for which no\n")
	fmt.Fprintf(out, "  runnable command can honestly exist, because naming one would name a dead\n")
	fmt.Fprintf(out, "  end. They are counted in `4d by_design` instead of `4c out_of_band`; they\n")
	fmt.Fprintf(out, "  are still blocks and still inside the block total. The exemption costs a\n")
	fmt.Fprintf(out, "  shape from a closed vocabulary and an exact quote of the product's own\n")
	fmt.Fprintf(out, "  next-action text, verified present in the bytes it emitted. Read the quote:\n")
	fmt.Fprintf(out, "  if it no longer tells the operator what to do, this is a defect wearing an\n")
	fmt.Fprintf(out, "  exemption, and that is the failure mode this class introduces.\n")
	for _, item := range applied {
		fmt.Fprintf(out, "    %s  %s\n", item.journey, item.command.Step)
		fmt.Fprintf(out, "        shape: %s (%s)\n", item.command.ByDesign.Shape, shapeMeaning(item.command.ByDesign.Shape))
		fmt.Fprintf(out, "        verified next action: %q\n", item.command.ByDesign.NextAction)
	}
	if len(stale) > 0 {
		fmt.Fprintf(out, "  Declared but NOT applied — stale declarations, counted as the class they\n")
		fmt.Fprintf(out, "  really are. A declaration is a claim about what the product prints, and\n")
		fmt.Fprintf(out, "  these claims failed:\n")
		for _, item := range stale {
			class := item.command.Block
			if class == NotABlock {
				class = "not a block"
			}
			fmt.Fprintf(out, "    %s  %s\n", item.journey, item.command.Step)
			fmt.Fprintf(out, "        counted as: %s\n", class)
			fmt.Fprintf(out, "        reason: %s\n", item.command.ByDesign.Reason)
		}
	}
}

// writeDeadEndSection reports declared dead ends the same way by-design
// exemptions are reported. An applied one is the corpus saying no continuation
// exists; a stale one is the corpus still saying that after the product grew
// an exit, and leaving it unsaid would let a closed gap keep advertising
// itself as open.
func writeDeadEndSection(out io.Writer, results Results) {
	type entry struct {
		journey string
		command CommandRecord
	}
	applied, stale := []entry{}, []entry{}
	for _, journey := range results.Journeys {
		for _, command := range journey.Commands {
			if command.DeadEnd == nil {
				continue
			}
			if command.DeadEnd.Applied {
				applied = append(applied, entry{journey.ID, command})
				continue
			}
			stale = append(stale, entry{journey.ID, command})
		}
	}
	if len(applied)+len(stale) == 0 {
		return
	}
	fmt.Fprintf(out, "\nDead-end declarations\n")
	for _, item := range applied {
		fmt.Fprintf(out, "    %s  %s\n", item.journey, item.command.Step)
		fmt.Fprintf(out, "        applied: no advertised operation admits this shape\n")
	}
	for _, item := range stale {
		class := item.command.Block
		if class == NotABlock {
			class = "not a block"
		}
		fmt.Fprintf(out, "    %s  %s\n", item.journey, item.command.Step)
		fmt.Fprintf(out, "        STALE, counted as: %s\n", class)
		fmt.Fprintf(out, "        reason: %s\n", item.command.DeadEnd.Reason)
	}
}

func shapeMeaning(name string) string {
	for _, shape := range byDesignShapes {
		if shape.Name == name {
			return shape.Meaning
		}
	}
	return "unrecognised shape"
}

func cell(journey JourneyResult, row dimensionRow) string {
	if journey.Status == StatusUnsupported {
		return "unsup"
	}
	if journey.Status == StatusFailed {
		return "fail"
	}
	value, present, _ := row.Value(journey.Metrics)
	return format(value, present)
}

func format(value int, present bool) string {
	if !present {
		return "null"
	}
	return strconv.Itoa(value)
}

func shortLabel(label string) string {
	fields := strings.Fields(label)
	if len(fields) < 2 {
		return label
	}
	name := fields[len(fields)-1]
	if strings.HasPrefix(label, "4 ") {
		return "blk"
	}
	switch name {
	case "human_prompts":
		return "prompt"
	case "manual_tokens":
		return "token"
	case "commands_to_completion":
		return "cmds"
	case "self_recovered":
		return "self"
	case "in_band":
		return "in"
	case "out_of_band":
		return "out"
	case "by_design":
		return "design"
	case "dead_end":
		return "dead"
	case "recovery_round_trips":
		return "recov"
	case "model_runs":
		return "model"
	case "human_surface_bytes":
		return "stderrB"
	default:
		return "git"
	}
}

// CompareReport is the machine-readable comparison.
//
// The dimension totals are computed over the COMPARABLE SUBSET only: journeys
// that completed in both runs. Summing a run of 14 completed journeys against
// a run of 5 would produce a large, meaningless delta that reads as a
// regression when it is really just a wider corpus. The excluded journeys are
// listed rather than silently dropped.
type CompareReport struct {
	Schema             string           `json:"schema"`
	Mode               string           `json:"mode"`
	Before             string           `json:"before"`
	After              string           `json:"after"`
	ComparableJourneys []string         `json:"comparable_journeys"`
	ExcludedJourneys   []string         `json:"excluded_journeys,omitempty"`
	Dimensions         []CompareRow     `json:"dimensions"`
	Journeys           []CompareJourney `json:"journeys"`
	Notes              []string         `json:"notes,omitempty"`
}

type CompareRow struct {
	Dimension  string `json:"dimension"`
	Before     *int   `json:"before"`
	After      *int   `json:"after"`
	Delta      *int   `json:"delta"`
	Derivation string `json:"derivation"`
}

type CompareJourney struct {
	ID           string            `json:"id"`
	BeforeStatus string            `json:"before_status"`
	AfterStatus  string            `json:"after_status"`
	Cells        map[string]string `json:"cells"`
}

func buildComparison(before, after Results) CompareReport {
	report := CompareReport{
		Schema: "gentle-ai-bench.comparison/v1",
		Mode:   before.Mode,
		Before: before.Binary,
		After:  after.Binary,
	}
	beforeIndex, afterIndex := byID(before), byID(after)
	comparable := []JourneyResult{}
	comparableAfter := []JourneyResult{}
	for _, journey := range after.Journeys {
		counterpart, ok := beforeIndex[journey.ID]
		if !ok || counterpart.Status != StatusCompleted || journey.Status != StatusCompleted {
			report.ExcludedJourneys = append(report.ExcludedJourneys, journey.ID)
			continue
		}
		report.ComparableJourneys = append(report.ComparableJourneys, journey.ID)
		comparable = append(comparable, counterpart)
		comparableAfter = append(comparableAfter, journey)
	}
	beforeTotals, _, _, _ := aggregate(comparable)
	afterTotals, _, _, _ := aggregate(comparableAfter)

	for _, row := range dimensionRows() {
		beforeValue, beforePresent, derivation := row.Value(beforeTotals)
		afterValue, afterPresent, afterDerivation := row.Value(afterTotals)
		entry := CompareRow{Dimension: strings.TrimSpace(row.Label), Derivation: derivation}
		if afterDerivation == DerivedProxy || derivation == DerivedProxy {
			entry.Derivation = DerivedProxy
		}
		if beforePresent {
			value := beforeValue
			entry.Before = &value
		}
		if afterPresent {
			value := afterValue
			entry.After = &value
		}
		if beforePresent && afterPresent {
			delta := afterValue - beforeValue
			entry.Delta = &delta
		}
		report.Dimensions = append(report.Dimensions, entry)
	}

	statuses := map[string][2]string{}
	journeyIDs := []string{}
	index := func(results Results, slot int) {
		for _, journey := range results.Journeys {
			current, seen := statuses[journey.ID]
			if !seen {
				journeyIDs = append(journeyIDs, journey.ID)
				current = [2]string{"absent", "absent"}
			}
			current[slot] = journey.Status
			statuses[journey.ID] = current
		}
	}
	index(before, 0)
	index(after, 1)
	sort.Strings(journeyIDs)

	for _, id := range journeyIDs {
		entry := CompareJourney{ID: id, BeforeStatus: statuses[id][0], AfterStatus: statuses[id][1], Cells: map[string]string{}}
		for _, row := range dimensionRows() {
			label := strings.TrimSpace(row.Label)
			entry.Cells[label] = journeyCell(beforeIndex, id, row) + " -> " + journeyCell(afterIndex, id, row)
		}
		report.Journeys = append(report.Journeys, entry)
	}
	sort.Strings(report.ExcludedJourneys)

	report.Notes = append(report.Notes,
		"Dimension totals cover only the journeys that completed in BOTH runs; summing different populations would produce a meaningless delta.",
		"No composite friction score is emitted: a single number can improve while dead ends increase.",
		"`unsup` means the binary lacks that CLI surface. It is never counted as 0 and never included in totals.")
	return report
}

func byID(results Results) map[string]JourneyResult {
	index := map[string]JourneyResult{}
	for _, journey := range results.Journeys {
		index[journey.ID] = journey
	}
	return index
}

func journeyCell(index map[string]JourneyResult, id string, row dimensionRow) string {
	journey, ok := index[id]
	if !ok {
		return "absent"
	}
	return cell(journey, row)
}

func writeCompareReport(out io.Writer, report CompareReport) {
	fmt.Fprintf(out, "gentle-ai-bench comparison — %s mode\n", report.Mode)
	fmt.Fprintf(out, "before: %s\nafter : %s\n", report.Before, report.After)
	fmt.Fprintf(out, "comparable journeys (completed in both): %d\n", len(report.ComparableJourneys))
	if len(report.ExcludedJourneys) > 0 {
		fmt.Fprintf(out, "excluded from the totals: %s\n", strings.Join(report.ExcludedJourneys, ", "))
	}
	fmt.Fprintln(out)

	table := [][]string{{"dimension", "before", "after", "delta", "derivation"}}
	for _, row := range report.Dimensions {
		table = append(table, []string{
			row.Dimension,
			optional(row.Before),
			optional(row.After),
			signed(row.Delta),
			row.Derivation,
		})
	}
	renderTable(out, table)

	fmt.Fprintf(out, "\nPer-journey breakdown (before -> after)\n")
	headers := []string{"journey", "status"}
	labels := []string{}
	for _, row := range dimensionRows() {
		labels = append(labels, strings.TrimSpace(row.Label))
		headers = append(headers, shortLabel(row.Label))
	}
	journeyTable := [][]string{headers}
	for _, journey := range report.Journeys {
		line := []string{journey.ID, journey.BeforeStatus + "->" + journey.AfterStatus}
		for _, label := range labels {
			line = append(line, journey.Cells[label])
		}
		journeyTable = append(journeyTable, line)
	}
	renderTable(out, journeyTable)

	for _, note := range report.Notes {
		fmt.Fprintf(out, "\nnote: %s\n", note)
	}
}

func optional(value *int) string {
	if value == nil {
		return "null"
	}
	return strconv.Itoa(*value)
}

func signed(value *int) string {
	if value == nil {
		return "n/a"
	}
	if *value > 0 {
		return "+" + strconv.Itoa(*value)
	}
	return strconv.Itoa(*value)
}

func renderTable(out io.Writer, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for index, cell := range row {
			if index < len(widths) && len(cell) > widths[index] {
				widths[index] = len(cell)
			}
		}
	}
	for rowIndex, row := range rows {
		parts := []string{}
		for index, cell := range row {
			width := 0
			if index < len(widths) {
				width = widths[index]
			}
			parts = append(parts, cell+strings.Repeat(" ", width-len(cell)))
		}
		fmt.Fprintf(out, "  %s\n", strings.TrimRight(strings.Join(parts, "  "), " "))
		if rowIndex == 0 {
			separators := []string{}
			for _, width := range widths {
				separators = append(separators, strings.Repeat("-", width))
			}
			fmt.Fprintf(out, "  %s\n", strings.Join(separators, "  "))
		}
	}
}
