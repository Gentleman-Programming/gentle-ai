package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestNegotiatedStatusPublishesClassifiedRepairAuthorizationTransition(t *testing.T) {
	repo, lineage, head := classifiedRepairCLIFixture(t, "status-classified-repair")
	args := []string{"status", "--contract", ReviewIntegrationContractV1, "--action-eligibility", "--next-transition", "--cwd", repo}
	var unauthorized bytes.Buffer
	if err := RunReview(args, &unauthorized); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decoder := json.NewDecoder(bytes.NewReader(unauthorized.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Applicability != reviewtransaction.TargetApplicabilityCorrupted ||
		status.Repair.Status != reviewtransaction.AuthorityRepairEligible || status.Repair.Candidate == nil ||
		status.Repair.Candidate.LineageID != lineage || status.Repair.Candidate.Revision != head ||
		status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionCollect ||
		status.NextTransition.ReasonCode != "repair_authorization_required" || status.NextTransition.Collect == nil ||
		len(status.NextTransition.Collect.Inputs) != 1 || status.NextTransition.Collect.Inputs[0].CaptureOperation != "external.authorize_repair" ||
		status.NextTransition.Collect.Inputs[0].Schema != reviewtransaction.AuthorityRepairAuthorizationSchema {
		t.Fatalf("unauthorized classified repair status = %#v\n%s", status, unauthorized.String())
	}
	if status.Eligibility == nil || len(status.Eligibility.AllowedActions) != 1 ||
		status.Eligibility.AllowedActions[0].Action != "review.repair" ||
		!reflect.DeepEqual(status.Eligibility.AllowedActions[0].RequiredInputs, []string{"actor", "reason", "maintainer_authorization"}) {
		t.Fatalf("classified repair eligibility = %#v", status.Eligibility)
	}
	if strings.Contains(unauthorized.String(), repo) || strings.Contains(unauthorized.String(), "maintainer@example.com") {
		t.Fatalf("repair collection leaked private path or invented authorization: %s", unauthorized.String())
	}

	actor, reason := "maintainer@example.com", "quarantine the approved historical alias"
	authorization := classifiedRepairAuthorization(status.Repair, actor, reason)
	authorizedArgs := append(append([]string{}, args...),
		"--repair-actor", actor, "--repair-reason", reason, "--repair-authorization", authorization,
	)
	var authorized bytes.Buffer
	if err := RunReview(authorizedArgs, &authorized); err != nil {
		t.Fatal(err)
	}
	status = ReviewTargetStatusResult{}
	decoder = json.NewDecoder(bytes.NewReader(authorized.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil {
		t.Fatal(err)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("authorized status validation: %v\n%s\n%#v", err, authorized.String(), status.NextTransition)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.ReasonCode != "repair_authorized" || status.NextTransition.Execute == nil ||
		status.NextTransition.Execute.Operation != "review.repair" ||
		status.NextTransition.Execute.Binding.LineageID != lineage || status.NextTransition.Execute.Binding.Revision != head {
		t.Fatalf("authorized classified repair status = %#v", status.NextTransition)
	}
	if strings.Contains(authorized.String(), authorization) {
		t.Fatalf("authorized classified repair status leaked completed authorization: %s", authorized.String())
	}
	unsafe := status
	unsafeTransition := *status.NextTransition
	unsafeExecution := *status.NextTransition.Execute
	unsafeExecution.Arguments = append([]ReviewTransitionArgument{}, unsafeExecution.Arguments...)
	for index := range unsafeExecution.Arguments {
		if unsafeExecution.Arguments[index].Name == "maintainer-authorization" {
			unsafeExecution.Arguments[index].Value = authorization
		}
	}
	unsafeTransition.Execute = &unsafeExecution
	unsafe.NextTransition = &unsafeTransition
	if err := unsafe.Validate(); err == nil {
		t.Fatal("classified repair status accepted a completed public authorization")
	}
}

func TestNegotiatedStatusKeepsExplicitHealthyTargetIsolatedFromUnrelatedRepair(t *testing.T) {
	repo := initReviewCLIRepo(t)
	appendClassifiedRepairCLIFixture(t, repo, "unrelated-repair-alias")
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	risk, lines, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	lenses := []string{}
	if risk == reviewtransaction.RiskMedium {
		lenses = []string{reviewtransaction.LensReliability}
	} else if risk == reviewtransaction.RiskHigh {
		lenses = []string{
			reviewtransaction.LensRisk,
			reviewtransaction.LensResilience,
			reviewtransaction.LensReadability,
			reviewtransaction.LensReliability,
		}
	}
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: "selected-healthy-compact", Mode: reviewtransaction.ModeOrdinaryBounded, Generation: 1,
		Snapshot: snapshot, PolicyHash: "sha256:" + strings.Repeat("d", 64),
		RiskLevel: risk, SelectedLenses: lenses,
		OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace("", "review/start", state); err != nil {
		t.Fatal(err)
	}

	for _, variant := range []struct {
		name string
		args []string
	}{
		{name: "status"},
		{name: "eligibility", args: []string{"--action-eligibility"}},
		{name: "transition", args: []string{"--next-transition"}},
		{name: "eligibility and transition", args: []string{"--action-eligibility", "--next-transition"}},
	} {
		t.Run(variant.name, func(t *testing.T) {
			args := []string{"status", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", state.LineageID}
			args = append(args, variant.args...)
			var output bytes.Buffer
			if err := RunReview(args, &output); err != nil {
				t.Fatalf("explicit healthy STATUS absorbed unrelated repair: %v\n%s", err, output.String())
			}
			var status ReviewTargetStatusResult
			decodeStrictReviewJSON(t, output.Bytes(), &status)
			if err := status.Validate(); err != nil {
				t.Fatalf("explicit healthy STATUS validation: %v\n%s", err, output.String())
			}
			if status.Applicability != reviewtransaction.TargetApplicabilityCurrent || status.Authority == nil ||
				status.Authority.LineageID != state.LineageID || status.Repair.Status != reviewtransaction.AuthorityRepairUnsupported ||
				status.Repair.Candidate != nil {
				t.Fatalf("explicit healthy STATUS was contaminated by unrelated repair: %#v", status)
			}
			if status.Eligibility != nil && len(status.Eligibility.AllowedActions) == 1 &&
				status.Eligibility.AllowedActions[0].Action == "review.repair" {
				t.Fatalf("explicit healthy STATUS offered unrelated repair: %#v", status.Eligibility)
			}
			if status.NextTransition != nil && status.NextTransition.Execute != nil &&
				status.NextTransition.Execute.Operation == "review.repair" {
				t.Fatalf("explicit healthy STATUS routed unrelated repair: %#v", status.NextTransition)
			}
		})
	}
}

func classifiedRepairAuthorization(assessment reviewtransaction.AuthorityRepairAssessment, actor, reason string) string {
	candidate := assessment.Candidate
	return reviewtransaction.AuthorityRepairAuthorizationSchema +
		"\nrepository=" + assessment.RepositoryBinding +
		"\nclass=" + string(assessment.Class) +
		"\nlineage=" + candidate.LineageID +
		"\nrevision=" + candidate.Revision +
		"\ncause=" + string(assessment.Cause) +
		"\ndisposition=" + string(assessment.Disposition) +
		"\nactor=" + actor +
		"\nreason=" + reason
}

func classifiedRepairCLIFixture(t *testing.T, lineage string) (string, string, string) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	head := appendClassifiedRepairCLIFixture(t, repo, lineage)
	return repo, lineage, head
}

func appendClassifiedRepairCLIFixture(t *testing.T, repo, lineage string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinary4R, Generation: 1, Snapshot: snapshot,
		PolicyHash: "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.StartReview(); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	head := appendLegacyCLIRecord(t, store, "", "review/start", *tx)
	finding := reviewtransaction.Finding{ID: "R1-ALIAS", Severity: "CRITICAL"}
	ledger, err := reviewtransaction.CanonicalLedger([]reviewtransaction.Finding{finding})
	if err != nil {
		t.Fatal(err)
	}
	ledgerSum := sha256.Sum256(ledger)
	ledgerHash := "sha256:" + hex.EncodeToString(ledgerSum[:])
	if err := tx.FreezeFindings([]reviewtransaction.Finding{finding}, ledger, ledgerHash); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/freeze-findings", *tx)
	if _, err := tx.ClassifyEvidence([]reviewtransaction.FindingEvidence{{
		FindingID: finding.ID, Class: reviewtransaction.EvidenceDeterministic,
		Causality: reviewtransaction.CausalIntroduced, Proof: "historical proof",
	}}); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/classify-evidence", *tx)
	if err := tx.BeginFix("sha256:" + strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/begin-fix", *tx)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fix, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetFixDiff, BaseRef: tx.FinalCandidateTree,
		IntendedUntracked: []string{}, LedgerIDs: []string{finding.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.CompleteFix(fix, "", fix.LedgerIDs); err != nil {
		t.Fatal(err)
	}
	head = appendLegacyCLIRecord(t, store, head, "review/complete-fix", *tx)
	validated := *tx
	if err := validated.ValidateFixDelta(validated.FixFindingIDs, true); err != nil {
		t.Fatal(err)
	}
	bad := reviewtransaction.Record{
		Schema: reviewtransaction.RecordSchema, Operation: "review/validate-fix", PreviousRevision: head, Transaction: validated,
	}
	payload, err := json.MarshalIndent(bad, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	head = "sha256:" + hex.EncodeToString(sum[:])
	eventPath := filepath.Join(store.Dir, "events", strings.TrimPrefix(head, "sha256:")+".json")
	if err := os.WriteFile(eventPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "HEAD"), []byte(head+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return head
}

func TestClassifiedRepairAuthorizationHelperIsExact(t *testing.T) {
	repo, _, _ := classifiedRepairCLIFixture(t, "repair-authorization-helper")
	assessment, err := reviewtransaction.AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	first := classifiedRepairAuthorization(assessment, "actor", "reason")
	second := classifiedRepairAuthorization(assessment, "actor", "reason")
	if first != second || strings.Contains(first, "\r") || len(strings.Split(first, "\n")) != 9 || reflect.DeepEqual(first, "") {
		t.Fatalf("authorization helper = %q", first)
	}
}

func TestReviewRepairPreflightIsReadOnlyAndExactExecutionReplays(t *testing.T) {
	repo, lineage, head := classifiedRepairCLIFixture(t, "repair-command")
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	headBefore, err := os.ReadFile(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	preflightArgs := []string{"repair", "--preflight", "--cwd", repo}
	var first, second bytes.Buffer
	if err := RunReview(preflightArgs, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview(preflightArgs, &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("repair preflight changed between reads:\n%s\n%s", first.String(), second.String())
	}
	var preflight ReviewRepairResult
	decoder := json.NewDecoder(bytes.NewReader(first.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&preflight); err != nil {
		t.Fatal(err)
	}
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	if preflight.Mode != ReviewRepairModePreflight || preflight.Assessment.Status != reviewtransaction.AuthorityRepairEligible ||
		preflight.ProviderInputs == nil || preflight.ProviderInputs.LineageID != lineage || preflight.ProviderInputs.ExpectedRevision != head ||
		!reflect.DeepEqual(preflight.RequiredInputs, []string{"actor", "reason", "maintainer_authorization"}) {
		t.Fatalf("repair preflight = %#v", preflight)
	}
	if strings.Contains(first.String(), repo) || strings.Contains(first.String(), "\nactor=") || strings.Contains(first.String(), "\nreason=") {
		t.Fatalf("repair preflight leaked path or completed authorization: %s", first.String())
	}
	if headAfter, err := os.ReadFile(filepath.Join(store.Dir, "HEAD")); err != nil || !bytes.Equal(headBefore, headAfter) {
		t.Fatalf("repair preflight changed HEAD: %q, %v", headAfter, err)
	}

	actor, reason := "maintainer@example.com", "quarantine the approved historical alias"
	authorization := classifiedRepairAuthorization(preflight.Assessment, actor, reason)
	executeArgs := []string{
		"repair", "--cwd", repo,
		"--class", string(preflight.ProviderInputs.Class), "--lineage", preflight.ProviderInputs.LineageID,
		"--expected-revision", preflight.ProviderInputs.ExpectedRevision, "--cause", string(preflight.ProviderInputs.Cause),
		"--disposition", string(preflight.ProviderInputs.Disposition), "--repository-binding", preflight.ProviderInputs.RepositoryBinding,
		"--actor", actor, "--reason", reason, "--maintainer-authorization", authorization,
	}
	var executed bytes.Buffer
	if err := RunReview(executeArgs, &executed); err != nil {
		t.Fatal(err)
	}
	var result ReviewRepairResult
	decoder = json.NewDecoder(bytes.NewReader(executed.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Mode != ReviewRepairModeExecute || result.Execution == nil || result.Execution.Status != "committed" ||
		result.Execution.LineageID != lineage || result.Execution.Revision != head || strings.Contains(executed.String(), repo) ||
		strings.Contains(executed.String(), authorization) {
		t.Fatalf("repair execution = %#v\n%s", result, executed.String())
	}
	if _, err := os.Stat(store.Dir); !os.IsNotExist(err) {
		t.Fatalf("repair execution left source: %v", err)
	}

	var replay bytes.Buffer
	if err := RunReview(executeArgs, &replay); err != nil {
		t.Fatal(err)
	}
	var replayed ReviewRepairResult
	if err := json.Unmarshal(replay.Bytes(), &replayed); err != nil || replayed.Execution == nil ||
		replayed.Execution.LineageID != lineage || replayed.Execution.Revision != head {
		t.Fatalf("repair replay = %#v, %v", replayed, err)
	}
}

func TestReviewRepairPreflightSelectorStillClassifiesCompleteInventory(t *testing.T) {
	repo := initReviewCLIRepo(t)
	appendClassifiedRepairCLIFixture(t, repo, "selected-alias")
	appendClassifiedRepairCLIFixture(t, repo, "other-alias")
	var output bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo, "--lineage", "selected-alias"}, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewRepairResult
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Assessment.Status != reviewtransaction.AuthorityRepairAmbiguous || result.Assessment.Candidate != nil || result.ProviderInputs != nil {
		t.Fatalf("lineage selector narrowed classified repair inventory: %#v", result)
	}
}

func TestNegotiatedReviewRepairFailureNeverPublishesAuthorizationOrPaths(t *testing.T) {
	repo, _, _ := classifiedRepairCLIFixture(t, "repair-private-failure")
	assessment, err := reviewtransaction.AssessAuthorityRepair(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	preflight := newReviewRepairPreflightResult(assessment)
	secret := "authorization token for /private/authority/path"
	args := []string{
		"repair", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--class", string(preflight.ProviderInputs.Class), "--lineage", preflight.ProviderInputs.LineageID,
		"--expected-revision", preflight.ProviderInputs.ExpectedRevision, "--cause", string(preflight.ProviderInputs.Cause),
		"--disposition", string(preflight.ProviderInputs.Disposition), "--repository-binding", preflight.ProviderInputs.RepositoryBinding,
		"--actor", "maintainer@example.com", "--reason", "approved repair", "--maintainer-authorization", secret,
	}
	var output bytes.Buffer
	err = RunReview(args, &output)
	if err == nil {
		t.Fatalf("inexact negotiated repair succeeded: %s", output.String())
	}
	failure := decodeReviewIntegrationFailure(t, output.Bytes())
	if failure.Operation != "review.repair" || failure.Code != "invalid_request" || failure.MutationOutcome != ReviewMutationNotStarted {
		t.Fatalf("negotiated repair failure = %#v", failure)
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), repo) || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), repo) {
		t.Fatalf("negotiated repair failure leaked private input: %v\n%s", err, output.String())
	}
}

func TestReviewRepairHelpRecommendsGenericClassifiedFlow(t *testing.T) {
	var help bytes.Buffer
	if err := RunReview([]string{"repair", "--help"}, &help); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--preflight", "provider-owned", reviewtransaction.AuthorityRepairAuthorizationSchema, "repair-legacy-alias"} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("generic repair help missing %q: %s", want, help.String())
		}
	}
}

// authorityDispositionAuthorization manually renders the exact
// gentle-ai.review-disposition-authorization/v1 binding
// authorityDispositionAuthorizationBinding (authority_disposition_plan.go)
// computes internally — mirroring how classifiedRepairAuthorization above
// replicates the legacy binding rather than exporting a production helper
// whose only caller would be test code.
func authorityDispositionAuthorization(plan reviewtransaction.AuthorityDispositionPlan) string {
	return "gentle-ai.review-disposition-authorization/v1" +
		"\nschema=" + plan.Schema +
		"\nrepository=" + plan.RepositoryBinding +
		"\nclass=" + plan.AnomalyClass +
		"\nplan_digest=" + plan.PlanDigest +
		"\ninventory_revision=" + plan.AuthorityInventoryRevision +
		"\nactor=" + plan.Actor +
		"\nreason=" + plan.Reason
}

// dispositionForgedAuthorization is schema-prefixed
// (gentle-ai.review-recovery-authorization/v1) but bound to content that can
// never match a real exact binding, so classifyCompactRecoveryEdgeAnomalies
// (compact_reconcile.go) always classifies it into the closed
// content_mismatched_recovery_authorization class rather than the
// pre-contract malformed_recovery_authorization AnomalyClasses class.
const dispositionForgedAuthorization = "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=impossible-mismatch\npredecessor_revision=impossible\ntarget_identity=impossible\nactor=maintainer@example.com\nreason=impossible"

// TestReviewRepairPreflightSurfacesAuthorityDispositionPlanForEligibleLeaf
// satisfies tasks.md 3.1: review repair --preflight emits the derived plan's
// digest and inventory revision for a content-mismatched leaf — the #1892
// shape that previously left AssessAuthorityRepair reporting "unsupported,
// no inputs" with no way forward.
func TestReviewRepairPreflightSurfacesAuthorityDispositionPlanForEligibleLeaf(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "leaf-disposition", false, dispositionForgedAuthorization)

	var first, second bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("disposition preflight changed between reads:\n%s\n%s", first.String(), second.String())
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, first.Bytes(), &preflight)
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	if preflight.Mode != ReviewRepairModePreflight || preflight.Assessment.Status != reviewtransaction.AuthorityRepairUnsupported ||
		preflight.DispositionProviderInputs == nil ||
		!validReviewCapabilitySHA256(preflight.DispositionProviderInputs.PlanDigest) ||
		!validReviewCapabilitySHA256(preflight.DispositionProviderInputs.AuthorityInventoryRevision) {
		t.Fatalf("disposition preflight = %#v\n%s", preflight, first.String())
	}
	if strings.Contains(first.String(), repo) || strings.Contains(first.String(), "leaf-disposition-successor") {
		t.Fatalf("disposition preflight leaked path or lineage identity: %s", first.String())
	}
}

// TestReviewRepairPreflightOmitsAuthorityDispositionPlanForMultiNodeShape
// proves the "otherwise unchanged" half of tasks.md 3.3/design decision 6:
// when derivation cannot close on exactly one seed (the #2014/#1656
// multi-lineage shape), preflight never surfaces a disposition plan.
func TestReviewRepairPreflightOmitsAuthorityDispositionPlanForMultiNodeShape(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "leaf-alpha", false, dispositionForgedAuthorization)
	writeInspectCLIRecoveryPair(t, repo, "leaf-bravo", false, dispositionForgedAuthorization)

	var output bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, output.Bytes(), &preflight)
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	if preflight.DispositionProviderInputs != nil {
		t.Fatalf("multi-node shape surfaced a disposition plan: %#v", preflight)
	}
}

// TestReviewRepairPreflightRejectsAuthorityDispositionExecutionInputs extends
// the existing execution-input guard to the new disposition flags.
func TestReviewRepairPreflightRejectsAuthorityDispositionExecutionInputs(t *testing.T) {
	repo := initReviewCLIRepo(t)
	args := []string{"repair", "--preflight", "--cwd", repo, "--plan-digest", "sha256:" + strings.Repeat("a", 64)}
	var output bytes.Buffer
	if err := RunReview(args, &output); err == nil {
		t.Fatalf("preflight accepted a disposition execution input: %s", output.String())
	}
}

// TestReviewRepairDispositionExecutionRequiresAllFlagsBeforeLockAcquisition
// satisfies tasks.md 3.2: execution requires --plan-digest
// --inventory-revision --actor --reason --authorization; missing any one
// refuses without ever reaching RepairAuthorityDisposition's maintenance
// lock acquisition, and never leaks the repository path.
func TestReviewRepairDispositionExecutionRequiresAllFlagsBeforeLockAcquisition(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "leaf-missing-flags", false, dispositionForgedAuthorization)
	var preflightOutput bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &preflightOutput); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, preflightOutput.Bytes(), &preflight)

	values := map[string]string{
		"--plan-digest":        preflight.DispositionProviderInputs.PlanDigest,
		"--inventory-revision": preflight.DispositionProviderInputs.AuthorityInventoryRevision,
		"--actor":              "maintainer@example.com",
		"--reason":             "quarantine content-mismatched leaf",
		"--authorization":      "placeholder-authorization",
	}
	for omit := range values {
		args := []string{"repair", "--cwd", repo}
		for flag, value := range values {
			if flag == omit {
				continue
			}
			args = append(args, flag, value)
		}
		var output bytes.Buffer
		if err := RunReview(args, &output); err == nil {
			t.Fatalf("disposition execution proceeded without %s: %s", omit, output.String())
		}
		if strings.Contains(output.String(), repo) {
			t.Fatalf("missing-flag disposition refusal leaked repo path: %s", output.String())
		}
	}
}

// TestReviewRepairDispositionExecutionQuarantinesEligibleLeafAndReplaysExactly
// is the real CLI invocation runtime harness for Unit 3: a fixture-damaged
// store is repaired black-box through `review repair`, and no path or
// authorization is ever published. Replaying the identical plan bytes is
// reviewtransaction's own concern (executeAuthorityDisposition, satisfied by
// Slice S2's TestAuthorityDispositionExecuteReplayConvergesWithoutDoubleMove)
// — RepairAuthorityDisposition re-derives fresh from the live graph on every
// call, so once the leaf is quarantined the same CLI flags naturally find
// nothing left to derive; this test proves that follow-on preflight then
// reports it has nothing more to surface, not a second identical execution.
func TestReviewRepairDispositionExecutionQuarantinesEligibleLeaf(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "leaf-execute", false, dispositionForgedAuthorization)
	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	sourceDir := filepath.Join(authorityRoot, "v2", "leaf-execute-successor")
	if _, err := os.Stat(sourceDir); err != nil {
		t.Fatal(err)
	}

	actor, reason := "maintainer@example.com", "quarantine content-mismatched leaf"
	plan, err := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(context.Background(), repo, actor, reason)
	if err != nil {
		t.Fatal(err)
	}
	authorization := authorityDispositionAuthorization(plan)

	executeArgs := []string{
		"repair", "--cwd", repo,
		"--plan-digest", plan.PlanDigest, "--inventory-revision", plan.AuthorityInventoryRevision,
		"--actor", actor, "--reason", reason, "--authorization", authorization,
	}
	var executed bytes.Buffer
	if err := RunReview(executeArgs, &executed); err != nil {
		t.Fatalf("disposition execution refused an eligible leaf: %v\n%s", err, executed.String())
	}
	var result ReviewRepairResult
	decodeStrictReviewJSON(t, executed.Bytes(), &result)
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Mode != ReviewRepairModeExecute || result.DispositionExecution == nil ||
		result.DispositionExecution.Status != string(reviewtransaction.CompactReclaimCommitted) ||
		result.DispositionExecution.LineageID != "leaf-execute-successor" ||
		result.DispositionExecution.PlanDigest != plan.PlanDigest ||
		result.DispositionExecution.AuthorityInventoryRevision != plan.AuthorityInventoryRevision {
		t.Fatalf("disposition execution = %#v\n%s", result, executed.String())
	}
	if strings.Contains(executed.String(), repo) || strings.Contains(executed.String(), authorization) {
		t.Fatalf("disposition execution leaked path or authorization: %s", executed.String())
	}
	if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
		t.Fatalf("disposition execution left the source entry: %v", err)
	}

	var followOn bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &followOn); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, followOn.Bytes(), &preflight)
	if err := preflight.Validate(); err != nil {
		t.Fatal(err)
	}
	if preflight.DispositionProviderInputs != nil {
		t.Fatalf("follow-on preflight still surfaced a plan for an already-quarantined leaf: %#v", preflight)
	}
}

