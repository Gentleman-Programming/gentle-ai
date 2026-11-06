package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const issue1800Lineage = "issue-1800-pre-plan-abandon"

// issue1800Journeys proves D1800's narrow exit without turning a pre-plan edit
// into a retrospective forecast, recovery, or delivery authorization.
func issue1800Journeys() []Journey {
	return []Journey{{
		ID:     "j91-audited-abandon-preplan-over-budget-correction",
		Review: reviewOptedIn,
		Title:  "#3587: an exact active-lineage pre-plan correction abandons audibly and requires a fresh review",
		Source: "issue #1800 D1800 under #3587: exact audited abandon is the supported pre-plan exit for an active lineage",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "clear any clone-local review override (a clone may only ever assert off)", Requires: modeCapability, Args: productArgs("review", "mode", "enable", "--scope", "clone", "--json")},
			{Name: "fixture: stage candidate", Fixture: stageWaveCandidate},
			{Name: "start product-created compact review with an exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", issue1800Lineage)},
			{Name: "capture candidate finding and the full selected lens set for the exact active lineage", Requires: captureResultCapability, Composite: func(r *journeyRun) error {
				return captureExactSelectedReviewerSlots(r, issue1800Lineage, true)
			}},
			{Name: "freeze the pre-plan correction authority", Requires: statusCapability, Composite: prepareIssue1800Authority},
			{Name: "fixture: edit the frozen path to budget plus one", Fixture: applyIssue1800PrePlanEdit},
			{Name: "STATUS remains correction-plan-required", Requires: statusCapability, Composite: proveIssue1800PrePlanStatus},
			{Name: "forged and stale bindings refuse without mutation", Requires: abandonCapability, Composite: rejectIssue1800AbandonDecoys},
			{Name: "exact native abandon preserves audited provenance", Requires: abandonCapability, Composite: abandonIssue1800Authority},
			{Name: "fresh selectorless review is required while delivery returns unmanaged ordinary policy", Requires: validateCapability, Composite: proveIssue1800FreshReviewRequired},
		},
	}}
}

type issue1800Authority struct {
	CorrectionBudget        int  `json:"correction_budget"`
	ProposedCorrectionLines *int `json:"proposed_correction_lines"`
	InitialSnapshot         struct {
		Identity string `json:"identity"`
	} `json:"initial_snapshot"`
}

func issue1800State(r *journeyRun) ([]byte, issue1800Authority, error) {
	common, err := gitOut(r.sandbox, r.sandbox.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, issue1800Authority{}, err
	}
	stateBytes, err := os.ReadFile(filepath.Join(strings.TrimSpace(common), "gentle-ai", "review-transactions", "v2", issue1800Lineage, "review-state.json"))
	var record struct {
		State issue1800Authority `json:"state"`
	}
	if err == nil {
		err = json.Unmarshal(stateBytes, &record)
	}
	return stateBytes, record.State, err
}

func issue1800Entry(r *journeyRun) (authorityEntry, error) {
	status, err := proveStoreStatus(r.sandbox)
	if err != nil {
		return authorityEntry{}, err
	}
	for _, entry := range status.Entries {
		if entry.LineageID == issue1800Lineage {
			return authorityEntry{LineageID: entry.LineageID, State: entry.State, Revision: entry.Revision, SnapshotIdentity: entry.SnapshotIdentity, DiscardedWork: entry.DiscardedWork}, nil
		}
	}
	return authorityEntry{}, fmt.Errorf("native inventory omits %q", issue1800Lineage)
}

func prepareIssue1800Authority(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, issue1800Lineage, reviewContractV2)
	stateBytes, state, stateErr := issue1800State(r)
	entry, entryErr := issue1800Entry(r)
	if err != nil || stateErr != nil || entryErr != nil || status.Authority == nil ||
		status.Authority.State != "correction_required" || status.NextTransition == nil ||
		status.NextTransition.Kind != "collect" || status.NextTransition.ReasonCode != "correction_plan_required" ||
		state.CorrectionBudget <= 0 || state.ProposedCorrectionLines != nil ||
		strings.Join(status.Projection.Paths, "\x00") != "candidate.go" ||
		entry.Revision != status.Authority.Revision || entry.SnapshotIdentity != state.InitialSnapshot.Identity {
		return fmt.Errorf("pre-plan authority = status:%+v state:%+v errors:%v/%v/%v", status, state, err, stateErr, entryErr)
	}
	r.sandbox.Scratch["issue1800-budget"] = strconv.Itoa(state.CorrectionBudget)
	r.sandbox.Scratch["issue1800-state"] = string(stateBytes)
	r.sandbox.Scratch["issue1800-lineage"] = entry.LineageID
	r.sandbox.Scratch["issue1800-revision"] = entry.Revision
	r.sandbox.Scratch["issue1800-snapshot"] = entry.SnapshotIdentity
	return nil
}

