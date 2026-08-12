package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReviewFacadeStartHighRiskCarriesConsentEvidencePhrases proves the
// non-interactive START result relays the same human evidence phrases the
// interactive consent prompt speaks, derived from the one shared helper, so a
// headless agent can explain WHY the deeper review was selected.
func TestReviewFacadeStartHighRiskCarriesConsentEvidencePhrases(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'candidate'\n", 0o644)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-high"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.RiskLevel != reviewtransaction.RiskHigh {
		t.Fatalf("service-token start risk = %q, want high", started.RiskLevel)
	}
	want := []string{"service credentials in service-token.ts"}
	if !reflect.DeepEqual(started.RiskEvidence, want) {
		t.Fatalf("start risk_evidence = %#v, want %#v", started.RiskEvidence, want)
	}
	// Prompt parity: the phrases must be exactly what the interactive consent
	// prompt would say for the same assessed candidate, from the same helper.
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intended,
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := builder.AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(started.RiskEvidence, reviewConsentEvidencePhrases(assessment.Reasons)) {
		t.Fatalf("start risk_evidence %#v does not derive from the shared consent helper %#v",
			started.RiskEvidence, reviewConsentEvidencePhrases(assessment.Reasons))
	}
	prompt := reviewConsentPrompt(assessment)
	for _, phrase := range started.RiskEvidence {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("interactive consent prompt %q does not speak phrase %q", prompt, phrase)
		}
	}
}

// TestReviewFacadeStartMediumRiskCarriesConsentReason proves a tier-1 start
// relays the same consolidated-review reason the interactive consent prompt
// speaks, so a headless agent can explain WHY a review is wanted. Issue #1822:
// the phrase must come from the one shared wording source, never a second copy.
// Issue #1827: the reason alone is not enough — the start must also name the
// evidence path that made the candidate non-passive.
func TestReviewFacadeStartMediumRiskCarriesConsentReason(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "view.go", "package view\n\nconst label = \"candidate\"\n", 0o644)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-medium"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.RiskLevel != reviewtransaction.RiskMedium {
		t.Fatalf("plain-source start risk = %q, want medium", started.RiskLevel)
	}
	if len(started.RiskEvidence) == 0 || started.RiskEvidence[0] != reviewConsentMediumReason {
		t.Fatalf("medium start risk_evidence = %#v, want the consolidated-review reason first", started.RiskEvidence)
	}
	want := "an executable change in view.go"
	if !reflect.DeepEqual(started.RiskEvidence[1:], []string{want}) {
		t.Fatalf("medium start risk_evidence = %#v, want the evidence phrase %q after the reason", started.RiskEvidence, want)
	}
	// Prompt parity: every phrase the START result carries must be spoken by
	// the interactive consent prompt for the same assessed candidate.
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	intended, err := builder.DiscoverUnignoredUntracked(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionWorkspace, IntendedUntracked: intended,
	})
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := builder.AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	prompt := reviewConsentPrompt(assessment)
	for _, phrase := range started.RiskEvidence {
		if !strings.Contains(prompt, phrase) {
			t.Fatalf("interactive consent prompt %q does not speak phrase %q", prompt, phrase)
		}
	}
}

// TestReviewConsentRiskEvidenceMediumNamesEvidencePath pins issue #1827: the
// medium tier keeps the consolidated-review sentence first and appends the
// same evidence phrases the high tier already projects, so the START result
// names WHICH path made the candidate non-passive.
func TestReviewConsentRiskEvidenceMediumNamesEvidencePath(t *testing.T) {
	assessment := reviewtransaction.RiskAssessment{
		Level: reviewtransaction.RiskMedium,
		Reasons: []reviewtransaction.RiskReason{
			{Code: reviewtransaction.RiskReasonExecutableChange, Path: "internal/counter/counter.go"},
		},
	}

	want := []string{reviewConsentMediumReason, "an executable change in internal/counter/counter.go"}
	if got := reviewConsentRiskEvidence(assessment); !reflect.DeepEqual(got, want) {
		t.Fatalf("medium risk_evidence = %#v, want %#v", got, want)
	}

	// The interactive Why line speaks the same facts from the same helpers.
	reason := reviewConsentReason(assessment)
	for _, phrase := range want {
		if !strings.Contains(reason, phrase) {
			t.Fatalf("consent reason %q does not speak phrase %q", reason, phrase)
		}
	}
}

