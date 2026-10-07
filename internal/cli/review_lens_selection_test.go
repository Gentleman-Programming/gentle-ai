package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

const lensSelectionReasonFixture = "changes credential parsing and the retry loop around the token store"

// TestReviewStartRunsTheLensesTheAgentSelected is verify-always-rdd-high S8:
// the agent chooses the pertinent 4R lenses from what it touched and how, on
// any reviewed tier. START runs exactly those lenses in canonical order,
// freezes the selection reason with the authority and its START binding, and
// resuming the lineage with another selection is a conflict.
func TestReviewStartRunsTheLensesTheAgentSelected(t *testing.T) {
	for _, tt := range []struct {
		name, lineage string
		extra         []string
		wantRisk      reviewtransaction.RiskLevel
		wantLenses    []string
	}{
		{
			name: "medium candidate, two lenses", lineage: "lens-selection-medium",
			extra:    []string{"--lenses", "reliability,risk"},
			wantRisk: reviewtransaction.RiskMedium, wantLenses: []string{reviewtransaction.LensRisk, reviewtransaction.LensReliability},
		},
		{
			name: "high candidate, one lens", lineage: "lens-selection-high",
			extra:    []string{"--lenses", "review-resilience", "--escalate-item", "2", "--escalate-reason", escalationReasonFixture},
			wantRisk: reviewtransaction.RiskHigh, wantLenses: []string{reviewtransaction.LensResilience},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
			args := append([]string{
				"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", tt.lineage,
				"--lenses-reason", lensSelectionReasonFixture,
			}, tt.extra...)
			var output bytes.Buffer
			if err := RunReview(boundNegotiatedStartArgs(t, args), &output); err != nil {
				t.Fatal(negotiatedReviewStartFailure(err, output.String()))
			}
			started := decodeNegotiatedReviewStart(t, output.Bytes())
			if err := started.Validate(); err != nil {
				t.Fatal(err)
			}
			if started.RiskLevel != tt.wantRisk || !reflect.DeepEqual(started.SelectedLenses, tt.wantLenses) {
				t.Fatalf("selected START = risk %q lenses %v, want %q %v", started.RiskLevel, started.SelectedLenses, tt.wantRisk, tt.wantLenses)
			}
			validatePublishedReviewSchema(t, compileWholePublishedReviewSchema(t, "v2", "start-v4.schema.json"), output.Bytes())

			record := loadRequestContextRecord(t, repo, started.LineageID)
			if record.State.LensSelectionReason != lensSelectionReasonFixture || !reflect.DeepEqual(record.State.SelectedLenses, tt.wantLenses) {
				t.Fatalf("START did not freeze the selection: lenses %v reason %q", record.State.SelectedLenses, record.State.LensSelectionReason)
			}
			if record.State.InitialAtomicStart == nil || record.State.InitialAtomicStart.LensSelectionReason != lensSelectionReasonFixture {
				t.Fatalf("START binding does not carry the selection reason: %#v", record.State.InitialAtomicStart)
			}

			output.Reset()
			conflicting := append([]string{
				"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", started.LineageID,
				"--lenses", "readability", "--lenses-reason", lensSelectionReasonFixture,
			}, tt.extra[2:]...)
			err := RunReview(boundNegotiatedStartArgs(t, conflicting), &output)
			if err == nil || !strings.Contains(err.Error(), "atomic_start_conflict") {
				t.Fatalf("START resumed with a different selection: %v\n%s", err, output.String())
			}
			if after := loadRequestContextRecord(t, repo, started.LineageID); after.Revision != record.Revision {
				t.Fatal("a refused resume rewrote the frozen authority")
			}
		})
	}
}

// TestReviewStartValidatesLensSelection: both flags or neither, at least one
// known lens without repeats, a bounded non-empty reason, and never together
// with --focus. Each refusal names how to rerun and writes nothing.
func TestReviewStartValidatesLensSelection(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"lenses without reason", []string{"--lenses", "risk"}, "review start --lenses and --lenses-reason must be passed together"},
		{"reason without lenses", []string{"--lenses-reason", "why"}, "review start --lenses and --lenses-reason must be passed together"},
		{"unknown lens", []string{"--lenses", "risk,security", "--lenses-reason", "why"}, `review start --lenses names unknown lens "security"`},
		{"empty lens", []string{"--lenses", "risk,", "--lenses-reason", "why"}, `review start --lenses names unknown lens ""`},
		{"repeated lens", []string{"--lenses", "risk,review-risk", "--lenses-reason", "why"}, `review start --lenses repeats lens "review-risk"`},
		{"blank reason", []string{"--lenses", "risk", "--lenses-reason", " "}, "review start --lenses-reason must be non-empty and at most 500 characters"},
		{"reason too long", []string{"--lenses", "risk", "--lenses-reason", strings.Repeat("x", 501)}, "review start --lenses-reason must be non-empty and at most 500 characters"},
		{"with focus", []string{"--lenses", "risk", "--lenses-reason", "why", "--focus", "risk"}, "review start --lenses replaces --focus"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Parsed before the repository is resolved, so nothing is written.
			var output bytes.Buffer
			err := RunReview(append([]string{"start", "--cwd", t.TempDir()}, tt.args...), &output)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "gentle-ai review start") {
				t.Fatalf("START selection error = %v, want %q and a rerun command", err, tt.want)
			}
		})
	}
}

