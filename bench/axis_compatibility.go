package main

import (
	"errors"
	"fmt"
	"strings"
)

// This axis exists because of a NAMED blind spot (issue #2447): the core
// corpus, and every axis added before this one, drive the NEGOTIATED review
// lifecycle exclusively (`review status --next-transition` -> execute /
// collect). The direct, manual, non-negotiated surfaces the project
// explicitly KEPT for "explicit/manual non-negotiated callers" --
// internal/assets/skills/_shared/review-ledger-contract.md: "Direct
// `gentle-ai review start` remains compatibility-supported for
// explicit/manual non-negotiated callers" -- were never in the corpus at
// all. A direct `review start --base-ref ... --committed-only` could create a
// lineage no reviewer lens could ever complete, and NOTHING in ~57 journeys
// and 4 prior axes would have noticed, because none of them ever drove that
// surface. This axis is the standing fix for that blind spot, not a one-off
// regression test for #2447 alone: it is where the next compatibility-surface
// gap belongs.
//
// Unlike damaged-store, this axis IS black-box: every journey drives the
// product's own CLI only, exactly like the core corpus. It is opt-in anyway,
// for the same reason real-world is: these are a different population
// (direct/manual invocations) from the negotiated-only core, and "N core
// journeys" must never silently grow to include them.
func init() {
	RegisterAxis(Axis{
		Name:     compatibilityAxis,
		Title:    "Non-negotiated, legacy-facing surfaces a real user (or a script, or an old integration) can still reach directly",
		BlackBox: true,
		Review:   reviewOptedIn,
		Properties: []string{
			"Black-box: every journey drives the product's own CLI only, exactly like the core corpus; nothing product-owned is read or written.",
			"Opt-in anyway: the core and every prior axis measure the NEGOTIATED lifecycle exclusively. This axis measures the DIRECT/manual compatibility surfaces the execution contract explicitly keeps alongside it. \"N core journeys\" and \"N plus compatibility\" must never look alike, because the whole point of #2447 is that the two populations are not interchangeable.",
			"cw01 pins the issue #2447 fix: a direct `review start --base-ref ... --committed-only` over a candidate large enough to select a reviewer lens now refuses before creating any lineage, naming the negotiated form. Before the fix (verified by running this exact journey against a pre-fix binary) it exited 0, created a lineage, and no reviewer lens could ever complete it -- the trap the issue reports. The journey also proves the refusal is idempotent (repeating the identical call produces byte-identical output), which is this benchmark's only black-box way to show nothing was persisted on the first attempt.",
			"cw02 pins a sibling surface of the same shape: the hyphenated `review-start` (note: no space) v1-compatibility verb, registered in internal/app/app.go alongside `review start`. It has refused unconditionally since before this axis existed. It belongs here because an axis about direct/legacy start surfaces that omitted the one already-closed door would be an incomplete map of exactly the territory #2447 is about.",
			"cw03 proves the surface #2447's fix deliberately leaves open still completes a review end to end: `review capture-result --cwd` (review-ledger-contract.md: \"Capture follows the native transition; opaque handles are cwd-independent and legacy bindings need --cwd\") drives a negotiated-started review through the terminal capture WITHOUT ever supplying --repository-context. This is the direct/manual capture path the execution contract still names as supported, and #3797 makes its final capture expose an exact acknowledgement before burn.",
			"cw04 reproduces issue #2885 at the public direct-route boundary, extracts the exact unbound recovery command from the refusal, and executes it without an agent guess or transport-admission failure.",
			"Surveyed from internal/app/app.go and excluded from this slice, with reasons: `review-resume`, `review-bundle-export`, `review-bundle-import`, and `review-validate` operate over authority the core corpus's ordinary lifecycle journeys already create and exercise through their own verbs -- they are not START-shaped, so they carry none of #2447's specific gap (a route that CREATES a lineage nothing can complete). `review-step` is structurally identical to cw02's already-closed shape (a permanent read-only refusal naming the compact verb) and would duplicate it rather than add coverage. The retired `--result` finalize flag no longer exists in the flag set at all (internal/cli/review_facade.go), so it is not a surface a caller can reach and is not a corpus member.",
		},
		Journeys: compatibilityJourneys,
	})
}

const compatibilityAxis = "compatibility"

// ---------------------------------------------------------------------------
// cw01 -- direct base-diff start refuses instead of trapping (issue #2447)
// ---------------------------------------------------------------------------

