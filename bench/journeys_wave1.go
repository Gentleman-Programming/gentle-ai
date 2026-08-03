package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	correctedDeliveryLineage = "wave-one-corrected-delivery"
	retrySourceLineage       = "wave-one-final-verification-source"
	retrySuccessorLineage    = "wave-one-final-verification-successor"
	stagedRecoveryLineage    = "wave-one-staged-recovery-source"
	stagedSuccessorLineage   = "wave-one-staged-recovery-successor"
	fullScopeLineage         = "wave-one-full-scope-source"
	fullScopeSuccessor       = "wave-one-full-scope-successor"
	noOpFirstSegment         = "wave-one-noop-chain-first-segment"
	noOpSecondSegment        = "wave-one-noop-chain-second-segment"
	noOpSelfLoopLineage      = "wave-one-noop-self-loop"
	declineCandidateLineage  = "wave-one-candidate-decline"
	declineCandidatePath     = "scripts/deploy.sh"
	declineCandidateContents = "#!/bin/sh\necho deploy\n"
	reviewContractV2         = "gentle-ai.review-integration/v2"
)

var startNamedCapability = &Capability{Verb: []string{"review", "start"}, Flags: []string{"--cwd", "--lineage"}}
var captureOutcomeEvidenceCapability = &Capability{Verb: []string{"review", "capture-evidence"},
	Flags: []string{"--cwd", "--lineage", "--target", "--expected-revision", "--outcome", "--input"}}
var finalizeValidationCapability = &Capability{Verb: []string{"review", "finalize"},
	Flags: []string{"--cwd", "--lineage", "--validation", "--captured-evidence"}}
var finalizeProceduralFailureCapability = &Capability{Verb: []string{"review", "finalize"},
	Flags: []string{"--cwd", "--lineage", "--captured-evidence", "--failed"}}
var retryFinalVerificationCapability = &Capability{Verb: []string{"review", "retry-final-verification"},
	Flags: []string{"--cwd", "--contract", "--predecessor-lineage", "--expected-predecessor-revision", "--successor-lineage", "--incident", "--actor", "--reason", "--maintainer-authorization"}}

type waveOperationResult struct {
	Operation            string `json:"operation"`
	LineageID            string `json:"lineage_id"`
	State                string `json:"state"`
	StoreRevision        string `json:"store_revision"`
	PredecessorLineageID string `json:"predecessor_lineage_id"`
	TargetIdentity       string `json:"target_identity"`
}

type waveCorrectionStatus struct {
	TargetIdentity string `json:"target_identity"`
	Authority      *struct {
		LineageID string `json:"lineage_id"`
		State     string `json:"state"`
		Revision  string `json:"revision"`
	} `json:"authority"`
	Receipt struct {
		Status   string `json:"status"`
		Identity string `json:"identity"`
	} `json:"receipt"`
	Action            string `json:"action"`
	ActionDisposition string `json:"action_disposition"`
	Projection        struct {
		Kind                    string   `json:"kind"`
		InitialSnapshotIdentity string   `json:"initial_snapshot_identity"`
		CurrentSnapshotIdentity string   `json:"current_snapshot_identity"`
		CurrentCandidateTree    string   `json:"current_candidate_tree"`
		PathsDigest             string   `json:"paths_digest"`
		Paths                   []string `json:"paths"`
	} `json:"projection"`
	ValidationRequest *struct {
		RequestHash              string `json:"request_hash"`
		CorrectionTargetIdentity string `json:"correction_target_identity"`
		CorrectionPathsDigest    string `json:"correction_paths_digest"`
	} `json:"validation_request"`
	NextTransition *struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
		Collect    *struct {
			Inputs []struct {
				Name       string                    `json:"name"`
				Submission *waveSubmissionDescriptor `json:"submission"`
			} `json:"inputs"`
		} `json:"collect"`
	} `json:"next_transition"`
}

type waveSubmissionDescriptor struct {
	OperationToken string   `json:"operation_token"`
	ArgumentTokens []string `json:"argument_tokens"`
	Value          struct {
		Slot                 string `json:"slot"`
		SubstitutionLocation int    `json:"substitution_location"`
	} `json:"value"`
}

type waveRetryStatus struct {
	Authority *struct {
		LineageID string `json:"lineage_id"`
		State     string `json:"state"`
		Revision  string `json:"revision"`
	} `json:"authority"`
	Action                 string `json:"action"`
	ActionDisposition      string `json:"action_disposition"`
	FinalVerificationRetry *struct {
		IncidentSchema        string `json:"incident_schema"`
		IncidentClass         string `json:"incident_class"`
		ValidatingRevision    string `json:"validating_revision"`
		TargetIdentity        string `json:"target_identity"`
		FailedEvidenceHash    string `json:"failed_evidence_hash"`
		FinalizeRequestDigest string `json:"finalize_request_digest"`
	} `json:"final_verification_retry"`
	NextTransition *struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
		Collect    *struct {
			Inputs []struct {
				Name             string `json:"name"`
				CaptureOperation string `json:"capture_operation"`
			} `json:"inputs"`
		} `json:"collect"`
	} `json:"next_transition"`
}

type waveFinalVerificationIncident struct {
	Schema                string `json:"schema"`
	Class                 string `json:"class"`
	LineageID             string `json:"lineage_id"`
	TerminalRevision      string `json:"terminal_revision"`
	ValidatingRevision    string `json:"validating_revision"`
	TargetIdentity        string `json:"target_identity"`
	FailedEvidenceHash    string `json:"failed_evidence_hash"`
	FinalizeRequestDigest string `json:"finalize_request_digest"`
}

type waveGateResult struct {
	Result   string `json:"result"`
	Allowed  bool   `json:"allowed"`
	Action   string `json:"action"`
	Delivery string `json:"delivery"`
	Context  struct {
		LineageID             string `json:"lineage_id"`
		Generation            int    `json:"generation"`
		StoreRevision         string `json:"store_revision"`
		GenesisRevision       string `json:"genesis_revision"`
		ChainIdentity         string `json:"chain_identity"`
		BundleDigest          string `json:"bundle_digest"`
		BaseTree              string `json:"base_tree"`
		CandidateTree         string `json:"candidate_tree"`
		PathsDigest           string `json:"paths_digest"`
		FixDeltaHash          string `json:"fix_delta_hash"`
		PolicyHash            string `json:"policy_hash"`
		LedgerHash            string `json:"ledger_hash"`
		EvidenceHash          string `json:"evidence_hash"`
		ReceiptBaseTree       string `json:"receipt_base_tree"`
		BaseRelationshipValid bool   `json:"base_relationship_valid"`
		Denial                *struct {
			Stage string `json:"stage"`
			Code  string `json:"code"`
		} `json:"denial"`
	} `json:"context"`
}

type waveInventory struct {
	Complete      bool `json:"complete"`
	Authoritative bool `json:"authoritative"`
	Entries       []struct {
		LineageID string `json:"lineage_id"`
		Status    string `json:"status"`
		State     string `json:"state"`
	} `json:"entries"`
}

type waveConsentResult struct {
	Action         string `json:"action"`
	TargetIdentity string `json:"target_identity"`
	Choices        []struct {
		Answer     string `json:"answer"`
		Invocation string `json:"invocation"`
	} `json:"choices"`
}

type waveDeclinedStart struct {
	Action         string `json:"action"`
	Consent        string `json:"consent"`
	LineageID      string `json:"lineage_id"`
	TargetIdentity string `json:"target_identity"`
}

func decodeWaveObservation(observation Observation, target any, label string) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("%s exited %d: %s", label, observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), target); err != nil {
		return fmt.Errorf("parse %s: %w (stderr: %s)", label, err, firstLine(observation.Stderr))
	}
	return nil
}

func waveReviewInvocationArgs(invocation string) ([]string, error) {
	fields := strings.Fields(strings.TrimSpace(invocation))
	if len(fields) < 3 || fields[0] != "gentle-ai" || fields[1] != "review" {
		return nil, fmt.Errorf("invalid emitted review invocation %q", invocation)
	}
	return fields[1:], nil
}

