package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// statusReviewDue is the one definition of "review due" (odd/tasks/
// rdd-simplification.md S1): the native STATUS offers a review.start for the
// candidate, i.e. there are unreviewed changes.
func statusReviewDue(status ReviewTargetStatusResult) bool {
	return status.NextTransition != nil && status.NextTransition.Kind == reviewNextTransitionExecute &&
		status.NextTransition.Execute != nil && status.NextTransition.Execute.Operation == "review.start"
}

func assessForOfferedSTART(t *testing.T, repo string, status ReviewTargetStatusResult) ReviewAssessmentResult {
	t.Helper()
	args := []string{"assess", "--cwd", repo, "--json"}
	if statusReviewDue(status) {
		args = append(args, reviewStopHookAssessSelectorsForTest(status)...)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("review assess: %v\n%s", err, output.String())
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode review assess: %v\n%s", err, output.String())
	}
	return result
}

func reviewStopHookAssessSelectorsForTest(status ReviewTargetStatusResult) []string {
	var baseRef, committedOnly string
	for _, argument := range status.NextTransition.Execute.Arguments {
		switch argument.Name {
		case "base-ref":
			baseRef = argument.Value
		case "committed-only":
			committedOnly = argument.Value
		}
	}
	if strings.TrimSpace(baseRef) == "" || committedOnly != "true" {
		return nil
	}
	return []string{"--base-ref", baseRef, "--committed-only"}
}

// TestReviewDueHasOneSourceOfTruth pins S1: for the same candidate, the Stop
// hook, the STATUS-derived decision and the assess report cannot disagree, and
// a medium candidate that used to be skipped as under_budget is now due.
func TestReviewDueHasOneSourceOfTruth(t *testing.T) {
	cases := []struct {
		name      string
		committed bool
		stage     func(t *testing.T, repo string)
		wantDue   bool // policy: only high risk is due (verify-always-rdd-high S2)
	}{
		{"medium workspace (#2997, stays not due)", false, func(t *testing.T, repo string) {
			writeReviewStartCandidate(t, repo, "tracked.txt", "changed\n", 0o644)
		}, false},
		{"medium committed range (#4815)", true, func(t *testing.T, repo string) {
			writeReviewStartCandidate(t, repo, "tracked.txt", "changed\n", 0o644)
		}, false},
		{"high workspace", false, stageReviewStopHookCandidate, true},
		{"high committed range", true, stageReviewStopHookCandidate, true},
		{"passive workspace", false, func(t *testing.T, repo string) {
			writeReviewStartCandidate(t, repo, "docs/guide.md", "hello\n", 0o644)
		}, false},
		{"clean committed tree", true, func(t *testing.T, repo string) {}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			enableReviewStopHookRDD(t, repo)
			if tc.committed {
				origin := filepath.Join(t.TempDir(), "origin.git")
				runReviewCLIGit(t, repo, "init", "-q", "--bare", origin)
				runReviewCLIGit(t, repo, "remote", "add", "origin", origin)
				runReviewCLIGit(t, repo, "push", "-q", "origin", "HEAD:refs/heads/main")
				runReviewCLIGit(t, repo, "remote", "set-head", "origin", "main")
			}
			tc.stage(t, repo)
			if tc.committed {
				runReviewCLIGit(t, repo, "add", "-A")
				runReviewCLIGit(t, repo, "commit", "-qm", "work unit", "--allow-empty")
			}

			status := derivedRangeTerminalStatus(t, repo)
			// The single due decision: STATUS must offer a review.start AND the
			// assessment (reviewAssessDue) must not decline it as medium risk.
			due := false
			if statusReviewDue(status) {
				due = assessForOfferedSTART(t, repo, status).ReviewDue
			}
			if due != tc.wantDue {
				t.Fatalf("due = %v, want %v: %#v", due, tc.wantDue, status.NextTransition)
			}

			var stdout, stderr bytes.Buffer
			stdin := strings.NewReader(reviewStopHookTestPayload(t, "sess-single-truth", repo, false, nil))
			if err := runReviewStopHook([]string{"--agent", "claude-code"}, stdin, &stdout, &stderr); err != nil {
				t.Fatalf("Stop run: %v\nstderr: %s", err, stderr.String())
			}
			blocked := strings.TrimSpace(stdout.String()) != ""
			if blocked != due {
				t.Fatalf("Stop blocked = %v but STATUS due = %v: %s", blocked, due, stdout.String())
			}
			_, statErr := os.Stat(reviewStopHookStateFile(home, "sess-single-truth"))
			if (statErr == nil) != due {
				t.Fatalf("reminder recorded = %v but STATUS due = %v", statErr == nil, due)
			}

		})
	}
}
