// Command gentle-ai-bench measures the FRICTION of driving gentle-ai's review
// lifecycle, so a "before" binary and an "after" binary can be compared.
//
// Its core corpus is a black box: it drives a gentle-ai binary as a subprocess
// and never instruments the product, so it works against any build including
// old releases. It is deterministic and offline: no real model call is ever
// made.
//
// A run may additionally select an opt-in AXIS with --axis. An axis measures
// states the CLI cannot construct, so it is not black-box and not portable
// across builds; it declares that itself and the report prints the declaration
// next to the journeys it contributed. No axis runs unless it is named. See
// axis.go and README.md.
//
// It deliberately does NOT measure wall-clock time or real model tokens, and
// it deliberately does NOT emit a single composite score. See README.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		os.Exit(commandRun(os.Args[2:]))
	case "compare":
		os.Exit(commandCompare(os.Args[2:]))
	case "record":
		os.Exit(commandRecord(os.Args[2:]))
	case "analyze":
		os.Exit(commandAnalyze(os.Args[2:]))
	case "__shim":
		os.Exit(commandShim(os.Args[2:]))
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `gentle-ai-bench — friction benchmark for the gentle-ai review lifecycle

  run      gentle-ai-bench run --binary <path> --out results.json
           Drive the built-in journey corpus against a binary (driven mode).
           --axis <name>[,<name>] adds an opt-in axis to the black-box core.

  record   gentle-ai-bench record --binary <path> --out session.jsonl
           Print the PATH line that records a real agent session (observed mode).

  analyze  gentle-ai-bench analyze --session session.jsonl --out results.json
           Compute the same dimensions from a recorded session.

  compare  gentle-ai-bench compare --before a.json --after b.json
           Per-dimension table. Refuses to compare driven against observed.

It does not measure wall-clock time, real model tokens, or a composite score.
`)
}

func commandRun(args []string) int {
	return commandRunWith(args, executable, Journeys)
}

