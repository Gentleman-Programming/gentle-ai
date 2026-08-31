package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunSDDAttemptBeginRefusesZeroBudgets pins the #1947 residual: an explicit
// zero budget refuses exactly like a negative one instead of being silently
// replaced by the default, while an absent flag still receives the default.
func TestRunSDDAttemptBeginRefusesZeroBudgets(t *testing.T) {
	repo := initReviewCLIRepo(t)
	args := []string{
		"begin", "--cwd", repo, "--change", "zero-budget", "--expected-revision=", "--request-id", "zero-begin",
		"--work-unit", "zero", "--evidence-goal", "prove zero budgets refuse",
	}
	for _, tc := range []struct{ flag, want string }{
		{"--max-changed-lines", "max_changed_lines must be within 1..1000000"},
		{"--max-attempts", "max_attempts must be within 1..100"},
	} {
		err := RunSDDAttempt(append(append([]string{}, args...), tc.flag, "0"), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s 0 was not refused with %q: %v", tc.flag, tc.want, err)
		}
	}
	status := runSDDAttemptStatus(t, args)
	if status.Objective == nil || status.Objective.MaxChangedLines != 200 || status.Objective.MaxAttempts != 2 {
		t.Fatalf("absent budgets did not default: %#v", status.Objective)
	}
}
