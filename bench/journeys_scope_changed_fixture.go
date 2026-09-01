package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	scopeChangedFixturePredecessor = "issue-2618-high-risk-predecessor"
	scopeChangedFixtureSuccessor   = "issue-2618-scope-changed-successor"
)

var scopeChangedFixtureLenses = []string{
	"review-risk",
	"review-resilience",
	"review-readability",
	"review-reliability",
}

var scopeChangedFixtureCaptureCapability = &Capability{Verb: []string{"review", "capture-result"},
	Flags: []string{"--repository-context", "--lineage", "--target", "--expected-revision", "--lens", "--order", "--input"}}

// scopeChangedFixtureJourneys closes #2618's coverage gap at the product
// boundary. The two short lens forms recreate the affected slot shape; the
// negative controls prove malformed and unbound output fail differently.
func scopeChangedFixtureJourneys() []Journey {
	return []Journey{
		{
			ID:     "j76-scope-changed-four-lens-successor",
			Review: reviewOptedIn,
			Title:  "Scope-changed high-risk successor: all four exact lens slots admit and reach approval",
			Source: "issue #2618: a bounded correction recovery must not strand resilience or readability capture slots",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: stage high-risk auth candidate", Fixture: stageAuthCode},
				{Name: "start original high-risk four-lens review", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", scopeChangedFixturePredecessor)},
				{Name: "capture three critical findings and the fourth original lens", Requires: scopeChangedFixtureCaptureCapability,
					Composite: captureScopeChangedFixturePredecessor},
				{Name: "finalize original review into correction-required", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", scopeChangedFixturePredecessor, "--captured-results=true"),
					After: requireReviewState("correction_required", scopeChangedFixturePredecessor)},
				{Name: "forecast one bounded correction", Requires: finalizeCorrectionCapability,
					Args:  productArgs("review", "finalize", "--lineage", scopeChangedFixturePredecessor, "--correction-lines", "2"),
					After: requireReviewState("correction_required", scopeChangedFixturePredecessor)},
				{Name: "fixture: apply bounded correction and expand its scope", Fixture: applyScopeChangedFixtureCorrection},
				{Name: "recover scope-changed successor", Requires: recoverCapability, Composite: recoverScopeChangedFixtureSuccessor},
				{Name: "reject malformed and unbound successor output without consuming its slot", Requires: scopeChangedFixtureCaptureCapability,
					Composite: rejectScopeChangedFixtureNegativeControls},
				{Name: "capture all four exact successor slots", Requires: scopeChangedFixtureCaptureCapability,
					Composite: captureScopeChangedFixtureSuccessor},
				{Name: "finalize successor reviewer results", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", scopeChangedFixtureSuccessor, "--captured-results=true"),
					After: requireReviewState("validating", scopeChangedFixtureSuccessor)},
				{Name: "capture successor verification evidence", Requires: captureEvidenceCapability,
					Composite: func(r *journeyRun) error {
						return captureFinalEvidenceFor(r, "--lineage", scopeChangedFixtureSuccessor)
					}},
				{Name: "approve the recovered successor", Requires: finalizeEvidenceCapability,
					Args:  productArgs("review", "finalize", "--lineage", scopeChangedFixtureSuccessor, "--captured-evidence=true"),
					After: requireReviewState("approved", scopeChangedFixtureSuccessor)},
				{Name: "terminal post-apply gate allows successor", Requires: validateCapability,
					Args: productArgs("review", "validate", "--lineage", scopeChangedFixtureSuccessor, "--gate", "post-apply")},
			},
		},
	}
}

type scopeChangedFixtureBinding struct {
	lens     string
	subject  string
	context  string
	lineage  string
	target   string
	revision string
	order    string
	paths    []string
}

func scopeChangedFixtureStatus(r *journeyRun, lineage string) (statusEnvelope, error) {
	return readStatusForContract(r, reviewContractV2, "--lineage", lineage)
}

