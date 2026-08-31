package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// startStatusContinuationExecution asserts the invariant shape every reviewing
// start/v4 continuation shares (issue #3894) and returns its execution: one
// executable review.status vector whose rows all carry their literal
// --name=value token, whose selector echo is byte-identical to the trailing
// scope rows, and whose command is exactly the tokens it publishes.
func startStatusContinuationExecution(t *testing.T, started ReviewIntegrationStartResult, wantSelectors []ReviewTransitionArgument) *ReviewTransitionExecution {
	t.Helper()
	if started.Schema != ReviewIntegrationStartSchemaV4 || started.State != reviewtransaction.StateReviewing {
		t.Fatalf("reviewing start/v4 identity = %q state %q", started.Schema, started.State)
	}
	transition := started.NextTransition
	if transition == nil || transition.Kind != "execute" || transition.ReasonCode != "review_status_required" ||
		transition.Execute == nil || transition.Execute.Operation != "review.status" {
		t.Fatalf("reviewing START continuation = %#v", transition)
	}
	execution := transition.Execute
	tokens := make([]string, 0, len(execution.Arguments))
	for _, argument := range execution.Arguments {
		if argument.Name == "cwd" {
			t.Fatalf("START continuation published a filesystem path argument: %#v", execution.Arguments)
		}
		if argument.Token != "--"+argument.Name+"="+argument.Value {
			t.Fatalf("START continuation token %#v is not its literal --name=value form", argument)
		}
		tokens = append(tokens, argument.Token)
	}
	if execution.Command != "gentle-ai review status "+strings.Join(tokens, " ") {
		t.Fatalf("START continuation command %q does not execute exactly its arguments %v", execution.Command, tokens)
	}
	for index, name := range []string{"contract", "next-transition", "lineage"} {
		if execution.Arguments[index].Name != name {
			t.Fatalf("START continuation argument order = %#v", execution.Arguments)
		}
	}
	if execution.Arguments[0].Value != ReviewIntegrationContractV2 || execution.Arguments[1].Value != "true" ||
		execution.Arguments[2].Value != started.LineageID {
		t.Fatalf("START continuation binding rows = %#v", execution.Arguments)
	}
	tokenizedSelectors := reviewTokenizedTransitionArguments(wantSelectors)
	trailing := execution.Arguments[len(execution.Arguments)-len(tokenizedSelectors):]
	if !reflect.DeepEqual(trailing, tokenizedSelectors) {
		t.Fatalf("START continuation scope rows = %#v, want %#v", trailing, tokenizedSelectors)
	}
	if execution.SelectorArguments == nil || !reflect.DeepEqual(*execution.SelectorArguments, tokenizedSelectors) {
		t.Fatalf("START continuation selector echo = %#v, want %#v", execution.SelectorArguments, tokenizedSelectors)
	}
	if started.RepositoryContext == nil ||
		execution.Binding.LineageID != started.LineageID ||
		execution.Binding.TargetIdentity != started.RepositoryContext.TargetIdentity ||
		execution.Binding.Revision != started.RepositoryContext.Revision {
		t.Fatalf("START continuation binding = %#v, context %#v", execution.Binding, started.RepositoryContext)
	}
	if len(execution.Preconditions) != 1 || execution.Preconditions[0] != (ReviewTransitionArgument{Name: "state", Value: string(reviewtransaction.StateReviewing)}) {
		t.Fatalf("START continuation preconditions = %#v", execution.Preconditions)
	}
	return execution
}

