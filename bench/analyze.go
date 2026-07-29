package main

// analyzeSession computes the seven dimensions from a recorded agent session
// using the SAME classifier as `run` mode. One classifier, two input sources.
//
// Observed mode is honest about being weaker on two dimensions:
//   - model_runs is NOT observable: the agent's own model calls never cross
//     the process boundary. The best available stand-in is one `review
//     capture-result` per lens run plus recaptures, and it is labelled proxy.
//   - human_prompts in an agent session is zero by construction (no TTY), so
//     what is actually counted is the consent-skipped notice: times the tool
//     WOULD have asked.
//
// blocks and recovery_round_trips are strong here: the invocation sequence is
// exactly what happened.
func analyzeSession(records []SessionRecord) JourneyResult {
	accumulator := newAccumulator()
	result := JourneyResult{
		ID:     "observed-session",
		Title:  "Observed agent session",
		Source: "recorded through the gentle-ai-bench shim",
		Status: StatusCompleted,
	}

	unsupported := 0
	for _, record := range records {
		observation := Observation{
			Args:           record.Argv,
			ExitCode:       record.ExitCode,
			Stdout:         record.Stdout,
			Stderr:         record.Stderr,
			StdoutCaptured: record.StdoutCaptured,
			StderrCaptured: record.StderrCaptured,
		}
		gitCalls := record.GitCalls
		commandRecord := accumulator.observe(joinArgs(record.Argv), observation, gitCalls, false)
		accumulator.records = append(accumulator.records, commandRecord)
		if commandRecord.Unsupported {
			unsupported++
			result.UnsupportedSteps = append(result.UnsupportedSteps, joinArgs(record.Argv))
		}
	}

	result.Metrics = accumulator.metrics(
		"NOT a measurement: the agent's model calls never cross the process boundary. " +
			"This counts `review capture-result` invocations (one per lens run, plus recaptures) as a stand-in.")
	result.Commands = accumulator.records
	return result
}

func joinArgs(args []string) string {
	text := ""
	for index, arg := range args {
		if index > 0 {
			text += " "
		}
		text += arg
	}
	return text
}
