package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	atomicSiblingBindingKey = "atomic-sibling-binding"
	atomicBurnInitialKey    = "atomic-burn-initial-binding"
	atomicCorrectionLineage = "atomic-four-lens-correction"
)

var atomicReviewStatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--lineage", "--next-transition",
}}
var captureCorrectionPlanCapability = &Capability{Verb: []string{"review", "capture-correction-plan"}, Flags: []string{
	"--repository-context", "--lineage", "--target", "--expected-revision", "--request-hash", "--correction-lines",
}}

// Wave 3 now ratifies #3417's compact atomic boundary. The former environment
// activation and receipt-governed gate journeys belonged to the removed lineage
// implementation; these journeys exercise only the shipped worktree/candidate
// binding and the active transaction's compact continuation.
type atomicReviewStartResult struct {
	Action         string   `json:"action"`
	LineageID      string   `json:"lineage_id"`
	State          string   `json:"state"`
	SelectedLenses []string `json:"selected_lenses"`
}

func stageAtomicSiblingWorktrees(sandbox *Sandbox) error {
	linked := filepath.Join(sandbox.Root, "atomic-sibling-worktree")
	if err := sandbox.git(sandbox.Repo, "worktree", "add", "--detach", linked, "HEAD"); err != nil {
		return err
	}
	for _, root := range []string{sandbox.Repo, linked} {
		path := filepath.Join(root, "internal", "auth", "session.go")
		content := "package auth\n\nfunc CheckToken(token string) bool { return token != \"\" }\n"
		if err := sandbox.write(path, content); err != nil {
			return err
		}
		if err := sandbox.git(root, "add", "--", "internal/auth/session.go"); err != nil {
			return err
		}
	}
	sandbox.Scratch["atomic-sibling-worktree"] = linked
	return nil
}