func requireCandidateDeclinedGate(sandbox *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := decodeWaveObservation(observation, &gate, "candidate-declined gate"); err != nil {
		return err
	}
	context := gate.Context
	if gate.Result != "invalidated" || gate.Allowed || gate.Action != "repository-policy" || gate.Delivery != "candidate_declined/unmanaged" ||
		context.BaseTree == "" || context.BaseTree != sandbox.Scratch["decline-base"] ||
		context.CandidateTree == "" || context.CandidateTree != sandbox.Scratch["decline-tree"] ||
		context.PathsDigest == "" || context.PathsDigest != sandbox.Scratch["decline-paths"] ||
		context.Denial == nil || context.Denial.Stage != "candidate-decline" || context.Denial.Code != "exact_candidate" {
		return fmt.Errorf("candidate decline did not preserve exact unmanaged identity: %+v", gate)
	}
	if context.LineageID != "" || context.Generation != 0 || context.StoreRevision != "" || context.GenesisRevision != "" ||
		context.ChainIdentity != "" || context.BundleDigest != "" || context.FixDeltaHash != "" || context.PolicyHash != "" ||
		context.LedgerHash != "" || context.EvidenceHash != "" || context.ReceiptBaseTree != "" {
		return fmt.Errorf("candidate decline fabricated review authority: %+v", context)
	}
	return nil
}

func decodeWaveOperation(observation Observation, label string) (waveOperationResult, error) {
	if observation.ExitCode != 0 {
		return waveOperationResult{}, fmt.Errorf("%s exited %d: %s", label, observation.ExitCode, firstLine(observation.Stderr))
	}
	payload := json.RawMessage(strings.TrimSpace(observation.Stdout))
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return waveOperationResult{}, fmt.Errorf("parse %s: %w", label, err)
	}
	if len(envelope.Result) > 0 && envelope.Result[0] == '{' {
		payload = envelope.Result
	}
	var result waveOperationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, fmt.Errorf("parse %s result: %w", label, err)
	}
	return result, nil
}

func linkedWorktreeWithRemote(sandbox *Sandbox) error {
	if err := linkedWorktree(sandbox); err != nil {
		return err
	}
	if err := withRemote(sandbox); err != nil {
		return err
	}
	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	upstream, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "@{upstream}")
	if err != nil {
		return err
	}
	if head != upstream {
		return fmt.Errorf("linked-worktree fixture starts ahead of its local remote: HEAD %s, upstream %s", head, upstream)
	}
	sandbox.Scratch["delivery-base"] = upstream
	return nil
}

func stageWaveCandidate(sandbox *Sandbox) error {
	const candidate = "package candidate\n\nfunc value() int { return 1 }\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), candidate); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	staged, err := gitOut(sandbox, sandbox.Repo, "show", ":candidate.go")
	if err != nil {
		return err
	}
	if paths != "candidate.go" || staged+"\n" != candidate {
		return fmt.Errorf("fixture did not stage only the exact candidate: paths %q, content %q", paths, staged)
	}
	return nil
}

func stageFullScopeCandidate(sandbox *Sandbox) error {
	if err := stageWaveCandidate(sandbox); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "companion.txt"), "reviewed companion\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "companion.txt"); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != "candidate.go\ncompanion.txt" {
		return fmt.Errorf("full-scope fixture paths = %q, %v", paths, err)
	}
	return nil
}

func commitStagedRecoveryCandidate(sandbox *Sandbox) error {
	base, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["staged-recovery-base"] = base
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int {\n\treturn 1\n}\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "feat: add reviewed candidate")
}
func stagedPredecessorSelectors(sandbox *Sandbox) []string {
	return []string{"--lineage", stagedRecoveryLineage, "--base-ref", sandbox.Scratch["staged-recovery-base"]}
}
func stagedRecoverySelectors(sandbox *Sandbox) []string {
	return []string{"--lineage", stagedSuccessorLineage, "--base-ref", sandbox.Scratch["staged-recovery-base"], "--projection", "staged", "--workspace-overlay"}
}
func stageExpandedCorrection(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int {\n\treturn 2\n}\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "migration.sql"), "CREATE TABLE recovered (id INTEGER);\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go", "migration.sql"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# unstaged noise\n"); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "scratch.txt"), "untracked noise\n"); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	sandbox.Scratch["staged-recovery-tree"] = tree
	return err
}

func recoverStagedCorrection(r *journeyRun) error {
	selectors := stagedRecoverySelectors(r.sandbox)
	selectors[1] = stagedRecoveryLineage
	probeObservation := r.run(productArgsFor(r, append([]string{"review", "status", "--contract", reviewContract, "--next-transition", "--action-eligibility"}, selectors...)...), false)
	var probe waveCorrectionStatus
	if err := decodeWaveObservation(probeObservation, &probe, "staged recovery probe"); err != nil {
		return err
	}
	if probe.Authority == nil || probe.Action != "recover" || probe.ActionDisposition != "scope_changed" || probe.NextTransition == nil || probe.NextTransition.Kind != "collect" {
		return fmt.Errorf("staged recovery was not negotiated: %+v", probe)
	}
	const actor, reason = "bench-maintainer", "authorize staged correction scope expansion"
	authorization := "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + stagedRecoveryLineage +
		"\npredecessor_revision=" + probe.Authority.Revision + "\ntarget_identity=" + probe.TargetIdentity +
		"\nsuccessor_lineage=" + stagedSuccessorLineage + "\nactor=" + actor + "\nreason=" + reason
	authorized := append(selectors, "--recovery-successor-lineage", stagedSuccessorLineage, "--recovery-reason", reason,
		"--recovery-actor", actor, "--recovery-authorization", authorization)
	envelope, err := readStatusFor(r, authorized...)
	if err != nil || envelope.NextTransition.Kind != "execute" || envelope.NextTransition.Execute.Operation != "review.recover" {
		return fmt.Errorf("authorized staged recovery is not executable: %+v, %v", envelope.NextTransition, err)
	}
	args := []string{"review", "recover", "--cwd", r.sandbox.Repo}
	for _, argument := range envelope.NextTransition.Execute.Arguments {
		args = append(args, argument.Token)
	}
	result, err := decodeWaveOperation(r.run(args, false), "staged correction recovery")
	if err != nil || result.LineageID != stagedSuccessorLineage || result.State != "reviewing" {
		return fmt.Errorf("staged correction successor = %+v, %v", result, err)
	}
	fresh, err := readStatusFor(r, stagedRecoverySelectors(r.sandbox)...)
	if err != nil || fresh.Authority.LineageID != stagedSuccessorLineage || fresh.Authority.State != "reviewing" ||
		fresh.NextTransition.Kind != "collect" || strings.Join(fresh.paths(), "\x00") != "candidate.go\x00migration.sql" {
		return fmt.Errorf("successor did not start a fresh exact-overlay review: %+v, %v", fresh, err)
	}
	return nil
}

func commitRecoveredStagedOverlay(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "fix: deliver staged recovery"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "restore", "README.md"); err != nil {
		return err
	}
	return os.Remove(filepath.Join(sandbox.Repo, "scratch.txt"))
}

func captureCorrectableFinding(r *journeyRun) error {
	return captureCorrectableFindingFor(r)
}

func captureCorrectableFindingFor(r *journeyRun, selectors ...string) error {
	envelope, err := readStatusFor(r, selectors...)
	if err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) != 1 ||
		envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		return fmt.Errorf("expected one reviewer-result transition, got %+v", envelope.NextTransition)
	}
	input := envelope.NextTransition.Collect.Inputs[0]
	payload, err := json.Marshal(map[string]any{
		"subject_hash": input.ArtifactSubject.SubjectHash,
		"inspection":   map[string]any{"status": "completed", "paths": envelope.paths()},
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
	path, err := writeScratch(r.sandbox, "blocking-reviewer.json", payload)
	if err != nil {
		return err
	}
	observation := r.run([]string{
		"review", "capture-result", "--cwd", r.sandbox.Repo,
		"--lineage", envelope.argument("lineage"), "--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"), "--lens", envelope.argument("lens"),
		"--order", envelope.argument("order"), "--input", path,
	}, true)
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture blocking reviewer result: %s", firstLine(observation.Stderr))
	}
	return captureAllLensesFor(r, selectors...)
}

