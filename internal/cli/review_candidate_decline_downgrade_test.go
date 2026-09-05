package cli

// Wave 5 (Gate Cutover), Slice 6: candidate decline downgrade to ordinary
// unmanaged delivery (design decision 6). This file carries the slice's two
// named RED tests (tasks 7.1, 7.2) plus the "after" pictures superseding
// internal/cli/review_wave5_characterization_test.go's S1 pin
// (TestCandidateDeclineCharacterization_ResolveCandidateDeclineForGate) and
// the pre-existing review_candidate_decline_test.go /
// review_candidate_decline_delivery_test.go suites, all of which drove the
// now-deleted ResolveCandidateDeclineForGate/RecordCandidateDecline gate-side
// path directly.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// candidateDeclineDowngradeScanRoots mirrors review_pre_pr_composition_deletion_test.go's
// own scan-root resolution (Wave 5 Slice 5 precedent) -- the same two
// production trees a call to ResolveCandidateDeclineForGate or
// RecordCandidateDecline could legitimately appear in.
func candidateDeclineDowngradeScanRoots(t *testing.T) []string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve candidate decline downgrade test source")
	}
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	return []string{
		filepath.Join(moduleRoot, "internal", "cli"),
		filepath.Join(moduleRoot, "internal", "reviewtransaction"),
	}
}

// scanForCandidateDeclineGateAuthorizationCalls walks every non-test .go file
// under each root and reports every CallExpr resolving to
// ResolveCandidateDeclineForGate or RecordCandidateDecline -- a pure
// name-match scan (no type resolution), the exact technique
// TestPrePRComposition_ZeroCallers (Slice 5) already established: after this
// slice deletes both functions, ANY match is unambiguously a regression,
// since no production code path can name a symbol that does not exist.
func scanForCandidateDeclineGateAuthorizationCalls(t *testing.T, roots []string) []string {
	t.Helper()
	forbidden := map[string]bool{"ResolveCandidateDeclineForGate": true, "RecordCandidateDecline": true}
	var violations []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read %s: %v", root, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !hasGoExtension(name) || isTestFile(name) {
				continue
			}
			path := filepath.Join(root, name)
			fileSet := token.NewFileSet()
			tree, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(tree, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				var callee string
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					callee = fun.Name
				case *ast.SelectorExpr:
					callee = fun.Sel.Name
				}
				if forbidden[callee] {
					violations = append(violations, fileSet.Position(call.Pos()).String()+": "+callee)
				}
				return true
			})
		}
	}
	return violations
}

func hasGoExtension(name string) bool { return len(name) > 3 && name[len(name)-3:] == ".go" }
func isTestFile(name string) bool     { return len(name) > 8 && name[len(name)-8:] == "_test.go" }

// TestCandidateDecline_ZeroCallers is task 7.1: no code path constructs
// delivery authorization from a decline record. Before this slice's
// deletion this scan finds review_facade.go's exact funnel branch
// (ResolveCandidateDeclineForGate) and consent-decline call site
// (RecordCandidateDecline); after deletion it finds zero, permanently,
// because neither symbol exists to name.
func TestCandidateDecline_ZeroCallers(t *testing.T) {
	violations := scanForCandidateDeclineGateAuthorizationCalls(t, candidateDeclineDowngradeScanRoots(t))
	if len(violations) != 0 {
		t.Fatalf("candidate decline gate authorization is still constructed from production code (must be zero after Slice 6's deletion): %v", violations)
	}
}

// TestCandidateDecline_UnmanagedDelivery_ByteIdenticalToDisabled is task
// 7.2: a candidate declined at review start, then gated while reviews are
// disabled, produces output byte-identical to the disabled-unmanaged
// golden an otherwise-IDENTICAL, never-declined candidate produces in the
// same disabled repo. Decline leaves zero durable trace to differentiate
// the two (RecordCandidateDecline is deleted; nothing is ever persisted),
// so the ONLY thing distinguishing the two runs is that one repo's
// operator once said "no" to a consent question that no longer has any
// review-authority ceremony left to matter.
func TestCandidateDecline_UnmanagedDelivery_ByteIdenticalToDisabled(t *testing.T) {
	const candidatePath = "scripts/deploy.sh"
	const candidateContents = "#!/bin/sh\necho deploy\n"

	// Declined repo: consent relay declines the exact candidate, THEN
	// reviews are disabled, THEN the identical candidate is gated.
	declinedRepo := func() string {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		stubReviewConsole(t, false, "")
		writeReviewStartCandidate(t, repo, candidatePath, candidateContents, 0o644)
		question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
			"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
			"--lineage", "review-decline-downgrade-byte-identical", "--consent", "relay",
		})).Bytes())
		declined := runConsentRelayStart(t, invocationArgs(t, question.Choices[1].Invocation))
		var declinedResult ReviewFacadeStartResult
		decodeStrictReviewJSON(t, declined.Bytes(), &declinedResult)
		if declinedResult.Action != "declined" {
			t.Fatalf("decline did not record: %#v", declinedResult)
		}
		runReviewCLIGit(t, repo, "add", candidatePath)
		disableReviewForClone(t, repo)
		return repo
	}()

	// Control repo: the byte-identical candidate, never declined, in an
	// otherwise-identical disabled repo.
	controlRepo := func() string {
		reviewEnabledHome(t)
		repo := initReviewCLIRepo(t)
		writeReviewStartCandidate(t, repo, candidatePath, candidateContents, 0o644)
		runReviewCLIGit(t, repo, "add", candidatePath)
		disableReviewForClone(t, repo)
		return repo
	}()

	var declinedOutput bytes.Buffer
	declinedErr := RunReviewFacadeValidate([]string{"--cwd", declinedRepo, "--gate", string(reviewtransaction.GatePreCommit)}, &declinedOutput)
	assertDisabledUnmanagedGate(t, declinedErr, declinedOutput.Bytes())

	var controlOutput bytes.Buffer
	controlErr := RunReviewFacadeValidate([]string{"--cwd", controlRepo, "--gate", string(reviewtransaction.GatePreCommit)}, &controlOutput)
	assertDisabledUnmanagedGate(t, controlErr, controlOutput.Bytes())

	if !bytes.Equal(declinedOutput.Bytes(), controlOutput.Bytes()) {
		t.Fatalf("declined candidate's disabled-unmanaged output is not byte-identical to a never-declined candidate's:\ndeclined:\n%s\ncontrol:\n%s",
			declinedOutput.String(), controlOutput.String())
	}
}
