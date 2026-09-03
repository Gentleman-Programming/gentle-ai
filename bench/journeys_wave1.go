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
	correctedDeliveryLineage   = "wave-one-corrected-delivery"
	retrySourceLineage         = "wave-one-final-verification-source"
	retrySuccessorLineage      = "wave-one-final-verification-successor"
	stagedRecoveryLineage      = "wave-one-staged-recovery-source"
	stagedSuccessorLineage     = "wave-one-staged-recovery-successor"
	fullScopeLineage           = "wave-one-full-scope-source"
	fullScopeSuccessor         = "wave-one-full-scope-successor"
	committedCorrectionLineage = "wave-one-committed-correction"
	declineCandidateLineage    = "wave-one-candidate-decline"
	declineCandidatePath       = "scripts/deploy.sh"
	declineCandidateContents   = "#!/bin/sh\necho deploy\n"
	reviewContractV2           = "gentle-ai.review-integration/v2"
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

type waveTransitionArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Token string `json:"token"`
}

type waveCorrectionStatus struct {
	Schema         string `json:"schema"`
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
				Name             string                         `json:"name"`
				CaptureOperation string                         `json:"capture_operation"`
				Arguments        []struct{ Name, Value string } `json:"arguments"`
				Submission       *waveSubmissionDescriptor      `json:"submission"`
				ProviderTask     *struct {
					Prompt string `json:"prompt"`
					Role   string `json:"role"`
				} `json:"provider_task"`
			} `json:"inputs"`
		} `json:"collect"`
		Execute *struct {
			Operation string                   `json:"operation"`
			Arguments []waveTransitionArgument `json:"arguments"`
		} `json:"execute"`
	} `json:"next_transition"`
}

type waveSubmissionDescriptor struct {
	OperationToken string                `json:"operation_token"`
	ArgumentTokens []string              `json:"argument_tokens"`
	Value          *waveSubmissionValue  `json:"value,omitempty"`
	Values         []waveSubmissionValue `json:"values,omitempty"`
}