func commandRunWith(args []string, isExecutable func(string) bool, journeys func() []Journey) int {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	binary := flags.String("binary", "", "path to the gentle-ai binary to drive")
	out := flags.String("out", "results.json", "where to write the machine-readable results")
	only := flags.String("only", "", "comma-separated journey ids to run (default: all)")
	axisFlag := flags.String("axis", "",
		"comma-separated opt-in axes to add to the core corpus, or `all` (default: none). Registered: "+
			strings.Join(append([]string{}, axisNames()...), ", "))
	_ = flags.Parse(args)

	if strings.TrimSpace(*binary) == "" {
		fmt.Fprintln(os.Stderr, "run requires --binary")
		return 2
	}
	resolved, err := filepath.Abs(*binary)
	if err != nil || !isExecutable(resolved) {
		fmt.Fprintf(os.Stderr, "cannot execute binary %q\n", *binary)
		return 2
	}

	axes, err := selectAxes(*axisFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	// The whole corpus is validated before anything runs, --only or not: a
	// broken author-declared exemption is a corpus error, and the run that
	// would have quietly reported a number based on it must not start. The same
	// applies to an axis, whose declaration is a claim about how its numbers
	// were obtained.
	core := journeys()
	planned := append([]Journey{}, core...)
	for _, axis := range axes {
		planned = append(planned, axis.Journeys()...)
	}
	if err := validateAxes(axes, core); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	if err := validateCorpus(planned); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	requested := requestedJourneyIDs(*only)
	selected := map[string]bool{}
	for _, id := range requested {
		selected[id] = true
	}
	resolvedIDs := []string{}
	for _, journey := range planned {
		if selected[journey.ID] {
			resolvedIDs = append(resolvedIDs, journey.ID)
		}
	}

	results := Results{
		Schema:        ResultsSchema,
		Mode:          ModeDriven,
		Binary:        resolved,
		BinaryVersion: binaryVersion(resolved),
	}
	if len(requested) > 0 && len(resolvedIDs) == 0 {
		results.RequestedSelectors = requested
		results.ResolvedIDs = &resolvedIDs
		results.RunStatus = "failed"
		results.FailureReason = "empty_selected_population"
		if err := writeJSON(*out, results); err != nil {
			fmt.Fprintf(os.Stderr, "write results: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "no journeys matched --only selectors: %s\n", strings.Join(requested, ", "))
		return runExitCode(results)
	}
	run := func(journey Journey, axis string) bool {
		if len(selected) > 0 && !selected[journey.ID] {
			return false
		}
		fmt.Fprintf(os.Stderr, "running %s ...\n", journey.ID)
		result := runJourney(resolved, journey)
		result.Axis = axis
		results.Journeys = append(results.Journeys, result)
		return true
	}
	coreRun := 0
	for _, journey := range core {
		if run(journey, "") {
			coreRun++
		}
	}
	results.CoreJourneys = coreRun
	// The provenance names the journeys that actually RAN, not the ones the
	// axis could have contributed: under --only the two differ, and a record
	// claiming a journey that never ran would be provenance that lies.
	for _, axis := range axes {
		contributed := []Journey{}
		for _, journey := range axis.Journeys() {
			if run(journey, axis.Name) {
				contributed = append(contributed, journey)
			}
		}
		results.Axes = append(results.Axes, axisRecord(axis, contributed))
	}
	sortJourneys(results.Journeys)
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)
	results.Notes = []string{
		"Driven mode: every journey ran in a fresh temp dir with its own HOME, XDG_*, throwaway git repo and local bare remote.",
		"That HOME is a fresh install, so receipt-driven development starts off. Every journey declares its own precondition: one that reviews opted in first through `gentle-ai review mode enable --scope global`, uncounted; one whose subject is the switch touched it not at all.",
		"Reviewer results were synthesized from the binary's own collect envelope. No model was called.",
		"No wall-clock timing is measured or reported.",
		"by_design is a carve-out from out_of_band, not a subtraction from it: those blocks are still blocks and still in the total. Every one is listed with its declared shape and the verified quote of the product's own next-action text.",
	}

	if err := writeJSON(*out, results); err != nil {
		fmt.Fprintf(os.Stderr, "write results: %v\n", err)
		return 1
	}
	writeRunReport(os.Stdout, results)
	fmt.Fprintf(os.Stderr, "\nwrote %s\n", *out)
	if exit := runExitCode(results); exit != 0 {
		fmt.Fprintf(os.Stderr,
			"%d journey(s) FAILED: the run measured nothing for those rows, so it exits nonzero rather than letting a gate read it as success. The results file above still holds everything that was observed.\n",
			results.JourneysFailed)
		return exit
	}
	return 0
}

func requestedJourneyIDs(value string) []string {
	requested := []string{}
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			requested = append(requested, id)
		}
	}
	return requested
}

// runExitCode is the fail-closed rule for `run`, pinned because community
// issue #1883 found the opposite: a corpus run with failed journeys exited 0,
// and a CI gate reading that exit saw success in a run that measured nothing
// for the failed rows.
//
// `failed` fails the run: a failed journey is the harness unable to build or
// prove its fixture, or an assertion (a secret echoed, a state that stopped
// being what it claims) firing — either way the row's numbers do not exist and
// an exit 0 would launder that into "measured clean".
//
// `unsupported` deliberately does NOT fail the run. Driving an older binary is
// a designed use of this tool, and "this build lacks that CLI surface" is a
// real measurement, honestly labelled, excluded from totals, and rendered as
// `unsup` in every table. Failing on it would make the cross-version
// comparison — the tool's whole reason to exist — impossible to script. The
// summary line and the per-journey table keep the two impossible to conflate.
func runExitCode(results Results) int {
	if results.RunStatus == "failed" || results.JourneysFailed > 0 {
		return 1
	}
	return 0
}

func commandRecord(args []string) int {
	flags := flag.NewFlagSet("record", flag.ExitOnError)
	binary := flags.String("binary", "", "path to the real gentle-ai binary the shim delegates to")
	out := flags.String("out", "session.jsonl", "where the shim appends recorded invocations")
	_ = flags.Parse(args)

	if strings.TrimSpace(*binary) == "" {
		fmt.Fprintln(os.Stderr, "record requires --binary")
		return 2
	}
	if err := setupRecording(*binary, *out); err != nil {
		fmt.Fprintf(os.Stderr, "record: %v\n", err)
		return 1
	}
	return 0
}

