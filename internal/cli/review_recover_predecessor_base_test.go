package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// TestReviewRecoverEscalatedBaseDiffKeepsPredecessorBase covers issues #5038
// and #5420: an escalated base-diff predecessor whose correction was
// committed over an unchanged base B recovers into a base-diff successor over
// that same base tree, whether the caller names B or omits the base selector.
func TestReviewRecoverEscalatedBaseDiffKeepsPredecessorBase(t *testing.T) {
	for _, tt := range []struct {
		name     string
		selector func(base string) []string
	}{
		{"base commit selector", func(base string) []string { return []string{"--base-ref", base, "--committed-only"} }},
		{"no base selector", func(string) []string { return nil }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("candidate\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runReviewCLIGit(t, repo, "add", "--", "tracked.txt")
			runReviewCLIGit(t, repo, "commit", "-m", "candidate")
			lineage := "recover-5038-" + strings.ReplaceAll(tt.name, " ", "-")
			predecessor := escalatedBaseDiffRecoveryFixture(t, repo, lineage, base)
			if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("corrected after escalation\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runReviewCLIGit(t, repo, "commit", "-am", "correction")

			successor := lineage + "-successor"
			identity := reviewRecoverBaseDiffSuccessorIdentity(t, repo, base)
			authorization := reviewRecoveryAuthorization(predecessor.State.LineageID, predecessor.Revision, identity, "maintainer", "corrected candidate")
			var output bytes.Buffer
			err := RunReviewRecover(append([]string{
				"--cwd", repo, "--predecessor-lineage", predecessor.State.LineageID,
				"--expected-predecessor-revision", predecessor.Revision, "--successor-lineage", successor,
				"--disposition", "escalated", "--reason", "corrected candidate", "--actor", "maintainer",
				"--maintainer-authorization", authorization,
			}, tt.selector(base)...), &output)
			if err != nil {
				t.Fatalf("recover over the unchanged predecessor base refused: %v", err)
			}
			var recovered ReviewRecoverResult
			if err := json.Unmarshal(output.Bytes(), &recovered); err != nil {
				t.Fatalf("decode recover result: %v\n%s", err, output.String())
			}
			store, _ := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, successor)
			record, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if record.State.InitialSnapshot.Kind != reviewtransaction.TargetBaseDiff ||
				record.State.InitialSnapshot.BaseTree != predecessor.State.InitialSnapshot.BaseTree ||
				record.State.InitialSnapshot.Identity != identity {
				t.Fatalf("successor snapshot = %#v, want base-diff over predecessor base %s", record.State.InitialSnapshot, predecessor.State.InitialSnapshot.BaseTree)
			}
		})
	}
}