func applyIssue1800PrePlanEdit(sandbox *Sandbox) error {
	budget, err := strconv.Atoi(sandbox.Scratch["issue1800-budget"])
	if err != nil || budget < 1 {
		return fmt.Errorf("correction budget = %q, %v", sandbox.Scratch["issue1800-budget"], err)
	}
	source := "package candidate\n\n" + strings.Repeat("// pre-plan correction detail\n", budget-1) + "func value() int { return 2 }\n"
	if err := sandbox.write(filepath.Join(sandbox.Repo, "candidate.go"), source); err != nil {
		return err
	}
	numstat, err := gitOut(sandbox, sandbox.Repo, "diff", "--numstat")
	fields := strings.Fields(numstat)
	if err != nil || len(fields) != 3 || fields[2] != "candidate.go" {
		return fmt.Errorf("pre-plan correction numstat = %q, %v", numstat, err)
	}
	added, addErr := strconv.Atoi(fields[0])
	removed, removeErr := strconv.Atoi(fields[1])
	if addErr != nil || removeErr != nil || added+removed != budget+1 {
		return fmt.Errorf("pre-plan correction lines = %d+%d, want %d: %v/%v", added, removed, budget+1, addErr, removeErr)
	}
	return nil
}

func proveIssue1800PrePlanStatus(r *journeyRun) error {
	// Once the would-be correction drifts beyond the frozen budget before its
	// plan is captured, STATUS refuses the stale read-only operation. The
	// refusal must leave the pre-plan authority bytes untouched so audited
	// abandon remains available; it must not revive FINALIZE or evidence.
	observation := r.run(productArgsFor(r, "review", "status", "--contract", reviewContractV2,
		"--next-transition", "--lineage", issue1800Lineage), false)
	stateBytes, state, stateErr := issue1800State(r)
	entry, entryErr := issue1800Entry(r)
	store, _, storeErr := frozenAuthorityInventory(r)
	candidate, candidateErr := os.ReadFile(filepath.Join(r.sandbox.Repo, "candidate.go"))
	if observation.ExitCode == 0 || !strings.Contains(observation.Stdout, "operation_failed") || stateErr != nil || entryErr != nil ||
		storeErr != nil || candidateErr != nil || state.ProposedCorrectionLines != nil ||
		!bytes.Equal(stateBytes, []byte(r.sandbox.Scratch["issue1800-state"])) ||
		entry.Revision != r.sandbox.Scratch["issue1800-revision"] {
		return fmt.Errorf("pre-plan drift response = exit=%d stdout=%q state=%+v errors=%v/%v/%v/%v", observation.ExitCode, observation.Stdout, state, stateErr, entryErr, storeErr, candidateErr)
	}
	r.sandbox.Scratch["issue1800-store"] = string(store)
	r.sandbox.Scratch["issue1800-candidate"] = string(candidate)
	return nil
}

func issue1800Unchanged(r *journeyRun) error {
	state, _, stateErr := issue1800State(r)
	store, _, storeErr := frozenAuthorityInventory(r)
	candidate, candidateErr := os.ReadFile(filepath.Join(r.sandbox.Repo, "candidate.go"))
	for _, check := range []struct {
		name string
		got  []byte
		want []byte
		err  error
	}{
		{"authority", state, []byte(r.sandbox.Scratch["issue1800-state"]), stateErr},
		{"active authority inventory", store, []byte(r.sandbox.Scratch["issue1800-store"]), storeErr},
		{"candidate source", candidate, []byte(r.sandbox.Scratch["issue1800-candidate"]), candidateErr},
	} {
		if check.err != nil || !bytes.Equal(check.got, check.want) {
			return fmt.Errorf("%s changed after abandoned authorization: %v", check.name, check.err)
		}
	}
	return nil
}

