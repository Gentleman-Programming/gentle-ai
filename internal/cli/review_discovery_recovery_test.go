package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestReceiptDiscoveryDenialNamesRunnableReviewStart is the guard for the
// friction the harness measured on the "lifecycle gate before any review
// exists" journey. A pre-commit gate over a repository that holds NO terminal
// receipt at all denied with
//
//	review lifecycle gate denied: invalidated: requires explicit maintainer action
//
// which sends the operator at a maintainer for a block the operator can clear
// themselves, and names nothing runnable. The negotiated envelope already
// carried the truth ("no terminal review receipt exists for gate validation",
// denial code receipt_missing); only the human surface threw it away.
//
// Both discovery kinds covered here mean the same thing -- discovery COMPLETED
// and proved that no terminal receipt governs this candidate -- so reviewing
// the candidate is the continuation, exactly as the already-shipped
// all-stale ambiguous branch words it.
//
// The test does not assert wording alone. It extracts the command out of the
// denial the product emitted, dispatches EXACTLY that command through the real
// CLI router, drives the review it starts to its own terminal receipt, and
// requires the same gate to then allow. A future denial whose named command
// dead-ends fails here.
func TestReceiptDiscoveryDenialNamesRunnableReviewStart(t *testing.T) {
	tests := []struct {
		name     string
		wantKind ReviewReceiptDiscoveryKind
		stage    func(t *testing.T, repo string)
	}{
		{
			// Nothing was ever reviewed in this repository: terminalCount == 0
			// in discoverCompactFacadeGateReview, so discovery returns
			// ReviewReceiptMissing.
			name:     "no terminal receipt exists at all",
			wantKind: ReviewReceiptMissing,
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				runReviewCLIGit(t, repo, "add", "alpha.txt")
			},
		},
		{
			// A terminal receipt exists, but it governs a target that shares no
			// path with the live candidate, so every leaf assesses as
			// CompactGateTargetUnrelated and discovery returns
			// ReviewReceiptUnrelated.
			name:     "terminal receipts exist only for unrelated targets",
			wantKind: ReviewReceiptUnrelated,
			stage: func(t *testing.T, repo string) {
				writeScopeRecoveryFile(t, repo, "alpha.txt", "alpha\n")
				runReviewCLIGit(t, repo, "add", "-N", "alpha.txt")
				finalizeFacadeReviewForRepo(t, repo)
				runReviewCLIGit(t, repo, "add", "alpha.txt")
				runReviewCLIGit(t, repo, "commit", "-qm", "feat: alpha")
				writeScopeRecoveryFile(t, repo, "beta.txt", "beta\n")
				runReviewCLIGit(t, repo, "add", "beta.txt")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			test.stage(t, repo)

			var denied bytes.Buffer
			err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &denied)
			if err == nil {
				t.Fatalf("pre-commit allowed a candidate no receipt governs: %s", denied.String())
			}
			var gateErr ReviewGateDeniedError
			if !errors.As(err, &gateErr) {
				t.Fatalf("pre-commit denial = %T %v, want ReviewGateDeniedError", err, err)
			}
			if gateErr.Result != reviewtransaction.GateInvalidated {
				t.Fatalf("pre-commit gate result = %q, want invalidated\n%s", gateErr.Result, denied.String())
			}
			wantDenial := reviewtransaction.GateDenial{Stage: "receipt-discovery", Code: string(test.wantKind)}
			if gateErr.Context.Denial == nil || *gateErr.Context.Denial != wantDenial {
				t.Fatalf("pre-commit denial = %#v, want %#v", gateErr.Context.Denial, wantDenial)
			}

			message := gateErr.Error()
			if strings.Contains(message, "requires explicit maintainer action") {
				t.Fatalf("discovery denial still routes the operator to a maintainer for a block they can clear: %q", message)
			}
			// Execute EXACTLY the command the denial named, read out of the
			// message itself so the test can only pass when the product's own
			// recommendation is the one that works.
			runNamedReviewContinuation(t, repo, message)

			var validated bytes.Buffer
			if err := RunReviewFacadeValidate([]string{"--cwd", repo, "--gate", "pre-commit"}, &validated); err != nil {
				t.Fatalf("gate still denied after running the continuation the message named: %v\n%s", err, validated.String())
			}
			assertReviewGateResult(t, validated.Bytes(), reviewtransaction.GateAllow)
		})
	}
}

// runNamedReviewContinuation parses the single runnable `gentle-ai review ...`
// command out of a denial message, dispatches it through the real CLI router,
// and drives the review it starts to a terminal receipt exactly as an operator
// following the message would. Every argument comes from the message; nothing
// the test knows independently is substituted.
func runNamedReviewContinuation(t *testing.T, repo, message string) {
	t.Helper()
	tokens := namedReviewCommandTokens(t, message)
	if len(tokens) < 2 || tokens[0] != "review" {
		t.Fatalf("denial named %v, want a gentle-ai review <verb> continuation: %q", tokens, message)
	}
	var output bytes.Buffer
	if err := RunReview(append(append([]string{}, tokens[1:]...), "--cwd", repo), &output); err != nil {
		t.Fatalf("the command the denial named exits non-zero (named dead end): gentle-ai %s: %v\n%s",
			strings.Join(tokens, " "), err, output.String())
	}
	var started ReviewFacadeStartResult
	if err := json.Unmarshal(output.Bytes(), &started); err != nil {
		t.Fatalf("continuation output is not a review start envelope: %v\n%s", err, output.String())
	}
	finalizeArgs := append([]string{"--cwd", repo, "--lineage", started.LineageID}, facadeReviewerResultArgs(t, repo, started)...)
	if len(started.SelectedLenses) > 0 {
		evidencePath := filepath.Join(t.TempDir(), "evidence.txt")
		if err := os.WriteFile(evidencePath, []byte("focused tests pass\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		finalizeArgs = append(finalizeArgs, "--evidence", evidencePath)
	}
	if err := RunReviewFacadeFinalize(finalizeArgs, io.Discard); err != nil {
		t.Fatalf("finalizing the review the named continuation started failed: %v", err)
	}
}

// namedReviewCommandTokens returns the argument tokens of the first
// `gentle-ai ...` command in a message, and fails when the message names none
// or leaves an unfilled <placeholder> the operator would still have to
// resolve. It mirrors what the friction harness's classifier accepts as a
// runnable command.
func namedReviewCommandTokens(t *testing.T, message string) []string {
	t.Helper()
	const product = "gentle-ai "
	index := strings.Index(message, product)
	if index < 0 {
		t.Fatalf("denial names no runnable gentle-ai command: %q", message)
	}
	tail := message[index+len(product):]
	if cut := strings.IndexAny(tail, "\n"); cut >= 0 {
		tail = tail[:cut]
	}
	tokens := []string{}
	for _, token := range strings.Fields(tail) {
		token = strings.Trim(token, ",.;:'\"`)]")
		if token == "" || strings.HasPrefix(token, "<") {
			break
		}
		tokens = append(tokens, token)
	}
	return tokens
}
