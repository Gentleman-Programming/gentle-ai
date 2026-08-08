package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue2539ApprovedLineage = "issue2539:approved-lineage"

func issue2539Journeys() []Journey {
	return []Journey{{
		ID:     "j94-retired-reviewer-schema-abandon",
		Title:  "Retired reviewer-result bytes remain dispositionable without becoming admitted evidence",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/2539",
		Steps: []Step{
			{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
			{Name: "fixture: stage unrelated docs", Fixture: stageDocs("unrelated-approved")},
			{Name: "review unrelated authority", Requires: startCapability, Args: productArgs("review", "start"), After: rememberIssue2539Approved},
			{Name: "approve unrelated authority", Requires: finalizeCapability, Args: productArgs("review", "finalize")},
			{Name: "fixture: commit approved docs", Fixture: commitStaged("docs: approved authority")},
			{Name: "fixture: stage retired review target", Fixture: stageOrdinaryCode},
			{Name: "review retired-schema target", Requires: startCapability, Args: productArgs("review", "start"), After: rememberLineage},
			{Name: "dispose retired-schema authority", Requires: abandonCapability, Composite: exerciseIssue2539RetiredAbandon},
		},
	}}
}

func rememberIssue2539Approved(sandbox *Sandbox, observation Observation) error {
	if err := rememberLineage(sandbox, observation); err != nil {
		return err
	}
	sandbox.Scratch[issue2539ApprovedLineage] = sandbox.Lineage
	return nil
}

func exerciseIssue2539RetiredAbandon(r *journeyRun) error {
	const artifactName = "00-review-reliability.json"
	payload := []byte(`{"lens":"review-reliability","findings":[],"evidence":["inspected stale.go"]}`)
	authorityRoot, err := issue2539AuthorityRoot(r.sandbox)
	if err != nil {
		return err
	}
	store := filepath.Join(authorityRoot, "v2", r.sandbox.Lineage)
	resultDir := filepath.Join(store, "reviewer-results")
	if err := os.MkdirAll(resultDir, 0o700); err != nil {
		return err
	}
	artifact := filepath.Join(resultDir, artifactName)
	digest := sha256.Sum256(payload)
	sidecar := []byte("sha256:" + hex.EncodeToString(digest[:]) + "\n")
	if err := os.WriteFile(artifact, payload, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(artifact+".sha256", sidecar, 0o600); err != nil {
		return err
	}

	status := r.run([]string{"review", "status", "--cwd", r.sandbox.Repo}, false)
	if status.ExitCode != 0 {
		return fmt.Errorf("status rejected retired bytes: %s", firstLine(status.Stderr))
	}
	var inventory authorityHead
	if err := json.Unmarshal([]byte(strings.TrimSpace(status.Stdout)), &inventory); err != nil {
		return fmt.Errorf("decode status: %w", err)
	}
	entry, found := inventory.entry(r.sandbox.Lineage)
	if !found || len(entry.DiscardedWork.CapturedLensResults) != 1 ||
		entry.DiscardedWork.CapturedLensResults[0] != artifactName ||
		entry.DiscardedWork.FindingsPresent || entry.DiscardedWork.EvidenceRecordsPresent {
		return fmt.Errorf("retired discarded_work = %#v; status problems: %s", entry.DiscardedWork, strings.Join(entry.Problems, "; "))
	}
	const actor, reason = "bench", "retired_schema"
	authorization := renderAbandonAuthorization(entry, actor, reason)
	stale := r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo, "--lineage", entry.LineageID,
		"--expected-revision", "sha256:" + strings.Repeat("0", 64), "--reason", reason,
		"--actor", actor, "--maintainer-authorization", authorization,
	}, false)
	if stale.ExitCode == 0 {
		return errors.New("stale abandonment binding was accepted")
	}

	abandoned := r.run([]string{
		"review", "abandon", "--cwd", r.sandbox.Repo, "--lineage", entry.LineageID,
		"--expected-revision", entry.Revision, "--reason", reason,
		"--actor", actor, "--maintainer-authorization", authorization,
	}, false)
	if abandoned.ExitCode != 0 {
		return fmt.Errorf("abandon retired bytes: %s", firstLine(abandoned.Stderr))
	}
	var result struct {
		Record struct {
			Status         string `json:"status"`
			QuarantinePath string `json:"quarantine_path"`
		} `json:"record"`
	}
	if err := json.Unmarshal([]byte(abandoned.Stdout), &result); err != nil || result.Record.Status != "committed" {
		return fmt.Errorf("decode committed abandonment: %#v, %v", result, err)
	}
	quarantined := filepath.Join(result.Record.QuarantinePath, "residue", "reviewer-results", artifactName)
	moved, readErr := os.ReadFile(quarantined)
	movedSidecar, sidecarErr := os.ReadFile(quarantined + ".sha256")
	if readErr != nil || sidecarErr != nil || !bytes.Equal(moved, payload) || !bytes.Equal(movedSidecar, sidecar) {
		return fmt.Errorf("quarantine did not preserve retired bytes whole: artifact=%v sidecar=%v", readErr, sidecarErr)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		return fmt.Errorf("abandoned authority remains live: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Record.QuarantinePath, "residue", "review-receipt.json")); !os.IsNotExist(err) {
		return fmt.Errorf("discarded authority retained a usable receipt: %v", err)
	}
	if unusable := r.run([]string{"review", "validate", "--cwd", r.sandbox.Repo, "--lineage", entry.LineageID, "--gate", "post-apply"}, false); unusable.ExitCode == 0 {
		return errors.New("discarded authority remained usable")
	}
	approved := r.run([]string{"review", "validate", "--cwd", r.sandbox.Repo,
		"--lineage", r.sandbox.Scratch[issue2539ApprovedLineage], "--gate", "pre-push"}, false)
	if approved.ExitCode != 0 {
		return fmt.Errorf("unrelated approved authority became unusable: %s", firstLine(approved.Stderr))
	}
	return nil
}

func issue2539AuthorityRoot(sandbox *Sandbox) (string, error) {
	common, err := gitOut(sandbox, sandbox.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(strings.TrimSpace(common), "gentle-ai", "review-transactions"), nil
}