func stageAtomicHighRiskCorrectionCandidate(sandbox *Sandbox) error {
	if err := stageCaptureEvidenceDescriptorCorrection(sandbox); err != nil {
		return err
	}
	path := filepath.Join(sandbox.Repo, "scripts", "atomic-review.sh")
	if err := sandbox.write(path, "#!/bin/sh\nprintf '%s\\n' atomic-review\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "--", "scripts/atomic-review.sh")
}

func readAtomicReviewStatus(r *journeyRun, lineage string) (statusEnvelope, error) {
	return readAtomicReviewStatusAt(r, r.sandbox.Repo, lineage)
}

func readAtomicReviewStatusAt(r *journeyRun, cwd, lineage string) (statusEnvelope, error) {
	args := []string{"review", "status", "--contract", reviewContractV2, "--next-transition", "--cwd", cwd}
	if lineage != "" {
		args = append(args, "--lineage", lineage)
	}
	observation := r.runAt(cwd, args, false)
	if observation.ExitCode != 0 {
		return statusEnvelope{}, fmt.Errorf("atomic STATUS exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	var status statusEnvelope
	if err := json.Unmarshal([]byte(observation.Stdout), &status); err != nil {
		return statusEnvelope{}, fmt.Errorf("parse atomic STATUS: %w", err)
	}
	return status, nil
}

func decodeAtomicReviewStart(observation Observation, wantLineage string) (atomicReviewStartResult, error) {
	var started atomicReviewStartResult
	if err := json.Unmarshal([]byte(observation.Stdout), &started); err != nil {
		return started, fmt.Errorf("parse atomic START: %w", err)
	}
	if started.Action != "created" || started.LineageID != wantLineage || started.State != "reviewing" {
		return started, fmt.Errorf("atomic START = %+v, want created reviewing lineage %q", started, wantLineage)
	}
	return started, nil
}

// resolveAtomicStartConsentAt executes the granted invocation emitted by the
// product only after the exact rendered START has returned its consent envelope.
// The benchmark never substitutes --consent itself or hand-assembles a START.
func resolveAtomicStartConsentAt(r *journeyRun, cwd string, status statusEnvelope, relay Observation) (Observation, error) {
	var response struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal([]byte(relay.Stdout), &response); err != nil {
		return Observation{}, fmt.Errorf("parse printed START response: %w", err)
	}
	if response.Action == "created" {
		return relay, nil
	}
	if response.Action != "consent_required" {
		return Observation{}, fmt.Errorf("printed START action = %q, want created or consent_required", response.Action)
	}
	var consent waveConsentResult
	if err := json.Unmarshal([]byte(relay.Stdout), &consent); err != nil {
		return Observation{}, fmt.Errorf("parse START consent relay: %w", err)
	}
	grantInvocation := ""
	for _, choice := range consent.Choices {
		if choice.Answer == "granted" {
			grantInvocation = choice.Invocation
			break
		}
	}
	if consent.TargetIdentity != status.TargetIdentity || grantInvocation == "" {
		return Observation{}, fmt.Errorf("START consent did not bind the selectorless target: %+v", consent)
	}
	grantArgs, err := waveReviewInvocationArgs(grantInvocation)
	if err != nil {
		return Observation{}, err
	}
	granted := r.runAt(cwd, grantArgs, false)
	if granted.ExitCode != 0 {
		return Observation{}, fmt.Errorf("emitted START consent grant exited %d: %s", granted.ExitCode, firstLine(granted.Stderr))
	}
	return granted, nil
}

// startAtomicTransactionFromSelectorlessStatusAt executes only the START command
// rendered by selectorless STATUS for cwd. It never reconstructs the legacy START
// flags, so a changed negotiated binding cannot be hidden by bench-owned arguments.
func startAtomicTransactionFromSelectorlessStatusAt(r *journeyRun, cwd string, forbiddenLineages ...string) (string, error) {
	status, err := readAtomicReviewStatusAt(r, cwd, "")
	if err != nil {
		return "", err
	}
	lineage := status.executeArgument("lineage")
	if status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.start" || lineage == "" {
		return "", fmt.Errorf("selectorless STATUS = %+v, want a runnable START with a derived lineage", status.NextTransition)
	}
	if status.Authority.LineageID != "" || status.Authority.State != "" {
		return "", fmt.Errorf("selectorless STATUS reused active authority instead of deriving a fresh compact transaction: %+v", status.Authority)
	}
	for _, forbidden := range forbiddenLineages {
		if forbidden != "" && (status.Authority.LineageID == forbidden || lineage == forbidden) {
			return "", fmt.Errorf("selectorless STATUS reused forbidden lineage %q: authority=%+v transition=%+v", forbidden, status.Authority, status.NextTransition)
		}
	}
	if status.executeArgument("expected-untracked-inventory") != "" || status.executeArgument("intended-untracked") != "" {
		return "", fmt.Errorf("selectorless STATUS unexpectedly reused untracked selection inventory: %+v", status.NextTransition.Execute.Arguments)
	}
	started, err := runPrintedTransitionAt(r, cwd, status)
	if err != nil {
		return "", err
	}
	if started.ExitCode != 0 {
		return "", fmt.Errorf("printed selectorless START exited %d: %s", started.ExitCode, firstLine(started.Stderr))
	}
	started, err = resolveAtomicStartConsentAt(r, cwd, status, started)
	if err != nil {
		return "", err
	}
	if _, err := decodeAtomicReviewStart(started, lineage); err != nil {
		return "", err
	}
	return lineage, nil
}

func startAtomicTransactionFromSelectorlessStatus(r *journeyRun, forbiddenLineages ...string) (string, error) {
	return startAtomicTransactionFromSelectorlessStatusAt(r, r.sandbox.Repo, forbiddenLineages...)
}

func proveCurrentStatusAndStartIgnoreSiblingWorktree(r *journeyRun) error {
	linked := r.sandbox.Scratch["atomic-sibling-worktree"]
	if linked == "" {
		return fmt.Errorf("atomic sibling worktree fixture did not record its path")
	}
	siblingLineage, err := startAtomicTransactionFromSelectorlessStatusAt(r, linked)
	if err != nil {
		return fmt.Errorf("start sibling transaction from its selectorless STATUS: %w", err)
	}
	r.sandbox.Scratch[atomicSiblingBindingKey] = siblingLineage

	currentLineage, err := startAtomicTransactionFromSelectorlessStatus(r, siblingLineage)
	if err != nil {
		return fmt.Errorf("start current worktree from its selectorless STATUS: %w", err)
	}
	selected, err := readAtomicReviewStatus(r, currentLineage)
	if err != nil {
		return err
	}
	if selected.Authority.LineageID != currentLineage || selected.Authority.State != "reviewing" {
		return fmt.Errorf("explicit current-worktree STATUS = authority=%+v, want independent current reviewing transaction", selected.Authority)
	}
	return nil
}

func requireExplicitAtomicFourLensStatus(r *journeyRun) error {
	status, err := readAtomicReviewStatus(r, atomicCorrectionLineage)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != atomicCorrectionLineage || status.Authority.State != "reviewing" ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(status.NextTransition.Collect.Inputs) != 4 {
		return fmt.Errorf("explicit active atomic STATUS = authority=%+v transition=%+v, want four compact lens slots", status.Authority, status.NextTransition)
	}
	for index, input := range status.NextTransition.Collect.Inputs {
		lineage, order := "", ""
		for _, argument := range input.Arguments {
			switch argument.Name {
			case "lineage":
				lineage = argument.Value
			case "order":
				order = argument.Value
			}
		}
		if input.Name != "reviewer_result" || input.ArtifactSubject.SubjectHash == "" || input.CaptureOperation != "review.capture-result" ||
			lineage != atomicCorrectionLineage || order != fmt.Sprintf("%d", index) {
			return fmt.Errorf("atomic lens slot %d = %+v, want a bound reviewer_result", index, input)
		}
	}
	return nil
}

func captureAtomicReviewerSlots(r *journeyRun, lineageID string, includeCorrectableFinding bool) error {
	for capture := 0; capture < 4; capture++ {
		status, err := readAtomicReviewStatus(r, lineageID)
		if err != nil {
			return err
		}
		if status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" ||
			len(status.NextTransition.Collect.Inputs) == 0 {
			return fmt.Errorf("atomic capture %d STATUS = %+v, want a reviewer-result slot", capture, status.NextTransition)
		}
		input := status.NextTransition.Collect.Inputs[0]
		lineage, target, revision, lens, order := "", "", "", "", ""
		for _, argument := range input.Arguments {
			switch argument.Name {
			case "lineage":
				lineage = argument.Value
			case "target":
				target = argument.Value
			case "expected-revision":
				revision = argument.Value
			case "lens":
				lens = argument.Value
			case "order":
				order = argument.Value
			}
		}
		if lineage != lineageID || target == "" || revision == "" || lens == "" || order == "" {
			return fmt.Errorf("atomic capture %d binding = %+v", capture, input)
		}

		paths := make([]string, 0, len(input.ChangedPathManifest))
		for _, entry := range input.ChangedPathManifest {
			paths = append(paths, entry.Path)
		}
		payload, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, paths)
		if err != nil {
			return err
		}
		if capture == 0 && includeCorrectableFinding {
			payload, err = json.Marshal(map[string]any{
				"subject_hash": input.ArtifactSubject.SubjectHash,
				"inspection":   map[string]any{"status": "completed", "paths": paths},
				"findings": []any{map[string]any{
					"location": "candidate.go:3", "severity": "CRITICAL", "claim": "candidate returns the wrong value",
					"proof_refs":     []string{"candidate.go:3 changed hunk fails the focused check"},
					"evidence_class": "deterministic", "causal_disposition": "introduced",
				}},
				"evidence": []string{"focused differential check failed on the frozen candidate"},
			})
			if err != nil {
				return err
			}
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("atomic-reviewer-%d.json", capture), payload)
		if err != nil {
			return err
		}
		observation := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", lineage, "--target", target, "--expected-revision", revision,
			"--lens", lens, "--order", order, "--input", path,
		}, true)
		if observation.ExitCode != 0 {
			return fmt.Errorf("capture atomic reviewer slot %d: %s", capture, firstLine(observation.Stderr))
		}
	}
	return nil
}