func rejectIssue1800AbandonDecoys(r *journeyRun) error {
	entry, err := issue1800Entry(r)
	args, argsErr := abandonArgs("issue1800-lineage", "issue1800-revision", "issue1800-snapshot", "")(r.sandbox)
	if err != nil || argsErr != nil {
		return fmt.Errorf("assemble native abandon authorization: %v %v", err, argsErr)
	}
	stale := entry.Revision[:len(entry.Revision)-1] + "0"
	if stale == entry.Revision {
		stale = entry.Revision[:len(entry.Revision)-1] + "1"
	}
	forged := append([]string{}, args...)
	forged[len(forged)-1] = strings.Replace(forged[len(forged)-1], "actor=bench", "actor=forged", 1)
	staleArgs := append([]string{}, args...)
	staleArgs[7] = stale
	staleArgs[len(staleArgs)-1] = renderAbandonAuthorization(authorityEntry{LineageID: entry.LineageID, Revision: stale, SnapshotIdentity: entry.SnapshotIdentity, DiscardedWork: entry.DiscardedWork}, "bench", "operator_disposition")
	for _, decoy := range []struct {
		name string
		args []string
	}{{"forged", forged}, {"stale", staleArgs}} {
		if observation := r.run(decoy.args, false); observation.ExitCode == 0 || strings.Contains(observation.Stdout, "quarantine_path") {
			return fmt.Errorf("%s abandon authorization succeeded", decoy.name)
		}
		if err := issue1800Unchanged(r); err != nil {
			return fmt.Errorf("%s decoy: %w", decoy.name, err)
		}
	}
	return nil
}

func abandonIssue1800Authority(r *journeyRun) error {
	entry, err := issue1800Entry(r)
	args, argsErr := abandonArgs("issue1800-lineage", "issue1800-revision", "issue1800-snapshot", "")(r.sandbox)
	if err != nil || argsErr != nil {
		return fmt.Errorf("assemble exact abandon authorization: %v %v", err, argsErr)
	}
	observation := r.run(args, false)
	var result struct {
		Operation string `json:"operation"`
		Record    struct {
			LineageID      string `json:"lineage_id"`
			Reason         string `json:"reason"`
			Actor          string `json:"actor"`
			QuarantinePath string `json:"quarantine_path"`
			Abandonment    struct {
				Schema           string                 `json:"schema"`
				LineageID        string                 `json:"lineage_id"`
				Revision         string                 `json:"revision"`
				SnapshotIdentity string                 `json:"snapshot_identity"`
				DiscardedWork    authorityDiscardedWork `json:"discarded_work"`
			} `json:"abandonment"`
		} `json:"record"`
	}
	if observation.ExitCode != 0 || json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result) != nil ||
		result.Operation != "review/abandon" || result.Record.LineageID != entry.LineageID || result.Record.Reason != "operator_disposition" || result.Record.Actor != "bench" ||
		result.Record.QuarantinePath == "" || result.Record.Abandonment.Schema != "gentle-ai.review-abandon-authorization/v2" ||
		result.Record.Abandonment.LineageID != entry.LineageID || result.Record.Abandonment.Revision != entry.Revision || result.Record.Abandonment.SnapshotIdentity != entry.SnapshotIdentity ||
		strings.Join(result.Record.Abandonment.DiscardedWork.CapturedLensResults, "\x00") != strings.Join(entry.DiscardedWork.CapturedLensResults, "\x00") ||
		result.Record.Abandonment.DiscardedWork.FindingsPresent != entry.DiscardedWork.FindingsPresent || result.Record.Abandonment.DiscardedWork.EvidenceRecordsPresent != entry.DiscardedWork.EvidenceRecordsPresent {
		return fmt.Errorf("exact abandon result = exit %d record=%+v", observation.ExitCode, result)
	}
	moved, moveErr := os.ReadFile(filepath.Join(result.Record.QuarantinePath, "residue", "review-state.json"))
	candidate, candidateErr := os.ReadFile(filepath.Join(r.sandbox.Repo, "candidate.go"))
	if moveErr != nil || candidateErr != nil || !bytes.Equal(moved, []byte(r.sandbox.Scratch["issue1800-state"])) || !bytes.Equal(candidate, []byte(r.sandbox.Scratch["issue1800-candidate"])) {
		return fmt.Errorf("abandon did not preserve quarantined authority or candidate bytes: %v/%v", moveErr, candidateErr)
	}
	active, activeErr := proveStoreStatus(r.sandbox)
	if activeErr != nil {
		return activeErr
	}
	for _, entry := range active.Entries {
		if entry.LineageID == issue1800Lineage {
			return fmt.Errorf("abandoned authority remains active")
		}
	}
	return nil
}

func proveIssue1800FreshReviewRequired(r *journeyRun) error {
	status, err := readCorrectionStatusForContract(r, "", reviewContractV2)
	gate := r.run([]string{"review", "validate", "--cwd", r.sandbox.Repo, "--gate", "post-apply"}, false)
	if err != nil || status.Receipt.Status != "not_applicable" || status.Authority != nil || status.NextTransition == nil ||
		status.NextTransition.Kind != "execute" || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		return fmt.Errorf("post-abandon fresh selectorless status=%+v", status)
	}
	if err := requireUnmanagedShippedGate(gate, "post-apply"); err != nil {
		return fmt.Errorf("post-abandon ordinary delivery: %w", err)
	}
	return nil
}