func writeCorrectedCandidate(sandbox *Sandbox) error {
	const corrected = "package candidate\n\nfunc value() int { return 2 }\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), corrected); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--name-only")
	if err != nil {
		return err
	}
	if paths != "candidate.go" {
		return fmt.Errorf("correction fixture changed %q, want candidate.go", paths)
	}
	return nil
}

func readCorrectionStatus(r *journeyRun) (waveCorrectionStatus, error) {
	return readCorrectionStatusFor(r, correctedDeliveryLineage)
}

func readCorrectionStatusFor(r *journeyRun, lineage string) (waveCorrectionStatus, error) {
	return readCorrectionStatusForContract(r, lineage, reviewContract)
}

func readCorrectionStatusForContract(r *journeyRun, lineage, contract string) (waveCorrectionStatus, error) {
	arguments := []string{"review", "status", "--contract", contract, "--next-transition", "--lineage", lineage}
	if contract == reviewContractV2 {
		arguments = append(arguments, "--agent", "claude-code")
	}
	observation := r.run(productArgsFor(r, arguments...), false)
	var status waveCorrectionStatus
	return status, decodeWaveObservation(observation, &status, "corrected review status")
}

func correctionSubmissionArguments(r *journeyRun, status waveCorrectionStatus, reason, slot, value string) ([]string, error) {
	if status.Authority == nil || status.NextTransition == nil || status.NextTransition.Kind != "collect" ||
		status.NextTransition.ReasonCode != reason || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 {
		return nil, fmt.Errorf("submission descriptor transition = %+v", status.NextTransition)
	}
	descriptor := status.NextTransition.Collect.Inputs[0].Submission
	if descriptor == nil || descriptor.OperationToken != "finalize" || descriptor.Value.Slot != slot ||
		descriptor.Value.SubstitutionLocation < 0 || descriptor.Value.SubstitutionLocation >= len(descriptor.ArgumentTokens) {
		return nil, fmt.Errorf("submission descriptor = %+v", descriptor)
	}
	placeholders := 0
	for index, token := range descriptor.ArgumentTokens {
		if !strings.HasPrefix(token, "--") || strings.ContainsAny(token, " \t\r\n") || strings.HasPrefix(token, "--cwd=") || strings.Contains(token, r.sandbox.Root) {
			return nil, fmt.Errorf("submission descriptor leaked a path or shell token: %q", token)
		}
		if strings.Contains(token, "{{value}}") {
			placeholders++
			if index != descriptor.Value.SubstitutionLocation || strings.Count(token, "{{value}}") != 1 {
				return nil, fmt.Errorf("submission descriptor slot = %q at %d", token, index)
			}
		}
	}
	if len(descriptor.ArgumentTokens) < 5 {
		return nil, fmt.Errorf("submission descriptor argv has %d tokens, need at least 5: %v", len(descriptor.ArgumentTokens), descriptor.ArgumentTokens)
	}
	if placeholders != 1 || !strings.HasPrefix(descriptor.ArgumentTokens[4], "--request-hash=") {
		return nil, fmt.Errorf("submission descriptor argv = %v", descriptor.ArgumentTokens)
	}
	arguments := append([]string{"review", descriptor.OperationToken}, descriptor.ArgumentTokens...)
	arguments[descriptor.Value.SubstitutionLocation+2] = strings.Replace(arguments[descriptor.Value.SubstitutionLocation+2], "{{value}}", value, 1)
	if strings.Contains(arguments[descriptor.Value.SubstitutionLocation+2], "{{value}}") {
		return nil, errors.New("submission descriptor did not replace its only value slot")
	}
	return arguments, nil
}

func submitCorrectionPlan(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, correctedDeliveryLineage, reviewContractV2)
	if err != nil {
		return err
	}
	arguments, err := correctionSubmissionArguments(r, status, "correction_plan_required", "correction_lines", "2")
	if err != nil {
		return err
	}
	result, err := decodeWaveOperation(r.runAt(r.sandbox.Root, arguments, false), "correction submission descriptor")
	if err != nil || result.State != "correction_required" || result.LineageID != correctedDeliveryLineage {
		return fmt.Errorf("correction submission descriptor result = %+v, %v", result, err)
	}
	return nil
}

func capturePassedCorrectionEvidence(r *journeyRun) error {
	return capturePassedCorrectionEvidenceFor(r, correctedDeliveryLineage)
}

func capturePassedCorrectionEvidenceFor(r *journeyRun, lineage string) error {
	status, err := readCorrectionStatusFor(r, lineage)
	if err != nil {
		return err
	}
	if status.Authority == nil || status.Authority.State != "correction_required" || status.ValidationRequest == nil ||
		status.NextTransition == nil || status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "correction_repository_verification_required" {
		return fmt.Errorf("correction is not waiting on repository verification: %+v", status)
	}
	path, err := writeScratch(r.sandbox, "correction-evidence.txt", []byte("focused and full repository verification passed\n"))
	if err != nil {
		return err
	}
	observation := r.run(productArgsFor(r, "review", "capture-evidence",
		"--lineage", status.Authority.LineageID, "--target", status.ValidationRequest.CorrectionTargetIdentity,
		"--expected-revision", status.Authority.Revision, "--outcome", "passed", "--input", path), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture correction evidence: %s", firstLine(observation.Stderr))
	}
	r.sandbox.Scratch["validation-request"] = status.ValidationRequest.RequestHash
	r.sandbox.Scratch["correction-target"] = status.ValidationRequest.CorrectionTargetIdentity
	r.sandbox.Scratch["correction-paths"] = status.ValidationRequest.CorrectionPathsDigest
	return nil
}

func completeCorrectedReview(r *journeyRun) error {
	return completeCorrectedReviewForContract(r, correctedDeliveryLineage, reviewContractV2)
}

func completeCorrectedReviewFor(r *journeyRun, lineage string) error {
	return completeCorrectedReviewForContract(r, lineage, reviewContract)
}

func completeCorrectedReviewForContract(r *journeyRun, lineage, contract string) error {
	status, err := readCorrectionStatusForContract(r, lineage, contract)
	if err != nil {
		return err
	}
	if status.ValidationRequest == nil || status.NextTransition == nil || status.NextTransition.ReasonCode != "targeted_validation_required" ||
		status.ValidationRequest.RequestHash != r.sandbox.Scratch["validation-request"] ||
		status.ValidationRequest.CorrectionTargetIdentity != r.sandbox.Scratch["correction-target"] {
		return fmt.Errorf("provider validation request changed after evidence capture: %+v", status)
	}
	validation, err := json.MarshalIndent(map[string]any{
		"targeted_validation_request_hash": status.ValidationRequest.RequestHash,
		"correction_target_identity":       status.ValidationRequest.CorrectionTargetIdentity,
		"original_criteria":                map[string]any{"passed": true, "evidence": []string{"original acceptance check passed"}},
		"correction_regression":            map[string]any{"passed": true, "evidence": []string{"targeted regression check passed"}},
		"follow_ups":                       []any{},
	}, "", "  ")
	if err != nil {
		return err
	}
	path, err := writeScratch(r.sandbox, "targeted-validation.json", append(validation, '\n'))
	if err != nil {
		return err
	}
	if contract == reviewContractV2 {
		arguments, err := correctionSubmissionArguments(r, status, "targeted_validation_required", "validation", path)
		if err != nil {
			return err
		}
		result, err := decodeWaveOperation(r.runAt(r.sandbox.Root, arguments, false), "corrected review finalize")
		if err != nil {
			return err
		}
		if result.State != "approved" || result.LineageID != lineage {
			return fmt.Errorf("corrected review finalized as %+v", result)
		}
		return nil
	}
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", lineage,
		"--validation", path, "--captured-evidence=true"), false)
	result, err := decodeWaveOperation(observation, "corrected review finalize")
	if err != nil {
		return err
	}
	if result.State != "approved" || result.LineageID != lineage {
		return fmt.Errorf("corrected review finalized as %+v", result)
	}
	return nil
}

