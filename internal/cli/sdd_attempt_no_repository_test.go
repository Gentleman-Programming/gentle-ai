package cli

import (
	"bytes"
	"strings"
	"testing"
)

// #2612 / #3202: outside a Git repository the attempt ledger must name its exit
// instead of leaking the raw `git rev-parse` failure.
func TestRunSDDAttemptAcquireOutsideGitRepositoryNamesGitInit(t *testing.T) {
	var output bytes.Buffer
	err := RunSDDAttempt([]string{
		"acquire", "--cwd", t.TempDir(), "--change", "no-repo", "--request-id", "acquire-1",
		"--work-unit", "unit", "--evidence-goal", "prove the refusal names its exit",
		"--max-attempts", "1", "--max-changed-lines", "10",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "git init") || strings.Contains(err.Error(), "rev-parse") {
		t.Fatalf("acquire outside Git = %v (output %q), want a refusal naming `git init` without `rev-parse`", err, output.String())
	}
}
