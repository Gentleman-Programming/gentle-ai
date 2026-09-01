package main

import (
	"fmt"
	"strings"
)

func compatExecuteUnboundRecovery(r *journeyRun) error {
	refused := r.run(compatDirectBaseDiffStartArgs(r.sandbox), false)
	if err := compatAssertNamedRefusal(refused, "unbound direct base-diff start"); err != nil {
		return err
	}

	combined := refused.Stdout + "\n" + refused.Stderr
	const marker = "`gentle-ai review start "
	start := strings.Index(combined, marker)
	if start < 0 {
		return fmt.Errorf("direct refusal named no backtick-delimited review start command: %q", combined)
	}
	tail := combined[start+1:]
	end := strings.IndexByte(tail, '`')
	if end < 0 {
		return fmt.Errorf("direct refusal did not close its recovery command: %q", combined)
	}
	command := tail[:end]
	if strings.Contains(command, "--agent") || strings.Contains(command, "<your-runtime-identity>") {
		return fmt.Errorf("unbound recovery guessed a runtime identity: %q", command)
	}
	args, err := printedCommandArguments(command)
	if err != nil {
		return err
	}
	recovered := r.run(args, false)
	if recovered.ExitCode != 0 {
		return fmt.Errorf("exact printed recovery command failed: exit=%d stdout=%q stderr=%q", recovered.ExitCode, recovered.Stdout, recovered.Stderr)
	}
	if strings.Contains(recovered.Stdout+recovered.Stderr, "review_transport_capability_unsupported") {
		return fmt.Errorf("unbound recovery reached transport admission: stdout=%q stderr=%q", recovered.Stdout, recovered.Stderr)
	}
	return nil
}

func compatibilityRuntimeIdentityJourneys() []Journey {
	return []Journey{{
		ID:     "cw04-unbound-recovery-executes-without-agent-guess",
		Review: reviewOptedIn,
		Title:  "An unbound direct-route refusal emits and executes a negotiated recovery without an agent guess",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2885",
		Steps: []Step{
			{Name: "fixture: base commit, feature branch, committed candidate", Fixture: compatBaseDiffCandidate},
			{Name: "extract and execute the exact unbound recovery without transport admission", Requires: startCapability, Composite: compatExecuteUnboundRecovery},
		},
	}}
}