// TestReviewRepairDispositionExecutionAcceptsPreflightPublishedDigest proves
// the documented two-step CLI flow actually works: run `review repair
// --preflight` to obtain --plan-digest/--inventory-revision, then execute
// with exactly those values. plan_digest's pre-image excludes Actor and
// Reason (execution-time provenance, not plan identity — mirrors
// Authorization's treatment), so the digest --preflight publishes (derived
// with empty actor/reason) MUST equal the digest execution re-derives (with
// the real actor/reason) for the same graph state.
func TestReviewRepairDispositionExecutionAcceptsPreflightPublishedDigest(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "leaf-preflight-digest", false, dispositionForgedAuthorization)

	var preflightOutput bytes.Buffer
	if err := RunReview([]string{"repair", "--preflight", "--cwd", repo}, &preflightOutput); err != nil {
		t.Fatal(err)
	}
	var preflight ReviewRepairResult
	decodeStrictReviewJSON(t, preflightOutput.Bytes(), &preflight)
	if preflight.DispositionProviderInputs == nil {
		t.Fatalf("preflight surfaced no disposition plan: %s", preflightOutput.String())
	}

	actor, reason := "maintainer@example.com", "quarantine content-mismatched leaf"
	plan, err := reviewtransaction.DeriveAuthorityDispositionPlanAtRepo(context.Background(), repo, actor, reason)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanDigest != preflight.DispositionProviderInputs.PlanDigest {
		t.Fatalf("execution-time digest %q does not match preflight-published digest %q — actor/reason leaked into plan identity", plan.PlanDigest, preflight.DispositionProviderInputs.PlanDigest)
	}
	authorization := authorityDispositionAuthorization(plan)

	executeArgs := []string{
		"repair", "--cwd", repo,
		"--plan-digest", preflight.DispositionProviderInputs.PlanDigest,
		"--inventory-revision", preflight.DispositionProviderInputs.AuthorityInventoryRevision,
		"--actor", actor, "--reason", reason, "--authorization", authorization,
	}
	var executed bytes.Buffer
	if err := RunReview(executeArgs, &executed); err != nil {
		t.Fatalf("disposition execution refused the exact preflight-published plan_digest/inventory_revision: %v\n%s", err, executed.String())
	}
	var result ReviewRepairResult
	decodeStrictReviewJSON(t, executed.Bytes(), &result)
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Mode != ReviewRepairModeExecute || result.DispositionExecution == nil ||
		result.DispositionExecution.Status != string(reviewtransaction.CompactReclaimCommitted) ||
		result.DispositionExecution.PlanDigest != preflight.DispositionProviderInputs.PlanDigest {
		t.Fatalf("disposition execution = %#v\n%s", result, executed.String())
	}
}

