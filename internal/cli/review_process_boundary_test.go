package cli

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// TestReviewStartSelectsCanonicalFourRForSubprocessWrapper is the issue #1438
// regression fixture: an unstaged, untracked subprocess.run wrapper at 318
// authored changed lines (below the 400-line size rule) must classify high and
// select the canonical 4R lens set, never a single-lens medium route.
func TestReviewStartSelectsCanonicalFourRForSubprocessWrapper(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewCLIModule(t, repo, "tools/procwrap/runner.py", issue1438WrapperModule(200))
	writeReviewCLIModule(t, repo, "tools/procwrap/policy.py", paddedReviewCLIModule([]string{`"""Argv, cwd, timeout, and environment policy tables."""`}, 80))
	writeReviewCLIModule(t, repo, "tools/procwrap/errors.py", paddedReviewCLIModule([]string{`"""Process output and error policy."""`}, 30))
	writeReviewCLIModule(t, repo, "tools/procwrap/__init__.py", paddedReviewCLIModule([]string{`"""Package marker."""`}, 8))

	var startOutput bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
	}), &startOutput); err != nil {
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

func TestReviewAssessExtensionlessShebang(t *testing.T) {
	const (
		bash     = "#!/usr/bin/env bash\n"
		ash      = "#!/bin/ash\n"
		envSplit = "#!/usr/bin/env -S MODE=prod bash -e\n"
		python   = "#!/usr/bin/env python3\n"
	)
	largeData := strings.Repeat("x", 9<<20) + "\n"
	for _, fixture := range []struct {
		name, path, base, after string
		risk, dueReason         string
		codes                   []reviewtransaction.RiskReasonCode
	}{
		{"command", "bin/deploy", bash, bash + "echo ready\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"comment", "bin/deploy", bash, bash + "# deployment note\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"ash", "bin/deploy", ash, ash + "echo ready\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"env-split", "bin/deploy", envSplit, envSplit + "echo ready\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"python", "bin/deploy", python, python + "print('ready')\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"bare-marker", "data/catalog", "#!\n", "#!\nordinary data\n", "medium", "under_budget", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonExecutableChange}},
		{"spaces-only", "data/catalog", "#!   \n", "#!   \nordinary data\n", "medium", "under_budget", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonExecutableChange}},
		{"test-fixture", "__tests__/deploy", bash, bash + "echo ready\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary}},
		{"removed-shebang", "bin/deploy", bash + "echo ready\n", "echo ready\n", "medium", "under_budget", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonExecutableChange}},
		{"large-data", "data/catalog", largeData, largeData + "ordinary data\n", "medium", "under_budget", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonExecutableChange}},
		{"shell-source", "bin/deploy.sh", bash, bash + "# deployment note\n", "high", "high_risk", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonProcessBoundary, reviewtransaction.RiskReasonShellSource}},
		{"ordinary-go", "value.go", "package value\n", "package value\nconst ready = true\n", "medium", "under_budget", []reviewtransaction.RiskReasonCode{reviewtransaction.RiskReasonExecutableChange}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			repo := initReviewCLIRepo(t)
			writeReviewStartCandidate(t, repo, fixture.path, fixture.base, 0o644)
			runReviewCLIGit(t, repo, "add", "--", fixture.path)
			runReviewCLIGit(t, repo, "commit", "-qm", "base candidate")
			writeReviewStartCandidate(t, repo, fixture.path, fixture.after, 0o644)
			before := runReviewCLIGit(t, repo, "status", "--porcelain=v1")

			var output bytes.Buffer
			if err := RunReview([]string{"assess", "--cwd", repo, "--json"}, &output); err != nil {
				t.Fatalf("review assess: %v\n%s", err, output.String())
			}
			var result ReviewAssessmentResult
			decodeStrictReviewJSON(t, output.Bytes(), &result)
			var wantReasons []ReviewAssessmentReason
			for _, code := range fixture.codes {
				reason := ReviewAssessmentReason{Code: string(code), Path: fixture.path}
				if code != reviewtransaction.RiskReasonExecutableChange {
					reason.Detail = "signal: shell_process"
				}
				wantReasons = append(wantReasons, reason)
			}
			wantDue := fixture.risk == "high"
			if result.Schema != ReviewAssessmentSchema || result.Risk != fixture.risk || !reflect.DeepEqual(result.Reasons, wantReasons) ||
				result.ChangedPaths != 1 || result.ChangedLines != 1 || result.ReviewDue != wantDue || result.ReviewDueReason != fixture.dueReason ||
				result.Candidate != (ReviewAssessmentCandidate{Kind: string(reviewtransaction.TargetCurrentChanges)}) {
				t.Errorf("review assess wanted %s, reasons %#v, 1 path/line, review due %t/%s, and fresh candidate; got %s", fixture.risk, wantReasons, wantDue, fixture.dueReason, output.String())
			}
			if wantDue && result.ReviewDue {
				assertReviewAssessNextTransition(t, result.NextTransition, repo, "", "")
			} else if !wantDue && result.NextTransition != nil {
				t.Errorf("review assess published an unexpected next transition: %#v", result.NextTransition)
			}
			if after := runReviewCLIGit(t, repo, "status", "--porcelain=v1"); before != after {
				t.Errorf("review assess changed git status: %q -> %q", before, after)
			}
		})
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
	writeReviewStartCandidate(t, repo, filepath.ToSlash(name), content, 0o644)
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

func TestNegotiatedReviewStartUsesCurrentV2ActionVocabulary(t *testing.T) {
	assessment := reviewtransaction.RiskAssessment{
		Level: reviewtransaction.RiskMedium, ChangedLines: 20,
		Reasons: []reviewtransaction.RiskReason{{Code: reviewtransaction.RiskReasonExecutableChange, Path: "process_helper.go"}},
	}
	contextResult := reviewtransaction.FrozenCandidateContext{
		BaseTree: strings.Repeat("a", 40), CandidateTree: strings.Repeat("b", 40),
		ChangedPathManifest: []reviewtransaction.ChangedPathManifestEntry{{Path: "process_helper.go", Status: reviewtransaction.CandidatePathAdded, OldMode: "000000", NewMode: "100644"}},
	}
	for _, action := range []string{"created", "replayed"} {
		t.Run(action, func(t *testing.T) {
			start := ReviewFacadeStartResult{Action: action, LineageID: "current-v2-action", RiskLevel: reviewtransaction.RiskMedium, SelectedLenses: []string{reviewtransaction.LensReliability}, Projection: reviewtransaction.ProjectionWorkspace, ChangedFiles: 1, ChangedLines: 20, CorrectionBudget: 10}
			result, err := newReviewIntegrationStartResult(start, assessment, "", &contextResult, nil, nil)
			if err != nil || result.Action != action || result.RiskLevel != reviewtransaction.RiskMedium || !reflect.DeepEqual(result.SelectedLenses, start.SelectedLenses) {
				t.Fatalf("v2 START action %q = %#v, error %v", action, result, err)
			}
		})
	}
}
