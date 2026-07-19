package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

func TestNegotiatedReviewStartMatchesVersionedFixture(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", "review-start-fixture",
	}, &output); err != nil {
		t.Fatal(err)
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	wantReasons := []reviewtransaction.RiskReason{{
		Code: reviewtransaction.RiskReasonShellSource, Signal: reviewtransaction.SignalShellProcess, Path: "scripts/deploy.sh",
	}}
	wantLenses := []string{
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	if result.Schema != ReviewIntegrationStartSchema || result.Contract != ReviewIntegrationContractV1 ||
		result.Operation != "review.start" || result.Action != "created" || !result.LensesRequired ||
		result.LineageID != "review-start-fixture" || result.State != reviewtransaction.StateReviewing ||
		result.RiskLevel != reviewtransaction.RiskHigh || !reflect.DeepEqual(result.SelectedLenses, wantLenses) ||
		result.Projection != reviewtransaction.ProjectionWorkspace || result.ChangedFiles != 1 ||
		result.ChangedLines != 1 || result.CorrectionBudget != 1 || !reflect.DeepEqual(result.RiskReasons, wantReasons) {
		t.Fatalf("negotiated START = %#v\n%s", result, output.String())
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v1", "fixtures", "start.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), fixture) {
		t.Fatalf("START fixture mismatch:\ngot=%s\nwant=%s", output.String(), fixture)
	}
}

func TestNegotiatedReviewStartRiskReasonsUseOnlyImmutableSnapshotEvidence(t *testing.T) {
	t.Run("mode-only executable transition", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Git worktree executable-bit transitions are POSIX-only")
		}
		repo := initReviewCLIRepo(t)
		if runtime.GOOS != "windows" {
			if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		result := runNegotiatedReviewStart(t, repo, "review-start-mode")
		want := []reviewtransaction.RiskReason{{
			Code: reviewtransaction.RiskReasonExecutableMode, Signal: reviewtransaction.SignalPermissions,
			Path: "tracked.txt", OldMode: "100644", NewMode: "100755",
		}}
		if result.RiskLevel != reviewtransaction.RiskHigh || result.ChangedLines != 0 ||
			!reflect.DeepEqual(result.RiskReasons, want) {
			t.Fatalf("mode-only negotiated START = %#v", result)
		}
	})

	t.Run("canonical stable sorting", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
		writeReviewStartCandidate(t, repo, "security/check.go", "package security\n", 0o644)
		result := runNegotiatedReviewStart(t, repo, "review-start-sorted")
		want := []reviewtransaction.RiskReason{
			{Code: reviewtransaction.RiskReasonHotPath, Signal: reviewtransaction.SignalSecurity, Path: "security/check.go"},
			{Code: reviewtransaction.RiskReasonShellSource, Signal: reviewtransaction.SignalShellProcess, Path: "scripts/deploy.sh"},
		}
		if runtime.GOOS != "windows" {
			want = append([]reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonExecutableMode, Signal: reviewtransaction.SignalPermissions, Path: "tracked.txt", OldMode: "100644", NewMode: "100755"}}, want...)
		}
		if !reflect.DeepEqual(result.RiskReasons, want) {
			t.Fatalf("canonical risk reasons = %#v, want %#v", result.RiskReasons, want)
		}
	})

	t.Run("semantic filename near misses", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "internal/model-provider-profile-data-exposure-data-loss.go", "package internal\n", 0o644)
		writeReviewStartCandidate(t, repo, "scripts/deploy.sh.txt", "not shell source\n", 0o644)
		writeReviewStartCandidate(t, repo, "tokens/service-tokenizer.go", "package tokens\n", 0o644)
		result := runNegotiatedReviewStart(t, repo, "review-start-near-miss")
		if result.RiskLevel != reviewtransaction.RiskMedium ||
			!reflect.DeepEqual(result.SelectedLenses, []string{reviewtransaction.LensReliability}) {
			t.Fatalf("near-miss tier/lenses = %q/%v", result.RiskLevel, result.SelectedLenses)
		}
		for _, reason := range result.RiskReasons {
			if reason.Signal == reviewtransaction.SignalDataExposure || reason.Signal == reviewtransaction.SignalDataLoss ||
				reason.Code == reviewtransaction.RiskReasonShellSource || reason.Code == reviewtransaction.RiskReasonServiceToken {
				t.Fatalf("near-miss filename created semantic reason %#v", reason)
			}
		}
		payload, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		forbidden := map[string]struct{}{"model": {}, "provider": {}, "profile": {}}
		if field := findCapabilityForbiddenField(document, forbidden); field != "" {
			t.Fatalf("negotiated START exposed classifier input field %q", field)
		}
	})
}

