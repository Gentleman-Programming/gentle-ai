package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestStatusStartTransitionPreservesFrozenTarget(t *testing.T) {
	t.Run("workspace and explicit lineage", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "tracked.txt", "workspace\n", 0o644)
		status := negotiatedStartStatus(t, repo, "--lineage", "status-start-workspace")
		assertStartTransition(t, status, []string{"contract", "target", "projection", "lineage"})
		started := executeStartTransition(t, repo, status)
		if negotiatedStartTarget(started) != status.TargetIdentity || started.LineageID != "status-start-workspace" {
			t.Fatalf("START = %#v, status target = %q", started, status.TargetIdentity)
		}
	})

	t.Run("symbolic base movement", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		runReviewCLIGit(t, repo, "branch", "moving-base", "HEAD")
		writeReviewStartCandidate(t, repo, "tracked.txt", "committed\n", 0o644)
		runReviewCLIGit(t, repo, "add", "tracked.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
		status := negotiatedStartStatus(t, repo, "--base-ref", "moving-base")
		assertStartTransition(t, status, []string{"contract", "target", "projection", "base-ref", "committed-only"})
		if got := startTransitionArgumentValue(t, status, "base-ref"); got != status.Projection.BaseTree {
			t.Fatalf("emitted base-ref = %q, want resolved tree %q", got, status.Projection.BaseTree)
		}
		runReviewCLIGit(t, repo, "update-ref", "refs/heads/moving-base", "HEAD")
		if started := executeStartTransition(t, repo, status); negotiatedStartTarget(started) != status.TargetIdentity {
			t.Fatalf("START target = %q, want %q", negotiatedStartTarget(started), status.TargetIdentity)
		}
	})

	t.Run("workspace overlay", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
		writeReviewStartCandidate(t, repo, "committed.txt", "committed\n", 0o644)
		runReviewCLIGit(t, repo, "add", "committed.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
		writeReviewStartCandidate(t, repo, "tracked.txt", "overlay\n", 0o644)
		status := negotiatedStartStatus(t, repo, "--base-tree", base, "--workspace-overlay")
		assertStartTransition(t, status, []string{"contract", "target", "projection", "base-ref", "workspace-overlay"})
		if started := executeStartTransition(t, repo, status); started.TargetIdentity != status.TargetIdentity || started.TargetMode != reviewtransaction.TargetBaseWorkspaceOverlay {
			t.Fatalf("overlay START = %#v, want target %q", started, status.TargetIdentity)
		}
	})

	t.Run("staged unborn", func(t *testing.T) {
		repo := t.TempDir()
		runReviewCLIGit(t, repo, "init", "-q")
		runReviewCLIGit(t, repo, "config", "user.email", "test@example.com")
		runReviewCLIGit(t, repo, "config", "user.name", "Test")
		writeReviewStartCandidate(t, repo, "first.txt", "first\n", 0o644)
		runReviewCLIGit(t, repo, "add", "first.txt")
		status := negotiatedStartStatus(t, repo, "--projection", "staged")
		assertStartTransition(t, status, []string{"contract", "target", "projection"})
		if started := executeStartTransition(t, repo, status); negotiatedStartTarget(started) != status.TargetIdentity || started.Projection != reviewtransaction.ProjectionStaged {
			t.Fatalf("unborn START = %#v, want target %q", started, status.TargetIdentity)
		}
	})
}

func TestNegotiatedV2FreshStatusIncludesExactConsentRelay(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	status := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--lineage", "status-v2-consent-relay")
	assertStartTransition(t, status, []string{"contract", "target", "projection", "lineage", "consent"})
	consent := status.NextTransition.Execute.Arguments[len(status.NextTransition.Execute.Arguments)-1]
	if consent != (ReviewTransitionArgument{Name: "consent", Value: "relay", Token: "--consent=relay"}) {
		t.Fatalf("v2 START consent argument = %#v", consent)
	}
	wantCommand := "gentle-ai review start" +
		" --contract=" + ReviewIntegrationContractV2 +
		" --target=" + status.TargetIdentity +
		" --projection=workspace" +
		" --lineage=status-v2-consent-relay" +
		" --consent=relay"
	if status.NextTransition.Execute.Command != wantCommand {
		t.Fatalf("v2 START command = %q, want %q", status.NextTransition.Execute.Command, wantCommand)
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("v2 fresh STATUS does not validate: %v", err)
	}
	invalid := status
	transition := *status.NextTransition
	execution := *status.NextTransition.Execute
	execution.Arguments = append([]ReviewTransitionArgument(nil), execution.Arguments[:len(execution.Arguments)-1]...)
	transition.Execute = &execution
	invalid.NextTransition = &transition
	if err := invalid.Validate(); err == nil {
		t.Fatal("v2 fresh STATUS accepted a START transition without consent=relay")
	}

	legacy := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV1, "--lineage", "status-v1-compatible")
	assertStartTransition(t, legacy, []string{"contract", "target", "projection", "lineage"})
	if strings.Contains(legacy.NextTransition.Execute.Command, "--consent") {
		t.Fatalf("v1 START command changed compatibility behavior: %s", legacy.NextTransition.Execute.Command)
	}
}

