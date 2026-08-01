package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		Kind                    string `json:"kind"`
		InitialSnapshotIdentity string `json:"initial_snapshot_identity"`
		CurrentSnapshotIdentity string `json:"current_snapshot_identity"`
		CurrentCandidateTree    string `json:"current_candidate_tree"`
	} `json:"projection"`
	ValidationRequest *struct {
		RequestHash              string `json:"request_hash"`
		CorrectionTargetIdentity string `json:"correction_target_identity"`
	} `json:"validation_request"`
	NextTransition *struct {
		Kind       string `json:"kind"`
		ReasonCode string `json:"reason_code"`
	} `json:"next_transition"`
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
	Result  string `json:"result"`
	Allowed bool   `json:"allowed"`
	Context struct {
		LineageID             string `json:"lineage_id"`
		BaseRelationshipValid bool   `json:"base_relationship_valid"`
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

func decodeWaveObservation(observation Observation, target any, label string) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("%s exited %d: %s", label, observation.ExitCode, firstLine(observation.Stderr))
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), target); err != nil {
		return fmt.Errorf("parse %s: %w (stderr: %s)", label, err, firstLine(observation.Stderr))
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
	observation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContract, "--next-transition", "--lineage", correctedDeliveryLineage), false)
	var status waveCorrectionStatus
	return status, decodeWaveObservation(observation, &status, "corrected review status")
}

func capturePassedCorrectionEvidence(r *journeyRun) error {
	status, err := readCorrectionStatus(r)
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
	return nil
}

func completeCorrectedReview(r *journeyRun) error {
	status, err := readCorrectionStatus(r)
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
	observation := r.run(productArgsFor(r, "review", "finalize", "--lineage", correctedDeliveryLineage,
		"--validation", path, "--captured-evidence=true"), false)
	result, err := decodeWaveOperation(observation, "corrected review finalize")
	if err != nil {
		return err
	}
	if result.State != "approved" || result.LineageID != correctedDeliveryLineage {
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
				{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability,
					Args: productArgs("review", "finalize", "--lineage", correctedDeliveryLineage, "--correction-lines", "2")},
				{Name: "fixture: corrected candidate proven to change only the reviewed path", Fixture: writeCorrectedCandidate},
				{Name: "capture passed repository evidence for the provider correction target", Requires: captureOutcomeEvidenceCapability, Composite: capturePassedCorrectionEvidence},
				{Name: "complete targeted validation and approve the corrected receipt", Requires: finalizeValidationCapability, Composite: completeCorrectedReview},
				{Name: "corrected receipt and advanced current snapshot are proven", Requires: statusCapability,
					Args: productArgs("review", "status", "--contract", reviewContract, "--lineage", correctedDeliveryLineage), After: proveCorrectedReceipt},
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
	}
}
