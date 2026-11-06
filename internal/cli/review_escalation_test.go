package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

const escalationReasonFixture = "rewrites how service tokens are parsed"

var agentEscalationReason = reviewtransaction.RiskReason{
	Code: reviewtransaction.RiskReasonAgentEscalation, Signal: reviewtransaction.SignalAgentEscalation,
}

// TestReviewStartEscalatesToHighLikeAssess is S14: START honors the agent
// escalation exactly as assess does. A medium candidate is raised to high
// with the canonical 4R lenses and the agent_escalation reason, the
// escalation is frozen in the authority and its START binding, and resuming
// the lineage with another escalation is a conflict, not a silent rebind.
func TestReviewStartEscalatesToHighLikeAssess(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "escalate-start",
		"--escalate-item", "2", "--escalate-reason", escalationReasonFixture,
	}), &output); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, output.String()))
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := started.Validate(); err != nil {
		t.Fatal(err)
	}
	wantLenses := []string{
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	if started.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(started.SelectedLenses, wantLenses) ||
		len(started.RiskReasons) == 0 || started.RiskReasons[0] != agentEscalationReason {
		t.Fatalf("escalated START = risk %q lenses %v reasons %#v", started.RiskLevel, started.SelectedLenses, started.RiskReasons)
	}
	validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v2", "start-v4.schema.json"), output.Bytes())

	record := loadRequestContextRecord(t, repo, started.LineageID)
	want := reviewtransaction.CompactAgentEscalation{Item: 2, Reason: escalationReasonFixture}
	if record.State.AgentEscalation == nil || *record.State.AgentEscalation != want {
		t.Fatalf("START did not freeze the escalation: %#v", record.State.AgentEscalation)
	}
	if record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.AgentEscalation == nil ||
		*record.State.InitialAtomicStart.AgentEscalation != want {
		t.Fatalf("START binding does not carry the escalation: %#v", record.State.InitialAtomicStart)
	}

	output.Reset()
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", started.LineageID,
		"--escalate-item", "3", "--escalate-reason", escalationReasonFixture,
	}), &output)
	if err == nil || !strings.Contains(err.Error(), "atomic_start_conflict") {
		t.Fatalf("START resumed with a different escalation: %v\n%s", err, output.String())
	}
	if after := loadRequestContextRecord(t, repo, started.LineageID); after.Revision != record.Revision {
		t.Fatal("a refused resume rewrote the frozen authority")
	}
}