func TestReviewingStartPublishesExecutableStatusContinuation(t *testing.T) {
	reviewEnabledHome(t)

	t.Run("current projection", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc Candidate() int { return 1 }\n", 0o644)
		started := runNegotiatedReviewStart(t, repo, "continuation-current")
		startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
			{Name: "projection", Value: string(reviewtransaction.ProjectionWorkspace)},
		})
	})

	t.Run("committed base-ref", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte("package candidate\n\nfunc Candidate() int { return 2 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "candidate.go")
		runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
		var output bytes.Buffer
		if err := RunReview(boundNegotiatedStartArgs(t, []string{
			"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
			"--lineage", "continuation-committed", "--base-ref", "HEAD~1",
		}), &output); err != nil {
			t.Fatal(negotiatedReviewStartFailure(err, output.String()))
		}
		started := decodeNegotiatedReviewStart(t, output.Bytes())
		if err := started.Validate(); err != nil {
			t.Fatal(err)
		}
		execution := startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
			{Name: "base-ref", Value: started.BaseTree},
			{Name: "committed-only", Value: "true"},
		})
		if !validReviewGitTree(execution.Arguments[len(execution.Arguments)-2].Value) {
			t.Fatalf("committed continuation base-ref is not the frozen base tree: %#v", execution.Arguments)
		}
	})

	t.Run("workspace overlay", func(t *testing.T) {
		repo := initReviewCLIRepo(t)
		if err := os.WriteFile(filepath.Join(repo, "candidate.go"), []byte("package candidate\n\nfunc Candidate() int { return 3 }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "candidate.go")
		runReviewCLIGit(t, repo, "commit", "-qm", "candidate")
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\noverlay\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := RunReview(boundNegotiatedStartArgs(t, []string{
			"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
			"--lineage", "continuation-overlay", "--base-ref", "HEAD~1", "--workspace-overlay",
		}), &output); err != nil {
			t.Fatal(negotiatedReviewStartFailure(err, output.String()))
		}
		started := decodeNegotiatedReviewStart(t, output.Bytes())
		if err := started.Validate(); err != nil {
			t.Fatal(err)
		}
		startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
			{Name: "base-ref", Value: started.BaseTree},
			{Name: "workspace-overlay", Value: "true"},
		})
	})
}

// TestOpenCodeRunsTheStartStatusContinuationVerbatim is the runtime proof
// issue #3894 requires: the OpenCode runtime declares itself on the negotiated
// START, takes the emitted continuation, and mechanically executes its command
// tokens unchanged through the CLI entrypoint — no selector is re-derived, no
// flag is appended — and receives the negotiated STATUS for the same lineage.
func TestOpenCodeRunsTheStartStatusContinuationVerbatim(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	writeReviewStartCandidate(t, repo, "candidate.go", "package candidate\n\nfunc Candidate() int { return 4 }\n", 0o644)
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "continuation-opencode", "--agent", "opencode",
	}), &output); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, output.String()))
	}
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := started.Validate(); err != nil {
		t.Fatal(err)
	}
	execution := startStatusContinuationExecution(t, started, []ReviewTransitionArgument{
		{Name: "projection", Value: string(reviewtransaction.ProjectionWorkspace)},
	})
	if agent := execution.Arguments[3]; agent.Name != "agent" || agent.Value != "opencode" {
		t.Fatalf("declared runtime row = %#v", execution.Arguments)
	}

	// The continuation names no --cwd (a negotiated START publishes no
	// filesystem path), so the shipped contract says to run it with the
	// repository as the process working directory.
	fields := strings.Fields(execution.Command)
	if len(fields) < 3 || fields[0] != "gentle-ai" || fields[1] != "review" || fields[2] != "status" {
		t.Fatalf("continuation command = %q", execution.Command)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	var statusOutput bytes.Buffer
	if err := RunReview(fields[2:], &statusOutput); err != nil {
		t.Fatalf("continuation STATUS: %v\n%s", err, statusOutput.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, statusOutput.Bytes(), &status)
	if status.Schema != ReviewIntegrationStatusSchema || status.Authority == nil ||
		status.Authority.LineageID != started.LineageID || status.Authority.State != reviewtransaction.StateReviewing {
		t.Fatalf("continuation STATUS authority = %#v", status.Authority)
	}
	if status.NextTransition == nil {
		t.Fatal("continuation STATUS returned no next transition for the reviewing lineage")
	}
}