func commandShim(args []string) int {
	real, logPath := "", ""
	forwarded := []string{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--real":
			index++
			if index < len(args) {
				real = args[index]
			}
		case "--log":
			index++
			if index < len(args) {
				logPath = args[index]
			}
		case "--":
			forwarded = append(forwarded, args[index+1:]...)
			index = len(args)
		}
	}
	if real == "" || logPath == "" {
		fmt.Fprintln(os.Stderr, "gentle-ai-bench shim: missing --real or --log")
		return 126
	}
	return runShim(real, logPath, forwarded)
}

func commandAnalyze(args []string) int {
	flags := flag.NewFlagSet("analyze", flag.ExitOnError)
	session := flags.String("session", "", "recorded session JSONL")
	out := flags.String("out", "results-observed.json", "where to write the machine-readable results")
	_ = flags.Parse(args)

	if strings.TrimSpace(*session) == "" {
		fmt.Fprintln(os.Stderr, "analyze requires --session")
		return 2
	}
	records, err := readSession(*session)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analyze: %v\n", err)
		return 1
	}

	journey := analyzeSession(records)
	results := Results{
		Schema:   ResultsSchema,
		Mode:     ModeObserved,
		Binary:   *session,
		Journeys: []JourneyResult{journey},
	}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)
	results.Notes = []string{
		"Observed mode: dimensions are computed from what a real agent session actually invoked.",
		"model_runs is a PROXY here, not a measurement: the agent's model calls never cross the process boundary.",
		"human_prompts counts the consent-skipped notice, i.e. times the tool WOULD have asked; an agent session has no TTY.",
		"blocks and recovery_round_trips are exact: the invocation sequence is what happened.",
		"out_of_band counts every block whose output named no runnable command, including ones that are correct behaviour: a refusal while reviews are switched off, and the disabled/unmanaged delivery report that exits 0. Read each block's message before treating the count as a defect count.",
	}

	if err := writeJSON(*out, results); err != nil {
		fmt.Fprintf(os.Stderr, "write results: %v\n", err)
		return 1
	}
	writeRunReport(os.Stdout, results)
	fmt.Fprintf(os.Stderr, "\nwrote %s\n", *out)
	return 0
}

func commandCompare(args []string) int {
	flags := flag.NewFlagSet("compare", flag.ExitOnError)
	beforePath := flags.String("before", "", "results JSON from the before binary")
	afterPath := flags.String("after", "", "results JSON from the after binary")
	out := flags.String("out", "", "optional machine-readable comparison JSON")
	_ = flags.Parse(args)

	if strings.TrimSpace(*beforePath) == "" || strings.TrimSpace(*afterPath) == "" {
		fmt.Fprintln(os.Stderr, "compare requires --before and --after")
		return 2
	}
	before, err := readResults(*beforePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		return 1
	}
	after, err := readResults(*afterPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		return 1
	}
	if before.Mode != after.Mode {
		fmt.Fprintf(os.Stderr,
			"refusing to compare %s mode against %s mode: they measure different populations.\n"+
				"A driven run executes a fixed corpus; an observed run captures whatever one agent happened to do.\n"+
				"Compare driven against driven, or observed against observed.\n",
			before.Mode, after.Mode)
		return 2
	}

	report := buildComparison(before, after)
	if strings.TrimSpace(*out) != "" {
		if err := writeJSON(*out, report); err != nil {
			fmt.Fprintf(os.Stderr, "write comparison: %v\n", err)
			return 1
		}
	}
	writeCompareReport(os.Stdout, report)
	return 0
}

func readResults(path string) (Results, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Results{}, err
	}
	var results Results
	if err := json.Unmarshal(content, &results); err != nil {
		return Results{}, fmt.Errorf("%s: %w", path, err)
	}
	if results.Mode == "" {
		return Results{}, fmt.Errorf("%s: results carry no mode", path)
	}
	return results, nil
}

func writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func executable(path string) bool {
	return executableForGOOS(path, runtime.GOOS)
}

func executableForGOOS(path, goos string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return executableMode(info.Mode(), goos)
}

func executableMode(mode os.FileMode, goos string) bool {
	// Windows does not preserve Unix executable bits for native binaries.
	return goos == "windows" || mode&0o111 != 0
}

func binaryVersion(path string) string {
	output, err := exec.Command(path, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