func proveCorrectedReceipt(sandbox *Sandbox, observation Observation) error {
	var status waveCorrectionStatus
	if err := decodeWaveObservation(observation, &status, "corrected receipt status"); err != nil {
		return err
	}
	if status.Authority == nil || status.Authority.LineageID != correctedDeliveryLineage || status.Authority.State != "approved" ||
		status.Receipt.Status != "present" || status.Receipt.Identity == "" ||
		status.Projection.Kind != "current-changes" ||
		status.Projection.InitialSnapshotIdentity == status.Projection.CurrentSnapshotIdentity || status.Projection.CurrentCandidateTree == "" {
		return fmt.Errorf("corrected receipt was not proven: %+v", status)
	}
	sandbox.Scratch["corrected-tree"] = status.Projection.CurrentCandidateTree
	return nil
}

func stageCorrectedTree(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil {
		return err
	}
	if tree != sandbox.Scratch["corrected-tree"] {
		return fmt.Errorf("staged corrected tree %s does not match receipt tree %s", tree, sandbox.Scratch["corrected-tree"])
	}
	return nil
}

func stageFullCorrectedTree(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err == nil {
		sandbox.Scratch["full-scope-tree"] = tree
	}
	return err
}

func proveFullScopeStatus(lineage, treeKey string) func(*Sandbox, Observation) error {
	return func(sandbox *Sandbox, observation Observation) error {
		var status waveCorrectionStatus
		if err := decodeWaveObservation(observation, &status, "full-scope status"); err != nil {
			return err
		}
		if status.Authority == nil || status.Authority.LineageID != lineage || status.Authority.State != "approved" ||
			status.Receipt.Status != "present" || status.Projection.CurrentCandidateTree != sandbox.Scratch[treeKey] ||
			strings.Join(status.Projection.Paths, "\x00") != "candidate.go\x00companion.txt" || status.Projection.PathsDigest == "" ||
			status.Projection.PathsDigest == sandbox.Scratch["correction-paths"] {
			return fmt.Errorf("full-scope authority was narrowed: %+v", status)
		}
		return nil
	}
}

func driftFullScopeCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "companion.txt"), "reviewed companion after recovery\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "companion.txt"); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err == nil {
		sandbox.Scratch["recovered-tree"] = tree
	}
	return err
}

func recoverFullScopeCandidate(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "validate", "--lineage", fullScopeLineage, "--gate", "pre-commit"), false)
	var denial gateDenial
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &denial); err != nil {
		return fmt.Errorf("parse full-scope denial: %w", err)
	}
	change := denial.Context.ScopeChange
	if denial.Result != "scope-changed" || change.PredecessorLineageID != fullScopeLineage || change.PredecessorRevision == "" {
		return fmt.Errorf("full-scope drift did not negotiate recovery: %+v", denial)
	}
	result, err := decodeWaveOperation(r.run(productArgsFor(r, "review", "recover",
		"--predecessor-lineage", change.PredecessorLineageID, "--expected-predecessor-revision", change.PredecessorRevision,
		"--successor-lineage", fullScopeSuccessor, "--disposition", "scope_changed"), false), "full-scope recovery")
	if err != nil || result.LineageID != fullScopeSuccessor || result.State != "reviewing" {
		return fmt.Errorf("full-scope successor = %+v, %v", result, err)
	}
	return nil
}

func addFullScopePathDrift(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "outside.txt"), "outside reviewed scope\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "outside.txt")
}

func requireFullScopeDrift(_ *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
		return fmt.Errorf("parse full-scope path drift: %w", err)
	}
	if gate.Allowed || gate.Result != "scope-changed" || gate.Context.Denial == nil ||
		gate.Context.Denial.Stage != "receipt-binding" || gate.Context.Denial.Code != "candidate-or-paths-mismatch" {
		return fmt.Errorf("path drift did not fail closed: %+v", gate)
	}
	return nil
}

func commitExactCorrectedDelivery(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "fix: correct candidate"); err != nil {
		return err
	}
	upstream, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "@{upstream}")
	if err != nil {
		return err
	}
	parent, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^")
	if err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	count, err := gitOut(sandbox, sandbox.Repo, "rev-list", "--count", upstream+"..HEAD")
	if err != nil {
		return err
	}
	status, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain")
	if err != nil {
		return err
	}
	if upstream != sandbox.Scratch["delivery-base"] || parent != upstream || tree != sandbox.Scratch["corrected-tree"] || count != "1" || status != "" {
		return fmt.Errorf("delivery proof failed: upstream=%s parent=%s tree=%s commits=%s status=%q", upstream, parent, tree, count, status)
	}
	return nil
}

func requireGateForLineage(observation Observation, lineage string, baseRelationship bool) error {
	var gate waveGateResult
	if err := decodeWaveObservation(observation, &gate, "review gate"); err != nil {
		return err
	}
	if !gate.Allowed || gate.Result != "allow" || gate.Context.LineageID != lineage || baseRelationship && !gate.Context.BaseRelationshipValid {
		return fmt.Errorf("gate did not allow lineage %q with the required binding: %+v", lineage, gate)
	}
	return nil
}
func requireStagedSuccessorGate(_ *Sandbox, observation Observation) error {
	return requireGateForLineage(observation, stagedSuccessorLineage, false)
}

// noOpChainGate is the composed pre-PR envelope reduced to the two facts this
// journey compares across the no-op: whether delivery was authorized, and which
// tree transition the composed proof spans. waveGateResult omits the trees.
type noOpChainGate struct {
	Result  string `json:"result"`
	Allowed bool   `json:"allowed"`
	Context struct {
		LineageID     string `json:"lineage_id"`
		BaseTree      string `json:"base_tree"`
		CandidateTree string `json:"candidate_tree"`
		Denial        *struct {
			Stage string `json:"stage"`
			Code  string `json:"code"`
		} `json:"denial"`
	} `json:"context"`
}

// recordNoOpChainComposition stores the composed two-segment proof span so the
// same gate can be compared byte for byte after an unrelated no-op authority is
// approved. Storing the span, not just "allow", is what makes the later
// assertion meaningful: a fix that authorized delivery while silently narrowing
// the composed range would still pass an allow-only check.
func recordNoOpChainComposition(sandbox *Sandbox, observation Observation) error {
	var gate noOpChainGate
	if err := decodeWaveObservation(observation, &gate, "composed pre-PR chain"); err != nil {
		return err
	}
	if !gate.Allowed || gate.Result != "allow" || gate.Context.LineageID != noOpSecondSegment {
		return fmt.Errorf("two-segment delivery was not authorized before the no-op: %+v", gate)
	}
	if gate.Context.BaseTree == "" || gate.Context.CandidateTree == "" || gate.Context.BaseTree == gate.Context.CandidateTree {
		return fmt.Errorf("composed proof does not span a real tree transition: %+v", gate)
	}
	sandbox.Scratch["noop-chain-base-tree"] = gate.Context.BaseTree
	sandbox.Scratch["noop-chain-candidate-tree"] = gate.Context.CandidateTree
	return nil
}

// requireNoOpChainCompositionUnchanged is the regression this journey exists
// for. A clean approved no-op reviewed a candidate identical to its own base, so
// its receipt edge is a self-loop that delivers nothing. Before the fix it
// entered the delivery graph, tripped cycle detection, and denied composition
// for every unrelated lineage in the repository. Delivery must stay authorized
// over the exact same composed span it had before that authority existed.
func requireNoOpChainCompositionUnchanged(sandbox *Sandbox, observation Observation) error {
	var gate noOpChainGate
	if err := decodeWaveObservation(observation, &gate, "composed pre-PR chain after no-op"); err != nil {
		return err
	}
	if !gate.Allowed || gate.Result != "allow" {
		return fmt.Errorf("an unrelated clean no-op authority denied two-segment delivery: %+v", gate)
	}
	if gate.Context.BaseTree != sandbox.Scratch["noop-chain-base-tree"] ||
		gate.Context.CandidateTree != sandbox.Scratch["noop-chain-candidate-tree"] {
		return fmt.Errorf("no-op authority changed the composed delivery span: got base %q candidate %q, want base %q candidate %q",
			gate.Context.BaseTree, gate.Context.CandidateTree,
			sandbox.Scratch["noop-chain-base-tree"], sandbox.Scratch["noop-chain-candidate-tree"])
	}
	return nil
}