// TestReviewStartRefusesRepeatedLensSelection keeps the START binding
// allowlist closed: a second selection would silently replace the first.
func TestReviewStartRefusesRepeatedLensSelection(t *testing.T) {
	for _, flag := range []string{"lenses", "lenses-reason"} {
		err := validateReviewStartBinding([]string{"--" + flag, "risk", "--" + flag, "reliability"}, false, "", "workspace", "", "", false, false, "", "", "")
		if err == nil || !strings.Contains(err.Error(), "repeats --"+flag) {
			t.Fatalf("repeated --%s error = %v", flag, err)
		}
	}
}

// TestReviewConsentFollowUpKeepsLensSelection proves the relayed consent
// answer reruns START with the same selection, never silently without it.
func TestReviewConsentFollowUpKeepsLensSelection(t *testing.T) {
	if reviewLensSelectionFollowUpArguments(nil) != "" {
		t.Fatal("absent selection changed the consent follow-up")
	}
	got := reviewLensSelectionFollowUpArguments(&reviewLensSelection{
		Lenses: []string{reviewtransaction.LensRisk, reviewtransaction.LensReliability}, Reason: "token parsing changed",
	})
	if got != " --lenses review-risk,review-reliability --lenses-reason 'token parsing changed'" {
		t.Fatalf("consent follow-up selection = %q", got)
	}
}

// TestReviewRecoverInheritsLensSelection proves recovery, which re-assesses
// risk, keeps the predecessor's selection and reason instead of falling back
// to the tier default.
func TestReviewRecoverInheritsLensSelection(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\none\ntwo\nthree\nfour\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := runNegotiatedReviewStartWith(t, repo, "lens-selection-recover", "--lenses", "risk,readability", "--lenses-reason", lensSelectionReasonFixture)
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
		"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", "lens-selection-successor",
		"--disposition", string(reviewtransaction.RecoveryEscalated),
	}, &output); err != nil {
		t.Fatalf("recover: %v\n%s", err, output.String())
	}
	successor := loadRequestContextRecord(t, repo, "lens-selection-successor")
	if !reflect.DeepEqual(successor.State.SelectedLenses, predecessor.State.SelectedLenses) ||
		successor.State.LensSelectionReason != lensSelectionReasonFixture {
		t.Fatalf("recovered successor lost the selection: lenses %v reason %q", successor.State.SelectedLenses, successor.State.LensSelectionReason)
	}
}

// TestReviewStartWithoutLensSelectionIsUnchanged is the phase-A PRESERVE
// contract: no selection keeps today's tier default (one reliability lens for
// medium) and persists no selection field.
func TestReviewStartWithoutLensSelectionIsUnchanged(t *testing.T) {
	repo, _, started := startRequestContextReview(t, "lens-selection-absent")
	if started.RiskLevel != reviewtransaction.RiskMedium || !reflect.DeepEqual(started.SelectedLenses, []string{reviewtransaction.LensReliability}) {
		t.Fatalf("START without selection = risk %q lenses %v", started.RiskLevel, started.SelectedLenses)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(t.Context(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "lens_selection_reason") {
		t.Fatalf("START without selection persisted a selection field:\n%s", payload)
	}
}

// TestReviewStatusPreflightCarriesLensSelectionIntoStart: a runtime that only
// executes provider-issued tokens passes the selection to STATUS, and the
// START that STATUS renders freezes exactly those lenses and reason. Invalid
// selections are refused by STATUS before any authority exists.
func TestReviewStatusPreflightCarriesLensSelectionIntoStart(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	if _, payload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo, "--lenses", "risk"); err == nil {
		t.Fatalf("STATUS accepted an unpaired selection:\n%s", payload)
	} else if failure := decodeReviewIntegrationFailure(t, payload); failure.Code != "invalid_request" ||
		!strings.Contains(failure.Cause, "review status --lenses and --lenses-reason must be passed together") ||
		!strings.Contains(failure.Cause, "gentle-ai review status --contract") {
		t.Fatalf("unpaired STATUS selection refusal = %#v", failure)
	}
	status, payload, err := runStartPreflightStatus(t, ReviewIntegrationContractV2, repo,
		"--lenses", "readability,risk", "--lenses-reason", lensSelectionReasonFixture)
	if err != nil {
		t.Fatalf("STATUS preflight refused a valid selection: %v\n%s", err, payload)
	}
	validatePublishedReviewSchema(t, compileWholeNativeStatusSchema(t, "status-v9.schema.json"), payload)
	lineage := status.NextTransition.Execute.Binding.LineageID
	var output bytes.Buffer
	if err := RunReview(startPreflightTokens(status), &output); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, output.String()))
	}
	var question ReviewIntegrationConsentResult
	decodeStrictReviewJSON(t, output.Bytes(), &question)
	if question.Action == "consent_required" {
		words := reviewShellWords(t, question.Choices[0].Invocation)
		output.Reset()
		if err := RunReview(words[2:], &output); err != nil {
			t.Fatal(negotiatedReviewStartFailure(err, output.String()))
		}
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	want := []string{reviewtransaction.LensRisk, reviewtransaction.LensReadability}
	if started.LineageID != lineage || !reflect.DeepEqual(started.SelectedLenses, want) {
		t.Fatalf("START from the preflighted vector = lineage %q lenses %v, want %q %v", started.LineageID, started.SelectedLenses, lineage, want)
	}
	if record := loadRequestContextRecord(t, repo, lineage); record.State.LensSelectionReason != lensSelectionReasonFixture {
		t.Fatalf("START did not freeze the preflighted selection reason: %q", record.State.LensSelectionReason)
	}
}
