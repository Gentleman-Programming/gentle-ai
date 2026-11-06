package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// #4772: a committed-only correction must keep HEAD semantics even when a
// tracked path outside the frozen review scope is dirty. Neither an unchanged
// HEAD nor an uncommitted edit is a corrected candidate or a new plan request.
func issue4772Journeys() []Journey {
	var journeys []Journey
	for index, mode := range []string{"commit", "amend"} {
		mode := mode
		journeys = append(journeys, Journey{
			ID:     fmt.Sprintf("j%d-committed-correction-dirty-%s", 4772+index, mode),
			Review: reviewOptedIn,
			Title:  "Committed correction with unrelated tracked dirt: " + mode,
			Source: "#4772 frozen committed selector semantics through validator and acknowledgement",
			Steps: []Step{
				{Name: "fixture: public Codex committed candidate and tracked unrelated note", Fixture: issue4772Fixture},
				{Name: "execute exact negotiated committed START", Requires: statusCapability, Composite: issue3748StartCodexReview},
				{Name: "capture public provider blocker", Requires: issue3748CodexCaptureCapability, Composite: issue3748CaptureCodexBlocker},
				{Name: "execute returned correction STATUS and capture the plan once", Requires: captureCorrectionPlanCapability, Composite: issue4772Plan},
				{Name: "unchanged and uncommitted candidates stop without another plan", Requires: statusCapability, Composite: issue4772Controls},
				{Name: "fixture: scoped correction " + mode + " leaves tracked note dirty", Fixture: func(s *Sandbox) error { return issue4772Commit(s, mode) }},
				{Name: "exact offered validator completes and acknowledgement burns; note untouched", Requires: statusCapability, Composite: issue4772Validate},
			},
		})
	}
	return journeys
}

func issue4772Fixture(s *Sandbox) error {
	if err := issue3748CodexFixture(s); err != nil {
		return err
	}
	// Put the note in the base, not the reviewed candidate, without rewriting the
	// candidate: a second base commit followed by the original candidate tree.
	if err := s.write(filepath.Join(s.Repo, "unrelated.md"), "base note\n"); err != nil {
		return err
	}
	if err := s.git(s.Repo, "add", "unrelated.md"); err != nil {
		return err
	}
	if err := s.git(s.Repo, "commit", "-qm", "fixture note"); err != nil {
		return err
	}
	// The pinned base tree contains the same candidate path bytes as the original
	// base, plus the note. Git's plumbing writes fixture objects only.
	base := s.Scratch["issue-3748-base-tree"]
	if err := s.git(s.Repo, "read-tree", base); err != nil {
		return err
	}
	if err := s.git(s.Repo, "add", "unrelated.md"); err != nil {
		return err
	}
	tree, err := gitOut(s, s.Repo, "write-tree")
	if err != nil {
		return err
	}
	s.Scratch["issue-3748-base-tree"] = tree
	if err := s.git(s.Repo, "read-tree", "HEAD"); err != nil {
		return err
	}
	scriptPath := filepath.Join(s.PathOverride, "codex")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	// The fixture provider accepts only the exact validation hash negotiated by
	// STATUS; the product still compiles the prompt and admits the completion.
	branch := `if test -f "$0.validation"; then
  hash=$(grep -o '"targeted_validation_request_hash":"[^"]*"' "$0.validation" | cut -d '"' -f 4)
  grep -q "$hash" "$prompt"
  cp "$0.validation" "$output"
  exit 0
fi
`
	return os.WriteFile(scriptPath, []byte(strings.Replace(string(script), "if grep -q 'gentle-ai.review-provider-refuter-request/v1'", branch+"if grep -q 'gentle-ai.review-provider-refuter-request/v1'", 1)), 0o755)
}

func issue4772Plan(r *journeyRun) error {
	if err := issue3748ExecuteCodexContinuation(r); err != nil {
		return err
	}
	return captureCorrectionPlanFor(r, issue3748CodexLineage, 2,
		"--base-ref", r.sandbox.Scratch["issue-3748-base-tree"], "--committed-only", "--agent", "codex")
}

func issue4772Stop(r *journeyRun) error {
	status, err := issue3748Status(r)
	if err != nil {
		return err
	}
	if status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "corrected_candidate_unavailable" {
		return fmt.Errorf("accepted plan must stop, not recollect: %+v", status.NextTransition)
	}
	return nil
}

func issue4772Controls(r *journeyRun) error {
	if err := issue4772Stop(r); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int {\n\treturn 2\n}\n"); err != nil {
		return err
	}
	return issue4772Stop(r)
}

const issue4772DirtyNote = "dirty note outside review scope\n"

func issue4772Commit(s *Sandbox, mode string) error {
	if err := s.write(filepath.Join(s.Repo, "unrelated.md"), issue4772DirtyNote); err != nil {
		return err
	}
	if err := s.git(s.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	if mode == "amend" {
		return s.git(s.Repo, "commit", "--amend", "--no-edit", "-q")
	}
	return s.git(s.Repo, "commit", "-qm", "correct candidate only")
}

func issue4772Validate(r *journeyRun) error {
	status, err := readProviderValidatorStatus(r, issue3748CodexLineage, false,
		"--base-ref", r.sandbox.Scratch["issue-3748-base-tree"], "--committed-only", "--agent", "codex")
	if err != nil {
		return err
	}
	if status.ValidationRequest == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != "targeted_validation_required" || len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("committed correction validator unavailable: %+v", status.NextTransition)
	}
	payload, err := json.Marshal(map[string]any{
		"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity":       status.ValidationRequest.CorrectionTargetIdentity,
		"original_criteria":                map[string]any{"passed": true, "evidence": []string{"candidate.go now returns 2"}},
		"correction_regression":            map[string]any{"passed": true, "evidence": []string{"only candidate.go changed"}},
		"follow_ups":                       []any{},
	})
	if err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.PathOverride, "codex.validation"), string(payload)); err != nil {
		return err
	}
	exact, err := issue3748Status(r)
	if err != nil {
		return err
	}
	input := exact.NextTransition.Collect.Inputs[0]
	if input.CaptureOperation != "review.capture-validation" {
		return fmt.Errorf("wrong validator operation: %s", input.CaptureOperation)
	}
	args := []string{"review", "capture-validation", "--cwd=" + r.sandbox.Repo}
	for _, argument := range input.Arguments {
		args = append(args, argument.Token)
	}
	result := r.runAt(r.sandbox.Root, args, false)
	if result.ExitCode != 0 {
		return fmt.Errorf("exact offered validator exit %d: %s", result.ExitCode, result.Stderr)
	}
	if err := requireAtomicLineageAcknowledged(r, issue3748CodexLineage,
		"--base-ref", r.sandbox.Scratch["issue-3748-base-tree"], "--committed-only"); err != nil {
		return err
	}
	note, err := os.ReadFile(filepath.Join(r.sandbox.Repo, "unrelated.md"))
	if err != nil || string(note) != issue4772DirtyNote {
		return fmt.Errorf("unrelated note changed: %q, %v", note, err)
	}
	dirty, err := gitOut(r.sandbox, r.sandbox.Repo, "diff", "--name-only")
	if err != nil || dirty != "unrelated.md" {
		return fmt.Errorf("remaining dirty paths = %q, %v", dirty, err)
	}
	return nil
}
