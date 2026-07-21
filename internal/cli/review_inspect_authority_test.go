package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const (
	inspectAuthorityTestSchema    = "gentle-ai.review-authority-inspection/v1"
	inspectAuthorityTestOperation = "review/inspect-authority"
)

type inspectAuthorityTestResult struct {
	Schema           string                                             `json:"schema"`
	Operation        string                                             `json:"operation"`
	RepositoryRoot   string                                             `json:"repository_root"`
	Complete         bool                                               `json:"complete"`
	Valid            bool                                               `json:"valid"`
	Totals           reviewtransaction.CompactRecoveryInspectionTotals  `json:"totals"`
	Edges            []reviewtransaction.CompactRecoveryEdgeInspection  `json:"edges"`
	EntryDiagnostics []reviewtransaction.CompactRecoveryEntryDiagnostic `json:"entry_diagnostics"`
}

func TestReviewInspectAuthorityHelpAndArguments(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var output bytes.Buffer
		if err := RunReview([]string{"inspect-authority", "--help"}, &output); err != nil {
			t.Fatalf("review inspect-authority --help: %v", err)
		}
		if !strings.Contains(output.String(), "Usage: gentle-ai review inspect-authority [flags]") ||
			!strings.Contains(output.String(), "--cwd <value>") {
			t.Fatalf("inspect-authority help:\n%s", output.String())
		}
	})

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "positional argument", args: []string{"inspect-authority", "extra"}, want: `unexpected review inspect-authority argument "extra"`},
		{name: "unknown flag", args: []string{"inspect-authority", "--unknown"}, want: "flag provided but not defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunReview(test.args, &output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunReview(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestReviewInspectAuthorityCallsDomainOnceWithCanonicalRoot(t *testing.T) {
	repo := initReviewCLIRepo(t)
	nested := filepath.Join(repo, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	wantRoot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	original := inspectCompactRecoveryEdges
	t.Cleanup(func() { inspectCompactRecoveryEdges = original })
	calls := 0
	inspectCompactRecoveryEdges = func(_ context.Context, gotRoot string) (reviewtransaction.CompactRecoveryInspectionReport, error) {
		calls++
		if gotRoot != wantRoot {
			t.Fatalf("inspector root = %q, want %q", gotRoot, wantRoot)
		}
		return reviewtransaction.CompactRecoveryInspectionReport{Complete: true, Valid: true}, nil
	}

	var output bytes.Buffer
	if err := RunReview([]string{"inspect-authority", "--cwd", nested}, &output); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("InspectCompactRecoveryEdges calls = %d, want 1", calls)
	}
	var result inspectAuthorityTestResult
	decodeStrictReviewJSON(t, output.Bytes(), &result)
	if result.Schema != inspectAuthorityTestSchema || result.Operation != inspectAuthorityTestOperation ||
		result.RepositoryRoot != wantRoot || !result.Complete || !result.Valid || result.Edges == nil || result.EntryDiagnostics == nil {
		t.Fatalf("inspect-authority result = %#v", result)
	}
}