func TestNegotiatedReviewStartRoutesLargePureDocumentationToReadability(t *testing.T) {
	full4R := []string{
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	tests := []struct {
		name       string
		path       string
		lines      int
		prefix     string
		focus      string
		wantRisk   reviewtransaction.RiskLevel
		wantLenses []string
	}{
		{name: "400 pure doc lines remain low", path: "docs/guide.md", lines: 400, wantRisk: reviewtransaction.RiskLow, wantLenses: []string{}},
		{name: "401 pure doc lines select readability", path: "docs/guide.md", lines: 401, focus: "risk", wantRisk: reviewtransaction.RiskMedium, wantLenses: []string{reviewtransaction.LensReadability}},
		{name: "401 static MDX lines select readability", path: "book/chapter.mdx", lines: 401, wantRisk: reviewtransaction.RiskMedium, wantLenses: []string{reviewtransaction.LensReadability}},
		{name: "active MDX keeps high routing", path: "book/chapter.mdx", lines: 400, prefix: "import Widget from './widget'\n", wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "SVG keeps high routing", path: "docs/diagram.svg", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "semantic doc path keeps high routing", path: "docs/security/guide.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "prompt markdown keeps normal large routing", path: "prompts/system.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "compound prompt filename keeps normal large routing", path: "docs/system-prompt.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "agent rules keep normal large routing", path: "AGENTS.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "workflow markdown keeps normal large routing", path: ".github/workflows/release.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "runtime docs keep normal large routing", path: "runtime/README.md", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "configuration keeps normal large routing", path: "config/settings.yaml", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
		{name: "code keeps normal large routing", path: "internal/app.go", lines: 401, wantRisk: reviewtransaction.RiskHigh, wantLenses: full4R},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, tt.path, tt.prefix+strings.Repeat("line\n", tt.lines), 0o644)
			args := []string{
				"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
				"--lineage", fmt.Sprintf("large-doc-routing-%d", index),
			}
			if tt.focus != "" {
				args = append(args, "--focus", tt.focus)
			}
			var output bytes.Buffer
			if err := RunReview(args, &output); err != nil {
				t.Fatal(err)
			}
			result := decodeNegotiatedReviewStart(t, output.Bytes())
			wantLines := tt.lines + strings.Count(tt.prefix, "\n")
			if result.RiskLevel != tt.wantRisk || result.ChangedLines != wantLines || !reflect.DeepEqual(result.SelectedLenses, tt.wantLenses) {
				t.Fatalf("routing = risk %q, lines %d, lenses %v; want %q, %d, %v", result.RiskLevel, result.ChangedLines, result.SelectedLenses, tt.wantRisk, wantLines, tt.wantLenses)
			}
		})
	}

	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/guide.md", strings.Repeat("line\n", 401), 0o644)
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "large-doc-invalid-focus", "--focus", "unknown"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unsupported review focus") {
		t.Fatalf("large pure documentation invalid focus error = %v", err)
	}
}

func TestNegotiatedReviewStartPreservesPrePolicyLargeDocumentationAuthority(t *testing.T) {
	full4R := []string{
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	legacy := ReviewFacadeStartResult{RiskLevel: reviewtransaction.RiskHigh, SelectedLenses: full4R, ChangedLines: 401}
	assessment := reviewtransaction.RiskAssessment{
		Level: reviewtransaction.RiskMedium, ChangedLines: 401, DominantLens: reviewtransaction.LensReadability,
		Reasons: []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonLargeChange}, {Code: reviewtransaction.RiskReasonNonExecutableOnly}},
	}
	aligned, err := reviewStartAssessmentForFrozenAuthority(legacy, assessment)
	if err != nil {
		t.Fatal(err)
	}
	if aligned.Level != reviewtransaction.RiskHigh || aligned.DominantLens != "" ||
		!reflect.DeepEqual(aligned.Reasons, []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonLargeChange}}) {
		t.Fatalf("pre-policy aligned assessment = %#v", aligned)
	}
}