type waveSubmissionValue struct {
	Slot                 string   `json:"slot"`
	Domain               string   `json:"domain"`
	Schema               string   `json:"schema,omitempty"`
	AllowedValues        []string `json:"allowed_values,omitempty"`
	SubstitutionLocation int      `json:"substitution_location"`
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
	Reason   string `json:"reason"`
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

// waveReviewInvocationArgs is printedCommandArguments narrowed to the `review`
// family. It delegates rather than re-splitting on whitespace: an emitted
// invocation can carry a single-quoted value holding spaces or newlines (a
// recovery authorization is six LF-joined lines), and strings.Fields shatters
// exactly those into stray positional arguments the product then refuses.
func waveReviewInvocationArgs(invocation string) ([]string, error) {
	args, err := printedCommandArguments(invocation)
	if err != nil {
		return nil, fmt.Errorf("invalid emitted review invocation: %w", err)
	}
	if len(args) < 2 || args[0] != "review" {
		return nil, fmt.Errorf("invalid emitted review invocation %q", invocation)
	}
	return args, nil
}

// requireCandidateDeclineGateDeniesGenerically is Wave 5 Slice 6's
// downgrade (design decision 6): a declined candidate no longer resolves
// to a decline-specific unmanaged delivery at the gate at all -- nothing is
// ever recorded (RecordCandidateDecline is deleted), so a later gate call
// reaches the SAME generic denial any never-reviewed candidate reaches.
// Supersedes requireCandidateDeclinedGate (deleted along with j50), which
// asserted the OLD decline-specific unmanaged shape
// (Delivery: "candidate_declined/unmanaged", Denial.Stage:
// "candidate-decline") this function's own name would now contradict.
func requireCandidateDeclineGateDeniesGenerically(_ *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &gate); err != nil {
		return fmt.Errorf("parse candidate-declined gate denial: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if observation.ExitCode == 0 || gate.Allowed || gate.Result != "invalidated" || gate.Delivery != "" {
		return fmt.Errorf("candidate-declined gate = exit=%d gate=%+v, want a plain (non-unmanaged) denial", observation.ExitCode, gate)
	}
	if gate.Context.Denial == nil || gate.Context.Denial.Stage != "receipt-discovery" || gate.Context.Denial.Code != "receipt_missing" {
		return fmt.Errorf("candidate-declined gate denial = %+v, want the generic receipt-discovery/receipt_missing denial any never-reviewed candidate reaches", gate.Context.Denial)
	}
	return nil
}

// requireDisabledUnmanagedGate asserts the kill-switch-off gate shape:
// exits 0, reports Delivery "disabled/unmanaged", never an allow. Mirrors
// internal/cli's assertDisabledUnmanagedGate at the bench black-box layer.
func requireDisabledUnmanagedGate(_ *Sandbox, observation Observation) error {
	var gate waveGateResult
	if err := decodeWaveObservation(observation, &gate, "disabled-unmanaged gate"); err != nil {
		return err
	}
	if gate.Allowed || gate.Result == "allow" || gate.Delivery != "disabled/unmanaged" {
		return fmt.Errorf("disabled-unmanaged gate = %+v, want Delivery disabled/unmanaged, never an allow", gate)
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
	recovered, err := runPrintedTransition(r, envelope)
	if err != nil {
		return err
	}
	result, err := decodeWaveOperation(recovered, "staged correction recovery")
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
	if status, terminal, err := correctionStatusFromLastEventCapture(r, observation); err != nil {
		return err
	} else if terminal {
		return rememberCorrectionStatusContinuation(r, status.Authority.LineageID, status)
	}
	last, err := captureAllLensesWithLastCaptureFor(r, selectors...)
	if err != nil {
		return err
	}
	if status, terminal, err := correctionStatusFromLastEventCapture(r, last); err != nil {
		return err
	} else if !terminal {
		return errors.New("last reviewer capture did not open correction-required status continuation")
	} else {
		return rememberCorrectionStatusContinuation(r, status.Authority.LineageID, status)
	}
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
	if lineage != "" {
		if payload, found, err := readCorrectionPlanStatusContinuation(r, lineage); err != nil {
			return waveCorrectionStatus{}, err
		} else if found {
			var status waveCorrectionStatus
			if err := json.Unmarshal([]byte(payload), &status); err != nil {
				return waveCorrectionStatus{}, fmt.Errorf("decode carried correction-plan STATUS: %w", err)
			}
			return status, nil
		}
	}
	// These journeys create their authority through the manual compatibility
	// path, so they must not invent a runtime identity for a later STATUS read.
	arguments := []string{"review", "status", "--contract", contract, "--next-transition"}
	if lineage != "" {
		arguments = append(arguments, "--lineage", lineage)
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
	if descriptor == nil || descriptor.OperationToken != "finalize" || descriptor.Value == nil || descriptor.Value.Slot != slot ||
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
	return capturePassedCorrectionEvidenceForContract(r, lineage, reviewContract)
}

func capturePassedCorrectionEvidenceForContract(r *journeyRun, lineage, contract string) error {
	status, err := readCorrectionStatusForContract(r, lineage, contract)
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

func completeBurnedCorrectedReview(r *journeyRun) error {
	if err := completeCorrectedReview(r); err != nil {
		return err
	}
	return requireAtomicLineageAcknowledged(r, correctedDeliveryLineage)
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
	arguments := []string{"review", "finalize"}
	if lineage != "" {
		arguments = append(arguments, "--lineage", lineage)
	}
	arguments = append(arguments, "--validation", path, "--captured-evidence=true")
	observation := r.run(productArgsFor(r, arguments...), false)
	result, err := decodeWaveOperation(observation, "corrected review finalize")
	if err != nil {
		return err
	}
	if result.State != "approved" || lineage != "" && result.LineageID != lineage {
		return fmt.Errorf("corrected review finalized as %+v", result)
	}
	return nil
}

func startCommittedCorrection(r *journeyRun) error {
	status, err := readStatusFor(r, "--lineage", committedCorrectionLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"])
	if err != nil || status.NextTransition.Kind != "execute" {
		return fmt.Errorf("committed correction start = %+v, %v", status.NextTransition, err)
	}
	result, err := runPrintedTransition(r, status)
	if err != nil {
		return err
	}
	operation, err := decodeWaveOperation(result, "committed correction start")
	if err != nil || operation.LineageID != committedCorrectionLineage || operation.State != "reviewing" {
		return fmt.Errorf("committed correction start result = %+v, %v", operation, err)
	}
	return nil
}

func commitCorrectedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\n\nfunc value() int {\n\treturn 2\n}\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "fix: correct reviewed candidate")
}

func commitSelectorlessCorrectionCandidate(sandbox *Sandbox) error {
	base, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	sandbox.Scratch["staged-recovery-base"] = base
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\nfunc value() int {\n\treturn 1\n}\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "feat: add selectorless candidate")
}

func commitSelectorlessCorrectedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), "package candidate\nfunc value() int {\n\treturn 2\n}\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "candidate.go"); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "commit", "-qm", "fix: correct selectorless candidate")
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

func requireFreshNegotiatedStart(_ *Sandbox, observation Observation) error {
	var status struct {
		Applicability  string           `json:"applicability"`
		Action         string           `json:"action"`
		Authority      *json.RawMessage `json:"authority"`
		NextTransition *struct {
			Kind    string `json:"kind"`
			Execute *struct {
				Operation string `json:"operation"`
			} `json:"execute"`
		} `json:"next_transition"`
	}
	if err := decodeWaveObservation(observation, &status, "fresh negotiated review status"); err != nil {
		return err
	}
	if status.Applicability != "unrelated" || status.Action != "start" || status.NextTransition == nil ||
		status.NextTransition.Kind != "execute" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("fresh negotiated status did not offer review.start: %+v", status)
	}
	// A fresh target has no authority. Publishing one here would mean status
	// bound the candidate to unrelated review history, so the no-authority
	// invariant is asserted rather than assumed by the start offer alone.
	if status.Authority != nil {
		return fmt.Errorf("fresh negotiated status published an authority: %s", string(*status.Authority))
	}
	return nil
}

