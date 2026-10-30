package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// acknowledgedRestartCommand extracts the one `gentle-ai review start` command
// the shipped target_already_acknowledged row documents.
var acknowledgedRestartCommand = regexp.MustCompile("`(gentle-ai review start [^`]*)`")

// A consumed target is terminal for STATUS: re-querying it never offers a
// START, so the documented deliberate restart has to be the START path itself,
// declaring the runtime like every other shipped review command.
func TestAcknowledgedTargetRestartIsTheDocumentedStartNotStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	runReviewCLIGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	runReviewCLIGit(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeZeroLensDocumentationCandidate(t, repo)
	runReviewCLIGit(t, repo, "add", "docs/ordinary-guide.md")
	runReviewCLIGit(t, repo, "commit", "-qm", "documentation candidate")
	offered := derivedRangeTerminalStatus(t, repo)
	if offered.NextTransition == nil || offered.NextTransition.Execute == nil {
		t.Fatalf("no committed-range START: %#v", offered)
	}
	runTransitionTokens(t, "start", offered.NextTransition.Execute.Arguments)
	pending := derivedRangeTerminalStatus(t, repo)
	if pending.NextTransition == nil || pending.NextTransition.ReasonCode != "approved_acknowledgement_required" {
		t.Fatalf("no pending acknowledgement: %#v", pending.NextTransition)
	}
	runTransitionTokens(t, "acknowledge-approved", pending.NextTransition.Execute.Arguments)

	var acknowledged ReviewTargetStatusResult
	for attempt := range 2 {
		acknowledged = derivedRangeTerminalStatus(t, repo)
		if acknowledged.TargetIdentity != offered.TargetIdentity || acknowledged.NextTransition == nil ||
			acknowledged.NextTransition.Kind != reviewNextTransitionStop || acknowledged.NextTransition.ReasonCode != "target_already_acknowledged" {
			t.Fatalf("STATUS %d after acknowledgement = %#v, want the target_already_acknowledged stop", attempt, acknowledged.NextTransition)
		}
	}

	row := reviewStopReasonDocsContinuations(t, reviewStopReasonDocsSection(t, assets.MustRead(reviewLedgerContractAsset)))["target_already_acknowledged"]
	matches := acknowledgedRestartCommand.FindAllStringSubmatch(row, -1)
	if len(matches) != 1 {
		t.Fatalf("target_already_acknowledged row names %d `gentle-ai review start` commands, want exactly 1: %s", len(matches), row)
	}
	documented := matches[0][1]
	if !strings.Contains(documented, "--agent {{GENTLE_AI_RUNTIME_AGENT_ID}} ") {
		t.Fatalf("documented restart does not declare the runtime: %s", documented)
	}
	if strings.Contains(row, "gentle-ai review status") {
		t.Fatalf("target_already_acknowledged row still routes the restart through STATUS, which keeps returning this stop: %s", row)
	}
	// Fill the documented slots from this STATUS, as an orchestrator would;
	// this target is a committed range, so the documented range pair applies.
	command := strings.NewReplacer(
		"{{GENTLE_AI_RUNTIME_AGENT_ID}}", string(model.AgentClaudeCode),
		"<B>", repo,
		"<target_identity>", acknowledged.TargetIdentity,
		"<projection.projection>", string(acknowledged.Projection.Projection),
	).Replace(documented)
	if !strings.Contains(row, "`--base-ref <projection.base_tree> --committed-only`") {
		t.Fatalf("documented restart does not name the committed-range pair: %s", row)
	}
	words := append(strings.Fields(command), "--base-ref", acknowledged.Projection.BaseTree, "--committed-only")
	var output bytes.Buffer
	if err := RunReview(words[2:], &output); err != nil {
		t.Fatalf("documented restart %v refused: %v\n%s", words, err, output.String())
	}
	if bytes.Contains(output.Bytes(), []byte("target_already_acknowledged")) {
		t.Fatalf("documented restart did not reach START: %s", output.String())
	}
}

func runTransitionTokens(t *testing.T, verb string, arguments []ReviewTransitionArgument) {
	t.Helper()
	args := []string{verb}
	for _, argument := range arguments {
		args = append(args, argument.Token)
	}
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("%s: %v\n%s", verb, err, output.String())
	}
}
