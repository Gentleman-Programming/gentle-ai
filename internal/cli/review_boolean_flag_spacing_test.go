package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestReviewBooleanFlagSpacedValueNamesTheEqualsForm closes fisidj finding 3
// (organic-dx Phase 3f task 3f.3): `--committed-only true` (space-separated)
// failed with a bare "unexpected review start argument \"true\"" while
// `--committed-only=true` worked. Issue 1745 fixed what the product itself
// emits; this is the shape a human or a doc-following agent may paste by
// hand. DECISION (recorded per the task's non-negotiable requirement): the
// parser stays strict -- detached boolean values are NOT accepted CLI-wide,
// because doing so would be a parser change with a wide blast radius across
// every command. Instead the refusal names the exact form the parser expects.
// The fix lives in parseReviewFlags (review.go), the single site every review
// verb's flag parsing already funnels through, so every verb benefits without
// touching each verb's own "unexpected review <verb> argument" call site.
func TestReviewBooleanFlagSpacedValueNamesTheEqualsForm(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "docs/second.md", "# second\n\nplain prose, no executable content.\n", 0o644)
	runReviewCLIGit(t, repo, "add", "docs/second.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "second")

	t.Run("review start --committed-only true names the = form", func(t *testing.T) {
		var output bytes.Buffer
		err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", "HEAD", "--committed-only", "true"}, &output)
		if err == nil {
			t.Fatal("expected a refusal for a detached boolean flag value")
		}
		message := err.Error()
		if strings.Contains(message, `unexpected review start argument "true"`) {
			t.Fatalf("refusal still names the bare positional argument instead of the boolean flag misuse: %q", message)
		}
		if !strings.Contains(message, "--committed-only") || !strings.Contains(message, "--committed-only=true") {
			t.Fatalf("refusal does not name both the bare and = forms of --committed-only: %q", message)
		}
	})

	t.Run("review start --committed-only=true still works", func(t *testing.T) {
		var output bytes.Buffer
		err := RunReviewFacadeStart([]string{"--cwd", repo, "--base-ref", "HEAD~1", "--committed-only=true"}, &output)
		if err != nil {
			t.Fatalf("the = form must keep working: %v\n%s", err, output.String())
		}
	})

	t.Run("review status --action-eligibility true names the = form on a second verb without per-verb changes", func(t *testing.T) {
		err := RunReviewStatus([]string{"--cwd", repo, "--action-eligibility", "true"}, io.Discard)
		if err == nil {
			t.Fatal("expected a refusal for a detached boolean flag value")
		}
		message := err.Error()
		if !strings.Contains(message, "--action-eligibility") || !strings.Contains(message, "--action-eligibility=true") {
			t.Fatalf("refusal does not name both forms of --action-eligibility on a second verb: %q", message)
		}
	})

	t.Run("a genuinely unrelated extra positional argument keeps the original refusal", func(t *testing.T) {
		var output bytes.Buffer
		err := RunReviewFacadeStart([]string{"--cwd", repo, "extra-positional-argument"}, &output)
		if err == nil {
			t.Fatal("expected a refusal for an unrelated extra argument")
		}
		message := err.Error()
		if !strings.Contains(message, `unexpected review start argument "extra-positional-argument"`) {
			t.Fatalf("unrelated extra-argument refusal changed shape unexpectedly: %q", message)
		}
	})
}
