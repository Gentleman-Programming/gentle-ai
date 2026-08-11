package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// journeys_advisory.go covers the `gentle-ai review advisory prompt` and
// `gentle-ai review advisory validate` CLI surface (internal/cli/review_advisory.go),
// added once the shared Go advisory reviewer transport landed
// (rdd-advisory-transport SKILL.md, shared-advisory-transport-proposal.md).
//
// Both subcommands are read-only by contract: they derive their complete
// request from frozen native authority (the same opaque repository-context
// handle and lens `review lens-context` already consumes) and never create
// review authority, a receipt, or a gate decision. That is exactly what these
// journeys prove by continuing the ORDINARY high-risk lifecycle around every
// advisory call: if advisory transport ever mutated capture state or stranded
// a lineage, the surrounding capture-all-lenses/finalize/gate steps would
// fail instead of completing exactly as they do in j02.
//
// Every journey here starts through the NEGOTIATED v2 contract
// (gentle-ai.review-integration/v2), not the bare non-negotiated `review
// start` j02 uses. That is a deliberate, verified choice, not a stylistic
// one: advisory prompt/validate resolve their whole request from the opaque
// --repository-context handle alone (resolveAdvisoryRequest), and driving
// this corpus live surfaced that the bare "manual compatibility path"
// (docs/review-integration.md) can publish a repository context whose
// STATUS-displayed artifact_subject disagrees with what
// resolveAdvisoryRequest independently recomputes for the same handle and
// lens -- advisory validate then correctly refuses it as a subject mismatch.
// That asymmetry is a pre-existing product question this change does not
// touch (capture path/lifecycle untouchable per this change's hard
// constraints); it is also not a gap in THESE journeys, because the
// negotiated v2 start is the exact setup the advisory contract's own tests
// use (internal/cli/review_advisory_test.go's newCandidateInspectionReview)
// and the substrate the shared contract is documented as being built on
// (docs/review-integration.md: "v2 is the native-Git contract").

// advisoryMismatchedSubjectHash is a well-formed but never-bound subject
// hash, used to prove `review advisory validate` refuses a result that does
// not echo the requested subject (reviewtransaction's canonical binding
// check), the same way a fabricated reviewer result is refused everywhere
// else in this corpus (j12).
var advisoryMismatchedSubjectHash = "sha256:" + strings.Repeat("0", 64)

var advisoryPromptCapability = &Capability{
	Verb: []string{"review", "advisory", "prompt"}, Flags: []string{"--repository-context", "--lens", "--runtime"},
}
var advisoryValidateCapability = &Capability{
	Verb: []string{"review", "advisory", "validate"}, Flags: []string{"--repository-context", "--lens", "--input"},
}

// startAdvisoryReview starts a fresh review through the negotiated v2
// contract, running the printed execute transition. Every advisory journey
// below stages a high-risk auth-code change (stageAuthCode), so the printed
// command always carries a relayed one-time consent question; these
// journeys are not testing consent UX, so it is answered "granted" exactly
// once here, the same value waveOneJourneys already uses for its own v2
// starts.
func startAdvisoryReview(r *journeyRun) error {
	envelope, err := readStatusFor(r, "--contract", reviewContractV2)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "execute" {
		return fmt.Errorf("expected an execute transition to start the review, got %q", envelope.NextTransition.Kind)
	}
	args, err := printedCommandArguments(envelope.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	for index, argument := range args {
		if argument == "--consent=relay" {
			args[index] = "--consent=granted"
		}
	}
	r.run(args, false)
	return nil
}

// captureAdvisoryBinding reads the negotiated v2 collect envelope exactly as
// captureAllLensesFor does, but stops after the FIRST pending reviewer_result
// input instead of capturing it: advisory prompt/validate journeys need the
// same opaque repository-context handle, lens, subject hash, and manifest
// path a real reviewer's binding would carry, without spending that lens's
// one capture yet. Later steps in each journey below still call
// captureAllLensesFor to actually finish the review, proving the advisory
// calls in between left this exact binding capturable.
func captureAdvisoryBinding(r *journeyRun) error {
	envelope, err := readStatusFor(r, "--contract", reviewContractV2)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 {
		return fmt.Errorf("expected a reviewer-result collect transition, got kind %q", envelope.NextTransition.Kind)
	}
	input := envelope.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" {
		return fmt.Errorf("expected a reviewer_result collect input, got %q", input.Name)
	}
	repositoryContext := envelope.argument("repository-context")
	lens := envelope.argument("lens")
	if repositoryContext == "" || lens == "" {
		return errors.New("reviewer_result collect input carried no repository-context/lens argument")
	}
	paths := envelope.paths()
	if len(paths) == 0 {
		return errors.New("reviewer_result collect input carried no changed-path manifest")
	}
	r.sandbox.Scratch["advisory-repository-context"] = repositoryContext
	r.sandbox.Scratch["advisory-lens"] = lens
	r.sandbox.Scratch["advisory-subject-hash"] = input.ArtifactSubject.SubjectHash
	r.sandbox.Scratch["advisory-path"] = paths[0]
	return nil
}