// TestReviewStartValidatesEscalationLikeAssess applies assess's own
// validation to START: both flags or neither, an item from 1 to 6, and a
// non-empty reason of at most 500 characters.
func TestReviewStartValidatesEscalationLikeAssess(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"item without reason", []string{"--escalate-item", "2"}, "review start --escalate-item and --escalate-reason must be passed together"},
		{"reason without item", []string{"--escalate-reason", "why"}, "review start --escalate-item and --escalate-reason must be passed together"},
		{"item out of range", []string{"--escalate-item", "7", "--escalate-reason", "why"}, `review start --escalate-item "7" must be an integer from 1 to 6`},
		{"item not a number", []string{"--escalate-item", "two", "--escalate-reason", "why"}, `review start --escalate-item "two" must be an integer from 1 to 6`},
		{"blank reason", []string{"--escalate-item", "2", "--escalate-reason", " "}, "review start --escalate-reason must be non-empty and at most 500 characters"},
		{"reason too long", []string{"--escalate-item", "2", "--escalate-reason", strings.Repeat("x", 501)}, "review start --escalate-reason must be non-empty and at most 500 characters"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(append([]string{"start", "--cwd", t.TempDir()}, tt.args...), &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("START escalation error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestEscalateReviewStartAssessmentNeverLowersATier proves the escalation
// only raises: passive and medium become high, high stays high, and every
// tier gains the one agent_escalation reason ahead of the classifier's.
func TestEscalateReviewStartAssessmentNeverLowersATier(t *testing.T) {
	escalation := &reviewtransaction.CompactAgentEscalation{Item: 4, Reason: "shares a lock with the gate"}
	classifier := reviewtransaction.RiskReason{Code: reviewtransaction.RiskReasonExecutableChange, Path: "tracked.txt"}
	for _, level := range []reviewtransaction.RiskLevel{reviewtransaction.RiskLow, reviewtransaction.RiskMedium, reviewtransaction.RiskHigh} {
		got := escalateReviewStartAssessment(reviewtransaction.RiskAssessment{
			Level: level, ChangedLines: 3, Reasons: []reviewtransaction.RiskReason{classifier},
		}, escalation)
		if got.Level != reviewtransaction.RiskHigh || got.ChangedLines != 3 ||
			!reflect.DeepEqual(got.Reasons, []reviewtransaction.RiskReason{agentEscalationReason, classifier}) {
			t.Fatalf("escalated %s assessment = %#v", level, got)
		}
	}
}

// TestReviewStartContractValidatesAgentEscalationReason is S14's contract:
// the agent_escalation reason has its own shape (its own signal, no path, no
// modes), names the consent evidence in English and Spanish, and every
// published START schema that names reason codes admits it.
func TestReviewStartContractValidatesAgentEscalationReason(t *testing.T) {
	valid := agentEscalationReason
	if err := validateReviewStartRiskReasons([]reviewtransaction.RiskReason{valid}); err != nil {
		t.Fatalf("validateReviewStartRiskReasons(agent escalation) = %v", err)
	}
	if got := reviewStartRiskLevel([]reviewtransaction.RiskReason{valid}); got != reviewtransaction.RiskHigh {
		t.Fatalf("reviewStartRiskLevel(agent escalation) = %q, want %q", got, reviewtransaction.RiskHigh)
	}
	for name, malformed := range map[string]reviewtransaction.RiskReason{
		"missing signal": {Code: valid.Code},
		"wrong signal":   {Code: valid.Code, Signal: reviewtransaction.SignalSecurity},
		"stray path":     {Code: valid.Code, Signal: valid.Signal, Path: "tracked.txt"},
		"stray modes":    {Code: valid.Code, Signal: valid.Signal, OldMode: "100644", NewMode: "100755"},
	} {
		if err := validateReviewStartRiskReasons([]reviewtransaction.RiskReason{malformed}); err == nil {
			t.Fatalf("validateReviewStartRiskReasons accepted malformed %s reason", name)
		}
	}
	for locale, got := range map[string][]string{
		"English": reviewConsentEvidencePhrases([]reviewtransaction.RiskReason{valid}),
		"Spanish": reviewConsentSpanishEvidencePhrases([]reviewtransaction.RiskReason{valid}),
	} {
		want := map[string]string{
			"English": "the agent that made this change flagged it as high risk",
			"Spanish": "el agente que hizo este cambio lo marcó como de alto riesgo",
		}[locale]
		if !reflect.DeepEqual(got, []string{want}) {
			t.Fatalf("%s agent-escalation evidence = %#v, want %#v", locale, got, []string{want})
		}
	}
	reason, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []struct{ version, name string }{
		{"v1", "start-v2.schema.json"}, {"v2", "start.schema.json"}, {"v2", "start-v4.schema.json"},
	} {
		validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, schema.version, schema.name+"#/$defs/risk_reason"), reason)
	}
}

// TestReviewStartRefusesRepeatedEscalation keeps the START binding allowlist
// closed: a second item or reason would silently replace the first.
func TestReviewStartRefusesRepeatedEscalation(t *testing.T) {
	for _, flag := range []string{"escalate-item", "escalate-reason"} {
		err := validateReviewStartBinding([]string{"--" + flag, "1", "--" + flag, "2"}, false, "", "workspace", "", "", false, false, "", "", "")
		if err == nil || !strings.Contains(err.Error(), "repeats --"+flag) {
			t.Fatalf("repeated --%s error = %v", flag, err)
		}
	}
}

// TestReviewConsentFollowUpKeepsEscalation proves the relayed consent answer
// reruns START with the same escalation, never silently without it.
func TestReviewConsentFollowUpKeepsEscalation(t *testing.T) {
	if reviewEscalationFollowUpArguments(nil) != "" {
		t.Fatal("absent escalation changed the consent follow-up")
	}
	got := reviewEscalationFollowUpArguments(&reviewtransaction.CompactAgentEscalation{Item: 2, Reason: "token parsing changed"})
	if got != " --escalate-item 2 --escalate-reason 'token parsing changed'" {
		t.Fatalf("consent follow-up escalation = %q", got)
	}
}

// TestReviewRecoverInheritsAgentEscalation proves recovery, which re-assesses
// risk, keeps the predecessor's escalation instead of dropping to the
// classifier's tier.
func TestReviewRecoverInheritsAgentEscalation(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStartWith(t, repo, "escalate-recover", "--escalate-item", "2", "--escalate-reason", escalationReasonFixture)
	escalateReviewForRecovery(t, repo, ReviewFacadeStartResult{
		LineageID: started.LineageID, TargetIdentity: started.RepositoryContext.TargetIdentity, SelectedLenses: started.SelectedLenses,
	})
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfixed\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	predecessor := loadRequestContextRecord(t, repo, started.LineageID)
	var output bytes.Buffer
	if err := RunReview([]string{
		"recover", "--cwd", repo, "--predecessor-lineage", started.LineageID,
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "escalate-successor",
		"--disposition", string(reviewtransaction.RecoveryEscalated),
	}, &output); err != nil {
		t.Fatalf("recover: %v\n%s", err, output.String())
	}
	successor := loadRequestContextRecord(t, repo, "escalate-successor")
	if successor.State.AgentEscalation == nil || *successor.State.AgentEscalation != *predecessor.State.AgentEscalation ||
		successor.State.RiskLevel != reviewtransaction.RiskHigh {
		t.Fatalf("recovered successor lost the escalation: risk %q escalation %#v", successor.State.RiskLevel, successor.State.AgentEscalation)
	}
}

// TestReviewStartWithoutEscalationIsUnchanged is the PRESERVE contract: no
// escalate flags means the classifier's tier, no escalation reason, and no
// escalation field in the persisted authority bytes.
func TestReviewStartWithoutEscalationIsUnchanged(t *testing.T) {
	repo, _, started := startRequestContextReview(t, "escalate-absent")
	if started.RiskLevel != reviewtransaction.RiskMedium || len(started.SelectedLenses) != 1 {
		t.Fatalf("START without escalation = risk %q lenses %v", started.RiskLevel, started.SelectedLenses)
	}
	for _, reason := range started.RiskReasons {
		if reason.Code == reviewtransaction.RiskReasonAgentEscalation {
			t.Fatalf("START without escalation published %#v", reason)
		}
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "agent_escalation") {
		t.Fatalf("START without escalation persisted an escalation field:\n%s", payload)
	}
}