func TestNegotiatedStartRejectsIncompleteBindingsWithoutAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	status := negotiatedStartStatus(t, repo)
	valid := []string{"--contract=" + ReviewIntegrationContractV1, "--cwd=" + repo, "--target=" + status.TargetIdentity, "--projection=workspace", "--lineage=invalid-start-binding"}
	otherTarget := "sha256:" + strings.Repeat("0", 64)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing target", []string{valid[0], valid[1], valid[3], valid[4]}, ""},
		{"missing contract", valid[1:], ""},
		{"missing projection", append(append([]string{}, valid[:3]...), valid[4]), ""},
		{"mixed contract aliases", append([]string{"-contract=unsupported"}, valid...), "repeats --contract"},
		{"mixed target aliases", append(append([]string{}, valid...), "-target="+otherTarget), "repeats --target"},
		{"mixed projection aliases", append(append([]string{}, valid...), "-projection=staged"), "repeats --projection"},
		{"mixed lineage aliases", append(append([]string{}, valid...), "-lineage=other-lineage"), "repeats --lineage"},
		{"mixed base-ref aliases", append(append([]string{}, valid...), "--base-ref=HEAD", "-base-ref=HEAD^"), "repeats --base-ref"},
		{"mixed committed-only aliases", append(append([]string{}, valid...), "--committed-only=true", "-committed-only=false"), "repeats --committed-only"},
		{"mixed workspace-overlay aliases", append(append([]string{}, valid...), "--workspace-overlay=true", "-workspace-overlay=false"), "repeats --workspace-overlay"},
		{"malformed target", []string{valid[0], valid[1], "--target=not-a-target", valid[3], valid[4]}, ""},
		{"malformed lineage", []string{valid[0], valid[1], valid[2], valid[3], "--lineage=Not_Canonical"}, ""},
		{"mismatched target", []string{valid[0], valid[1], "--target=" + otherTarget, valid[3], valid[4]}, ""},
		{"partial base diff", append(append([]string{}, valid...), "--base-ref=HEAD"), ""},
		{"partial overlay", append(append([]string{}, valid...), "--workspace-overlay"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunReviewFacadeStart(tt.args, io.Discard)
			if err == nil {
				t.Fatalf("START accepted %v", tt.args)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("START error = %v, want %q", err, tt.want)
			}
			stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if len(stores) != 0 {
				t.Fatalf("invalid START created authority: %#v", stores)
			}
		})
	}

	t.Run("failure envelope preserves lineage", func(t *testing.T) {
		var output bytes.Buffer
		args := append([]string{"start"}, valid...)
		args = append(args, "-target="+otherTarget)
		if err := RunReview(args, &output); err == nil {
			t.Fatal("duplicate negotiated START succeeded")
		}
		failure := decodeReviewIntegrationFailure(t, output.Bytes())
		if failure.LineageID != "invalid-start-binding" || failure.MutationOutcome != ReviewMutationNotStarted {
			t.Fatalf("START failure = %#v", failure)
		}
	})
}

func TestLegacyStartPreservesDuplicateNonBindingFlags(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	if err := RunReviewFacadeStart([]string{
		"--cwd", repo, "--cwd", repo,
		"--policy=", "--policy=",
		"--focus=risk", "--focus=reliability",
		"--trace=", "--trace=",
		"--lineage", "legacy-duplicate-flags",
	}, io.Discard); err != nil {
		t.Fatalf("legacy duplicate flags: %v", err)
	}
}