// proveNoOpSelfLoopApproved fails closed if the fixture stopped producing the
// shape under test. The journey only proves something if the extra authority is
// genuinely approved and genuinely a no-op; a refused or non-degenerate start
// would make the later comparison pass for the wrong reason.
func proveNoOpSelfLoopApproved(_ *Sandbox, observation Observation) error {
	var result waveOperationResult
	if err := decodeWaveObservation(observation, &result, "no-op self-loop finalize"); err != nil {
		return err
	}
	if result.LineageID != noOpSelfLoopLineage || result.State != "approved" {
		return fmt.Errorf("no-op self-loop authority is not an approved no-op: %+v", result)
	}
	return nil
}

func proveCorrectedPrePush(sandbox *Sandbox, observation Observation) error {
	if err := requireGateForLineage(observation, correctedDeliveryLineage, true); err != nil {
		return err
	}
	var inventory waveInventory
	proof := sandbox.readBack("review", "status", "--cwd", sandbox.Repo)
	if err := decodeWaveObservation(proof, &inventory, "post-delivery inventory"); err != nil {
		return err
	}
	if !inventory.Complete || !inventory.Authoritative || len(inventory.Entries) != 1 ||
		inventory.Entries[0].LineageID != correctedDeliveryLineage || inventory.Entries[0].Status != "approved" || inventory.Entries[0].State != "approved" {
		return fmt.Errorf("pre-push required another review or recovery: %+v", inventory)
	}
	return nil
}

func captureProceduralFinalVerification(r *journeyRun) error {
	envelope, err := readStatus(r)
	if err != nil {
		return err
	}
	if envelope.Authority.LineageID != retrySourceLineage || envelope.Authority.State != "validating" ||
		len(envelope.NextTransition.Collect.Inputs) != 1 || envelope.NextTransition.Collect.Inputs[0].CaptureOperation != "review.capture-evidence" {
		return fmt.Errorf("source is not waiting on final verification evidence: %+v", envelope)
	}
	path, err := writeScratch(r.sandbox, "procedural-failure.txt", []byte("provider verification runner failed before it could execute tests\n"))
	if err != nil {
		return err
	}
	observation := r.run(productArgsFor(r, "review", "capture-evidence",
		"--lineage", envelope.Authority.LineageID, "--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"), "--outcome", "procedural_tooling_failed", "--input", path), false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture procedural evidence: %s", firstLine(observation.Stderr))
	}
	return nil
}

func requireReviewState(state, lineage string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		result, err := decodeWaveOperation(observation, "review operation")
		if err != nil {
			return err
		}
		if result.State != state || result.LineageID != lineage {
			return fmt.Errorf("review operation = %+v, want lineage %q state %q", result, lineage, state)
		}
		return nil
	}
}

func retryFinalVerification(r *journeyRun) error {
	statusObservation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContract,
		"--action-eligibility", "--next-transition", "--lineage", retrySourceLineage), false)
	var status waveRetryStatus
	if err := decodeWaveObservation(statusObservation, &status, "failed final-verification status"); err != nil {
		return err
	}
	retry := status.FinalVerificationRetry
	if status.Authority == nil || status.Authority.State != "escalated" || status.Action != "retry_final_verification" ||
		status.ActionDisposition != "final_verification_retry" || retry == nil || status.NextTransition == nil ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "final_verification_retry_authorization_required" ||
		status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) != 1 ||
		status.NextTransition.Collect.Inputs[0].CaptureOperation != "external.authorize_final_verification_retry" {
		return fmt.Errorf("status did not publish the provider retry boundary: %+v", status)
	}
	incident := waveFinalVerificationIncident{
		Schema: retry.IncidentSchema, Class: retry.IncidentClass,
		LineageID: status.Authority.LineageID, TerminalRevision: status.Authority.Revision,
		ValidatingRevision: retry.ValidatingRevision, TargetIdentity: retry.TargetIdentity,
		FailedEvidenceHash: retry.FailedEvidenceHash, FinalizeRequestDigest: retry.FinalizeRequestDigest,
	}
	payload, err := json.Marshal(incident)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	incidentPath, err := writeScratch(r.sandbox, "final-verification-incident.json", payload)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	const actor = "bench-maintainer"
	const reason = "retry after provider tooling failure"
	authorization := strings.Join([]string{
		"gentle-ai.review-final-verification-retry-authorization/v1",
		"predecessor_lineage=" + status.Authority.LineageID,
		"predecessor_revision=" + status.Authority.Revision,
		"successor_lineage=" + retrySuccessorLineage,
		"validating_revision=" + retry.ValidatingRevision,
		"target_identity=" + retry.TargetIdentity,
		"failed_evidence_hash=" + retry.FailedEvidenceHash,
		"finalize_request_digest=" + retry.FinalizeRequestDigest,
		"incident_class=" + retry.IncidentClass,
		"incident_digest=sha256:" + hex.EncodeToString(digest[:]),
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
	observation := r.run(productArgsFor(r, "review", "retry-final-verification", "--contract", reviewContract,
		"--predecessor-lineage", status.Authority.LineageID, "--expected-predecessor-revision", status.Authority.Revision,
		"--successor-lineage", retrySuccessorLineage, "--incident", incidentPath,
		"--actor", actor, "--reason", reason, "--maintainer-authorization", authorization), false)
	result, err := decodeWaveOperation(observation, "retry final verification")
	if err != nil {
		return err
	}
	if result.Operation != "review.retry_final_verification" || result.LineageID != retrySuccessorLineage ||
		result.PredecessorLineageID != retrySourceLineage || result.State != "validating" || result.TargetIdentity != retry.TargetIdentity {
		return fmt.Errorf("provider-derived retry result = %+v", result)
	}
	r.sandbox.Scratch["retry-target"] = retry.TargetIdentity
	return nil
}

func captureSuccessfulRetryEvidence(r *journeyRun) error {
	observation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContract,
		"--next-transition", "--lineage", retrySuccessorLineage), false)
	var envelope statusEnvelope
	if err := decodeWaveObservation(observation, &envelope, "retry successor status"); err != nil {
		return err
	}
	if envelope.Authority.LineageID != retrySuccessorLineage || envelope.Authority.State != "validating" ||
		envelope.argument("target") != r.sandbox.Scratch["retry-target"] {
		return fmt.Errorf("retry successor changed the provider target: %+v", envelope)
	}
	path, err := writeScratch(r.sandbox, "retry-passed.txt", []byte("retry verification completed successfully\n"))
	if err != nil {
		return err
	}
	captured := r.run(productArgsFor(r, "review", "capture-evidence",
		"--lineage", retrySuccessorLineage, "--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"), "--outcome", "passed", "--input", path), false)
	if captured.ExitCode != 0 {
		return fmt.Errorf("capture retry evidence: %s", firstLine(captured.Stderr))
	}
	return nil
}

func proveCompletedRetryInventory(_ *Sandbox, observation Observation) error {
	var inventory waveInventory
	if err := decodeWaveObservation(observation, &inventory, "completed retry inventory"); err != nil {
		return err
	}
	if !inventory.Complete || !inventory.Authoritative || len(inventory.Entries) != 2 {
		return fmt.Errorf("completed retry inventory is not authoritative: %+v", inventory)
	}
	statuses := map[string]string{}
	states := map[string]string{}
	for _, entry := range inventory.Entries {
		statuses[entry.LineageID] = entry.Status
		states[entry.LineageID] = entry.State
	}
	if statuses[retrySourceLineage] != "superseded" || states[retrySourceLineage] != "escalated" ||
		statuses[retrySuccessorLineage] != "recovered" || states[retrySuccessorLineage] != "approved" {
		return fmt.Errorf("completed retry inventory statuses = %+v, states = %+v", statuses, states)
	}
	return nil
}

func rememberArchiveAuthority(sandbox *Sandbox, observation Observation) error {
	if err := rememberLineage(sandbox, observation); err != nil {
		return err
	}
	if sandbox.Lineage == "" || sandbox.Revision == "" {
		return errors.New("approved archive authority published no lineage or revision")
	}
	sandbox.Scratch["archive-authority-revision"] = sandbox.Revision
	return nil
}