// requireDisabledUnmanagedArchiveStatus asserts the kill-switch-off shape at
// sdd-status: the optional offer is structurally absent and archive remains
// governed only by tasks and independent verification.
func requireDisabledUnmanagedArchiveStatus(name string) func(*Sandbox, Observation) error {
	return sddStatusAssertion(name, func(status sddStatusV2) error {
		if status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || len(status.BlockedReasons) != 0 {
			return fmt.Errorf("archive=%q next=%q blocked=%v, want ready/archive with no blockers",
				status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
		}
		if status.ReviewOffer != nil {
			return fmt.Errorf("disabled status reviewOffer=%+v, want structural absence", status.ReviewOffer)
		}
		return nil
	})
}

// snapshotJ47StatusInput records the repository state that the public status
// read must preserve. The completed SDD fixture commits its own artifacts, so a
// changed head or worktree here would be a side effect of status itself.
func snapshotJ47StatusInput(sandbox *Sandbox) error {
	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read status input head: %w", err)
	}
	worktree, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain=v1")
	if err != nil {
		return fmt.Errorf("read status input worktree: %w", err)
	}
	sandbox.Scratch["j47-status-head"] = head
	sandbox.Scratch["j47-status-worktree"] = worktree
	return nil
}

// requireJ47DisabledV2ArchiveStatus pins #3564's public V2 projection: completed
// SDD work proceeds to archive under the clone-local disabled mode without any
// retired review data, and status does not alter its input repository.
func requireJ47DisabledV2ArchiveStatus(sandbox *Sandbox, observation Observation) error {
	if observation.ExitCode != 0 {
		return fmt.Errorf("disabled V2 sdd-status exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &document); err != nil {
		return fmt.Errorf("parse disabled V2 sdd-status: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	var schemaVersion int
	if raw, ok := document["schemaVersion"]; !ok || json.Unmarshal(raw, &schemaVersion) != nil || schemaVersion != 2 {
		return fmt.Errorf("schemaVersion = %s, want 2", document["schemaVersion"])
	}
	for _, retired := range []string{"reviewOffer", "reviewGate", "reviewTransaction", "runtimeStatus", "reVerify", "receipt", "lineage"} {
		if _, present := document[retired]; present {
			return fmt.Errorf("disabled V2 sdd-status exposed retired key %q", retired)
		}
	}

	var status sddStatusV2
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return fmt.Errorf("decode disabled V2 status: %w", err)
	}
	if !status.TaskProgress.AllComplete || status.TaskProgress.Total == 0 || status.Dependencies.Verify != "all_done" ||
		status.Dependencies.Archive != "ready" || status.NextRecommended != "archive" || len(status.BlockedReasons) != 0 {
		return fmt.Errorf("disabled V2 status = tasks %d/%d complete=%v verify=%q archive=%q next=%q blocked=%v, want completed/all_done/ready/archive/no blockers",
			status.TaskProgress.Completed, status.TaskProgress.Total, status.TaskProgress.AllComplete,
			status.Dependencies.Verify, status.Dependencies.Archive, status.NextRecommended, status.BlockedReasons)
	}

	head, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("read status output head: %w", err)
	}
	worktree, err := gitOut(sandbox, sandbox.Repo, "status", "--porcelain=v1")
	if err != nil {
		return fmt.Errorf("read status output worktree: %w", err)
	}
	if head != sandbox.Scratch["j47-status-head"] || worktree != sandbox.Scratch["j47-status-worktree"] {
		return fmt.Errorf("disabled V2 sdd-status mutated its repository input: head %q/%q worktree %q/%q",
			sandbox.Scratch["j47-status-head"], head, sandbox.Scratch["j47-status-worktree"], worktree)
	}
	return nil
}

func prepareDeclinedCandidate(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, declineCandidatePath), declineCandidateContents); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", declineCandidatePath); err != nil {
		return err
	}
	paths, err := gitOut(sandbox, sandbox.Repo, "diff", "--cached", "--name-only")
	if err != nil || paths != declineCandidatePath {
		return fmt.Errorf("decline fixture declared paths = %q, %v", paths, err)
	}
	return nil
}