func scopeChangedFixtureNextBinding(r *journeyRun, lineage, wantLens string) (scopeChangedFixtureBinding, error) {
	envelope, err := scopeChangedFixtureStatus(r, lineage)
	if err != nil {
		return scopeChangedFixtureBinding{}, err
	}
	if envelope.Authority.LineageID != lineage || envelope.Authority.State != "reviewing" ||
		envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 {
		return scopeChangedFixtureBinding{}, fmt.Errorf("%s does not offer reviewing capture slots: %+v", lineage, envelope)
	}
	for _, input := range envelope.NextTransition.Collect.Inputs {
		if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" {
			continue
		}
		binding := scopeChangedFixtureBinding{subject: input.ArtifactSubject.SubjectHash}
		for _, entry := range input.ChangedPathManifest {
			binding.paths = append(binding.paths, entry.Path)
		}
		for _, argument := range input.Arguments {
			switch argument.Name {
			case "lens":
				binding.lens = argument.Value
			case "repository-context":
				binding.context = argument.Value
			case "lineage":
				binding.lineage = argument.Value
			case "target":
				binding.target = argument.Value
			case "expected-revision":
				binding.revision = argument.Value
			case "order":
				binding.order = argument.Value
			}
		}
		if binding.lens != wantLens {
			continue
		}
		if binding.subject == "" || binding.context == "" || binding.lineage != lineage || binding.target == "" ||
			binding.revision == "" || binding.order == "" || len(binding.paths) == 0 {
			return scopeChangedFixtureBinding{}, fmt.Errorf("%s capture binding = %+v, want complete exact %q slot", lineage, input, wantLens)
		}
		return binding, nil
	}
	return scopeChangedFixtureBinding{}, fmt.Errorf("%s does not offer exact %q capture slot", lineage, wantLens)
}

func scopeChangedFixtureBindingKey(kind, lens string) string {
	return "scope-changed-fixture-" + kind + "-" + lens
}

func requireScopeChangedFixtureFourLensSlots(envelope statusEnvelope) error {
	inputs := envelope.NextTransition.Collect.Inputs
	if len(inputs) != len(scopeChangedFixtureLenses) {
		return fmt.Errorf("high-risk status publishes %d reviewer slots, want %d", len(inputs), len(scopeChangedFixtureLenses))
	}
	want := make(map[string]bool, len(scopeChangedFixtureLenses))
	for _, lens := range scopeChangedFixtureLenses {
		want[lens] = true
	}
	seen := make(map[string]bool, len(scopeChangedFixtureLenses))
	for _, input := range inputs {
		lens := ""
		for _, argument := range input.Arguments {
			if argument.Name == "lens" {
				lens = argument.Value
			}
		}
		if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" ||
			!want[lens] || seen[lens] {
			return fmt.Errorf("high-risk capture slot = %+v, want each canonical lens exactly once", input)
		}
		seen[lens] = true
	}
	return nil
}

func scopeChangedFixturePayload(binding scopeChangedFixtureBinding, lensForm string, paths []string, critical bool) ([]byte, error) {
	findings := []any{}
	if critical {
		findings = append(findings, map[string]any{
			"location": "internal/auth/session.go:5", "severity": "CRITICAL",
			"claim": "the reviewed authentication path accepts an unsafe token", "proof_refs": []string{"internal/auth/session.go:5"},
			"evidence_class": "deterministic", "causal_disposition": "introduced",
		})
	}
	return json.Marshal(map[string]any{
		"subject_hash": binding.subject,
		"inspection":   map[string]any{"status": "completed", "paths": paths},
		"lens":         lensForm,
		"findings":     findings,
		"evidence":     []string{"inspected the complete frozen candidate scope named by the capture binding"},
	})
}

