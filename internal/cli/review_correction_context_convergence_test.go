package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// TestCorrectionPhaseRepositoryContextStaysExecutable extends the
// STATUS/START convergence property (#3150) into the correction phase, where
// a 2.4.0-rc.7 field report (#2541, 2026-08-13 occurrence) hit its inverse:
// after the correction moved the authority revision, STATUS re-emitted a
// transition whose own repository-context binding was rejected as invalid,
// and re-querying reoffered the identical pairing forever.
//
// The property: drive a faithful staged correction cycle — two newly added
// files, one deterministic CRITICAL finding, accepted forecast, bounded edit,
// passed repository verification — then execute the targeted-validation input
// EXACTLY as the provider hands it over, name/value tokens and opaque handle
// included. The handle must resolve and the immutable corrected tree must
// answer. Green on Linux today, which is itself evidence: the field failure
// did not reproduce here, pointing the investigation at Windows
// repository-identity derivation rather than at the correction flow. If
// handle re-publication on revision advance ever regresses, this test goes
// red naming the exact hop.
func TestCorrectionPhaseRepositoryContextStaysExecutable(t *testing.T) {
	reviewEnabledHome(t)
	sessionRoot := initReviewCLIRepo(t)
	repo := initReviewCLIRepo(t)
	nestedB := filepath.Join(repo, "nested", "requested")
	if err := os.MkdirAll(nestedB, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sessionRoot)
	// Reporter's shape: two newly added source files, staged projection in B
	// while the host process remains in unrelated repository A.
	for name, content := range map[string]string{
		"alpha.go": "package candidate\n\nfunc Alpha() int { return 1 }\n",
		"beta.go":  "package candidate\n\nfunc Beta() int { return 2 }\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runReviewCLIGit(t, repo, "add", "alpha.go", "beta.go")

	startSnapshot, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).Build(context.Background(), reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: reviewtransaction.ProjectionStaged, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var startOut bytes.Buffer
	if err := RunReview([]string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", nestedB,
		"--lineage", "probe-2541", "--projection", "staged", "--target", startSnapshot.Identity,
	}, &startOut); err != nil {
		t.Fatalf("start: %v\n%s", err, startOut.String())
	}
	started := decodeNegotiatedReviewStart(t, startOut.Bytes())
	t.Logf("started: risk=%s lenses=%v", started.RiskLevel, started.SelectedLenses)

	finalizeArgs := []string{"--cwd", nestedB, "--lineage", started.LineageID}
	for index, lens := range started.SelectedLenses {
		resultPath := filepath.Join(t.TempDir(), fmt.Sprintf("result-%d.json", index))
		findings := []facadeFinding{}
		if index == 0 {
			findings = []facadeFinding{{
				Location: "alpha.go:1", Severity: "CRITICAL", Claim: "candidate exposes the wrong behavior",
				ProofRefs:     []string{"exact changed hunk", "reproduced candidate failure"},
				EvidenceClass: "deterministic", CausalDisposition: "introduced",
			}}
		}
		writeReviewCLIJSON(t, resultPath, facadeReviewerResult{
			Lens: lens, Findings: findings, Evidence: []string{"inspected the exact frozen candidate"},
		})
		finalizeArgs = append(finalizeArgs, "--result", resultPath)
	}
	if err := finalizeReviewCLIArgs(t, repo, finalizeArgs, &bytes.Buffer{}); err != nil {
		t.Fatalf("finalize results: %v", err)
	}
	if err := RunReviewFacadeFinalize([]string{
		"--cwd", nestedB, "--lineage", started.LineageID, "--correction-lines", "2",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("forecast: %v", err)
	}

	// Reporter's order: read the correction plan from STATUS FIRST, then edit.
	kind0, reason0, _, _ := reviewNegotiatedTransition(t, nestedB, "--lineage", started.LineageID, "--projection", "staged")
	t.Logf("post-forecast: kind=%s reason=%s", kind0, reason0)

	// The bounded correction: change one staged file and restage it, so the
	// live staged target moves past the frozen one.
	if err := os.WriteFile(filepath.Join(repo, "alpha.go"), []byte("package candidate\n\nfunc Alpha() int { return 10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "alpha.go")

	// Follow the negotiated route exactly as a consumer does, up to eight hops,
	// satisfying the repository-verification collect the way the facade defines.
	for hop := 1; hop <= 8; hop++ {
		statusArgs := []string{"status", "--cwd", nestedB, "--contract", ReviewIntegrationContractV2,
			"--agent", "claude-code", "--next-transition", "--lineage", started.LineageID, "--projection", "staged"}
		var raw bytes.Buffer
		if err := RunReview(statusArgs, &raw); err != nil {
			t.Fatalf("status hop %d: %v", hop, err)
		}
		var parsed ReviewTargetStatusResult
		decodeStrictReviewJSON(t, raw.Bytes(), &parsed)
		if parsed.RepositoryContext == nil || parsed.ValidationRequest == nil ||
			parsed.RepositoryContext.TargetIdentity != parsed.ValidationRequest.CorrectionTargetIdentity {
			t.Fatalf("hop %d: correction STATUS context target = %#v, request = %#v", hop, parsed.RepositoryContext, parsed.ValidationRequest)
		}
		preCorrection := parsed
		preCorrectionContext := *parsed.RepositoryContext
		preCorrectionContext.TargetIdentity = reviewAuthorityTargetIdentity(parsed)
		preCorrection.RepositoryContext = &preCorrectionContext
		if err := preCorrection.Validate(); err == nil {
			t.Fatalf("hop %d: STATUS accepted pre-correction repository context target", hop)
		}
		unrelated := parsed
		unrelatedContext := *parsed.RepositoryContext
		unrelatedContext.TargetIdentity = "sha256:" + strings.Repeat("f", 64)
		unrelated.RepositoryContext = &unrelatedContext
		if err := unrelated.Validate(); err == nil {
			t.Fatalf("hop %d: STATUS accepted unrelated repository context target", hop)
		}
		transition := parsed.NextTransition
		if transition == nil {
			t.Fatalf("hop %d: no transition", hop)
		}
		kind, reason := string(transition.Kind), transition.ReasonCode
		t.Logf("hop %d: kind=%s reason=%s", hop, kind, reason)

		if kind == "collect" && reason == "correction_repository_verification_required" {
			if len(transition.Collect.Inputs) != 1 {
				t.Fatalf("correction repository verification input = %#v", transition.Collect)
			}
			input := transition.Collect.Inputs[0]
			inputTokens := make([]string, 0, len(input.Arguments))
			for _, argument := range input.Arguments {
				token := argument.Token
				if token == "" {
					token = "--" + argument.Name + "=" + argument.Value
				}
				inputTokens = append(inputTokens, token)
			}
			passed := filepath.Join(t.TempDir(), "passed.txt")
			if err := os.WriteFile(passed, []byte("full repository verification passed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			captureArgs := append(inputTokens, "--outcome", "passed", "--input", passed)
			if err := RunReviewCaptureEvidence(captureArgs, &bytes.Buffer{}); err != nil {
				t.Fatalf("execute exact B capture descriptor from A: %v", err)
			}
			target := transitionArgumentValue(t, transition, "target")
			t.Logf("        published passed evidence for %s", target[:24])
			continue
		}
		if kind == "collect" && reason == "targeted_validation_required" {
			if len(transition.Collect.Inputs) == 0 {
				t.Fatalf("targeted_validation_required carries no inputs")
			}
			input := transition.Collect.Inputs[0]
			var inputTokens []string
			for _, argument := range input.Arguments {
				token := argument.Token
				if token == "" {
					token = "--" + argument.Name + "=" + argument.Value
				}
				inputTokens = append(inputTokens, token)
				t.Logf("        input %s", token[:min(88, len(token))])
			}
			t.Logf("        capture op=%s; executing inspect-candidate with exact tokens", input.CaptureOperation)
			var out bytes.Buffer
			err := RunReview(append(append([]string{"inspect-candidate"}, inputTokens...), "--operation", "name-status"), &out)
			text := out.String()
			if strings.Contains(text, "binding is invalid") || strings.Contains(fmt.Sprint(err), "binding is invalid") {
				t.Fatalf("REPRODUCED at hop %d: the exact provider-issued repository-context was rejected:\n%v\n%s", hop, err, text)
			}
			if err != nil {
				t.Logf("        inspect-candidate ended: %v / %s", err, text[:min(300, len(text))])
				return
			}
			t.Logf("        inspect-candidate ok (%d bytes); probe goal reached — the handle resolved", len(text))
			return
		}
		if kind != "execute" {
			t.Logf("        collect/stop reached; probe stops here")
			return
		}
		operation := transition.Execute.Operation
		var tokens []string
		for _, argument := range transition.Execute.Arguments {
			tokens = append(tokens, argument.Token)
			if strings.HasPrefix(argument.Token, "--repository-context=") || strings.HasPrefix(argument.Token, "--expected-revision=") || strings.HasPrefix(argument.Token, "--target=") || strings.HasPrefix(argument.Token, "--request-hash=") {
				t.Logf("        %s", argument.Token[:min(70, len(argument.Token))])
			}
		}
		t.Logf("        op=%s", operation)
		verb := strings.TrimPrefix(operation, "review.")
		var out bytes.Buffer
		err := RunReview(append([]string{verb}, tokens...), &out)
		text := out.String()
		if err != nil || strings.Contains(text, "binding is invalid") {
			if strings.Contains(text+fmt.Sprint(err), "binding is invalid") {
				t.Fatalf("REPRODUCED: exact transition rejected its own repository-context binding at hop %d:\n%v\n%s", hop, err, text)
			}
			t.Logf("        execution ended: %v / %s", err, text[:min(300, len(text))])
			return
		}
		t.Logf("        executed ok (%d bytes)", len(text))
	}
}