func requireDiscoveredArchivePremise(_ *Sandbox, observation Observation) error {
	return sddStatusAssertion("discovered receipt premise", func(status sddStatusV1) error {
		if status.ReviewGate == nil || status.ReviewGate.Result != "allow" || status.ReviewGate.Delivery != "" {
			return fmt.Errorf("reviewGate = %+v, want discovered allow without a delivery override", status.ReviewGate)
		}
		if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || len(status.BlockedReasons) != 0 {
			return fmt.Errorf("archive=%q next=%q blocked=%v, want ready/archive with no blockers",
				status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
		}
		return nil
	})(nil, observation)
}

func requireDiscoveredArchiveStatus(disabled bool) func(*Sandbox, Observation) error {
	return sddStatusAssertion("discovered invalidated archive authority", func(status sddStatusV1) error {
		if status.ReviewGate == nil || status.ReviewGate.Result != "invalidated" {
			return fmt.Errorf("reviewGate = %+v, want invalidated", status.ReviewGate)
		}
		if !strings.Contains(status.ReviewGate.Reason, "review receipt was invalidated") {
			return fmt.Errorf("reviewGate.reason = %q, want the discovered authority reason", status.ReviewGate.Reason)
		}
		if disabled {
			if status.ReviewGate.Delivery != deliveryDisabledUnmanaged || status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" {
				return fmt.Errorf("disabled gate=%+v archive=%q next=%q, want invalidated disabled/unmanaged ready/archive",
					status.ReviewGate, status.Dependencies.Archive, status.NextRecommended)
			}
			return nil
		}
		if status.ReviewGate.Delivery != "" || status.Dependencies.Archive != "blocked" || status.NextRecommended != "resolve-review" {
			return fmt.Errorf("enabled gate=%+v archive=%q next=%q, want invalidated blocked/resolve-review",
				status.ReviewGate, status.Dependencies.Archive, status.NextRecommended)
		}
		return nil
	})
}

func introduceArchiveGateInvalidation(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sandbox.Repo, "outside-reviewed-scope.txt"), "unreviewed\n")
}

func writeExplicitInvalidArchiveReceipt(sandbox *Sandbox) error {
	return sandbox.write(filepath.Join(sddChangeRoot(sandbox), "reviews", "receipt.json"), "{\n")
}

func requireExplicitInvalidArchiveStatus(_ *Sandbox, observation Observation) error {
	return sddStatusAssertion("explicit invalid archive authority", func(status sddStatusV1) error {
		if status.ReviewGate == nil || status.ReviewGate.Result != "invalidated" || status.ReviewGate.Delivery != "" {
			return fmt.Errorf("reviewGate = %+v, want explicit invalidated without disabled delivery", status.ReviewGate)
		}
		if !strings.Contains(status.ReviewGate.Reason, "invalid or non-terminal") || status.Dependencies.Archive != "blocked" || status.NextRecommended != "resolve-review" {
			return fmt.Errorf("explicit gate=%+v archive=%q next=%q, want fail-closed blocked/resolve-review",
				status.ReviewGate, status.Dependencies.Archive, status.NextRecommended)
		}
		return nil
	})(nil, observation)
}

func requireDisabledUnmanagedArchiveStatus(name string) func(*Sandbox, Observation) error {
	return sddStatusAssertion(name, func(status sddStatusV1) error {
		if status.ReviewGate == nil || status.ReviewGate.Delivery != deliveryDisabledUnmanaged || status.ReviewGate.Result == "allow" {
			return fmt.Errorf("reviewGate = %+v, want non-authorizing disabled/unmanaged delivery", status.ReviewGate)
		}
		if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || len(status.BlockedReasons) != 0 {
			return fmt.Errorf("archive=%q next=%q blocked=%v, want ready/archive with no blockers",
				status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
		}
		return nil
	})
}

func proveArchiveAuthorityUnchanged(sandbox *Sandbox) error {
	head, err := proveAuthorities(sandbox)
	if err != nil {
		return err
	}
	for _, entry := range head.Entries {
		if entry.LineageID == sandbox.Lineage {
			if entry.State != "approved" || entry.Revision != sandbox.Scratch["archive-authority-revision"] {
				return fmt.Errorf("archive authority changed to state=%q revision=%q", entry.State, entry.Revision)
			}
			return nil
		}
	}
	return fmt.Errorf("archive authority %q disappeared", sandbox.Lineage)
}

func prepareDeclinedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, declineCandidatePath), declineCandidateContents); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "ls-files", "--others", "--exclude-standard")
	if err != nil || paths != declineCandidatePath {
		return fmt.Errorf("decline fixture untracked paths = %q, %v", paths, err)
	}
	return nil
}

func declineCandidateFromStatus(r *journeyRun) error {
	statusObservation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2,
		"--agent", "claude-code", "--next-transition", "--lineage", declineCandidateLineage), false)
	var status statusEnvelope
	if err := decodeWaveObservation(statusObservation, &status, "candidate decline status"); err != nil {
		return err
	}
	projection := status.Projection
	if status.TargetIdentity == "" || projection.BaseTree == "" || projection.CurrentCandidateTree == "" ||
		projection.PathsDigest == "" || strings.Join(projection.Paths, "\x00") != declineCandidatePath ||
		status.NextTransition.Kind != "execute" || status.NextTransition.Execute.Operation != "review.start" || status.NextTransition.Execute.Command == "" {
		return fmt.Errorf("status did not derive an exact v2 START: %+v", status)
	}
	r.sandbox.Scratch["decline-target"] = status.TargetIdentity
	r.sandbox.Scratch["decline-base"] = projection.BaseTree
	r.sandbox.Scratch["decline-tree"] = projection.CurrentCandidateTree
	r.sandbox.Scratch["decline-paths"] = projection.PathsDigest

	relayArgs, err := waveReviewInvocationArgs(status.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	var consent waveConsentResult
	if err := decodeWaveObservation(r.run(relayArgs, false), &consent, "candidate consent relay"); err != nil {
		return err
	}
	declineInvocation := ""
	for _, choice := range consent.Choices {
		if choice.Answer == "declined" {
			declineInvocation = choice.Invocation
		}
	}
	if consent.Action != "consent_required" || consent.TargetIdentity != status.TargetIdentity || declineInvocation == "" {
		return fmt.Errorf("consent relay did not preserve the status target: %+v", consent)
	}
	declineArgs, err := waveReviewInvocationArgs(declineInvocation)
	if err != nil {
		return err
	}
	var declined waveDeclinedStart
	if err := decodeWaveObservation(r.run(declineArgs, false), &declined, "candidate decline"); err != nil {
		return err
	}
	if declined.Action != "declined" || declined.Consent != "declined_this_candidate" || declined.LineageID != "" || declined.TargetIdentity != status.TargetIdentity {
		return fmt.Errorf("decline result created or changed authority: %+v", declined)
	}
	var inventory waveInventory
	if err := decodeWaveObservation(r.run(productArgsFor(r, "review", "status"), false), &inventory, "post-decline authority inventory"); err != nil {
		return err
	}
	if !inventory.Complete || !inventory.Authoritative || len(inventory.Entries) != 0 {
		return fmt.Errorf("decline created review authority: %+v", inventory)
	}
	return nil
}

func stageDeclinedCandidate(sandbox *Sandbox) error {
	if err := sandbox.git(sandbox.Repo, "add", declineCandidatePath); err != nil {
		return err
	}
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil || tree != sandbox.Scratch["decline-tree"] {
		return fmt.Errorf("staged decline tree = %q, want %q: %v", tree, sandbox.Scratch["decline-tree"], err)
	}
	return nil
}

func driftDeclinedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, declineCandidatePath), "#!/bin/sh\necho drift\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", declineCandidatePath)
}

func addDeclinedPathDrift(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, declineCandidatePath), declineCandidateContents); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "scripts/extra.sh"), "#!/bin/sh\necho extra\n"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", declineCandidatePath, "scripts/extra.sh")
}

func requireCandidateDeclineRejected(name string) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var gate waveGateResult
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
			return fmt.Errorf("parse %s denial: %w", name, err)
		}
		if observation.ExitCode == 0 || gate.Allowed || gate.Result == "allow" || gate.Delivery != "" || gate.Action == "repository-policy" ||
			gate.Context.Denial != nil && gate.Context.Denial.Stage == "candidate-decline" {
			return fmt.Errorf("%s inherited candidate decline: exit=%d gate=%+v", name, observation.ExitCode, gate)
		}
		return nil
	}
}