func TestNegotiatedReviewStartAndStatusExposeWorkspaceOverlay(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "committed.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "committed.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "branch")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("overlay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--cwd", repo, "--workspace-overlay"},
		{"--cwd", repo, "--base-ref", base, "--workspace-overlay", "--committed-only"},
		{"--cwd", repo, "--base-ref", base, "--workspace-overlay", "--projection", "staged"},
	} {
		if err := RunReviewFacadeStart(args, io.Discard); err == nil {
			t.Fatalf("invalid overlay START succeeded: %v", args)
		}
	}

	var startOutput bytes.Buffer
	args := []string{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-ref", base, "--workspace-overlay", "--lineage", "review-overlay"}
	if err := RunReviewFacadeStart(args, &startOutput); err != nil {
		t.Fatal(err)
	}
	start := decodeNegotiatedReviewStart(t, startOutput.Bytes())
	var statusOutput bytes.Buffer
	if err := RunReviewStatus(args, &statusOutput); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if start.TargetMode != reviewtransaction.TargetBaseWorkspaceOverlay || start.TargetMode != status.Projection.Kind ||
		start.TargetIdentity != status.TargetIdentity || start.BaseTree != status.Projection.BaseTree || start.CandidateTree != status.Projection.CurrentCandidateTree {
		t.Fatalf("overlay START/status mismatch: start=%#v status=%#v", start, status)
	}
	for _, selector := range [][]string{
		{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--workspace-overlay"},
		{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-ref", base, "--base-tree", start.BaseTree, "--workspace-overlay"},
		{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-tree", start.BaseTree},
		{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-tree", base, "--workspace-overlay"},
		{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-tree", "HEAD", "--workspace-overlay"},
		{"--cwd", repo, "--base-ref", base, "--workspace-overlay"},
	} {
		if err := RunReviewStatus(selector, io.Discard); err == nil {
			t.Fatalf("invalid overlay status succeeded: %v", selector)
		}
	}
}

func TestNegotiatedOverlayStatusUsesResolvedStartBaseAfterSymbolicRefAdvances(t *testing.T) {
	repo := initReviewCLIRepo(t)
	baseCommit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	runReviewCLIGit(t, repo, "branch", "review-base", baseCommit)
	if err := os.WriteFile(filepath.Join(repo, "committed.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "committed.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "branch")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("overlay\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lineage := "review-overlay-resolved-base"
	var startOutput bytes.Buffer
	if err := RunReviewFacadeStart([]string{
		"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-ref", "review-base", "--workspace-overlay", "--lineage", lineage,
	}, &startOutput); err != nil {
		t.Fatal(err)
	}
	start := decodeNegotiatedReviewStart(t, startOutput.Bytes())
	runReviewCLIGit(t, repo, "update-ref", "refs/heads/review-base", "HEAD")

	var exactOutput bytes.Buffer
	if err := RunReviewStatus([]string{
		"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-tree", start.BaseTree, "--workspace-overlay", "--lineage", lineage,
	}, &exactOutput); err != nil {
		t.Fatal(err)
	}
	var exact ReviewTargetStatusResult
	decodeStrictReviewJSON(t, exactOutput.Bytes(), &exact)
	if exact.Applicability != reviewtransaction.TargetApplicabilityCurrent || exact.TargetIdentity != start.TargetIdentity ||
		exact.Projection.BaseTree != start.BaseTree || exact.Projection.CurrentCandidateTree != start.CandidateTree {
		t.Fatalf("resolved-base status = %#v, START %#v", exact, start)
	}

	var advancedOutput bytes.Buffer
	if err := RunReviewStatus([]string{
		"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--base-ref", "review-base", "--workspace-overlay", "--lineage", lineage,
	}, &advancedOutput); err != nil {
		t.Fatal(err)
	}
	var advanced ReviewTargetStatusResult
	decodeStrictReviewJSON(t, advancedOutput.Bytes(), &advanced)
	if advanced.Applicability == reviewtransaction.TargetApplicabilityCurrent {
		t.Fatalf("advanced symbolic base remained current: %#v", advanced)
	}
}

func TestReviewRecoverRetainsWorkspaceOverlayBaseAndScope(t *testing.T) {
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "committed.txt"), []byte("committed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "committed.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "branch")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("overlay\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lineage := "overlay-recovery-predecessor"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", base, "--workspace-overlay", "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := predecessor.State
	results := make([]reviewtransaction.LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(predecessor.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--cwd", repo, "--predecessor-lineage", lineage, "--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "overlay-recovery-successor", "--disposition", "scope_changed", "--reason", "scope changed", "--actor", "maintainer"}
	if err := RunReviewRecover(args, io.Discard); err == nil || !strings.Contains(err.Error(), "scope has not changed") {
		t.Fatalf("unchanged overlay recovery error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new scope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewRecover(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "overlay-recovery-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := successor.State.InitialSnapshot
	if snapshot.Kind != reviewtransaction.TargetBaseWorkspaceOverlay || snapshot.BaseTree != predecessor.State.InitialSnapshot.BaseTree || snapshot.Identity == predecessor.State.InitialSnapshot.Identity ||
		!reflect.DeepEqual(snapshot.Paths, []string{"committed.txt", "new.txt", "tracked.txt"}) {
		t.Fatalf("recovered overlay snapshot = %#v", snapshot)
	}
}

func TestReviewRecoverSelectsAuthorizedStagedProjection(t *testing.T) {
	repo, predecessor, status := escalatedRecoveryProjectionFixture(t, "staged-projection-success")
	reason, actor := "select exact staged target", "maintainer"
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, status.TargetIdentity, actor, reason)
	args := recoveryProjectionArgs(repo, predecessor, "staged-projection-successor", reason, actor)
	args = append(args, "--projection", "staged", "--maintainer-authorization", authorization)

	var output bytes.Buffer
	if err := RunReviewRecover(args, &output); err != nil {
		t.Fatal(err)
	}
	var result ReviewRecoverResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, result.LineageID)
	recovered, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.InitialSnapshot.Projection != reviewtransaction.ProjectionStaged || recovered.State.InitialSnapshot.Identity != status.TargetIdentity {
		t.Fatalf("selected recovery target = %#v, status identity = %s", recovered.State.InitialSnapshot, status.TargetIdentity)
	}
}

func TestReviewRecoverProjectionFailuresDoNotMutateAuthority(t *testing.T) {
	for _, tt := range []struct {
		name       string
		projection string
		authorize  func(ReviewTargetStatusResult, reviewtransaction.CompactRecord, string, string) string
		want       string
	}{
		{name: "omitted projection defaults to predecessor", want: "projection=workspace", authorize: func(status ReviewTargetStatusResult, predecessor reviewtransaction.CompactRecord, actor, reason string) string {
			return reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, status.TargetIdentity, actor, reason)
		}},
		{name: "wrong authorization", projection: "staged", want: "projection=staged", authorize: func(_ ReviewTargetStatusResult, _ reviewtransaction.CompactRecord, _, _ string) string {
			return "wrong"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, predecessor, status := escalatedRecoveryProjectionFixture(t, "staged-projection-"+strings.ReplaceAll(tt.name, " ", "-"))
			reason, actor := "select exact staged target", "maintainer"
			successor := "failed-" + strings.ReplaceAll(tt.name, " ", "-")
			args := recoveryProjectionArgs(repo, predecessor, successor, reason, actor)
			if tt.projection != "" {
				args = append(args, "--projection", tt.projection)
			}
			args = append(args, "--maintainer-authorization", tt.authorize(status, predecessor, actor, reason))
			err := RunReviewRecover(args, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), repo) {
				t.Fatalf("recovery error = %v, want path-free %q", err, tt.want)
			}
			store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
			if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
				t.Fatalf("failed recovery created successor: %v", statErr)
			}
		})
	}
}

func TestReviewRecoverRejectsStagedIndexMutationBeforePersistence(t *testing.T) {
	repo, predecessor, status := escalatedRecoveryProjectionFixture(t, "staged-projection-race")
	reason, actor := "select exact staged target", "maintainer"
	successor := "staged-projection-race-successor"
	authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, status.TargetIdentity, actor, reason)
	args := append(recoveryProjectionArgs(repo, predecessor, successor, reason, actor), "--projection", "staged", "--maintainer-authorization", authorization)

	original := reviewRecoverBeforePersist
	reviewRecoverBeforePersist = func() {
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("raced index\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "tracked.txt")
	}
	t.Cleanup(func() { reviewRecoverBeforePersist = original })
	if err := RunReviewRecover(args, io.Discard); err == nil || !strings.Contains(err.Error(), "repository evidence") {
		t.Fatalf("index race error = %v", err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
	if _, statErr := os.Stat(store.StatePath()); !os.IsNotExist(statErr) {
		t.Fatalf("index race created successor: %v", statErr)
	}
}

func TestReviewRecoverHelpDocumentsProjectionAndCanonicalAuthorization(t *testing.T) {
	var output bytes.Buffer
	if err := RunReviewRecover([]string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	for _, required := range []string{"--projection", "default: predecessor projection", "gentle-ai.review-recovery-authorization/v1", "target_identity"} {
		if !strings.Contains(help, required) {
			t.Fatalf("review recover help missing %q:\n%s", required, help)
		}
	}
}

func escalatedRecoveryProjectionFixture(t *testing.T, lineage string) (string, reviewtransaction.CompactRecord, ReviewTargetStatusResult) {
	t.Helper()
	repo := initReviewCLIRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := record.State
	finding := reviewtransaction.Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:1", Severity: "CRITICAL", Claim: "observable failure", ProofRefs: []string{"reproduced"}}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{
		LensResults:     []reviewtransaction.LensResult{{Lens: "reliability", Findings: []reviewtransaction.Finding{finding}, Evidence: []string{"reviewed"}}},
		Classifications: []reviewtransaction.FindingEvidence{{FindingID: finding.ID, Class: reviewtransaction.EvidenceDeterministic, Causality: reviewtransaction.CausalUnknown, Proof: "requires maintainer recovery"}}, RefuterOutcomes: []reviewtransaction.EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	predecessor, _ := store.Load()
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("staged successor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "tracked.txt")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("unstaged divergence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunReviewStatus([]string{"--contract", ReviewIntegrationContractV1, "--cwd", repo, "--projection", "staged"}, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	return repo, predecessor, status
}

func recoveryProjectionArgs(repo string, predecessor reviewtransaction.CompactRecord, successor, reason, actor string) []string {
	return []string{"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID, "--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", successor, "--disposition", "escalated", "--reason", reason, "--actor", actor}
}

func reviewRecoveryAuthorization(lineage, revision, identity, actor, reason string) string {
	return "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + lineage + "\npredecessor_revision=" + revision +
		"\ntarget_identity=" + identity + "\nactor=" + actor + "\nreason=" + reason
}

func TestReviewRecoverReleaseScopeExpandsMergedSliceToFirstParentDiff(t *testing.T) {
	repo := initReviewCLIRepo(t)
	mainBranch := strings.TrimSpace(runReviewCLIGit(t, repo, "branch", "--show-current"))
	runReviewCLIGit(t, repo, "checkout", "-qb", "release-candidate")
	if err := os.MkdirAll(filepath.Join(repo, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".github", "workflows", "release.yml"), []byte("name: Release\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", ".github/workflows/release.yml")
	runReviewCLIGit(t, repo, "commit", "-qm", "release workflow")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("reviewed slice\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lineage := "release-slice-predecessor"
	if err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", lineage}, io.Discard); err != nil {
		t.Fatal(err)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	predecessor, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	state := predecessor.State
	results := make([]reviewtransaction.LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = reviewtransaction.LensResult{Lens: lens, Findings: []reviewtransaction.Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(reviewtransaction.CompactReviewInput{LensResults: results}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(predecessor.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	predecessor, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}

	runReviewCLIGit(t, repo, "add", "tracked.txt")
	runReviewCLIGit(t, repo, "commit", "-qm", "reviewed slice")
	runReviewCLIGit(t, repo, "checkout", "-q", mainBranch)
	runReviewCLIGit(t, repo, "merge", "--no-ff", "-qm", "release candidate", "release-candidate")
	mergedTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	if mergedTree != predecessor.State.CurrentSnapshot.CandidateTree {
		t.Fatalf("merged tree = %s, reviewed candidate = %s", mergedTree, predecessor.State.CurrentSnapshot.CandidateTree)
	}

	args := []string{"--cwd", repo, "--predecessor-lineage", lineage, "--expected-predecessor-revision", predecessor.Revision,
		"--successor-lineage", "release-scope-successor", "--disposition", "scope_changed", "--reason", "prepare complete release scope", "--actor", "maintainer", "--release-scope"}
	conflicting := append(append([]string{}, args...), "--base-ref", "HEAD^1")
	if err := RunReviewRecover(conflicting, io.Discard); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("release-scope selector conflict error = %v", err)
	}
	if err := RunReviewRecover(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	successorStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "release-scope-successor")
	if err != nil {
		t.Fatal(err)
	}
	successor, err := successorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := successor.State.InitialSnapshot
	wantBase := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^1^{tree}"))
	wantPaths := []string{".github/workflows/release.yml", "tracked.txt"}
	if snapshot.Kind != reviewtransaction.TargetBaseDiff || snapshot.BaseTree != wantBase || snapshot.CandidateTree != mergedTree ||
		!reflect.DeepEqual(snapshot.Paths, wantPaths) || successor.State.RiskLevel != reviewtransaction.RiskHigh || len(successor.State.SelectedLenses) != 4 {
		t.Fatalf("release-scope successor = %#v", successor.State)
	}
}

func TestNegotiatedReviewStartPreservesLegacyPayloadAndAuthorityIdentity(t *testing.T) {
	legacyRepo := initReviewCLIRepo(t)
	negotiatedRepo := initReviewCLIRepo(t)
	for _, repo := range []string{legacyRepo, negotiatedRepo} {
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lineage := "review-start-authority-parity"
	var legacyOutput bytes.Buffer
	if err := RunReview([]string{"start", "--cwd", legacyRepo, "--lineage", lineage}, &legacyOutput); err != nil {
		t.Fatal(err)
	}
	var legacyFields map[string]json.RawMessage
	if err := json.Unmarshal(legacyOutput.Bytes(), &legacyFields); err != nil {
		t.Fatal(err)
	}
	gotFields := make([]string, 0, len(legacyFields))
	for field := range legacyFields {
		gotFields = append(gotFields, field)
	}
	sortStrings(gotFields)
	wantFields := []string{
		"action", "changed_files", "changed_lines", "correction_budget", "lens_bindings", "lenses_required",
		"lineage_id", "operation", "projection", "risk_level", "selected_lenses", "state", "target_identity",
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("unnegotiated START fields = %v, want %v\n%s", gotFields, wantFields, legacyOutput.String())
	}
	var legacy ReviewFacadeStartResult
	decoder := json.NewDecoder(bytes.NewReader(legacyOutput.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Operation != "review/start" {
		t.Fatalf("legacy operation = %q", legacy.Operation)
	}

	var negotiatedOutput bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", negotiatedRepo, "--lineage", lineage,
	}, &negotiatedOutput); err != nil {
		t.Fatal(err)
	}
	negotiated := decodeNegotiatedReviewStart(t, negotiatedOutput.Bytes())
	if negotiated.Operation != "review.start" || negotiated.Contract != ReviewIntegrationContractV1 {
		t.Fatalf("negotiated identity = %#v", negotiated)
	}

	legacyStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), legacyRepo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	negotiatedStore, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), negotiatedRepo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	legacyAuthority, err := os.ReadFile(legacyStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	negotiatedAuthority, err := os.ReadFile(negotiatedStore.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyAuthority, negotiatedAuthority) {
		t.Fatalf("contract negotiation changed compact authority bytes:\nlegacy=%s\nnegotiated=%s", legacyAuthority, negotiatedAuthority)
	}
	for _, path := range []string{legacyStore.ReceiptPath(), negotiatedStore.ReceiptPath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("START unexpectedly materialized receipt %q: %v", path, err)
		}
	}
}

func TestNegotiatedReviewStartRejectsInvalidContractsBeforeAuthorityMutation(t *testing.T) {
	for _, contract := range []string{"", "gentle-ai.review-integration/v2", " " + ReviewIntegrationContractV1} {
		t.Run(strings.ReplaceAll(contract, "/", "_"), func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			contractArg := "--contract=" + contract
			err := RunReview([]string{"start", contractArg, "--cwd", repo, "--lineage", "review-invalid-contract"}, &output)
			if err == nil {
				t.Fatalf("contract %q result = %q, %v", contract, output.String(), err)
			}
			failure := decodeReviewIntegrationFailure(t, output.Bytes())
			if failure.MutationOutcome != ReviewMutationNotStarted {
				t.Fatalf("contract %q failure = %#v", contract, failure)
			}
			stores, discoverErr := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
			if discoverErr != nil {
				t.Fatal(discoverErr)
			}
			if len(stores) != 0 {
				t.Fatalf("invalid contract created authority stores: %#v", stores)
			}
		})
	}
}

func TestExplicitReviewStartRetriesAcrossSharedCommonDirWithoutReconstruction(t *testing.T) {
	repo := initReviewCLIRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	runReviewCLIGit(t, repo, "worktree", "add", "--detach", linked, "HEAD")
	for _, root := range []string{repo, linked} {
		if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("same candidate\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	lineage := "review-common-dir-retry"
	start := func(root string) ([]byte, ReviewIntegrationStartResult) {
		t.Helper()
		var output bytes.Buffer
		if err := RunReview([]string{"start", "--contract", ReviewIntegrationContractV1, "--cwd", root, "--lineage", lineage}, &output); err != nil {
			t.Fatalf("START in %s: %v\n%s", root, err, output.String())
		}
		return append([]byte(nil), output.Bytes()...), decodeNegotiatedReviewStart(t, output.Bytes())
	}
	_, created := start(repo)
	if created.Action != string(reviewtransaction.CompactStartCreated) || created.LineageID != lineage {
		t.Fatalf("initial START = %#v", created)
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	commonDir := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	broken := filepath.Join(commonDir, "gentle-ai", "review-transactions", "v2", "unrelated-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstRetry, resumed := start(linked)
	secondRetry, resumedAgain := start(linked)
	if resumed.Action != string(reviewtransaction.CompactStartResumed) || resumedAgain.Action != resumed.Action || !bytes.Equal(firstRetry, secondRetry) {
		t.Fatalf("explicit START retries = %#v, %#v\n%s\n%s", resumed, resumedAgain, firstRetry, secondRetry)
	}
	if err := os.WriteFile(filepath.Join(linked, "tracked.txt"), []byte("different candidate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, blocked := start(linked)
	if blocked.Action != string(reviewtransaction.CompactStartBlocked) || blocked.LineageID != lineage {
		t.Fatalf("mismatched explicit START = %#v", blocked)
	}
	after, err := os.ReadFile(store.StatePath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("START retry mutated selected authority: %v", err)
	}
}

func TestNegotiatedReviewStartSchemaAndFixtureAreStrict(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	schemaPayload, err := os.ReadFile(filepath.Join(root, "schemas", "start.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaPayload, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != ReviewIntegrationStartSchemaID || schema["additionalProperties"] != false {
		t.Fatalf("START schema header = %#v", schema)
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "start.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	result := decodeNegotiatedReviewStart(t, fixture)
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ReviewIntegrationStartResult){
		func(value *ReviewIntegrationStartResult) {
			value.TargetMode = reviewtransaction.TargetBaseWorkspaceOverlay
		},
		func(value *ReviewIntegrationStartResult) { value.TargetIdentity = "sha256:" + strings.Repeat("a", 64) },
		func(value *ReviewIntegrationStartResult) { value.BaseTree = strings.Repeat("a", 40) },
		func(value *ReviewIntegrationStartResult) { value.CandidateTree = strings.Repeat("a", 40) },
	} {
		invalid := result
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("START accepted partial overlay identity: %#v", invalid)
		}
	}
	var raw map[string]any
	if err := json.Unmarshal(fixture, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	malformed, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(malformed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&ReviewIntegrationStartResult{}); err == nil {
		t.Fatal("strict negotiated START decoder accepted unknown top-level field")
	}
}

func runNegotiatedReviewStart(t *testing.T, repo, lineage string) ReviewIntegrationStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--lineage", lineage,
	}, &output); err != nil {
		t.Fatal(err)
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeNegotiatedReviewStart(t *testing.T, payload []byte) ReviewIntegrationStartResult {
	t.Helper()
	var result ReviewIntegrationStartResult
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func writeReviewStartCandidate(t *testing.T, repo, path, contents string, mode os.FileMode) {
	t.Helper()
	fullPath := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}
