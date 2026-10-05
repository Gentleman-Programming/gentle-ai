package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const issue2879Lineage = "review-f8708016a901b4ff"
const issue2879Digest = "sha256:7adf81305d031f928d9898c03be98d5e942132bfc98d209ee99f6fe3dd008259"
const issue2879Class = "retired_compact_snapshot_identity"

// The released v2.2.4 asset gentle-ai_2.2.4_linux_amd64.tar.gz (SHA-256 a6de9f21999fad2b22fed87e267bbb222782505b7316457bcb9af5e5a2217809) created these exact issue2879Digest bytes.
//
//go:embed testdata/issue2879/released-v2.2.4-review-state.json
var issue2879ReleasedRecord []byte

func issue2879Journeys() []Journey {
	return []Journey{{
		ID: "j92-released-compact-history-quarantines-without-compatibility", Title: "Released compact history is quarantined through the bound repair plan", Source: "issue #2879 D-2879",
		Review: reviewOptedIn,
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "clear any clone-local review override (a clone may only ever assert off)", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "fixture: stage unrelated current target", Fixture: stageProse("", "issue2879-current")},
			{Name: "fixture: replay exact released v2.2.4 authority bytes", Fixture: issue2879HistoricalFixture},
			{Name: "unrelated malformed entry blocks historical preflight", Requires: repairPreflightCapability, Composite: issue2879Plan},
			{Name: "inspect historical authority offers repair", Requires: inspectAuthorityCapability, Args: productArgs("review", "inspect-authority"), After: inspectionAssertion("historical inspection", requireIssue2879HistoricalInspection)},
			{Name: "preflight binds historical repair", Requires: repairPreflightCapability, Args: productArgs("review", "repair", "--preflight"), After: requireDispositionPlanEligible},
			{Name: "forged and stale repair bindings refuse unchanged", Requires: repairDispositionExecuteCapability, Composite: issue2879Refuse},
			{Name: "authorized repair quarantines exactly once", Requires: repairDispositionExecuteCapability, Composite: issue2879Execute},
			{Name: "unrelated current target remains routable", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", "issue2879-current")},
			{Name: "decoy impossible identity remains ineligible", Requires: repairPreflightCapability, Composite: issue2879Decoy},
		},
	}}
}

func issue2879HistoricalFixture(sandbox *Sandbox) error {
	sum := sha256.Sum256(issue2879ReleasedRecord)
	if "sha256:"+hex.EncodeToString(sum[:]) != issue2879Digest {
		return errors.New("released fixture digest drifted")
	}
	path, err := storeStatePath(sandbox, issue2879Lineage)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, issue2879ReleasedRecord, 0o644)
}

func issue2879Plan(r *journeyRun) error {
	historicalPath, historicalPathErr := storeStatePath(r.sandbox, issue2879Lineage)
	unrelatedPath, err := storeStatePath(r.sandbox, "z-unrelated")
	if historicalPathErr != nil || err != nil || os.MkdirAll(filepath.Dir(unrelatedPath), 0o755) != nil || os.WriteFile(unrelatedPath, []byte("{\n"), 0o644) != nil {
		return errors.New("prepare unrelated malformed authority")
	}
	blockedInspection := r.run(productArgsFor(r, "review", "inspect-authority"), false)
	blockedPreflight := r.run(productArgsFor(r, "review", "repair", "--preflight"), false)
	var blockedReport storeInspection
	var blockedResult dispositionRepairResult
	afterHistorical, historicalErr := os.ReadFile(historicalPath)
	afterMalformed, malformedErr := os.ReadFile(unrelatedPath)
	if blockedInspection.ExitCode != 0 || json.Unmarshal([]byte(blockedInspection.Stdout), &blockedReport) != nil || len(blockedReport.EntryDiagnostics) != 2 ||
		blockedReport.EntryDiagnostics[0].LineageID != issue2879Lineage || blockedReport.EntryDiagnostics[0].Problem != "outdated_compact_state" ||
		blockedReport.EntryDiagnostics[1].LineageID != "z-unrelated" || blockedReport.EntryDiagnostics[1].Problem != "malformed_compact_state" ||
		blockedPreflight.ExitCode != 0 || json.Unmarshal([]byte(blockedPreflight.Stdout), &blockedResult) != nil || blockedResult.DispositionProviderInputs != nil ||
		historicalErr != nil || malformedErr != nil || !bytes.Equal(afterHistorical, issue2879ReleasedRecord) || !bytes.Equal(afterMalformed, []byte("{\n")) {
		return errors.New("unrelated malformed authority produced a historical disposition plan")
	}
	if err := os.RemoveAll(filepath.Dir(unrelatedPath)); err != nil {
		return err
	}
	return nil
}
func requireIssue2879HistoricalInspection(report storeInspection) error {
	if len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].LineageID != issue2879Lineage ||
		report.EntryDiagnostics[0].Problem != "outdated_compact_state" || len(report.SanctionedExits) != 1 ||
		report.SanctionedExits[0].LineageID != issue2879Lineage || report.SanctionedExits[0].Operation != "review repair" {
		return fmt.Errorf("historical inspection = %+v", report)
	}
	return nil
}
func issue2879Args(sandbox *Sandbox, plan, inventory, reason, authorization string) []string {
	return []string{"review", "repair", "--cwd", sandbox.Repo, "--plan-digest", plan, "--inventory-revision", inventory,
		"--actor", "bench", "--reason", reason, "--authorization", authorization}
}
func issue2879Refuse(r *journeyRun) error {
	path, pathErr := storeStatePath(r.sandbox, issue2879Lineage)
	before, readErr := os.ReadFile(path)
	base, baseErr := reviewTransactionsBase(r.sandbox)
	binding, bindErr := dispositionRepositoryBinding(r.sandbox)
	if pathErr != nil || readErr != nil || baseErr != nil || bindErr != nil {
		return errors.New("prepare repair refusals")
	}
	plan, inventory, reason := r.sandbox.Scratch[scratchDispositionPlanDigest], r.sandbox.Scratch[scratchDispositionInventoryRevision], "quarantine released historical record"
	for _, test := range []struct{ inventory, authorizationReason string }{{inventory, "forged authorization"}, {inventory + "0", reason}, {inventory, "stale authorization"}} {
		authorization := dispositionAuthorization(binding, plan, test.inventory, "bench", test.authorizationReason, issue2879Class)
		observation := r.run(issue2879Args(r.sandbox, plan, test.inventory, reason, authorization), false)
		after, readErr := os.ReadFile(path)
		if observation.ExitCode == 0 || readErr != nil || !bytes.Equal(before, after) {
			return fmt.Errorf("refused repair %q changed active authority", test.authorizationReason)
		}
		if _, err := os.Stat(filepath.Join(base, "quarantine")); !os.IsNotExist(err) {
			return fmt.Errorf("refused repair %q created quarantine residue", test.authorizationReason)
		}
	}
	return nil
}