// TestReviewConsentRiskEvidenceMediumWithoutSpeakableReasonsStaysSingle proves
// the fallback shape is untouched: a medium assessment with no speakable
// evidence still carries exactly the one consolidated-review reason.
func TestReviewConsentRiskEvidenceMediumWithoutSpeakableReasonsStaysSingle(t *testing.T) {
	assessment := reviewtransaction.RiskAssessment{Level: reviewtransaction.RiskMedium}

	want := []string{reviewConsentMediumReason}
	if got := reviewConsentRiskEvidence(assessment); !reflect.DeepEqual(got, want) {
		t.Fatalf("medium risk_evidence without speakable reasons = %#v, want %#v", got, want)
	}
	if reason := reviewConsentReason(assessment); reason != reviewConsentMediumReason {
		t.Fatalf("consent reason without speakable reasons = %q, want %q", reason, reviewConsentMediumReason)
	}
}

// TestReviewConsentRiskEvidenceHighTierUnchanged guards the tier-2 projection:
// high evidence stays the bare phrases, never the consolidated-review sentence.
func TestReviewConsentRiskEvidenceHighTierUnchanged(t *testing.T) {
	assessment := reviewtransaction.RiskAssessment{
		Level: reviewtransaction.RiskHigh,
		Reasons: []reviewtransaction.RiskReason{
			{Code: reviewtransaction.RiskReasonServiceToken, Signal: reviewtransaction.SignalAuth, Path: "service-token.ts"},
		},
	}

	want := []string{"service credentials in service-token.ts"}
	if got := reviewConsentRiskEvidence(assessment); !reflect.DeepEqual(got, want) {
		t.Fatalf("high risk_evidence = %#v, want %#v", got, want)
	}
}

// TestReviewFacadeStartDocsOnlyOmitsRiskEvidence proves a tier-0 start carries
// no risk_evidence key at all: absence, not empty-string noise.
func TestReviewFacadeStartDocsOnlyOmitsRiskEvidence(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "docs/guide.md", "passive documentation\n", 0o644)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-docs"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.RiskLevel != reviewtransaction.RiskLow {
		t.Fatalf("docs-only start risk = %q, want low", started.RiskLevel)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["risk_evidence"]; ok {
		t.Fatalf("tier-0 start leaked risk_evidence: %s", output.String())
	}
}

// TestReviewFacadeStartResultOmitsAdditiveFieldsWhenAbsent guards the omitempty
// contract: consumers of the previous shape see byte-identical output when no
// evidence and no hint apply.
func TestReviewFacadeStartResultOmitsAdditiveFieldsWhenAbsent(t *testing.T) {
	payload, err := json.Marshal(ReviewFacadeStartResult{
		Operation: "review/start", Action: "created", LineageID: "shape-guard",
		SelectedLenses: []string{}, LensBindings: []ReviewFacadeLensBinding{},
		Projection: reviewtransaction.ProjectionWorkspace, ChangedFiles: 1, ChangedLines: 1, CorrectionBudget: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"risk_evidence", "hint"} {
		if bytes.Contains(payload, []byte(field)) {
			t.Fatalf("absent additive field %q still serialized: %s", field, payload)
		}
	}
}

// TestReviewFacadeStartEmptyCandidateHintsBaseRef proves the committed-work
// recovery path is discoverable: a clean worktree yields an empty candidate,
// and the result itself names --base-ref as the way to review committed work.
func TestReviewFacadeStartEmptyCandidateHintsBaseRef(t *testing.T) {
	repo := initReviewCLIRepo(t)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-empty"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.ChangedFiles != 0 {
		t.Fatalf("clean-worktree start changed_files = %d, want 0", started.ChangedFiles)
	}
	if started.Hint != reviewStartEmptyCandidateHint || !strings.Contains(started.Hint, "--base-ref") {
		t.Fatalf("empty-candidate start hint = %q, want %q", started.Hint, reviewStartEmptyCandidateHint)
	}
}

// TestReviewFacadeStartNonEmptyCandidateWithoutLensesHasNoHint proves the hint
// stays scoped to its two typed cases (empty candidate; lenses required on the
// unnegotiated form) and never becomes ambient noise: a real, non-empty
// candidate that is low risk and selects zero lenses has nothing to hint
// about, so no hint field appears. (A non-empty candidate that DOES require
// lenses now carries the negotiated-contract hint — see
// TestReviewFacadeStartLensesRequiredHintsNegotiatedContract — which used to
// be this test's fixture before that gap was fixed.)
func TestReviewFacadeStartNonEmptyCandidateWithoutLensesHasNoHint(t *testing.T) {
	repo := initReviewCLIRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeReviewStartCandidate(t, repo, "docs/guide.md", "passive documentation\n", 0o644)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-nonempty"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.RiskLevel != reviewtransaction.RiskLow || started.LensesRequired {
		t.Fatalf("docs-only start risk/lenses_required = %q/%v, want low/false", started.RiskLevel, started.LensesRequired)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["hint"]; ok {
		t.Fatalf("non-empty low-risk start leaked hint: %s", output.String())
	}
}