// captureExactSelectedReviewerSlots follows one explicit active lineage through
// every lens STATUS selected, in its published order. Unlike the fixed 4R helper
// above, this also covers standard-risk one-lens reviews without treating their
// single slot as an incomplete high-risk set.
func captureExactSelectedReviewerSlots(r *journeyRun, lineageID string, includeCorrectableFinding bool) error {
	initial, err := readAtomicReviewStatus(r, lineageID)
	if err != nil {
		return err
	}
	if initial.Authority.LineageID != lineageID || initial.Authority.State != "reviewing" ||
		initial.NextTransition.Kind != "collect" || initial.NextTransition.ReasonCode != "reviewer_results_required" ||
		len(initial.NextTransition.Collect.Inputs) == 0 {
		return fmt.Errorf("exact active STATUS = authority=%+v transition=%+v, want selected reviewer slots", initial.Authority, initial.NextTransition)
	}

	type selectedSlot struct {
		lineage, target, revision, lens, order string
	}
	bindings := func(status statusEnvelope) ([]selectedSlot, error) {
		slots := make([]selectedSlot, 0, len(status.NextTransition.Collect.Inputs))
		for index, input := range status.NextTransition.Collect.Inputs {
			slot := selectedSlot{}
			for _, argument := range input.Arguments {
				switch argument.Name {
				case "lineage":
					slot.lineage = argument.Value
				case "target":
					slot.target = argument.Value
				case "expected-revision":
					slot.revision = argument.Value
				case "lens":
					slot.lens = argument.Value
				case "order":
					slot.order = argument.Value
				}
			}
			if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" ||
				input.ArtifactSubject.SubjectHash == "" || slot.lineage != lineageID || slot.target == "" ||
				slot.revision == "" || slot.lens == "" || slot.order != fmt.Sprintf("%d", index) {
				return nil, fmt.Errorf("selected reviewer slot %d = %+v", index, input)
			}
			slots = append(slots, slot)
		}
		return slots, nil
	}

	expected, err := bindings(initial)
	if err != nil {
		return err
	}
	for capture := range expected {
		status, err := readAtomicReviewStatus(r, lineageID)
		if err != nil {
			return err
		}
		got, err := bindings(status)
		if err != nil || len(got) != len(expected)-capture {
			return fmt.Errorf("selected reviewer slots after %d captures = %+v, %v", capture, status.NextTransition, err)
		}
		for index := range got {
			if got[index] != expected[capture+index] {
				return fmt.Errorf("selected reviewer slot order changed after %d captures: got=%+v want=%+v", capture, got, expected[capture:])
			}
		}
		input := status.NextTransition.Collect.Inputs[0]
		paths := make([]string, 0, len(input.ChangedPathManifest))
		for _, entry := range input.ChangedPathManifest {
			paths = append(paths, entry.Path)
		}
		payload, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, paths)
		if err != nil {
			return err
		}
		if capture == 0 && includeCorrectableFinding {
			payload, err = json.Marshal(map[string]any{
				"subject_hash": input.ArtifactSubject.SubjectHash,
				"inspection":   map[string]any{"status": "completed", "paths": paths},
				"findings": []any{map[string]any{
					"location": "candidate.go:3", "severity": "CRITICAL", "claim": "candidate returns the wrong value",
					"proof_refs":     []string{"candidate.go:3 changed hunk fails the focused check"},
					"evidence_class": "deterministic", "causal_disposition": "introduced",
				}},
				"evidence": []string{"focused differential check failed on the frozen candidate"},
			})
			if err != nil {
				return err
			}
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("exact-selected-reviewer-%d.json", capture), payload)
		if err != nil {
			return err
		}
		observation := r.run([]string{
			"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", got[0].lineage, "--target", got[0].target, "--expected-revision", got[0].revision,
			"--lens", got[0].lens, "--order", got[0].order, "--input", path,
		}, true)
		if observation.ExitCode != 0 {
			return fmt.Errorf("capture selected reviewer slot %d: %s", capture, firstLine(observation.Stderr))
		}
	}

	after, err := readAtomicReviewStatus(r, lineageID)
	if err != nil {
		return err
	}
	if includeCorrectableFinding {
		if after.Authority.LineageID != lineageID || after.Authority.State != "correction_required" ||
			after.NextTransition.Kind != "collect" || after.NextTransition.ReasonCode != "correction_plan_required" ||
			len(after.NextTransition.Collect.Inputs) != 1 || after.NextTransition.Collect.Inputs[0].Name != "correction_lines" ||
			after.NextTransition.Collect.Inputs[0].CaptureOperation != "review.capture-correction-plan" {
			return fmt.Errorf("full selected lens set did not advance to its correction-plan capture: authority=%+v transition=%+v", after.Authority, after.NextTransition)
		}
		return nil
	}
	if after.Authority.LineageID != "" || after.Authority.State != "" || after.NextTransition.Kind != "execute" ||
		after.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("clean full selected lens set did not close and burn: authority=%+v transition=%+v", after.Authority, after.NextTransition)
	}
	return nil
}