func issue2879Execute(r *journeyRun) error {
	plan, inventory := r.sandbox.Scratch[scratchDispositionPlanDigest], r.sandbox.Scratch[scratchDispositionInventoryRevision]
	binding, err := dispositionRepositoryBinding(r.sandbox)
	if err != nil {
		return err
	}
	reason := "quarantine released historical record"
	args := issue2879Args(r.sandbox, plan, inventory, reason, dispositionAuthorization(binding, plan, inventory, "bench", reason, issue2879Class))
	for attempt := 0; attempt < 2; attempt++ {
		observation := r.run(args, false)
		var result dispositionRepairResult
		if observation.ExitCode != 0 || json.Unmarshal([]byte(observation.Stdout), &result) != nil || result.DispositionExecution == nil || result.DispositionExecution.Status != "committed" {
			return fmt.Errorf("historical repair replay %d = %s", attempt, observation.Stdout)
		}
	}
	path, err := storeStatePath(r.sandbox, issue2879Lineage)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.New("historical authority remains active")
	}
	base, err := reviewTransactionsBase(r.sandbox)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(base, "quarantine"))
	if err != nil || len(entries) != 1 {
		return errors.New("historical quarantine missing")
	}
	residue, err := os.ReadFile(filepath.Join(base, "quarantine", entries[0].Name(), "residue", "review-state.json"))
	if err != nil || !bytes.Equal(residue, issue2879ReleasedRecord) {
		return errors.New("historical quarantine did not preserve original bytes")
	}
	if _, err := os.Stat(filepath.Join(base, "quarantine", entries[0].Name(), "residue", "review-receipt.json")); !os.IsNotExist(err) {
		return errors.New("historical repair created a receipt")
	}
	if status := r.run(productArgsFor(r, "review", "status"), false); status.ExitCode != 0 || strings.Contains(status.Stdout, issue2879Lineage) {
		return fmt.Errorf("status after historical repair = %s", status.Stdout)
	}
	return nil
}
func issue2879Decoy(r *journeyRun) error {
	path, err := storeStatePath(r.sandbox, issue2879Lineage)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil || os.WriteFile(path, issue2879ReleasedRecord, 0o644) != nil {
		return errors.New("write checksum-valid historical decoy")
	}
	record, err := loadStoreRecord(path)
	if err != nil {
		return err
	}
	for _, key := range []string{"initial_snapshot", "current_snapshot"} {
		snapshot, ok := orderedMember(record.state, key)
		if !ok || !setOrderedMember(snapshot, "identity", "sha256:"+strings.Repeat("0", 64)) {
			return errors.New("cannot forge impossible snapshot identity")
		}
	}
	if _, err := record.save(); err != nil {
		return err
	}
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	inspection := r.run(productArgsFor(r, "review", "inspect-authority"), false)
	var report storeInspection
	preflight := r.run(productArgsFor(r, "review", "repair", "--preflight"), false)
	var result dispositionRepairResult
	after, readErr := os.ReadFile(path)
	if inspection.ExitCode != 0 || json.Unmarshal([]byte(inspection.Stdout), &report) != nil || len(report.EntryDiagnostics) != 1 || report.EntryDiagnostics[0].LineageID != issue2879Lineage || report.EntryDiagnostics[0].Problem != "malformed_compact_state" || len(report.SanctionedExits) != 0 ||
		preflight.ExitCode != 0 || json.Unmarshal([]byte(preflight.Stdout), &result) != nil || result.DispositionProviderInputs != nil || readErr != nil || !bytes.Equal(before, after) {
		return errors.New("checksum-valid impossible identity received historical treatment")
	}
	return nil
}