// TestReviewFacadeStartLensesRequiredHintsNegotiatedContract proves the
// unnegotiated summary is self-describing: when lenses are required, this
// response cannot itself carry the frozen tree/changed_path_manifest
// (that payload only exists on the negotiated form), so the hint must name
// the exact --contract invocation — reusing this response's own
// target_identity and projection — that returns it. This closes the reported
// gap where a caller who obeys the reviewer contract, but invoked the plain
// form, was blocked by a refusal that never mentioned the negotiated form.
func TestReviewFacadeStartLensesRequiredHintsNegotiatedContract(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'candidate'\n", 0o644)
	var output bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--lineage", "evidence-hint-contract"}, &output); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if !started.LensesRequired || len(started.SelectedLenses) == 0 {
		t.Fatalf("service-token start lenses_required = %v, selected_lenses = %v, want lenses selected", started.LensesRequired, started.SelectedLenses)
	}
	// The direct route refuses --agent, so this caller never declared a
	// runtime and the hint must omit the complete agent segment (issue #2885).
	wantCommand := fmt.Sprintf("gentle-ai review start --contract %s --target %s --projection %s", ReviewIntegrationContractV2, started.TargetIdentity, started.Projection)
	if !strings.Contains(started.Hint, wantCommand) {
		t.Fatalf("lenses-required start hint = %q, want it to contain %q", started.Hint, wantCommand)
	}
}

// TestReviewFacadeStartBaseDiffHintReplaysFrozenSelector proves the hint-replay
// contract for a base-diff START that ALREADY has legacy (compact-v2)
// authority: replaying the hint's named negotiated command resolves into the
// SAME frozen lineage rather than creating a second one, and a stale replay
// (after the candidate moved) is refused with nothing new persisted. This is
// the "replay this hint verbatim" workflow the Wave 7 v2-collision start
// guard (runReviewFacadeStart) explicitly carves an exact-content exception
// for. Since issue #2447 (see the sibling
// TestReviewFacadeStartBaseDiffRefusalReplaysFrozenSelector below), a direct
// CLI START can no longer create this authority itself for a lens-selecting
// base-diff candidate, so this test constructs it directly via
// runLegacyFacadeStartForTest and starts the replay from an authority that
// already exists -- the complementary starting condition to the sibling test,
// which starts with none.
func TestReviewFacadeStartBaseDiffHintReplaysFrozenSelector(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "dependency.go", "package dependency\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "dependency.go")
	runReviewCLIGit(t, repo, "commit", "-m", "feature dependency")
	runReviewCLIGit(t, repo, "branch", "feature-base")
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'candidate'\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "service-token.ts")
	runReviewCLIGit(t, repo, "commit", "-m", "feature candidate")

	var plain bytes.Buffer
	if err := runLegacyFacadeStartForTest(t, []string{"--cwd", repo, "--base-ref", "feature-base", "--committed-only"}, &plain); err != nil {
		t.Fatal(err)
	}
	var started ReviewFacadeStartResult
	decodeStrictReviewJSON(t, plain.Bytes(), &started)
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	baseTree := record.State.InitialSnapshot.BaseTree
	if !strings.Contains(started.Hint, "--base-ref "+baseTree+" --committed-only") || strings.Contains(started.Hint, "--base-ref feature-base") {
		t.Fatalf("base-diff hint did not carry the immutable resolved selector: %q", started.Hint)
	}

	opening := strings.IndexByte(started.Hint, '`')
	closing := strings.IndexByte(started.Hint[opening+1:], '`')
	if opening < 0 || closing < 0 {
		t.Fatalf("hint has no executable command: %q", started.Hint)
	}
	command := strings.Fields(started.Hint[opening+1 : opening+1+closing])
	if len(command) < 4 || !reflect.DeepEqual(command[:3], []string{"gentle-ai", "review", "start"}) {
		t.Fatalf("hint command = %v", command)
	}
	args := append([]string{"start", "--cwd", repo}, withoutReplayRuntimeIdentity(t, command[3:])...)
	var replay bytes.Buffer
	if err := RunReview(args, &replay); err != nil {
		t.Fatalf("hinted negotiated START failed: %v\n%s", err, replay.String())
	}
	var negotiated ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, replay.Bytes(), &negotiated)
	if negotiated.RepositoryContext == nil || negotiated.RepositoryContext.TargetIdentity != started.TargetIdentity || negotiated.LineageID != started.LineageID {
		t.Fatalf("hint replay selected context/lineage %#v/%q, want target %q lineage %q", negotiated.RepositoryContext, negotiated.LineageID, started.TargetIdentity, started.LineageID)
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil || len(stores) != 1 {
		t.Fatalf("hint replay authorities = %d, %v; want exactly one", len(stores), err)
	}

	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'mutated'\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "service-token.ts")
	runReviewCLIGit(t, repo, "commit", "-m", "mutate candidate")
	var refused bytes.Buffer
	if err := RunReview(args, &refused); err == nil {
		t.Fatalf("stale hinted START succeeded: %s", refused.String())
	}
	failure := decodeReviewIntegrationFailure(t, refused.Bytes())
	if failure.Code != reviewPreflightStaleTargetCode {
		t.Fatalf("mutated hinted START code = %q, want %q", failure.Code, reviewPreflightStaleTargetCode)
	}
	stores, err = reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil || len(stores) != 1 {
		t.Fatalf("stale hint created authority: stores=%d error=%v", len(stores), err)
	}
}