// compatBaseDiffCandidate reproduces the issue #2447 repro shape: a base
// commit, a feature branch, and a committed candidate on top -- exactly the
// sequence `gentle-ai review start --base-ref <base-sha> --committed-only`
// was reported against.
func compatBaseDiffCandidate(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	base, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["compat-base"] = base
	if err := sandbox.git(sandbox.Repo, "checkout", "-q", "-b", "feature/change"); err != nil {
		return err
	}
	content := "package feature\n\nfunc Compute(x int) int {\n\tif x > 10 {\n\t\treturn x * 2\n\t}\n\treturn x + 1\n}\n"
	if err := sandbox.write(sandbox.Repo+"/feature.go", content); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "add a method")
}

func compatDirectBaseDiffStartArgs(sandbox *Sandbox) []string {
	return []string{"review", "start", "--cwd", sandbox.Repo,
		"--base-ref", sandbox.Scratch["compat-base"], "--committed-only=true"}
}

// compatDirectBaseDiffRefusesTwice is the counted step: it drives the direct
// route once, asserts the refusal names a runnable `gentle-ai` continuation,
// then drives the IDENTICAL call again and asserts byte-identical output.
// A trap that had created a lineage on the first call would behave
// differently on the second (resume, collision, or a different error), so
// identical output across both calls is this black-box corpus's evidence
// that nothing was persisted -- it cannot read the authority store directly
// without giving up black-box status, which this axis declares it keeps.
func compatDirectBaseDiffRefusesTwice(r *journeyRun) error {
	args := compatDirectBaseDiffStartArgs(r.sandbox)
	first := r.run(args, false)
	if err := compatAssertNamedRefusal(first, "direct base-diff start with a real candidate"); err != nil {
		return err
	}
	second := r.run(args, false)
	if second.ExitCode != first.ExitCode || second.Stdout != first.Stdout || second.Stderr != first.Stderr {
		return fmt.Errorf(
			"repeating the identical refused start changed the result, which means the first attempt left something behind:\n"+
				"first:  exit=%d stdout=%q stderr=%q\nsecond: exit=%d stdout=%q stderr=%q",
			first.ExitCode, first.Stdout, first.Stderr, second.ExitCode, second.Stdout, second.Stderr)
	}
	return nil
}

// compatAssertNamedRefusal is the shared detector for cw01 and cw02: the
// observation must be a genuine block (nonzero exit) whose emitted bytes
// name a runnable `gentle-ai <verb> ...` continuation. It reuses the same
// HasRunnableCommand detector the corpus's own Classify function uses, so a
// journey's private assertion and the corpus's own in_band/out_of_band
// bookkeeping can never silently disagree about the same bytes.
func compatAssertNamedRefusal(observation Observation, what string) error {
	if observation.ExitCode == 0 {
		return fmt.Errorf("%s succeeded, want an up-front refusal: %s", what, observation.Stdout)
	}
	if !HasRunnableCommand(observation.Stdout + "\n" + observation.Stderr) {
		return fmt.Errorf("%s refused but named no runnable `gentle-ai` continuation: stdout=%q stderr=%q",
			what, observation.Stdout, observation.Stderr)
	}
	return nil
}

// ---------------------------------------------------------------------------
// cw02 -- the hyphenated `review-start` v1-compatibility verb always refuses
// ---------------------------------------------------------------------------

var compatHyphenatedStartCapability = &Capability{Verb: []string{"review-start"}, Flags: []string{"--cwd"}}

func compatHyphenatedStartArgs(sandbox *Sandbox) ([]string, error) {
	return []string{"review-start", "--cwd", sandbox.Repo,
		"--lineage", "compat-legacy-v1", "--policy-file", "unused-compatibility-probe.md"}, nil
}

// compatAssertHyphenatedStartRefuses checks the one property that makes cw02
// worth pinning: the refusal explicitly redirects to the real verb, `review
// start` (with a space), not just to "gentle-ai" generically -- a caller who
// typed the retired hyphenated form needs to land on the live one.
func compatAssertHyphenatedStartRefuses(_ *Sandbox, observation Observation) error {
	if err := compatAssertNamedRefusal(observation, "hyphenated `review-start` v1-compatibility verb"); err != nil {
		return err
	}
	combined := observation.Stdout + "\n" + observation.Stderr
	if !strings.Contains(combined, "gentle-ai review start") {
		return fmt.Errorf("review-start refusal did not name `gentle-ai review start` (with a space): %q", combined)
	}
	return nil
}

// ---------------------------------------------------------------------------
// cw03 -- negotiated start, then the direct --cwd last-capture closure path
// ---------------------------------------------------------------------------