func captureScopeChangedFixtureResult(r *journeyRun, binding scopeChangedFixtureBinding, payload []byte, name string) (Observation, error) {
	path, err := writeScratch(r.sandbox, name, payload)
	if err != nil {
		return Observation{}, err
	}
	return r.run([]string{
		"review", "capture-result", "--repository-context", binding.context,
		"--lineage", binding.lineage, "--target", binding.target,
		"--expected-revision", binding.revision, "--lens", binding.lens,
		"--order", binding.order, "--input", path,
	}, true), nil
}

func (binding scopeChangedFixtureBinding) equals(other scopeChangedFixtureBinding) bool {
	return binding.lens == other.lens && binding.subject == other.subject && binding.context == other.context &&
		binding.lineage == other.lineage && binding.target == other.target && binding.revision == other.revision &&
		binding.order == other.order && strings.Join(binding.paths, "\x00") == strings.Join(other.paths, "\x00")
}

func requireScopeChangedFixtureCapture(observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture successor slot: %s", firstLine(observation.Stderr))
	}
	var result struct {
		AdmissionDecision string `json:"admission_decision"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil || result.AdmissionDecision != "completed" {
		return fmt.Errorf("capture successor slot result = %q, %v; want completed", observation.Stdout, err)
	}
	return nil
}

func captureScopeChangedFixturePredecessor(r *journeyRun) error {
	for index, lens := range scopeChangedFixtureLenses {
		binding, err := scopeChangedFixtureNextBinding(r, scopeChangedFixturePredecessor, lens)
		if err != nil {
			return err
		}
		if index == 0 {
			status, err := scopeChangedFixtureStatus(r, scopeChangedFixturePredecessor)
			if err != nil {
				return err
			}
			if err := requireScopeChangedFixtureFourLensSlots(status); err != nil {
				return err
			}
			r.sandbox.Scratch["scope-changed-fixture-predecessor-target"] = binding.target
		}
		r.sandbox.Scratch[scopeChangedFixtureBindingKey("predecessor-subject", lens)] = binding.subject
		r.sandbox.Scratch[scopeChangedFixtureBindingKey("predecessor-context", lens)] = binding.context
		payload, err := scopeChangedFixturePayload(binding, lens, binding.paths, index < 3)
		if err != nil {
			return err
		}
		observation, err := captureScopeChangedFixtureResult(r, binding, payload, fmt.Sprintf("predecessor-%d.json", index))
		if err != nil {
			return err
		}
		if err := requireScopeChangedFixtureCapture(observation); err != nil {
			return err
		}
	}
	return nil
}

func applyScopeChangedFixtureCorrection(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "internal", "auth", "session.go"), "package auth\n\n// CheckToken reports whether a session token is long enough.\nfunc CheckToken(token string) bool {\n\treturn len(token) >= 16\n}\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "internal", "auth", "policy.go"), "package auth\n\nconst minimumTokenLength = 16\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "internal/auth/session.go", "internal/auth/policy.go"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != "internal/auth/policy.go\ninternal/auth/session.go" {
		return fmt.Errorf("scope-changed correction paths = %q, %v", paths, err)
	}
	return nil
}

func recoverScopeChangedFixtureSuccessor(r *journeyRun) error {
	status, err := scopeChangedFixtureStatus(r, scopeChangedFixturePredecessor)
	if err != nil {
		return err
	}
	if status.Authority.LineageID != scopeChangedFixturePredecessor || status.Authority.State != "correction_required" {
		return fmt.Errorf("scope-changed recovery predecessor = %+v", status.Authority)
	}
	recovered, err := decodeWaveOperation(r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", scopeChangedFixturePredecessor, "--expected-predecessor-revision", status.Authority.Revision,
		"--successor-lineage", scopeChangedFixtureSuccessor, "--disposition", "scope_changed"), false), "scope-changed fixture recovery")
	if err != nil || recovered.LineageID != scopeChangedFixtureSuccessor || recovered.State != "reviewing" ||
		recovered.TargetIdentity == r.sandbox.Scratch["scope-changed-fixture-predecessor-target"] {
		return fmt.Errorf("scope-changed fixture successor = %+v, %v", recovered, err)
	}
	return nil
}

func assertScopeChangedFixtureSlotUnconsumed(r *journeyRun, lineage string, want scopeChangedFixtureBinding) error {
	got, err := scopeChangedFixtureNextBinding(r, lineage, want.lens)
	if err != nil {
		return err
	}
	if !got.equals(want) {
		return fmt.Errorf("rejected %s capture consumed or changed its slot: got %+v, want %+v", lineage, got, want)
	}
	return nil
}

func rejectScopeChangedFixtureNegativeControls(r *journeyRun) error {
	binding, err := scopeChangedFixtureNextBinding(r, scopeChangedFixtureSuccessor, scopeChangedFixtureLenses[0])
	if err != nil {
		return err
	}
	malformed, err := captureScopeChangedFixtureResult(r, binding, []byte("{"), "malformed-successor.json")
	if err != nil {
		return err
	}
	if malformed.ExitCode == 0 || !strings.Contains(malformed.Stderr, "incomplete") || strings.Contains(malformed.Stderr, "out_of_scope") {
		return fmt.Errorf("malformed successor output = exit %d stderr %q; want incomplete, not out_of_scope", malformed.ExitCode, malformed.Stderr)
	}
	if err := assertScopeChangedFixtureSlotUnconsumed(r, scopeChangedFixtureSuccessor, binding); err != nil {
		return err
	}
	wrongSubject := "sha256:" + strings.Repeat("0", 64)
	unboundBinding := binding
	unboundBinding.subject = wrongSubject
	payload, err := scopeChangedFixturePayload(unboundBinding, binding.lens, binding.paths, false)
	if err != nil {
		return err
	}
	unbound, err := captureScopeChangedFixtureResult(r, binding, payload, "unbound-successor.json")
	if err != nil {
		return err
	}
	if unbound.ExitCode == 0 || !strings.Contains(unbound.Stderr, "binding_mismatch") || strings.Contains(unbound.Stderr, "out_of_scope") {
		return fmt.Errorf("unbound successor output = exit %d stderr %q; want binding_mismatch, not out_of_scope", unbound.ExitCode, unbound.Stderr)
	}
	return assertScopeChangedFixtureSlotUnconsumed(r, scopeChangedFixtureSuccessor, binding)
}

func captureScopeChangedFixtureSuccessor(r *journeyRun) error {
	for index, lens := range scopeChangedFixtureLenses {
		binding, err := scopeChangedFixtureNextBinding(r, scopeChangedFixtureSuccessor, lens)
		if err != nil {
			return err
		}
		if binding.subject == r.sandbox.Scratch[scopeChangedFixtureBindingKey("predecessor-subject", lens)] ||
			binding.context == r.sandbox.Scratch[scopeChangedFixtureBindingKey("predecessor-context", lens)] {
			return fmt.Errorf("successor %s binding did not receive new subject and repository context", lens)
		}
		if index == 0 {
			status, err := scopeChangedFixtureStatus(r, scopeChangedFixtureSuccessor)
			if err != nil {
				return err
			}
			if err := requireScopeChangedFixtureFourLensSlots(status); err != nil {
				return err
			}
		}
		lensForm := lens
		if lens == "review-resilience" || lens == "review-readability" {
			lensForm = strings.TrimPrefix(lens, "review-")
		}
		payload, err := scopeChangedFixturePayload(binding, lensForm, binding.paths, false)
		if err != nil {
			return err
		}
		observation, err := captureScopeChangedFixtureResult(r, binding, payload, fmt.Sprintf("successor-%d.json", index))
		if err != nil {
			return err
		}
		if err := requireScopeChangedFixtureCapture(observation); err != nil {
			return err
		}
	}
	return nil
}
