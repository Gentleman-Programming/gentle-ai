package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

// TestReviewStartSelectsCanonicalFourRForSubprocessWrapper is the issue #1438
// regression fixture: an unstaged, untracked subprocess.run wrapper at 318
// authored changed lines (below the 400-line size rule) must classify high and
// select the canonical 4R lens set, never a single-lens medium route.
func TestReviewStartSelectsCanonicalFourRForSubprocessWrapper(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewCLIModule(t, repo, "tools/procwrap/runner.py", issue1438WrapperModule(200))
	writeReviewCLIModule(t, repo, "tools/procwrap/policy.py", paddedReviewCLIModule([]string{`"""Argv, cwd, timeout, and environment policy tables."""`}, 80))
	writeReviewCLIModule(t, repo, "tools/procwrap/errors.py", paddedReviewCLIModule([]string{`"""Process output and error policy."""`}, 30))
	writeReviewCLIModule(t, repo, "tools/procwrap/__init__.py", paddedReviewCLIModule([]string{`"""Package marker."""`}, 8))

	var startOutput bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}, &startOutput); err != nil {
		t.Fatal(err)
	}
	var started ReviewIntegrationStartResult
	decodeStrictReviewJSON(t, startOutput.Bytes(), &started)

	if started.ChangedLines != 318 {
		t.Fatalf("changed_lines = %d, want the 318-line reproduction", started.ChangedLines)
	}
	if started.RiskLevel != reviewtransaction.RiskHigh {
		t.Fatalf("risk_level = %q with lenses %v and reasons %#v, want %q", started.RiskLevel, started.SelectedLenses, started.RiskReasons, reviewtransaction.RiskHigh)
	}
	wantLenses := []string{
		reviewtransaction.LensRisk, reviewtransaction.LensResilience,
		reviewtransaction.LensReadability, reviewtransaction.LensReliability,
	}
	if !reflect.DeepEqual(started.SelectedLenses, wantLenses) {
		t.Fatalf("selected_lenses = %v, want canonical 4R %v", started.SelectedLenses, wantLenses)
	}
	wantReason := reviewtransaction.RiskReason{
		Code: reviewtransaction.RiskReasonCode("process_boundary"), Signal: reviewtransaction.SignalShellProcess,
		Path: "tools/procwrap/runner.py",
	}
	if !reflect.DeepEqual(started.RiskReasons, []reviewtransaction.RiskReason{wantReason}) {
		t.Fatalf("risk_reasons = %#v, want %#v", started.RiskReasons, []reviewtransaction.RiskReason{wantReason})
	}
}

func issue1438WrapperModule(lines int) string {
	return paddedReviewCLIModule([]string{
		`"""Executable module wrapping subprocess.run with argv/cwd/timeout/env policy."""`,
		"import subprocess",
		"",
		"def run_command(argv, cwd, timeout, env):",
		"    completed = subprocess.run(argv, cwd=cwd, timeout=timeout, env=env, capture_output=True, check=True)",
		"    return completed.stdout, completed.stderr",
		"",
		"def ls_remote(url):",
		`    return run_command(["git", "ls-remote", url], cwd=None, timeout=30, env={})`,
	}, lines)
}

func paddedReviewCLIModule(header []string, lines int) string {
	rendered := append([]string{}, header...)
	for index := len(rendered); index < lines; index++ {
		rendered = append(rendered, fmt.Sprintf("# policy line %03d", index+1))
	}
	return strings.Join(rendered[:lines], "\n") + "\n"
}

func writeReviewCLIModule(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReviewStartContractValidatesProcessBoundaryReason(t *testing.T) {
	valid := reviewtransaction.RiskReason{
		Code: reviewtransaction.RiskReasonCode("process_boundary"), Signal: reviewtransaction.SignalShellProcess,
		Path: "tools/runner.py",
	}
	if err := validateReviewStartRiskReasons([]reviewtransaction.RiskReason{valid}); err != nil {
		t.Fatalf("validateReviewStartRiskReasons(valid process boundary) = %v", err)
	}
	if got := reviewStartRiskLevel([]reviewtransaction.RiskReason{valid}); got != reviewtransaction.RiskHigh {
		t.Fatalf("reviewStartRiskLevel(process boundary) = %q, want %q", got, reviewtransaction.RiskHigh)
	}
	for name, malformed := range map[string]reviewtransaction.RiskReason{
		"wrong signal": {Code: valid.Code, Signal: reviewtransaction.SignalAuth, Path: valid.Path},
		"missing path": {Code: valid.Code, Signal: reviewtransaction.SignalShellProcess},
		"stray modes":  {Code: valid.Code, Signal: reviewtransaction.SignalShellProcess, Path: valid.Path, OldMode: "100644", NewMode: "100755"},
	} {
		if err := validateReviewStartRiskReasons([]reviewtransaction.RiskReason{malformed}); err == nil {
			t.Fatalf("validateReviewStartRiskReasons accepted malformed %s reason", name)
		}
	}
}

func TestNegotiatedReviewStartResumesFrozenMediumAuthorityAfterClassifierUpgrade(t *testing.T) {
	legacy := ReviewFacadeStartResult{Action: string(reviewtransaction.CompactStartResumed), LineageID: "pre-upgrade", RiskLevel: reviewtransaction.RiskMedium, SelectedLenses: []string{reviewtransaction.LensReliability}, Projection: reviewtransaction.ProjectionWorkspace, ChangedFiles: 1, ChangedLines: 20, CorrectionBudget: 10}
	assessment := reviewtransaction.RiskAssessment{Level: reviewtransaction.RiskHigh, ChangedLines: 20, Reasons: []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonCode("process_boundary"), Signal: reviewtransaction.SignalShellProcess, Path: "runner.py"}}}
	result, err := newReviewIntegrationStartResult(legacy, assessment, "")
	if err != nil || result.RiskLevel != reviewtransaction.RiskMedium || !reflect.DeepEqual(result.SelectedLenses, legacy.SelectedLenses) {
		t.Fatalf("resumed authority = risk %q lenses %v error %v, want frozen medium %v", result.RiskLevel, result.SelectedLenses, err, legacy.SelectedLenses)
	}
}
