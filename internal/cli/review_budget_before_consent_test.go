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
			if strings.Contains(stdout.String(), `"consent_required"`) {
				question := decodeConsentQuestion(t, stdout.Bytes())
				t.Fatalf("START requested consent before refusing: schema=%s blocking=%t risk=%s error=%v", question.Schema, question.Blocking, question.RiskLevel, err)
			}
			var failure *ReviewIntegrationFailureError
			if !errors.As(err, &failure) {
				t.Fatalf("START = %v, want a typed budget refusal; stdout: %s", err, &stdout)
			}
			var envelope ReviewIntegrationFailure
			decodeStrictReviewJSON(t, stdout.Bytes(), &envelope)
			if !reflect.DeepEqual(envelope, failure.Failure) || envelope.Code != "lens_context_budget_exceeded" ||
				envelope.MutationOutcome != ReviewMutationNotStarted || envelope.Phase != "preflight" ||
				envelope.NextAction != "stop" || envelope.AuthorityApplicability != "not_evaluated" {
				t.Fatalf("START failure envelope = %#v; want a preflight budget stop", envelope)
			}
			t.Logf("START refused with %s (%s)", envelope.Code, envelope.MutationOutcome)

			if mode == "relay" {
				// HEAD^ is already a narrower base in this fixture. Run the
				// printed command, then the fresh START it offers for that commit.
				_, tail, found := strings.Cut(envelope.Cause, "by running `")
				command, _, closed := strings.Cut(tail, "`")
				if !found || !closed || command != "gentle-ai review status --contract "+ReviewIntegrationContractV2+" --next-transition --base-ref HEAD^ --committed-only" {
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