func TestReviewRepairContractAndPreflightFixtureAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	for _, item := range []struct {
		name string
		id   string
	}{
		{name: "authority-repair-assessment.schema.json", id: reviewtransaction.AuthorityRepairAssessmentSchemaID},
		{name: "repair.schema.json", id: ReviewIntegrationRepairSchemaID},
	} {
		payload, err := os.ReadFile(filepath.Join(root, "schemas", item.name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(payload, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != item.id || schema["additionalProperties"] != false {
			t.Fatalf("%s header = %#v", item.name, schema)
		}
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "repair-preflight.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(fixture))
	decoder.DisallowUnknownFields()
	var result ReviewRepairResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		t.Fatal(err)
	}
	raw["maintainer_authorization"] = "must never be public"
	malformed, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoder = json.NewDecoder(bytes.NewReader(malformed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ReviewRepairResult{}); err == nil {
		t.Fatal("strict repair result accepted a completed authorization field")
	}
}

func TestReviewRepairSchemasRejectShortContractArrays(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "contracts", "review-integration", "v1"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "repair-preflight.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(fixture, &document); err != nil {
		t.Fatal(err)
	}
	compile := func(name string) *jsonschema.Schema {
		t.Helper()
		compiler := jsonschema.NewCompiler()
		for _, resourceName := range []string{"authority-repair-assessment.schema.json", "repair.schema.json", "failure.schema.json"} {
			resourcePath := filepath.Join(root, "schemas", resourceName)
			payload, readErr := os.ReadFile(resourcePath)
			if readErr != nil {
				t.Fatalf("read %s: %v", resourceName, readErr)
			}
			var resource any
			if decodeErr := json.Unmarshal(payload, &resource); decodeErr != nil {
				t.Fatalf("decode %s: %v", resourceName, decodeErr)
			}
			location := "https://gentle-ai.dev/contracts/review-integration/v1/schemas/" + resourceName
			if addErr := compiler.AddResource(location, resource); addErr != nil {
				t.Fatalf("add %s: %v", resourceName, addErr)
			}
		}
		location := "https://gentle-ai.dev/contracts/review-integration/v1/schemas/" + name
		schema, compileErr := compiler.Compile(location)
		if compileErr != nil {
			t.Fatalf("compile %s: %v", name, compileErr)
		}
		return schema
	}
	repairSchema := compile("repair.schema.json")
	if err := repairSchema.Validate(document); err != nil {
		t.Fatalf("valid repair fixture rejected: %v", err)
	}
	shortInputs := cloneReviewJSONDocument(t, document)
	shortInputs["required_inputs"] = []any{"actor", "reason"}
	if err := repairSchema.Validate(shortInputs); err == nil {
		t.Fatal("repair schema accepted an eligible preflight with only two required inputs")
	}
	assessmentSchema := compile("authority-repair-assessment.schema.json")
	assessment, ok := document["assessment"].(map[string]any)
	if !ok {
		t.Fatalf("repair assessment shape = %T", document["assessment"])
	}
	if err := assessmentSchema.Validate(assessment); err != nil {
		t.Fatalf("valid repair assessment rejected: %v", err)
	}
	shortOperations := cloneReviewJSONDocument(t, assessment)
	shortOperations["supported_operations"] = []any{"review/complete-fix"}
	if err := assessmentSchema.Validate(shortOperations); err == nil {
		t.Fatal("authority repair schema accepted only one supported operation")
	}
	failureSchema := compile("failure.schema.json")
	progress := reviewtransaction.ClassifiedAuthorityRepairExecution{
		Status: reviewtransaction.CompactReclaimCommitted, LineageID: "repair-schema-progress",
		RequestDigest: "sha256:" + strings.Repeat("d", 64), RecordIdentity: "sha256:" + strings.Repeat("e", 64),
	}
	failure := newReviewIntegrationFailure("review.repair", []string{"--lineage", progress.LineageID}, &reviewtransaction.ClassifiedAuthorityRepairProgressError{
		Progress: progress, Cause: context.DeadlineExceeded,
	})
	payload, err := json.Marshal(failure)
	if err != nil {
		t.Fatal(err)
	}
	var failureDocument map[string]any
	if err := json.Unmarshal(payload, &failureDocument); err != nil {
		t.Fatal(err)
	}
	if err := failureSchema.Validate(failureDocument); err != nil {
		t.Fatalf("valid repair progress failure rejected: %v", err)
	}
	delete(failureDocument, "progress_identity")
	if err := failureSchema.Validate(failureDocument); err == nil {
		t.Fatal("failure schema accepted repair request digest without progress identity")
	}
}

func cloneReviewJSONDocument(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(payload, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestWindowsRuntimeIncludesRepairAndMaintenanceLockRegressions(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"TestAuthorityLockCancellationPreservesSentinelAndCallerDeadline",
		"TestCompactStatusLoadContextCancelsUnderExclusiveMaintenance",
		"TestRepairClassifiedAuthorityConcurrentExecutionCommitsAndReplays",
		"TestRepairClassifiedAuthorityResumesEachDurablePhase",
	} {
		if !bytes.Contains(payload, []byte(name)) {
			t.Fatalf("Windows PR runtime allowlist is missing %s", name)
		}
	}
}
