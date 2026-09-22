package kickoff

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeAncestryChecker is the double VerifyIntegrationEvidence's own tests
// exercise instead of the real reviewtransaction-backed implementation
// (phase 19): this leaf package never imports internal/reviewtransaction
// for a single boolean question (design.md S4.6).
type fakeAncestryChecker struct {
	isAncestor bool
	err        error
	called     bool
	calledWith [2]string
}

func (f *fakeAncestryChecker) IsAncestor(_ context.Context, ancestor, descendant string) (bool, error) {
	f.called = true
	f.calledWith = [2]string{ancestor, descendant}
	return f.isAncestor, f.err
}

// TestVerifyIntegrationEvidence covers INC-21 phase 20 (P6b), task 20.1's
// four rules: Axiom proves what it can (pr_merged, via checker) and is
// honest about what it cannot (deployment/attestation), and it never
// fabricates a verdict it did not successfully check (T-11).
func TestVerifyIntegrationEvidence(t *testing.T) {
	t.Run("pr_merged with a confirmed ancestor reports verified true", func(t *testing.T) {
		checker := &fakeAncestryChecker{isAncestor: true}
		ev, err := VerifyIntegrationEvidence(context.Background(), Evidence{
			Kind: EvidencePRMerged, Ref: "abc123def", BaseRef: "main",
		}, checker)
		if err != nil {
			t.Fatalf("VerifyIntegrationEvidence() error = %v, want nil", err)
		}
		if !ev.Verified {
			t.Fatal("Verified = false, want true for a confirmed ancestor")
		}
		if !checker.called || checker.calledWith != [2]string{"abc123def", "main"} {
			t.Fatalf("checker called = %v with %v, want called with (abc123def, main)", checker.called, checker.calledWith)
		}
	})

	t.Run("pr_merged without a confirmed ancestor is rejected naming both revisions, never true", func(t *testing.T) {
		checker := &fakeAncestryChecker{isAncestor: false}
		ev, err := VerifyIntegrationEvidence(context.Background(), Evidence{
			Kind: EvidencePRMerged, Ref: "deadbeef", BaseRef: "main",
		}, checker)
		if err == nil {
			t.Fatal("VerifyIntegrationEvidence() error = nil, want a named rejection")
		}
		if !strings.Contains(err.Error(), "deadbeef") || !strings.Contains(err.Error(), "main") {
			t.Fatalf("error = %q, want it to name both the commit and the branch compared", err.Error())
		}
		if ev.Verified {
			t.Fatal("Verified = true for an unconfirmed ancestor; must never fabricate a verdict")
		}
	})

	t.Run("deployment evidence is always verified false without consulting the checker", func(t *testing.T) {
		checker := &fakeAncestryChecker{isAncestor: true} // would lie if consulted
		ev, err := VerifyIntegrationEvidence(context.Background(), Evidence{
			Kind: EvidenceDeployment, Ref: "https://deploys.example/42",
		}, checker)
		if err != nil {
			t.Fatalf("VerifyIntegrationEvidence(deployment) error = %v, want nil", err)
		}
		if ev.Verified {
			t.Fatal("Verified = true for deployment evidence, want false: Axiom cannot prove a deployment happened")
		}
		if checker.called {
			t.Fatal("checker was consulted for deployment evidence; a kind that is never verifiable must never reach the checker")
		}
		if ev.Ref != "https://deploys.example/42" {
			t.Fatalf("Ref = %q, want the operator-supplied reference preserved", ev.Ref)
		}
	})

	t.Run("attestation evidence is always verified false without consulting the checker", func(t *testing.T) {
		checker := &fakeAncestryChecker{isAncestor: true}
		ev, err := VerifyIntegrationEvidence(context.Background(), Evidence{
			Kind: EvidenceAttestation, Ref: "signed off by the release manager",
		}, checker)
		if err != nil {
			t.Fatalf("VerifyIntegrationEvidence(attestation) error = %v, want nil", err)
		}
		if ev.Verified {
			t.Fatal("Verified = true for attestation evidence, want false: a declaration is not a check")
		}
		if checker.called {
			t.Fatal("checker was consulted for attestation evidence; a kind that is never verifiable must never reach the checker")
		}
	})

	t.Run("a checker error propagates and never reports verified true", func(t *testing.T) {
		checkerErr := errors.New("git merge-base failed")
		checker := &fakeAncestryChecker{err: checkerErr}
		ev, err := VerifyIntegrationEvidence(context.Background(), Evidence{
			Kind: EvidencePRMerged, Ref: "abc123def", BaseRef: "main",
		}, checker)
		if !errors.Is(err, checkerErr) {
			t.Fatalf("VerifyIntegrationEvidence() error = %v, want it to wrap %v", err, checkerErr)
		}
		if ev.Verified {
			t.Fatal("Verified = true despite a checker error; must never fabricate a verdict it cannot prove")
		}
	})
}