func waveOneJourneys() []Journey {
	return []Journey{
		{
			ID:     "j44-corrected-current-changes-delivery",
			Title:  "Corrected current-changes receipt: one exact linked-worktree delivery is discovered selector-free",
			Source: "issue #1819 + shape 3 (the squashed-delivery proof was hidden behind the wrong binding condition)",
			Steps: []Step{
				{Name: "fixture: linked worktree and local remote topology proven", Fixture: linkedWorktreeWithRemote},
				{Name: "fixture: one exact code candidate proven staged", Fixture: stageWaveCandidate},
				{Name: "review start in the linked worktree", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", correctedDeliveryLineage), After: rememberLineage},
				{Name: "capture one blocking finding and finish the lens set", Requires: captureResultCapability, Composite: captureCorrectableFinding},
				{Name: "finalize reviewer results into correction-required", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", correctedDeliveryLineage, "--captured-results=true"),
					After: requireReviewState("correction_required", correctedDeliveryLineage)},
				{Name: "derive and execute the correction submission descriptor", Requires: finalizeCorrectionCapability, Composite: submitCorrectionPlan},
				{Name: "fixture: corrected candidate proven to change only the reviewed path", Fixture: writeCorrectedCandidate},
				{Name: "capture passed repository evidence for the provider correction target", Requires: captureOutcomeEvidenceCapability, Composite: capturePassedCorrectionEvidence},
				{Name: "execute the targeted validation submission descriptor and approve the corrected receipt", Requires: finalizeValidationCapability, Composite: completeCorrectedReview},
				{Name: "corrected receipt and advanced current snapshot are proven", Requires: statusCapability,
					Args: productArgs("review", "status", "--contract", reviewContractV2, "--agent", "claude-code", "--lineage", correctedDeliveryLineage), After: proveCorrectedReceipt},
				{Name: "fixture: corrected candidate staged tree matches the receipt", Fixture: stageCorrectedTree},
				{Name: "gate pre-commit on the corrected receipt", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"),
					After: func(_ *Sandbox, observation Observation) error {
						return requireGateForLineage(observation, correctedDeliveryLineage, false)
					}},
				{Name: "fixture: exactly one clean delivery commit proven against upstream", Fixture: commitExactCorrectedDelivery},
				{Name: "selector-free pre-push discovers the corrected receipt without recovery or another review", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-push"), After: proveCorrectedPrePush},
			},
		},
		{
			ID:     "j45-completed-final-verification-retry",
			Title:  "Completed final-verification retry: provider successor remains authoritative in inventory and post-apply",
			Source: "issue #1915 + shape 3 (retry edge validation was gated on a validating-only successor)",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: one exact code candidate proven staged", Fixture: stageWaveCandidate},
				{Name: "review start", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", retrySourceLineage), After: rememberLineage},
				{Name: "capture every lens", Requires: captureResultCapability, Composite: captureAllLenses},
				{Name: "finalize reviewer results into final verification", Requires: finalizeResultsCapability,
					Args:  productArgs("review", "finalize", "--lineage", retrySourceLineage, "--captured-results=true"),
					After: requireReviewState("validating", retrySourceLineage)},
				{Name: "capture procedural final-verification failure", Requires: captureOutcomeEvidenceCapability, Composite: captureProceduralFinalVerification},
				{Name: "complete failed final verification", Requires: finalizeProceduralFailureCapability,
					Args:  productArgs("review", "finalize", "--lineage", retrySourceLineage, "--captured-evidence=true", "--failed=true"),
					After: requireReviewState("escalated", retrySourceLineage)},
				{Name: "derive provider retry binding and create its validating successor", Requires: retryFinalVerificationCapability, Composite: retryFinalVerification},
				{Name: "capture successful evidence against the frozen retry target", Requires: captureOutcomeEvidenceCapability, Composite: captureSuccessfulRetryEvidence},
				{Name: "complete the retry successor", Requires: finalizeValidationCapability,
					Args:  productArgs("review", "finalize", "--lineage", retrySuccessorLineage, "--captured-evidence=true"),
					After: requireReviewState("approved", retrySuccessorLineage)},
				{Name: "whole inventory remains complete and authoritative", Requires: statusOnlyCapability,
					Args: productArgs("review", "status"), After: proveCompletedRetryInventory},
				{Name: "selector-free post-apply remains owned by the completed retry successor", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "post-apply"),
					After: func(_ *Sandbox, observation Observation) error {
						return requireGateForLineage(observation, retrySuccessorLineage, false)
					}},
			},
		},
		{
			ID:     "j46-correction-required-staged-recovery",
			Title:  "Correction-required base diff: negotiated staged recovery receives a fresh review and delivers",
			Source: "issue #1921",
			Steps: []Step{
				{Name: "fixture: linked worktree and remote", Fixture: linkedWorktreeWithRemote},
				{Name: "fixture: commit base-diff candidate", Fixture: commitStagedRecoveryCandidate},
				{Name: "start workspace-projected base-diff review", Requires: startNamedCapability, Args: func(s *Sandbox) ([]string, error) {
					return append([]string{"review", "start", "--lineage", stagedRecoveryLineage, "--base-ref", s.Scratch["staged-recovery-base"], "--committed-only"}, "--cwd", s.Repo), nil
				}},
				{Name: "capture blocker on predecessor", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureCorrectableFindingFor(r, stagedPredecessorSelectors(r.sandbox)...)
				}},
				{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--captured-results=true")},
				{Name: "forecast correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", stagedRecoveryLineage, "--correction-lines", "3")},
				{Name: "fixture: exact correction and migration staged with noise outside index", Fixture: stageExpandedCorrection},
				{Name: "negotiate and execute staged recovery", Requires: recoverCapability, Composite: recoverStagedCorrection},
				{Name: "fresh successor reviewer pass", Requires: captureResultCapability, Composite: func(r *journeyRun) error { return captureAllLensesFor(r, stagedRecoverySelectors(r.sandbox)...) }},
				{Name: "finish successor review", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", stagedSuccessorLineage, "--captured-results=true")},
				{Name: "capture successor verification", Requires: captureOutcomeEvidenceCapability, Composite: func(r *journeyRun) error { return captureFinalEvidenceFor(r, stagedRecoverySelectors(r.sandbox)...) }},
				{Name: "approve fresh successor", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--lineage", stagedSuccessorLineage, "--captured-evidence=true")},
				{Name: "gate exact staged candidate at pre-commit", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", stagedSuccessorLineage, "--gate", "pre-commit"), After: requireStagedSuccessorGate},
				{Name: "fixture: commit reviewed overlay and remove unstaged noise", Fixture: commitRecoveredStagedOverlay},
				{Name: "gate recovered delivery at pre-push", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", stagedSuccessorLineage, "--gate", "pre-push"), After: requireStagedSuccessorGate},
				{Name: "gate recovered delivery at pre-pr", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", stagedSuccessorLineage, "--gate", "pre-pr", "--base-ref", "origin/feature"), After: requireStagedSuccessorGate},
			},
		},
		{
			ID:     "j47-disabled-mode-archives-discovered-invalidated-receipt",
			Title:  "Discovered invalidated archive receipt: disabled mode steps aside without weakening explicit authority",
			Source: "issue #2128",
			Steps: []Step{
				{Name: "fixture: archive-ready SDD change", Fixture: sddPlanningArtifacts(sddVerifyReport)},
				{Name: "fixture: stage the reviewed candidate", Fixture: stageProse("", "archive-authority")},
				{Name: "review start", Requires: startCapability, Args: productArgs("review", "start")},
				{Name: "review finalize", Requires: finalizeCapability, Args: productArgs("review", "finalize"), After: rememberArchiveAuthority},
				{Name: "discovered receipt allows before invalidation", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireDiscoveredArchivePremise},
				{Name: "fixture: add untracked content outside the reviewed scope", Fixture: introduceArchiveGateInvalidation},
				{Name: "enabled discovered receipt blocks", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireDiscoveredArchiveStatus(false)},
				{Name: "mode disable", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "disabled discovered receipt closes unmanaged", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireDiscoveredArchiveStatus(true)},
				{Name: "mode enable", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--json")},
				{Name: "re-enabled discovered receipt blocks again", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireDiscoveredArchiveStatus(false)},
				{Name: "fixture: write an explicit invalid receipt", Fixture: writeExplicitInvalidArchiveReceipt},
				{Name: "mode disable with explicit receipt", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
				{Name: "disabled explicit invalid receipt still blocks", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireExplicitInvalidArchiveStatus},
				{Name: "authority remained approved at its original revision", Fixture: proveArchiveAuthorityUnchanged},
			},
		},
		{
			ID:     "j48-recovered-workspace-preserves-full-candidate-scope",
			Title:  "Recovered workspace correction: terminal authorities preserve the complete candidate scope",
			Source: "issue #2090",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: two-path candidate staged", Fixture: stageFullScopeCandidate},
				{Name: "start two-path review", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", fullScopeLineage)},
				{Name: "capture blocker and complete lenses", Requires: captureResultCapability, Composite: captureCorrectableFinding},
				{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", fullScopeLineage, "--captured-results=true")},
				{Name: "forecast strict-subset correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", fullScopeLineage, "--correction-lines", "2")},
				{Name: "fixture: correction touches only candidate.go", Fixture: writeCorrectedCandidate},
				{Name: "capture correction-local repository evidence", Requires: captureOutcomeEvidenceCapability, Composite: func(r *journeyRun) error { return capturePassedCorrectionEvidenceFor(r, fullScopeLineage) }},
				{Name: "approve corrected full candidate", Requires: finalizeValidationCapability, Composite: func(r *journeyRun) error { return completeCorrectedReviewFor(r, fullScopeLineage) }},
				{Name: "fixture: stage exact corrected candidate", Fixture: stageFullCorrectedTree},
				{Name: "corrected authority preserves full tree and paths", Requires: statusCapability, Args: productArgs("review", "status", "--contract", reviewContract, "--lineage", fullScopeLineage), After: proveFullScopeStatus(fullScopeLineage, "full-scope-tree")},
				{Name: "immediate corrected pre-commit allows", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", fullScopeLineage, "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireGateForLineage(observation, fullScopeLineage, false)
				}},
				{Name: "fixture: reviewed companion bytes drift", Fixture: driftFullScopeCandidate},
				{Name: "follow scope denial into recovered successor", Requires: recoverCapability, Composite: recoverFullScopeCandidate},
				{Name: "complete successor lenses", Requires: captureResultCapability, Composite: func(r *journeyRun) error { return captureAllLensesFor(r, "--lineage", fullScopeSuccessor) }},
				{Name: "finish successor review", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", fullScopeSuccessor, "--captured-results=true")},
				{Name: "capture successor verification", Requires: captureOutcomeEvidenceCapability, Composite: func(r *journeyRun) error { return captureFinalEvidenceFor(r, "--lineage", fullScopeSuccessor) }},
				{Name: "approve recovered successor", Requires: finalizeEvidenceCapability, Args: productArgs("review", "finalize", "--lineage", fullScopeSuccessor, "--captured-evidence=true")},
				{Name: "successor preserves full tree and paths", Requires: statusCapability, Args: productArgs("review", "status", "--contract", reviewContract, "--lineage", fullScopeSuccessor), After: proveFullScopeStatus(fullScopeSuccessor, "recovered-tree")},
				{Name: "immediate successor pre-commit allows", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", fullScopeSuccessor, "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireGateForLineage(observation, fullScopeSuccessor, false)
				}},
				{Name: "fixture: add path outside recovered scope", Fixture: addFullScopePathDrift},
				{Name: "path drift still fails closed", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", fullScopeSuccessor, "--gate", "pre-commit"), After: requireFullScopeDrift},
			},
		},
		{
			ID:     "j49-status-without-cwd-honors-kill-switch",
			Title:  "SDD status without CWD: repository resolution and the kill switch share one workspace",
			Source: "issue #2129",
			Steps: []Step{
				{Name: "fixture: archive-ready SDD change", Fixture: sddPlanningArtifacts(sddVerifyReport)},
				{Name: "disable review mode for the clone", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone", "--json")},
				{Name: "explicit CWD honors disabled mode", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--json"), After: requireDisabledUnmanagedArchiveStatus("explicit CWD control")},
				{Name: "omitted CWD honors the same disabled mode", Requires: sddStatusCapability,
					Args: func(*Sandbox) ([]string, error) {
						return []string{"sdd-status", sddChange, "--json"}, nil
					}, After: requireDisabledUnmanagedArchiveStatus("omitted CWD")},
			},
		},
		{
			ID:     "j50-candidate-decline-preserves-frozen-delivery-identity",
			Title:  "Candidate decline: exact frozen identity reaches delivery without creating review authority",
			Source: "issue #2045",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: high-risk candidate remains untracked", Fixture: prepareDeclinedCandidate},
				{Name: "derive v2 START and execute emitted relay and decline", Requires: statusCapability, Composite: declineCandidateFromStatus},
				{Name: "fixture: stage the exact unchanged declined candidate", Fixture: stageDeclinedCandidate},
				{Name: "separate process preserves exact unmanaged identity", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: requireCandidateDeclinedGate},
				{Name: "release cannot inherit candidate decline", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "release"), After: requireCandidateDeclineRejected("release")},
				{Name: "fixture: candidate bytes drift", Fixture: driftDeclinedCandidate},
				{Name: "byte drift cannot inherit candidate decline", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: requireCandidateDeclineRejected("byte drift")},
				{Name: "fixture: restore bytes and add path drift", Fixture: addDeclinedPathDrift},
				{Name: "path drift cannot inherit candidate decline", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: requireCandidateDeclineRejected("path drift")},
			},
		},
		{
			// The two segments are approved and committed BEFORE the no-op exists,
			// and the composed span is recorded at that point. The no-op is then
			// approved on a clean worktree, which is what makes it a self-loop:
			// its candidate equals its own base, so it delivers no tree
			// transition. Re-running the identical gate is the whole test.
			ID:     "j51-unrelated-noop-authority-keeps-composed-delivery",
			Title:  "Composed pre-PR delivery: an unrelated clean no-op authority never denies a two-segment chain",
			Source: "issue #2125",
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage first segment", Fixture: stageDocs("segment-one")},
				{Name: "review first segment", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", noOpFirstSegment)},
				{Name: "approve first segment", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", noOpFirstSegment)},
				{Name: "first segment pre-commit allows", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", noOpFirstSegment, "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireGateForLineage(observation, noOpFirstSegment, false)
				}},
				{Name: "fixture: commit first segment", Fixture: commitStaged("docs: segment one")},
				{Name: "fixture: stage second segment", Fixture: stageDocs("segment-two")},
				{Name: "review second segment", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", noOpSecondSegment)},
				{Name: "approve second segment", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", noOpSecondSegment)},
				{Name: "second segment pre-commit allows", Requires: validateCapability, Args: productArgs("review", "validate", "--lineage", noOpSecondSegment, "--gate", "pre-commit"), After: func(_ *Sandbox, observation Observation) error {
					return requireGateForLineage(observation, noOpSecondSegment, false)
				}},
				{Name: "fixture: commit second segment", Fixture: commitStaged("docs: segment two")},
				// No --lineage: composition is only attempted for a selector-free
				// pre-PR gate (compact_chain.go rejects the request outright when a
				// lineage is named), because naming one asks for that single
				// receipt instead of the composed chain.
				{Name: "two delivered segments compose before the no-op", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-pr", "--base-ref", "origin/main"), After: recordNoOpChainComposition},
				{Name: "review the clean worktree as a no-op", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", noOpSelfLoopLineage)},
				{Name: "approve the no-op self-loop", Requires: finalizeCapability, Args: productArgs("review", "finalize", "--lineage", noOpSelfLoopLineage), After: proveNoOpSelfLoopApproved},
				{Name: "same two segments still compose after the no-op", Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-pr", "--base-ref", "origin/main"), After: requireNoOpChainCompositionUnchanged, AbortOnBlock: true},
			},
		},
	}
}