// finishAdvisoryReview completes the review exactly as j02 does, through the
// same negotiated v2 contract startAdvisoryReview began it with: capture
// every remaining lens, finalize results, capture final evidence, finalize
// evidence, and pass the post-apply gate. Reaching this cleanly is the
// journey's real assertion that the advisory calls before it never touched
// capture state or the lineage.
func finishAdvisoryReview(r *journeyRun) error {
	if err := captureAllLensesFor(r, "--contract", reviewContractV2); err != nil {
		return err
	}
	r.run(productArgsFor(r, "review", "finalize", "--captured-results=true"), false)
	if err := captureFinalEvidenceFor(r, "--contract", reviewContractV2); err != nil {
		return err
	}
	r.run(productArgsFor(r, "review", "finalize", "--captured-evidence=true"), false)
	r.run(productArgsFor(r, "review", "validate", "--gate", "post-apply"), false)
	return nil
}

// advisoryPromptArgs runs `review advisory prompt` against the bound handle
// and lens captureAdvisoryBinding recorded, for one runtime. It never appends
// --cwd: advisory prompt/validate resolve entirely from the opaque
// repository-context handle, exactly like `review lens-context`, and accept
// no other caller-supplied location.
func advisoryPromptArgs(runtime string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		return []string{
			"review", "advisory", "prompt",
			"--repository-context", sandbox.Scratch["advisory-repository-context"],
			"--lens", sandbox.Scratch["advisory-lens"],
			"--runtime", runtime,
		}, nil
	}
}

// advisoryPromptArgsForLens is advisoryPromptArgs with an explicit lens
// override, for the unselected-lens refusal below.
func advisoryPromptArgsForLens(lens, runtime string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		return []string{
			"review", "advisory", "prompt",
			"--repository-context", sandbox.Scratch["advisory-repository-context"],
			"--lens", lens,
			"--runtime", runtime,
		}, nil
	}
}

// advisoryValidateArgs runs `review advisory validate` against the bound
// handle and lens, validating whichever result file a prior Fixture step
// wrote to sandbox.Scratch[resultScratchKey].
func advisoryValidateArgs(resultScratchKey string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		return []string{
			"review", "advisory", "validate",
			"--repository-context", sandbox.Scratch["advisory-repository-context"],
			"--lens", sandbox.Scratch["advisory-lens"],
			"--input", sandbox.Scratch[resultScratchKey],
		}, nil
	}
}

// writeAdvisoryResult writes one reviewer-result fixture bound to the exact
// manifest path and lens captureAdvisoryBinding recorded. An empty
// subjectHash means "use the real bound subject" (the admitted-result
// journey); a non-empty one is a deliberately wrong subject (the refused
// mismatch journey), the same fabrication shape rejectedThenRecapture (j12)
// uses for capture-result.
func writeAdvisoryResult(resultScratchKey, subjectHash string) func(*Sandbox) error {
	return func(sandbox *Sandbox) error {
		hash := subjectHash
		if hash == "" {
			hash = sandbox.Scratch["advisory-subject-hash"]
		}
		path := sandbox.Scratch["advisory-path"]
		payload, err := json.Marshal(map[string]any{
			"subject_hash": hash,
			"lens":         sandbox.Scratch["advisory-lens"],
			"inspection":   map[string]any{"status": "completed", "paths": []string{path}},
			"findings":     []any{},
			"evidence":     []string{"inspected the complete frozen candidate scope named by the capture binding"},
		})
		if err != nil {
			return err
		}
		resultPath, err := writeScratch(sandbox, resultScratchKey+".json", payload)
		if err != nil {
			return err
		}
		sandbox.Scratch[resultScratchKey] = resultPath
		return nil
	}
}