// compatCaptureAllLensesDirectCwd is captureAllLenses's sibling: it drives
// the same collect loop STATUS dictates, but captures with --cwd on every
// round instead of the collect step's own --repository-context argument,
// proving the "legacy bindings need --cwd" compatibility path the execution
// contract names still completes a negotiated-started review end to end.
func compatCaptureAllLensesDirectCwd(r *journeyRun) error {
	for round := 0; round < 8; round++ {
		envelope, err := readStatus(r)
		if err != nil {
			return err
		}
		if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 ||
			envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
			return nil
		}
		result, err := synthesizeReviewerResult(
			envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, envelope.paths())
		if err != nil {
			return err
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("compat-reviewer-%d.json", round), result)
		if err != nil {
			return err
		}
		captured := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", envelope.argument("lineage"),
			"--target", envelope.argument("target"),
			"--expected-revision", envelope.argument("expected-revision"),
			"--lens", envelope.argument("lens"),
			"--order", envelope.argument("order"),
			"--input", path,
		}, true)
		if captured.ExitCode != 0 {
			return fmt.Errorf("direct --cwd capture-result failed: %s", captured.Stderr)
		}
		if err := compatAssertNoRepositoryContext(r.sandbox, captured); err != nil {
			return err
		}
	}
	return errors.New("direct --cwd capture loop did not converge")
}

// compatAssertNoRepositoryContext fails the journey the moment a counted
// invocation carries --repository-context: this journey exists specifically
// to prove the --cwd form alone completes the review, and a fixture that
// silently used the opaque-handle form instead would pass while proving
// nothing about the surface it claims to measure.
func compatAssertNoRepositoryContext(_ *Sandbox, observation Observation) error {
	for _, arg := range observation.Args {
		if arg == "--repository-context" || strings.HasPrefix(arg, "--repository-context=") {
			return fmt.Errorf("capture used --repository-context (args %v); this journey must complete on --cwd alone", observation.Args)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

func compatibilityJourneys() []Journey {
	journeys := []Journey{
		{
			ID:     "cw01-direct-start-refuses-uncompletable-base-diff",
			Review: reviewOptedIn,
			Title:  "Direct `review start --base-ref ... --committed-only` over a real candidate refuses up front and names the negotiated exit",
			Source: "issue #2447: the direct route created a lineage no reviewer lens could ever capture a result against, because its own response type never carries repository_context and the negotiated facade never rediscovers a lineage it did not create",
			Steps: []Step{
				{Name: "fixture: base commit, feature branch, committed candidate", Fixture: compatBaseDiffCandidate},
				{Name: "direct base-diff start refuses twice, identically, naming the negotiated form", Requires: startCapability,
					Composite: compatDirectBaseDiffRefusesTwice},
			},
		},
		{
			ID:     "cw02-hyphenated-review-start-always-refuses",
			Review: reviewOptedIn,
			Title:  "The hyphenated `review-start` v1-compatibility verb refuses unconditionally and names `gentle-ai review start`",
			Source: "internal/cli/review.go RunReviewStart: \"Read-only legacy v1 compatibility command. New authority is created with gentle-ai review start.\"",
			Steps: []Step{
				{Name: "fixture: base repo", Fixture: baseRepo},
				{Name: "hyphenated review-start always refuses", Requires: compatHyphenatedStartCapability,
					Args: compatHyphenatedStartArgs, After: compatAssertHyphenatedStartRefuses, AbortOnBlock: true},
			},
		},
		{
			ID:     "cw03-negotiated-start-then-direct-cwd-capture-completes",
			Review: reviewOptedIn,
			Title:  "#3587: a negotiated-started review closes on the final direct --cwd capture, never touching a repository-context handle",
			Source: "review-ledger-contract.md: \"Capture follows the native transition; opaque handles are cwd-independent and legacy bindings need --cwd\"; #3587 makes that final capture the terminal event for direct/manual callers",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "negotiated review start, run verbatim from next-transition", Requires: statusCapability,
					Composite: executeNextTransitionVerbatim},
				{Name: "capture every lens directly via --cwd, never --repository-context; the last capture closes", Requires: captureResultCapability,
					Composite: compatCaptureAllLensesDirectCwd},
				{Name: "the direct final capture exposes acknowledgement before the negotiated lineage burns", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageAcknowledged(r, r.sandbox.Lineage)
				}},
			},
		},
	}
	return append(journeys, compatibilityRuntimeIdentityJourneys()...)
}