func TestNegotiatedStartRevalidatesFrozenTargetBeforeCreate(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	status := negotiatedStartStatus(t, repo, "--lineage", "start-live-revalidation")
	original := renderReviewStartFrozenCandidateContext
	renderReviewStartFrozenCandidateContext = func(ctx context.Context, builder reviewtransaction.SnapshotBuilder, snapshot reviewtransaction.Snapshot) (reviewtransaction.FrozenCandidateContext, error) {
		frozen, err := original(ctx, builder, snapshot)
		if err == nil {
			err = os.WriteFile(repo+"/tracked.txt", []byte("drifted\n"), 0o644)
		}
		return frozen, err
	}
	t.Cleanup(func() { renderReviewStartFrozenCandidateContext = original })
	args := transitionStartArgs(repo, status)
	if err := RunReview(args, io.Discard); err == nil {
		t.Fatal("START published authority after live target drift")
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("drifted START created authority: %#v", stores)
	}
}

func TestStatusRejectsNonCanonicalStartTransition(t *testing.T) {
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "tracked.txt", "candidate\n", 0o644)
	valid := negotiatedStartStatus(t, repo)
	for _, mutate := range []func(*ReviewTargetStatusResult){
		func(status *ReviewTargetStatusResult) {
			status.NextTransition.Execute.Arguments = status.NextTransition.Execute.Arguments[1:]
		},
		func(status *ReviewTargetStatusResult) {
			status.NextTransition.Execute.Arguments = append(status.NextTransition.Execute.Arguments, ReviewTransitionArgument{Name: "base-tree", Value: status.Projection.BaseTree})
		},
		func(status *ReviewTargetStatusResult) {
			status.NextTransition.Execute.Preconditions[0].Value = "sha256:" + strings.Repeat("0", 64)
		},
	} {
		invalid := valid
		execution := *valid.NextTransition.Execute
		execution.Arguments = append([]ReviewTransitionArgument(nil), execution.Arguments...)
		execution.Preconditions = append([]ReviewTransitionArgument(nil), execution.Preconditions...)
		transition := *valid.NextTransition
		transition.Execute = &execution
		invalid.NextTransition = &transition
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("status accepted non-canonical START: %#v", invalid.NextTransition)
		}
	}
}

func negotiatedStartStatus(t *testing.T, repo string, selectors ...string) ReviewTargetStatusResult {
	return negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV1, selectors...)
}

func negotiatedStartStatusForContract(t *testing.T, repo, contract string, selectors ...string) ReviewTargetStatusResult {
	t.Helper()
	args := []string{"status", "--contract", contract, "--next-transition", "--action-eligibility", "--cwd", repo}
	args = append(args, selectors...)
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatal(err)
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		t.Fatalf("status did not emit START: %#v", status)
	}
	if len(status.Eligibility.AllowedActions) != 1 || status.Eligibility.AllowedActions[0].Action != "review.start" {
		t.Fatalf("fresh target does not allow START: %#v", status.Eligibility)
	}
	return status
}

func assertStartTransition(t *testing.T, status ReviewTargetStatusResult, names []string) {
	t.Helper()
	got := make([]string, len(status.NextTransition.Execute.Arguments))
	for index, argument := range status.NextTransition.Execute.Arguments {
		got[index] = argument.Name
	}
	if !reflect.DeepEqual(got, names) || len(status.NextTransition.Execute.Preconditions) != 1 ||
		status.NextTransition.Execute.Preconditions[0] != (ReviewTransitionArgument{Name: "target_identity", Value: status.TargetIdentity}) {
		t.Fatalf("START transition = %#v, want args %v", status.NextTransition.Execute, names)
	}
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == "base-tree" {
			t.Fatal("START transition emitted unsupported base-tree")
		}
	}
}

func executeStartTransition(t *testing.T, repo string, status ReviewTargetStatusResult) ReviewIntegrationStartResult {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(transitionStartArgs(repo, status), &output); err != nil {
		t.Fatal(err)
	}
	return decodeNegotiatedReviewStart(t, output.Bytes())
}

func transitionStartArgs(repo string, status ReviewTargetStatusResult) []string {
	args := []string{"start", "--cwd=" + repo}
	for _, argument := range status.NextTransition.Execute.Arguments {
		args = append(args, "--"+argument.Name+"="+argument.Value)
	}
	return args
}

func startTransitionArgumentValue(t *testing.T, status ReviewTargetStatusResult, name string) string {
	t.Helper()
	for _, argument := range status.NextTransition.Execute.Arguments {
		if argument.Name == name {
			return argument.Value
		}
	}
	t.Fatalf("START transition lacks %q: %#v", name, status.NextTransition)
	return ""
}

func negotiatedStartTarget(started ReviewIntegrationStartResult) string {
	if started.TargetIdentity != "" {
		return started.TargetIdentity
	}
	if started.RepositoryContext != nil {
		return started.RepositoryContext.TargetIdentity
	}
	return ""
}
