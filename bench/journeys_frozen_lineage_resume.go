package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	frozenLineageTracked   = "internal/frozen.go"
	frozenLineageUntracked = "docs/frozen-input.txt"
	frozenLineageDrift     = "docs/live-drift.txt"
	frozenLineageSource    = "package frozen\n\nfunc Value() int { return 1 }\n"
	frozenLineageChanged   = "package frozen\n\nfunc Value() int { return 2 }\n"
)

var frozenLineageStatusCapability = &Capability{Verb: []string{"review", "status"}, Flags: []string{
	"--cwd", "--contract", "--agent", "--lineage", "--next-transition",
	"--untracked-scope", "--expected-untracked-inventory", "--intended-untracked",
}}

func frozenLineageStatus(r *journeyRun, lineage string, selectors ...string) (statusEnvelope, map[string]json.RawMessage, error) {
	args := []string{"review", "status", "--cwd", r.sandbox.Repo, "--contract", reviewContractV2, "--agent", "opencode", "--next-transition"}
	if lineage != "" {
		args = append(args, "--lineage", lineage)
	}
	args = append(args, selectors...)
	observation := r.run(args, false)
	if observation.ExitCode != 0 {
		return statusEnvelope{}, nil, fmt.Errorf("STATUS exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	var status statusEnvelope
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return status, nil, fmt.Errorf("parse STATUS: %w", err)
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &document); err != nil {
		return status, nil, fmt.Errorf("parse complete STATUS: %w", err)
	}
	return status, document, nil
}
func frozenLineageSelectedStatus(r *journeyRun, lineage string, intended ...string) (statusEnvelope, map[string]json.RawMessage, error) {
	selection, _, err := frozenLineageStatus(r, lineage)
	if err != nil {
		return statusEnvelope{}, nil, err
	}
	if selection.NextTransition.Kind != "collect" || selection.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		return statusEnvelope{}, nil, fmt.Errorf("STATUS did not collect intended untracked selection: %+v", selection.NextTransition)
	}
	selectors := []string{"--untracked-scope=select", "--expected-untracked-inventory=" + selection.argument("expected_untracked_inventory")}
	for _, path := range intended {
		selectors = append(selectors, "--intended-untracked="+path)
	}
	return frozenLineageStatus(r, lineage, selectors...)
}
func frozenAuthorityInventory(r *journeyRun) ([]byte, int, error) {
	common, err := gitOut(r.sandbox, r.sandbox.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, 0, err
	}
	root := filepath.Join(strings.TrimSpace(common), "gentle-ai", "review-transactions", "v2")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, 0, err
	}
	var snapshot bytes.Buffer
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("authority inventory contains symlink %q", path)
		}
		if entry.IsDir() || entry.Name() == "LOCK" {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&snapshot, "%s\n%d\n", relative, len(payload))
		snapshot.Write(payload)
		return nil
	})
	return snapshot.Bytes(), len(entries), err
}
func requireFrozenReviewerBinding(status statusEnvelope, document map[string]json.RawMessage, lineage string) error {
	if status.Authority.LineageID != lineage || status.Authority.State != "reviewing" || status.Authority.Revision == "" || status.TargetIdentity == "" ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "reviewer_results_required" || len(status.NextTransition.Collect.Inputs) != 1 {
		return fmt.Errorf("frozen reviewer STATUS = authority=%+v target=%q transition=%+v", status.Authority, status.TargetIdentity, status.NextTransition)
	}
	input := status.NextTransition.Collect.Inputs[0]
	if input.Name != "reviewer_result" || input.CaptureOperation != "review.capture-result" || input.ArtifactSubject.SubjectHash == "" ||
		len(input.ChangedPathManifest) != 2 || status.argument("lineage") != lineage || status.argument("target") != status.TargetIdentity ||
		status.argument("expected-revision") != status.Authority.Revision || status.argument("lens") == "" || status.argument("order") != "0" || status.argument("repository-context") == "" {
		return fmt.Errorf("frozen reviewer collect binding = %+v", input)
	}
	for _, field := range []string{"authority", "frozen", "target_identity", "projection", "next_transition"} {
		if len(document[field]) == 0 {
			return fmt.Errorf("frozen reviewer STATUS omitted %s", field)
		}
	}
	var frozen struct {
		Tier                 string `json:"tier"`
		OriginalChangedLines int    `json:"original_changed_lines"`
		CorrectionBudget     int    `json:"correction_budget"`
	}
	var projection struct {
		BaseTree               string   `json:"base_tree"`
		InitialReviewTree      string   `json:"initial_review_tree"`
		CurrentCandidateTree   string   `json:"current_candidate_tree"`
		Paths                  []string `json:"paths"`
		IntendedUntracked      []string `json:"intended_untracked"`
		IntendedUntrackedProof string   `json:"intended_untracked_proof"`
	}
	if json.Unmarshal(document["frozen"], &frozen) != nil || json.Unmarshal(document["projection"], &projection) != nil ||
		frozen.Tier != "medium" || frozen.OriginalChangedLines <= 0 || frozen.CorrectionBudget <= 0 ||
		projection.BaseTree == "" || projection.InitialReviewTree == "" || projection.CurrentCandidateTree == "" ||
		projection.IntendedUntrackedProof == "" || len(projection.Paths) != 2 || len(projection.IntendedUntracked) != 1 ||
		projection.IntendedUntracked[0] != frozenLineageUntracked {
		return fmt.Errorf("frozen authority/projection binding = frozen=%+v projection=%+v", frozen, projection)
	}
	return nil
}
func requireSameFrozenBinding(before, after map[string]json.RawMessage) error {
	for _, field := range []string{"authority", "frozen", "target_identity", "projection", "next_transition"} {
		if !bytes.Equal(before[field], after[field]) {
			return fmt.Errorf("live drift changed frozen %s binding", field)
		}
	}
	return nil
}
func captureFrozenReviewerResult(r *journeyRun, status statusEnvelope) error {
	input := status.NextTransition.Collect.Inputs[0]
	payload, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, status.paths())
	if err != nil {
		return err
	}
	path, err := writeScratch(r.sandbox, "frozen-reviewer.json", payload)
	if err != nil {
		return err
	}
	observation := r.run([]string{"review", "capture-result", "--cwd", r.sandbox.Repo,
		"--lineage", status.argument("lineage"), "--target", status.argument("target"),
		"--expected-revision", status.argument("expected-revision"), "--lens", status.argument("lens"),
		"--order", status.argument("order"), "--input", path}, true)
	if observation.ExitCode != 0 {
		return fmt.Errorf("capture frozen result exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	return nil
}
func startFrozenLineageWithConsent(r *journeyRun, status statusEnvelope) error {
	args, err := printedCommandArguments(status.NextTransition.Execute.Command)
	if err != nil {
		return err
	}
	granted := false
	for index, argument := range args {
		if argument == "--consent=relay" {
			args[index], granted = "--consent=granted", true
		}
	}
	if !granted {
		return fmt.Errorf("printed START did not request consent: %q", status.NextTransition.Execute.Command)
	}
	observation := r.run(args, false)
	if observation.ExitCode != 0 || rememberLineage(r.sandbox, observation) != nil || r.sandbox.Lineage == "" {
		return fmt.Errorf("granted printed START = exit %d lineage %q: %s", observation.ExitCode, r.sandbox.Lineage, firstLine(observation.Stderr))
	}
	return nil
}
func exerciseFrozenLineageResume(r *journeyRun) error {
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageTracked), frozenLineageSource); err != nil {
		return err
	}
	if err := r.sandbox.git(r.sandbox.Repo, "add", "--", frozenLineageTracked); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageUntracked), "frozen\n"); err != nil {
		return err
	}
	initial, _, err := frozenLineageStatus(r, "")
	if err != nil || initial.NextTransition.Kind != "collect" || initial.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		return fmt.Errorf("initial selectorless STATUS = %+v, %v", initial.NextTransition, err)
	}
	start, _, err := frozenLineageSelectedStatus(r, "", frozenLineageUntracked)
	if err != nil || start.NextTransition.Kind != "execute" || start.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("selected v2 OpenCode START = %+v, %v", start.NextTransition, err)
	}
	if err := startFrozenLineageWithConsent(r, start); err != nil {
		return err
	}
	before, beforeDocument, err := frozenLineageStatus(r, r.sandbox.Lineage)
	if err != nil {
		return err
	}
	if err := requireFrozenReviewerBinding(before, beforeDocument, r.sandbox.Lineage); err != nil {
		return err
	}
	beforeInventory, beforeCount, err := frozenAuthorityInventory(r)
	if err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageTracked), frozenLineageChanged); err != nil {
		return err
	}
	if err := r.sandbox.git(r.sandbox.Repo, "add", "--", frozenLineageTracked); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageDrift), "live drift\n"); err != nil {
		return err
	}
	selectorless, _, err := frozenLineageStatus(r, "")
	if err != nil || selectorless.NextTransition.Kind != "collect" || selectorless.NextTransition.ReasonCode != "intended_untracked_selection_required" {
		return fmt.Errorf("drifted selectorless STATUS = %+v, %v", selectorless.NextTransition, err)
	}
	resumed, resumedDocument, err := frozenLineageStatus(r, r.sandbox.Lineage)
	if err != nil {
		return err
	}
	if err := requireFrozenReviewerBinding(resumed, resumedDocument, r.sandbox.Lineage); err != nil {
		return err
	}
	if err := requireSameFrozenBinding(beforeDocument, resumedDocument); err != nil {
		return err
	}
	afterInventory, afterCount, err := frozenAuthorityInventory(r)
	if err != nil || afterCount != beforeCount || !bytes.Equal(beforeInventory, afterInventory) {
		return fmt.Errorf("read-only frozen STATUS mutated authority inventory: count %d/%d err=%v", beforeCount, afterCount, err)
	}
	if err := captureFrozenReviewerResult(r, resumed); err != nil {
		return err
	}
	occupied, _, err := frozenLineageStatus(r, r.sandbox.Lineage)
	if err != nil {
		return err
	}
	if occupied.NextTransition.Kind != "stop" || occupied.NextTransition.ReasonCode != "native_stop_required" {
		return fmt.Errorf("drifted occupied frozen status = %+v", occupied.NextTransition)
	}
	// The drift guard must hold end to end: finalizing the drifted candidate
	// directly must refuse instead of publishing a receipt for bytes nobody
	// reviewed.
	if observation := r.run([]string{"review", "finalize", "--cwd", r.sandbox.Repo,
		"--lineage", r.sandbox.Lineage, "--captured-results=true"}, false); observation.ExitCode == 0 {
		return fmt.Errorf("drifted frozen finalize must refuse, got exit 0")
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageTracked), frozenLineageSource); err != nil {
		return err
	}
	if err := r.sandbox.git(r.sandbox.Repo, "add", "--", frozenLineageTracked); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(r.sandbox.Repo, frozenLineageDrift)); err != nil {
		return err
	}
	ready, _, err := frozenLineageStatus(r, r.sandbox.Lineage,
		"--untracked-scope=select", "--expected-untracked-inventory="+initial.argument("expected_untracked_inventory"),
		"--intended-untracked="+frozenLineageUntracked)
	if err != nil || ready.NextTransition.Kind != "execute" || ready.NextTransition.Execute.Operation != "review.finalize" {
		return fmt.Errorf("restored frozen finalize = %+v, %v", ready.NextTransition, err)
	}
	if observation, err := runPrintedTransition(r, ready); err != nil || observation.ExitCode != 0 {
		return fmt.Errorf("finalize captured result = %+v, %v", observation, err)
	}
	evidence, _, err := frozenLineageSelectedStatus(r, r.sandbox.Lineage, frozenLineageUntracked)
	if err != nil || evidence.NextTransition.Kind != "collect" || evidence.NextTransition.ReasonCode != "verification_evidence_required" {
		return fmt.Errorf("frozen verification evidence = %+v, %v", evidence.NextTransition, err)
	}
	evidencePath, err := writeScratch(r.sandbox, "frozen-evidence.txt", []byte("focused verification passed\n"))
	if err != nil {
		return err
	}
	if observation := r.run([]string{"review", "capture-evidence", "--cwd", r.sandbox.Repo,
		"--lineage", evidence.argument("lineage"), "--target", evidence.argument("target"),
		"--expected-revision", evidence.argument("expected-revision"), "--outcome", "passed", "--input", evidencePath}, false); observation.ExitCode != 0 {
		return fmt.Errorf("capture frozen evidence exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
	}
	approved, _, err := frozenLineageSelectedStatus(r, r.sandbox.Lineage, frozenLineageUntracked)
	if err != nil || approved.NextTransition.Kind != "execute" || approved.NextTransition.Execute.Operation != "review.finalize" {
		return fmt.Errorf("frozen approval finalize = %+v, %v", approved.NextTransition, err)
	}
	if observation, err := runPrintedTransition(r, approved); err != nil || observation.ExitCode != 0 {
		return fmt.Errorf("finalize frozen evidence = %+v, %v", observation, err)
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageTracked), frozenLineageChanged); err != nil {
		return err
	}
	if err := r.sandbox.git(r.sandbox.Repo, "add", "--", frozenLineageTracked); err != nil {
		return err
	}
	if err := r.sandbox.write(filepath.Join(r.sandbox.Repo, frozenLineageDrift), "live drift\n"); err != nil {
		return err
	}
	drifted, driftedDocument, err := frozenLineageSelectedStatus(r, r.sandbox.Lineage, frozenLineageUntracked, frozenLineageDrift)
	if err != nil {
		return err
	}
	var receipt struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(driftedDocument["receipt"], &receipt)
	if receipt.Status != "not_applicable" || drifted.NextTransition.Kind != "execute" || drifted.NextTransition.Execute.Operation != "review.start" || drifted.executeArgument("lineage") == r.sandbox.Lineage {
		return fmt.Errorf("drifted receipt applicability = %+v receipt=%q err=%v", drifted.NextTransition, receipt.Status, err)
	}
	delivery := r.run([]string{"review", "validate", "--cwd", r.sandbox.Repo, "--lineage", r.sandbox.Lineage, "--gate", "post-apply"}, false)
	if delivery.ExitCode == 0 {
		return fmt.Errorf("drifted delivery was authorized: %s", strings.TrimSpace(delivery.Stdout))
	}
	return nil
}
func frozenLineageResumeJourneys() []Journey {
	return []Journey{{
		ID:     "j90-explicit-frozen-reviewing-lineage-resumes-after-drift",
		Review: reviewOptedIn,
		Title:  "Explicit frozen reviewing lineage resumes its pending slot after live workspace drift",
		Source: "issue #2016: explicit compact-v2 selection resumes only immutable pending review input, never live workspace drift",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "clear any clone-local review override (a clone may only ever assert off)", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "v2 OpenCode frozen review resumes after tracked and intended-untracked drift", Requires: frozenLineageStatusCapability, Composite: exerciseFrozenLineageResume},
		},
	}}
}
