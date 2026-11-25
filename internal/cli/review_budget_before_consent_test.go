package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommittedStartChecksLensBudgetBeforeConsent(t *testing.T) {
	home := reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	writeRuntimeBudgetOverCandidate(t, repo)
	runReviewCLIGit(t, repo, "commit", "-qm", "large committed change")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	runReviewCLIGit(t, repo, "commit", "-qm", "high-risk committed change")
	args := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--base-ref", base, "--lineage", "review-budget-before-consent",
	})

	// Granting the exact candidate is a control: even before the fix it
	// reaches the existing budget check and must not create authority.
	for _, mode := range []string{"relay", "granted", "interactive"} {
		t.Run(mode, func(t *testing.T) {
			startArgs := args
			if mode != "interactive" {
				startArgs = append(append([]string(nil), args...), "--consent", mode)
			}
			console := stubReviewConsole(t, mode == "interactive", "1\n")
			stderr := captureReviewProcessStderr(t)
			store := filepath.Dir(reviewCLIAuthorityRoot(t, repo))
			storeBefore := snapshotAuthorityTree(t, store)
			homeBefore := snapshotAuthorityTree(t, home)

			var stdout bytes.Buffer
			err := RunReview(startArgs, &stdout)
			if got := stderr(); got != "" {
				t.Errorf("START wrote to stderr: %q", got)
			}
			if console.Len() != 0 {
				t.Errorf("START prompted on the console before refusing: %q", console.String())
			}
			if snapshotAuthorityTree(t, store) != storeBefore || snapshotAuthorityTree(t, home) != homeBefore {
				t.Error("over-budget START wrote review state")
			} else {
				t.Log("no authority, consent state, or other review files written")
			}
			envelope := requireStartLensBudgetRefusal(t, err, stdout.Bytes())

			if mode == "relay" {
				// HEAD~1 is already a narrower base in this fixture. Run the
				// printed command, then the fresh START it offers for that commit.
				_, tail, found := strings.Cut(envelope.Cause, "by running `")
				command, _, closed := strings.Cut(tail, "`")
				if !found || !closed || command != "gentle-ai review status --contract "+ReviewIntegrationContractV2+" --next-transition --base-ref HEAD~1 --committed-only" {
					t.Fatalf("budget refusal did not name the runnable smaller-commit STATUS: %q", envelope.Cause)
				}
				t.Chdir(repo)
				words, err := SplitPrintedCommandWords(command)
				if err != nil {
					t.Fatal(err)
				}
				var statusOut bytes.Buffer
				if err := RunReview(words[2:], &statusOut); err != nil {
					t.Fatalf("printed STATUS failed: %v\n%s", err, &statusOut)
				}
				var status ReviewTargetStatusResult
				decodeStrictReviewJSON(t, statusOut.Bytes(), &status)
				if status.NextTransition == nil || status.NextTransition.Execute == nil {
					t.Fatalf("printed STATUS did not offer START: %s", &statusOut)
				}
				questionOut := runConsentRelayStart(t, invocationArgs(t, status.NextTransition.Execute.Command))
				question := decodeConsentQuestion(t, questionOut.Bytes())
				if err := question.Validate(); err != nil || question.Action != "consent_required" || !question.Blocking || question.ChangedFiles != 1 {
					t.Fatalf("under-budget START did not relay consent for the smaller candidate: %v\n%s", err, questionOut)
				}
				if console.Len() != 0 || snapshotAuthorityTree(t, store) != storeBefore || snapshotAuthorityTree(t, home) != homeBefore {
					t.Fatal("under-budget relay START prompted directly or wrote review state")
				}
			}
		})
	}
}