func declineCandidateFromStatus(r *journeyRun) error {
	statusObservation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2,
		"--next-transition", "--lineage", declineCandidateLineage), false)
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

func proveDeclinedCandidateStaged(sandbox *Sandbox) error {
	tree, err := gitOut(sandbox, sandbox.Repo, "write-tree")
	if err != nil || tree != sandbox.Scratch["decline-tree"] {
		return fmt.Errorf("staged decline tree = %q, want %q: %v", tree, sandbox.Scratch["decline-tree"], err)
	}
	return nil
}

func waveOneJourneys() []Journey {
	return []Journey{
		{
			ID:     "j44-corrected-current-changes-delivery",
			Review: reviewOptedIn,
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
					Args: productArgs("review", "status", "--contract", reviewContractV2, "--lineage", correctedDeliveryLineage), After: proveCorrectedReceipt},
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
			Review: reviewOptedIn,
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
			Review: reviewOptedIn,
			Title:  "Correction-required base diff: negotiated staged recovery receives a fresh review and delivers",
			Source: "issue #1921",
			Steps: []Step{
				{Name: "fixture: linked worktree and remote", Fixture: linkedWorktreeWithRemote},
				{Name: "fixture: commit base-diff candidate", Fixture: commitStagedRecoveryCandidate},
				// Issue #2447: a direct (non-negotiated) `review start` over a
				// base-diff candidate that selects a lens now refuses up
				// front, so this step goes through the negotiated form
				// instead. `--lineage` on `review status` pins the exact
				// derived lineage name into the printed execute transition,
				// so stagedRecoveryLineage still names the review every
				// later step in this journey references.
				{Name: "start workspace-projected base-diff review", Requires: statusCapability, Composite: func(r *journeyRun) error {
					envelope, err := readStatusFor(r, "--lineage", stagedRecoveryLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"])
					if err != nil {
						return err
					}
					if envelope.NextTransition.Kind != "execute" {
						return fmt.Errorf("expected an execute review.start transition for the staged recovery base-diff candidate, got %q", envelope.NextTransition.Kind)
					}
					started, err := runPrintedTransition(r, envelope)
					if err != nil {
						return err
					}
					if started.ExitCode != 0 {
						return fmt.Errorf("negotiated staged base-diff start failed: exit=%d stderr=%s", started.ExitCode, started.Stderr)
					}
					return nil
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
			ID:     "j47-disabled-mode-archives-discovered-scope-changed-authority",
			Review: reviewUntouched,
			Title:  "Disabled clone mode: explicit-CWD SDD status v2 reports archive readiness",
			Source: "issue #3564 V2: completed SDD tasks and verification route directly to archive",
			Steps: []Step{
				{Name: "fixture: completed SDD change with passing verification", Fixture: sddPlanningArtifacts(sddVerifyReport)},
				{Name: "disable review mode for the clone", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone", "--json")},
				{Name: "fixture: capture status input state", Fixture: snapshotJ47StatusInput},
				{Name: "explicit-CWD disabled V2 status is archive-ready and read-only", Requires: sddStatusCapability,
					Args: productArgs("sdd-status", sddChange, "--contract", "gentle-ai.sdd-status/v2", "--json"), After: requireJ47DisabledV2ArchiveStatus},
			},
		},
		{
			ID:     "j48-recovered-workspace-preserves-full-candidate-scope",
			Review: reviewOptedIn,
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
			Review: reviewOptedIn,
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
			// j50-candidate-decline-preserves-frozen-delivery-identity
			// (issue #2045) is RENAMED and its Steps rewritten, not deleted
			// (Wave 5 Slice 6, design decision 6): decline no longer
			// preserves ANY frozen delivery identity at the gate --
			// RecordCandidateDecline is deleted, so nothing is left to
			// compare a later gate call against. The exact-candidate and
			// drifted-candidate scenarios that USED to diverge (one
			// unmanaged, the others rejected) now converge on the SAME
			// generic denial, so the drift-specific fixtures
			// (driftDeclinedCandidate, addDeclinedPathDrift,
			// requireCandidateDeclineRejected) added no remaining
			// differentiating value and are deleted along with them.
			// prepareDeclinedCandidate, declineCandidateFromStatus, and
			// proveDeclinedCandidateStaged survive unchanged: their own
			// assertions (empty post-decline authority inventory, staged
			// tree matches the frozen target) were never gate-side and
			// stay exactly as true as before.
			ID:     "j50-candidate-decline-denies-generically-then-disabled",
			Review: reviewOptedIn,
			Title:  "Candidate decline creates no review authority; a later gate denies generically, or reaches ordinary unmanaged delivery once reviews are disabled",
			Source: "issue #2045 (Wave 5 Slice 6 downgrade)",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: high-risk candidate declared in the index", Fixture: prepareDeclinedCandidate},
				{Name: "derive v2 START and execute emitted relay and decline", Requires: statusCapability, Composite: declineCandidateFromStatus},
				{Name: "fixture: exact declared candidate remains unchanged", Fixture: proveDeclinedCandidateStaged},
				{Name: "reviews still on: the declined candidate denies exactly like any never-reviewed candidate", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: requireCandidateDeclineGateDeniesGenerically},
				{Name: "disable reviews for the clone", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone", "--json")},
				{Name: "reviews off: the identical declined candidate reaches ordinary unmanaged delivery", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "pre-commit"), After: requireDisabledUnmanagedGate},
			},
		},
		{
			ID:     "j51-negotiated-status-correction-continuation",
			Review: reviewOptedIn,
			Title:  "#3587: selectorless STATUS starts only a fresh candidate; correction continues through its exact active lineage",
			Source: "issue #2044 under #3587: selectorless STATUS is fresh by design, while every active correction continuation carries its exact lineage",
			Steps: []Step{
				{Name: "fixture: repo", Fixture: baseRepo},
				{Name: "fixture: one exact code candidate proven staged", Fixture: stageWaveCandidate},
				{Name: "fixture: product process temp is unavailable", Fixture: unavailableProcessTemp},
				{Name: "fresh negotiated status offers review start without authority history", Requires: statusCapability,
					Args: productArgs("review", "status", "--contract", reviewContract, "--next-transition"), After: requireFreshNegotiatedStart},
				{Name: "review start with an exact active lineage", Requires: startNamedCapability,
					Args: productArgs("review", "start", "--lineage", correctedDeliveryLineage), After: rememberLineage},
				{Name: "capture one blocking finding and finish the full selected lens set for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureExactSelectedReviewerSlots(r, correctedDeliveryLineage, true)
				}},
				{Name: "capture the bounded correction plan from the exact STATUS binding", Requires: captureCorrectionPlanCapability, Composite: func(r *journeyRun) error {
					return captureCorrectionPlanFor(r, correctedDeliveryLineage, 2)
				}},
				{Name: "fixture: corrected candidate proven to change only the reviewed path", Fixture: writeCorrectedCandidate},
				{Name: "post-correction exact active-lineage validator capture emits acknowledgement on completion", Requires: capturedProviderValidatorStatusCapability, Composite: func(r *journeyRun) error {
					return captureProviderValidatorSlotFor(r, correctedDeliveryLineage)
				}},
				{Name: "no correction authority survives exact acknowledgement", Requires: statusCapability, Composite: func(r *journeyRun) error {
					return requireAtomicLineageAcknowledged(r, correctedDeliveryLineage)
				}},
			},
		},
		{
			ID:     "j65-selectorless-committed-correction-continuation",
			Review: reviewOptedIn,
			Title:  "Committed-only correction: selector-less status and finalize rebuild the frozen base boundary",
			Source: "issue #1925",
			Steps: []Step{
				{Name: "fixture: repository", Fixture: baseRepo},
				{Name: "fixture: committed four-line base-diff candidate", Fixture: commitSelectorlessCorrectionCandidate},
				{Name: "start committed-only review", Requires: statusCapability, Composite: startCommittedCorrection},
				{Name: "capture one blocking finding and finish the lens set", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
					return captureCorrectableFindingFor(r, "--lineage", committedCorrectionLineage, "--base-ref", r.sandbox.Scratch["staged-recovery-base"])
				}},
				{Name: "enter correction-required", Requires: finalizeResultsCapability, Args: productArgs("review", "finalize", "--lineage", committedCorrectionLineage, "--captured-results=true")},
				{Name: "forecast the bounded correction", Requires: finalizeCorrectionCapability, Args: productArgs("review", "finalize", "--lineage", committedCorrectionLineage, "--correction-lines", "2")},
				{Name: "fixture: commit the two-line in-budget correction", Fixture: commitSelectorlessCorrectedCandidate},
				{Name: "selector-less status requests and captures repository evidence", Requires: captureOutcomeEvidenceCapability, Composite: func(r *journeyRun) error { return capturePassedCorrectionEvidenceFor(r, "") }},
				{Name: "selector-less status submits targeted validation and mints receipt", Requires: finalizeValidationCapability, Composite: func(r *journeyRun) error { return completeCorrectedReviewFor(r, "") }},
			},
		},
	}
}

// j51-unrelated-noop-authority-keeps-composed-delivery (issue #2125) is
// DELETED, not superseded (Wave 5 Slice 5, pre-PR chain composition
// deletion): its regression was a cycle-detection bug inside
// EvaluateCompactPrePRChain's own composition graph -- an unrelated clean
// no-op authority's self-loop receipt edge tripped cycle detection and
// wrongly denied composition for every unrelated lineage. That composition
// graph, and everything that could enter a cycle in it, no longer exists
// (TestPrePRComposition_ZeroCallers, internal/cli, proves it by
// call-absence); there is no analogous "after" behavior to pin. The
// replacement bench evidence for this slice -- a multi-segment delivery
// that composition used to rescue now denying, with a runnable next step
// -- lives in bench/journeys_wave5.go
// (j61-pre-pr-multi-segment-delivery-denies-without-composition), per task
// 6.7.
