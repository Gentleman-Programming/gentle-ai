// Command crosslane is the local cross-lane integration battery.
//
// It drives one real gentle-ai binary (--binary) end to end across the
// integration boundaries where host runtimes meet the Go facade:
//
//   - opencode lane: the REAL OpenCode transport plugin bytes
//     (internal/assets/opencode/plugins/opencode-review-transport.ts) driven
//     through an emulated Task hook surface with HOST-assembled binding
//     frames, against a live scratch-repo lineage. Covers the lens frame and
//     the validator role frame, in both host-shaped and Go-shaped variants.
//   - claude lane: one low-risk lifecycle ending in authority burn and five
//     ordinary-policy gates, plus one medium-candidate consent/v3 round-trip.
//     With --with-model it additionally runs the real compiled claude-code
//     reviewer runtime, which is why this battery lives outside CI.
//   - advisory lane: the middle path neither of the two lanes above reaches —
//     a review that arrives at an approved receipt WHILE carrying findings
//     that do not block. Fully deterministic (the reviewer result is supplied
//     through capture-result --input), so it costs no model or host spend.
//   - schema lane: every envelope captured along the way is validated
//     against the published schemas in contracts/review-integration/.
//     Any emitter/schema divergence fails the battery.
//
// The battery is intentionally honest: known-red checks (host binding frames
// pending fix/opencode-host-binding, schema gaps) FAIL and are annotated,
// because red at the exact seam where field defects escaped is the battery
// proving its worth.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run())
}

// run holds the battery body so deferred cleanup (work-root removal or the
// --keep-work banner) always executes before the process exits nonzero.
func run() int {
	binary := flag.String("binary", "", "path to the gentle-ai binary under test (required)")
	withModel := flag.Bool("with-model", false, "include the real reviewer model run (uses the dev subscription)")
	withHost := flag.Bool("with-host", false, "spawn REAL host applications (codex exec, pi print mode, an opencode session) end to end (uses the dev subscription)")
	keepWork := flag.Bool("keep-work", false, "keep the scratch working directory for inspection")
	flag.Parse()

	if *binary == "" {
		fmt.Fprintln(os.Stderr, "crosslane: --binary <path> is required")
		return 2
	}
	resolved, err := filepath.Abs(*binary)
	if err == nil {
		_, err = os.Stat(resolved)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "crosslane: binary %q is not usable: %v\n", *binary, err)
		return 2
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "crosslane: %v\n", err)
		return 2
	}
	workRoot, err := os.MkdirTemp("", "gentle-ai-crosslane-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "crosslane: %v\n", err)
		return 2
	}

	b := &battery{
		binary:      resolved,
		repoRoot:    repoRoot,
		workRoot:    workRoot,
		withModel:   *withModel,
		withHost:    *withHost,
		sandboxHome: filepath.Join(workRoot, "home"),
		lineages:    map[string]lineageScope{},
	}
	if !*keepWork {
		defer os.RemoveAll(workRoot)
	} else {
		defer fmt.Printf("\nwork directory kept: %s\n", workRoot)
	}
	if err := os.MkdirAll(b.sandboxHome, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "crosslane: %v\n", err)
		return 2
	}
	// Receipt-driven development is opt-in, so the battery opts in for its own
	// sandbox. The lifecycle lanes exist to exercise reviews; leaving the switch
	// at its shipped default would make every one of them a refusal rather than
	// a test. This runs through the real `review mode enable` so the battery
	// depends on the same resolution path a user does.
	if _, stderr, code := b.run(b.sandboxHome, "review", "mode", "enable", "--scope", "global", "--cwd", repoRoot); code != 0 {
		fmt.Fprintf(os.Stderr, "crosslane: enable sandbox review mode: %s\n", firstLine(stderr))
		return 2
	}

	b.captureCapabilities()
	b.runOpenCodeLane()
	b.runClaudeLane()
	b.runAdvisoryLane()
	b.runCodexLane()
	b.runHostLanes()
	b.runSchemaLane()

	failed := b.printTable()
	if failed > 0 {
		return 1
	}
	return 0
}

func (b *battery) printTable() int {
	fmt.Println()
	fmt.Println("cross-lane battery results")
	fmt.Printf("binary: %s\n", b.binary)
	fmt.Println()
	laneWidth, nameWidth := len("lane"), len("check")
	for _, c := range b.checks {
		laneWidth = max(laneWidth, len(c.Lane))
		nameWidth = max(nameWidth, len(c.Name))
	}
	fmt.Printf("%-*s  %-*s  %-6s  %s\n", laneWidth, "lane", nameWidth, "check", "status", "note")
	failed := 0
	for _, c := range b.checks {
		fmt.Printf("%-*s  %-*s  %-6s  %s\n", laneWidth, c.Lane, nameWidth, c.Name, c.Status, c.Note)
		if c.Status == statusFail {
			failed++
		}
	}
	fmt.Println()
	fmt.Printf("total: %d checks, %d failed\n", len(b.checks), failed)
	if len(b.hostCosts) > 0 {
		fmt.Println()
		fmt.Println("token cost (real model runs on the dev subscription):")
		for _, cost := range b.hostCosts {
			fmt.Printf("  %s\n", cost)
		}
	}
	return failed
}