// TestReviewFacadeStartBaseDiffRefusalReplaysFrozenSelector is the
// complementary starting condition to
// TestReviewFacadeStartBaseDiffHintReplaysFrozenSelector above: no legacy
// authority exists yet, because issue #2447 made a direct (non-negotiated)
// base-diff START whose candidate selects lenses refuse up front, before
// anything is persisted (see runReviewFacadeStart), naming the exact
// negotiated `review start` continuation. This test proves that refusal's
// named continuation resolves the mutable `feature-base` ref into its
// immutable tree BEFORE anything is created, that running it verbatim
// creates exactly one fresh negotiated authority, and that a stale replay
// (after the candidate moved) is refused with nothing persisted, exactly
// like any other negotiated START would be.
func TestReviewFacadeStartBaseDiffRefusalReplaysFrozenSelector(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "dependency.go", "package dependency\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "dependency.go")
	runReviewCLIGit(t, repo, "commit", "-m", "feature dependency")
	runReviewCLIGit(t, repo, "branch", "feature-base")
	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'candidate'\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "service-token.ts")
	runReviewCLIGit(t, repo, "commit", "-m", "feature candidate")
	baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "feature-base^{tree}"))

	var refusedFirst bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", "feature-base", "--committed-only"}, &refusedFirst)
	if err == nil {
		t.Fatalf("direct base-diff start with lenses required succeeded = %s, want an up-front refusal", refusedFirst.String())
	}
	if stores, storesErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo); storesErr != nil || len(stores) != 0 {
		t.Fatalf("refused direct start persisted authority: stores=%d error=%v", len(stores), storesErr)
	}
	if !strings.Contains(err.Error(), "--base-ref "+baseTree+" --committed-only") || strings.Contains(err.Error(), "--base-ref feature-base") {
		t.Fatalf("refusal did not name the immutable resolved selector: %v", err)
	}

	opening := strings.IndexByte(err.Error(), '`')
	closing := strings.IndexByte(err.Error()[opening+1:], '`')
	if opening < 0 || closing < 0 {
		t.Fatalf("refusal has no executable command: %v", err)
	}
	command := strings.Fields(err.Error()[opening+1 : opening+1+closing])
	if len(command) < 3 || !reflect.DeepEqual(command[:3], []string{"gentle-ai", "review", "start"}) {
		t.Fatalf("refusal command = %v", command)
	}
	args := append([]string{"start", "--cwd", repo}, withoutReplayRuntimeIdentity(t, command[3:])...)
	var replay bytes.Buffer
	if err := RunReview(args, &replay); err != nil {
		t.Fatalf("named negotiated START failed: %v\n%s", err, replay.String())
	}
	var negotiated ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, replay.Bytes(), &negotiated)
	if negotiated.RepositoryContext == nil {
		t.Fatalf("named negotiated START carried no repository_context: %#v", negotiated)
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil || len(stores) != 1 {
		t.Fatalf("named negotiated START authorities = %d, %v; want exactly one", len(stores), err)
	}

	writeReviewStartCandidate(t, repo, "service-token.ts", "export const token = 'mutated'\n", 0o644)
	runReviewCLIGit(t, repo, "add", "--", "service-token.ts")
	runReviewCLIGit(t, repo, "commit", "-m", "mutate candidate")
	var refused bytes.Buffer
	if err := RunReview(args, &refused); err == nil {
		t.Fatalf("stale named START succeeded: %s", refused.String())
	}
	failure := decodeReviewIntegrationFailure(t, refused.Bytes())
	if failure.Code != reviewPreflightStaleTargetCode {
		t.Fatalf("mutated named START code = %q, want %q", failure.Code, reviewPreflightStaleTargetCode)
	}
	stores, err = reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil || len(stores) != 1 {
		t.Fatalf("stale replay created authority: stores=%d error=%v", len(stores), err)
	}
}