func TestLiveStartChecksLensBudgetBeforeConsent(t *testing.T) {
	for _, tc := range []struct {
		name, projection, statusXY string
	}{
		{name: "staged", projection: "staged", statusXY: "M."},
		{name: "unstaged", projection: "workspace", statusXY: ".M"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			const path = "tracked.txt"
			size := writeRuntimeBudgetCandidate(t, repo, path, 14_000)
			if size <= reviewRuntimeBudgetTestCapBytes || size >= reviewRuntimeBudgetTestCeilingBytes {
				t.Fatalf("live fixture is %d bytes, want it between the runtime cap and Git ceiling", size)
			}
			if tc.projection == "staged" {
				runReviewCLIGit(t, repo, "add", "--", path)
			}
			args := boundNegotiatedStartArgs(t, []string{
				"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
				"--projection", tc.projection, "--lineage", "review-live-budget-before-consent", "--consent", "relay",
			})
			console := stubReviewConsole(t, true, "1\n")
			stderr := captureReviewProcessStderr(t)
			store := filepath.Dir(reviewCLIAuthorityRoot(t, repo))
			storeBefore := snapshotAuthorityTree(t, store)
			homeBefore := snapshotAuthorityTree(t, home)

			status := func() string {
				return runReviewCLIGit(t, repo, "--no-optional-locks", "status", "--porcelain=v2", "--untracked-files=all")
			}
			fileHashes := func() string {
				return runReviewCLIGit(t, repo, "hash-object", "--no-filters", "--", ".git/index", ".git/HEAD", path)
			}
			statusBefore := status()
			if !strings.HasPrefix(statusBefore, "1 "+tc.statusXY+" ") || strings.Count(statusBefore, "\n") != 1 {
				t.Fatalf("expected only the %s change, got status: %s", tc.name, statusBefore)
			}
			treeBefore := runReviewCLIGit(t, repo, "write-tree")
			headBefore := runReviewCLIGit(t, repo, "rev-parse", "HEAD")
			hashesBefore := fileHashes()

			var stdout bytes.Buffer
			err := RunReview(args, &stdout)
			if got := stderr(); got != "" {
				t.Errorf("START wrote to stderr: %q", got)
			}
			if console.Len() != 0 {
				t.Errorf("START prompted on the console before refusing: %q", console.String())
			}
			if snapshotAuthorityTree(t, store) != storeBefore || snapshotAuthorityTree(t, home) != homeBefore {
				t.Error("over-budget START wrote review state")
			}
			// Hash the raw files before write-tree can refresh the index's cache tree.
			for _, check := range []struct{ name, before, after string }{
				{"index, HEAD and worktree file hashes", hashesBefore, fileHashes()},
				{"status", statusBefore, status()},
				{"index tree", treeBefore, runReviewCLIGit(t, repo, "write-tree")},
				{"HEAD commit", headBefore, runReviewCLIGit(t, repo, "rev-parse", "HEAD")},
			} {
				if check.before != check.after {
					t.Errorf("START changed %s: before %q, after %q", check.name, check.before, check.after)
				}
			}
			requireStartLensBudgetRefusal(t, err, stdout.Bytes())
		})
	}
}

func requireStartLensBudgetRefusal(t *testing.T, err error, stdout []byte) ReviewIntegrationFailure {
	t.Helper()
	if bytes.Contains(stdout, []byte(`"consent_required"`)) {
		question := decodeConsentQuestion(t, stdout)
		t.Fatalf("START requested consent before refusing: schema=%s blocking=%t risk=%s error=%v", question.Schema, question.Blocking, question.RiskLevel, err)
	}
	var failure *ReviewIntegrationFailureError
	if !errors.As(err, &failure) {
		t.Fatalf("START = %v, want a typed budget refusal; stdout: %s", err, stdout)
	}
	var envelope ReviewIntegrationFailure
	decodeStrictReviewJSON(t, stdout, &envelope)
	if !reflect.DeepEqual(envelope, failure.Failure) || envelope.Code != "lens_context_budget_exceeded" ||
		envelope.MutationOutcome != ReviewMutationNotStarted || envelope.Phase != "preflight" ||
		envelope.NextAction != "stop" || envelope.AuthorityApplicability != "not_evaluated" {
		t.Fatalf("START failure envelope = %#v; want a preflight budget stop", envelope)
	}
	t.Logf("START refused with %s (%s)", envelope.Code, envelope.MutationOutcome)
	return envelope
}