func TestReviewInspectAuthorityReportsMixedCorruptionDeterministicallyWithoutMutation(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeInspectCLIRecoveryPair(t, repo, "a-valid", false, "")
	writeInspectCLIRecoveryPair(t, repo, "b-unchanged", true, "")
	const rawAuthorization = "legacy approval for /private/secret/logical/path"
	writeInspectCLIRecoveryPair(t, repo, "c-malformed", false, rawAuthorization)

	authorityRoot := reviewCLIAuthorityRoot(t, repo)
	broken := filepath.Join(authorityRoot, "v2", "broken-entry")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "review-state.json"), []byte("{broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	validDir := filepath.Join(authorityRoot, "v2", "a-valid-successor")
	for name, contents := range map[string]string{
		"review-receipt.json": "receipt sentinel\n",
		"review-trace.jsonl":  "trace sentinel\n",
	} {
		if err := os.WriteFile(filepath.Join(validDir, name), []byte(contents), 0o640); err != nil {
			t.Fatal(err)
		}
	}

	before := inspectCLIAuthorityTree(t, authorityRoot)
	var first, second bytes.Buffer
	for _, output := range []*bytes.Buffer{&first, &second} {
		if err := RunReview([]string{"inspect-authority", "--cwd", repo}, output); err != nil {
			t.Fatalf("review inspect-authority treated corruption as command failure: %v\n%s", err, output.String())
		}
	}
	if first.String() != second.String() {
		t.Fatalf("inspect-authority output is not deterministic:\n%s\n%s", first.String(), second.String())
	}
	if after := inspectCLIAuthorityTree(t, authorityRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("inspect-authority mutated authority tree:\nbefore=%#v\nafter=%#v", before, after)
	}
	if strings.Contains(first.String(), rawAuthorization) || strings.Contains(first.String(), "/private/secret/logical/path") {
		t.Fatalf("inspect-authority exposed raw recovery authorization:\n%s", first.String())
	}

	var result inspectAuthorityTestResult
	decodeStrictReviewJSON(t, first.Bytes(), &result)
	wantRoot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantTotals := reviewtransaction.CompactRecoveryInspectionTotals{
		CompactEntries: 7, LoadedEntries: 6, Edges: 3, ValidEdges: 1, InvalidEdges: 2, EntryDiagnostics: 1,
	}
	if result.Schema != inspectAuthorityTestSchema || result.Operation != inspectAuthorityTestOperation || result.RepositoryRoot != wantRoot ||
		result.Complete || result.Valid || result.Totals != wantTotals || result.Edges == nil || result.EntryDiagnostics == nil {
		t.Fatalf("inspect-authority result = %#v, want totals %#v", result, wantTotals)
	}
	wantSuccessors := []string{"a-valid-successor", "b-unchanged-successor", "c-malformed-successor"}
	if len(result.Edges) != len(wantSuccessors) {
		t.Fatalf("inspect-authority edges = %#v", result.Edges)
	}
	for index, want := range wantSuccessors {
		if result.Edges[index].SuccessorLineageID != want {
			t.Fatalf("edge[%d] successor = %q, want %q", index, result.Edges[index].SuccessorLineageID, want)
		}
	}
	if !result.Edges[0].Valid || result.Edges[1].Valid || result.Edges[2].Valid ||
		!containsInspectCLIString(result.Edges[1].AnomalyClasses, "unchanged_target") ||
		!containsInspectCLIString(result.Edges[2].AnomalyClasses, "malformed_recovery_authorization") {
		t.Fatalf("inspect-authority edge classifications = %#v", result.Edges)
	}
	if len(result.EntryDiagnostics) != 1 || result.EntryDiagnostics[0].LineageID != "broken-entry" ||
		result.EntryDiagnostics[0].Problem != "malformed_compact_state" {
		t.Fatalf("inspect-authority entry diagnostics = %#v", result.EntryDiagnostics)
	}
}

func TestReviewInspectAuthorityPropagatesContextFailure(t *testing.T) {
	repo := initReviewCLIRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	err := runReviewInspectAuthority(ctx, []string{"--cwd", repo}, &output)
	if !errors.Is(err, context.Canceled) || output.Len() != 0 {
		t.Fatalf("canceled inspection error = %v, output = %q", err, output.String())
	}
}

func writeInspectCLIRecoveryPair(t *testing.T, repo, prefix string, unchanged bool, authorization string) {
	t.Helper()
	path := filepath.Join("docs", prefix+".md")
	absolute := filepath.Join(repo, path)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absolute, []byte("predecessor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	predecessorSnapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	predecessor := newInspectCLIState(t, builder, prefix+"-predecessor", 1, predecessorSnapshot)
	predecessor.State = reviewtransaction.StateEscalated
	predecessorRevision := writeReconcileCLIRecord(t, repo, predecessor)

	if !unchanged {
		if err := os.WriteFile(absolute, []byte("successor\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	successorSnapshot, err := builder.Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, IntendedUntracked: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	successor := newInspectCLIState(t, builder, prefix+"-successor", 2, successorSnapshot)
	successor.Recovery = &reviewtransaction.CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: predecessorRevision,
		Disposition: reviewtransaction.RecoveryEscalated, Reason: "retry", Actor: "maintainer@example.com",
		RecoveredAt: time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC), MaintainerAuthorization: authorization,
	}
	if authorization == "" {
		successor.Recovery.MaintainerAuthorization = "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + predecessor.LineageID +
			"\npredecessor_revision=" + predecessorRevision + "\ntarget_identity=" + successor.InitialSnapshot.Identity +
			"\nactor=maintainer@example.com\nreason=retry"
	}
	writeReconcileCLIRecord(t, repo, successor)
	if err := os.Remove(absolute); err != nil {
		t.Fatal(err)
	}
}

func newInspectCLIState(t *testing.T, builder reviewtransaction.SnapshotBuilder, lineage string, generation int, snapshot reviewtransaction.Snapshot) reviewtransaction.CompactState {
	t.Helper()
	risk, lines, err := builder.ClassifySnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	policy := sha256.Sum256([]byte("inspect-authority-policy-" + lineage))
	state, err := reviewtransaction.NewCompactState(reviewtransaction.Start{
		LineageID: lineage, Mode: reviewtransaction.ModeOrdinaryBounded, Generation: generation,
		Snapshot: snapshot, PolicyHash: "sha256:" + hex.EncodeToString(policy[:]), RiskLevel: risk,
		SelectedLenses: []string{}, OriginalChangedLines: &lines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func inspectCLIAuthorityTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		contents := []byte{}
		if !entry.IsDir() {
			contents, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		snapshot[relative] = info.Mode().String() + "\x00" + string(contents)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func containsInspectCLIString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