// rememberAdvisoryPrompt stashes one runtime's advisory prompt output so a
// later step can compare all three for byte-identity.
func rememberAdvisoryPrompt(scratchKey string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		sandbox.Scratch[scratchKey] = observation.Stdout
		return nil
	}
}

// assertAdvisoryPromptsMatch proves the one canonical provider-rendered
// prompt (runtimeReviewerPrompt / advisoryreview.Prompt) is genuinely shared:
// Claude Code, OpenCode, and Codex must receive byte-identical text for the
// same bound lens, never a runtime-specific duplicate.
func assertAdvisoryPromptsMatch(sandbox *Sandbox, observation Observation) error {
	sandbox.Scratch["advisory-prompt-codex"] = observation.Stdout
	claude := sandbox.Scratch["advisory-prompt-claude-code"]
	opencode := sandbox.Scratch["advisory-prompt-opencode"]
	codex := sandbox.Scratch["advisory-prompt-codex"]
	if claude == "" || claude != opencode || claude != codex {
		return fmt.Errorf("advisory prompt diverged across runtimes: claude-code=%dB opencode=%dB codex=%dB",
			len(claude), len(opencode), len(codex))
	}
	if !strings.Contains(claude, "advisory model output for native Go admission") {
		return fmt.Errorf("advisory prompt is missing its canonical advisory-boundary sentence: %s", claude)
	}
	return nil
}