// captureCorrectionPlanFor follows the correction-plan input STATUS published
// after the last severe reviewer capture. The plan is the one public pre-edit
// event; FINALIZE never participates in a last-event-closure correction.
func captureCorrectionPlanFor(r *journeyRun, lineageID string, correctionLines int) error {
	if correctionLines <= 0 {
		return fmt.Errorf("correction plan needs a positive line forecast")
	}
	status, err := readAtomicReviewStatus(r, lineageID)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != lineageID || status.Authority.State != "correction_required" ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "correction_plan_required" ||
		len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("correction plan STATUS = authority=%+v transition=%+v", status.Authority, status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "correction_lines" || input.CaptureOperation != "review.capture-correction-plan" ||
		status.argument("lineage") != lineageID || status.argument("target") == "" ||
		status.argument("expected-revision") == "" || status.argument("request-hash") == "" ||
		status.argument("repository-context") == "" {
		return fmt.Errorf("correction-plan binding = %+v", input)
	}
	arguments := []string{"review", "capture-correction-plan"}
	for _, argument := range input.Arguments {
		arguments = append(arguments, "--"+argument.Name+"="+argument.Value)
	}
	arguments = append(arguments, fmt.Sprintf("--correction-lines=%d", correctionLines))
	observation := r.runAt(r.sandbox.Root, arguments, false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture correction plan: %s", firstLine(observation.Stderr))
	}
	var captured struct {
		Schema    string `json:"schema"`
		Operation string `json:"operation"`
		LineageID string `json:"lineage_id"`
		State     string `json:"state"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &captured); err != nil {
		return fmt.Errorf("decode correction-plan capture: %w", err)
	}
	if captured.Schema != "gentle-ai.review-last-event-closure/v1" || captured.Operation != "review.capture-correction-plan" ||
		captured.LineageID != lineageID || captured.State != "correction_required" {
		return fmt.Errorf("correction-plan capture = %+v", captured)
	}
	return nil
}

func requireAtomicLineageBurned(r *journeyRun, lineageID string) error {
	observation := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	var head authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &head); err != nil {
		return fmt.Errorf("parse burned lineage inventory: %w", err)
	}
	for _, entry := range head.Entries {
		if entry.LineageID == lineageID {
			return fmt.Errorf("terminal #3417 lineage %q remained durable: %+v", lineageID, entry)
		}
	}
	return nil
}

func captureAtomicCorrectableFinding(r *journeyRun) error {
	return captureAtomicReviewerSlots(r, atomicCorrectionLineage, true)
}

func waveThreeJourneys() []Journey {
	return []Journey{
		{
			ID:     "j59-current-status-and-start-ignore-sibling-worktree-transaction",
			Review: reviewOptedIn,
			Title:  "#3587: selectorless current STATUS and START ignore an unrelated sibling worktree transaction",
			Source: "#3587: compact atomic review is bound to the selected worktree and candidate, never ambient sibling authority",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: sibling worktree stages the same candidate independently", Fixture: stageAtomicSiblingWorktrees},
				{Name: "selectorless STATUS and current-worktree START ignore the sibling worktree transaction", Requires: atomicReviewStatusCapability, Composite: proveCurrentStatusAndStartIgnoreSiblingWorktree},
			},
		},
		{
			ID:     "j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow",
			Review: reviewOptedIn,
			Title:  "#3587: explicit active lineage keeps its exact four lenses through correction and validator flow",
			Source: "#3587: active compact authority continues only through its bound four-lens correction plan and terminal validator capture",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: high-risk correction candidate", Fixture: stageAtomicHighRiskCorrectionCandidate},
				{Name: "START the compact high-risk transaction", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", atomicCorrectionLineage)},
				{Name: "explicit active STATUS exposes exactly four compact lenses", Requires: atomicReviewStatusCapability, Composite: requireExplicitAtomicFourLensStatus},
				{Name: "capture a correction finding and every remaining compact lens", Requires: captureResultCapability, Composite: captureAtomicCorrectableFinding},
				{Name: "capture the status-bound bounded correction plan", Requires: captureCorrectionPlanCapability, Composite: func(r *journeyRun) error {
					return captureCorrectionPlanFor(r, atomicCorrectionLineage, 2)
				}},
				{Name: "fixture: correct only the reviewed candidate", Fixture: writeCorrectedCandidate},
				{Name: "capture the Go-issued targeted validator that closes and burns", Requires: capturedProviderValidatorStatusCapability, Composite: func(r *journeyRun) error {
					return captureProviderValidatorSlotFor(r, atomicCorrectionLineage)
				}},
				{Name: "no correction authority survives the terminal validator capture", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageBurned(r, atomicCorrectionLineage)
				}},
			},
		},
	}
}