// assertAdvisoryValidateAdmitted proves validation of a clean result stayed
// transport-only: it must never mention a receipt, delivery, or a pass
// verdict (SKILL.md: "Runtime output is advisory: it cannot create
// authority, mutate review state, mint a receipt, or open a gate"), and it
// must actually report the result as transport-admitted.
func assertAdvisoryValidateAdmitted(sandbox *Sandbox, observation Observation) error {
	lower := strings.ToLower(observation.Stdout)
	for _, forbidden := range []string{"receipt", "delivery", "\"pass\""} {
		if strings.Contains(lower, forbidden) {
			return fmt.Errorf("advisory validate output claims authority (%q): %s", forbidden, observation.Stdout)
		}
	}
	var validated struct {
		TransportValidated bool `json:"transport_validated"`
		FindingCount       int  `json:"finding_count"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &validated); err != nil || !validated.TransportValidated {
		return fmt.Errorf("advisory validate did not admit the clean result (err=%v): %s", err, observation.Stdout)
	}
	return nil
}

// assertStderrContains is the shared refusal-proof helper for both advisory
// refusal journeys below: it fails the journey if the exact refusal this
// step claims to demonstrate did not actually happen, so a passing journey
// can never mean "some unrelated error happened to occur here".
func assertStderrContains(substr string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		if !strings.Contains(observation.Stderr, substr) {
			return fmt.Errorf("expected stderr to contain %q, got: %s", substr, observation.Stderr)
		}
		return nil
	}
}

func advisoryJourneys() []Journey {
	return []Journey{
		{
			ID:     "j68-review-advisory-prompt-shared-across-runtimes",
			Title:  "Advisory prompt: Claude Code, OpenCode, and Codex receive the identical canonical prompt",
			Source: "rdd-advisory-transport SKILL.md + shared-advisory-transport-proposal.md migration slice 6",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start (negotiated v2, consent granted)", Requires: startContractCapability, Composite: startAdvisoryReview},
				{Name: "bind the first pending lens", Requires: statusCapability, Composite: captureAdvisoryBinding},
				{Name: "advisory prompt for claude-code", Requires: advisoryPromptCapability,
					Args: advisoryPromptArgs("claude-code"), After: rememberAdvisoryPrompt("advisory-prompt-claude-code")},
				{Name: "advisory prompt for opencode", Requires: advisoryPromptCapability,
					Args: advisoryPromptArgs("opencode"), After: rememberAdvisoryPrompt("advisory-prompt-opencode")},
				{Name: "advisory prompt for codex", Requires: advisoryPromptCapability,
					Args: advisoryPromptArgs("codex"), After: assertAdvisoryPromptsMatch},
				{Name: "finish the review", Requires: captureResultCapability, Composite: finishAdvisoryReview},
			},
		},
		{
			ID:     "j69-review-advisory-prompt-refuses-unselected-lens-and-unadvertised-runtime",
			Title:  "Failure path: advisory prompt refuses an unselected lens and an unadvertised runtime, then the lifecycle still completes",
			Source: "rdd-advisory-transport SKILL.md: \"Capability advertisement requires shared-contract conformance plus organic runtime proof\"",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start (negotiated v2, consent granted)", Requires: startContractCapability, Composite: startAdvisoryReview},
				{Name: "bind the first pending lens", Requires: statusCapability, Composite: captureAdvisoryBinding},
				{Name: "advisory prompt refuses an unselected lens", Requires: advisoryPromptCapability,
					Args:  advisoryPromptArgsForLens("review-nonexistent", "claude-code"),
					After: assertStderrContains("lens_context_lens_not_selected")},
				{Name: "advisory prompt refuses an unadvertised runtime", Requires: advisoryPromptCapability,
					Args:  advisoryPromptArgs("gemini"),
					After: assertStderrContains("unavailable for runtime")},
				{Name: "finish the review", Requires: captureResultCapability, Composite: finishAdvisoryReview},
			},
		},
		{
			ID:     "j70-review-advisory-validate-admits-clean-result",
			Title:  "Advisory validate admits a clean model result without claiming receipt or delivery authority",
			Source: "rdd-advisory-transport SKILL.md: \"Runtime output is advisory ... Only Go admission does\"",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start (negotiated v2, consent granted)", Requires: startContractCapability, Composite: startAdvisoryReview},
				{Name: "bind the first pending lens", Requires: statusCapability, Composite: captureAdvisoryBinding},
				{Name: "fixture: write a clean advisory result", Fixture: writeAdvisoryResult("advisory-result-clean", "")},
				{Name: "advisory validate admits the clean result", Requires: advisoryValidateCapability,
					Args: advisoryValidateArgs("advisory-result-clean"), After: assertAdvisoryValidateAdmitted},
				{Name: "finish the review", Requires: captureResultCapability, Composite: finishAdvisoryReview},
			},
		},
		{
			ID:     "j71-review-advisory-validate-refuses-mismatched-subject",
			Title:  "Failure path: advisory validate refuses a result stamped with the wrong subject, then the lifecycle still completes",
			Source: "community failure path j12's fabricated-subject shape, applied to the advisory transport seam",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start (negotiated v2, consent granted)", Requires: startContractCapability, Composite: startAdvisoryReview},
				{Name: "bind the first pending lens", Requires: statusCapability, Composite: captureAdvisoryBinding},
				{Name: "fixture: write a mismatched-subject advisory result",
					Fixture: writeAdvisoryResult("advisory-result-mismatched", advisoryMismatchedSubjectHash)},
				{Name: "advisory validate refuses the mismatched subject", Requires: advisoryValidateCapability,
					Args:  advisoryValidateArgs("advisory-result-mismatched"),
					After: assertStderrContains("does not match the requested subject")},
				{Name: "finish the review", Requires: captureResultCapability, Composite: finishAdvisoryReview},
			},
		},
		{
			ID:     "j76-claude-advisory-result-reaches-delivery",
			Title:  "Claude advisory result: provider-bound review reaches terminal delivery without reviewer tools",
			Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2692",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: stage auth code", Fixture: stageAuthCode},
				{Name: "review start (negotiated v2, consent granted)", Requires: startContractCapability, Composite: startAdvisoryReview},
				{Name: "bind the first pending lens", Requires: statusCapability, Composite: captureAdvisoryBinding},
				{Name: "render the canonical Claude advisory prompt", Requires: advisoryPromptCapability,
					Args: advisoryPromptArgs("claude-code"), After: rememberAdvisoryPrompt("issue-2692-claude-prompt")},
				{Name: "fixture: write the provider-bound reviewer result", Fixture: writeAdvisoryResult("issue-2692-result", "")},
				{Name: "admit the provider-bound reviewer result", Requires: advisoryValidateCapability,
					Args: advisoryValidateArgs("issue-2692-result"), After: assertAdvisoryValidateAdmitted},
				{Name: "capture, finalize, and allow delivery", Requires: captureResultCapability, Composite: finishAdvisoryReview},
			},
		},
	}
}
