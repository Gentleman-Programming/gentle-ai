package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

func TestDeriveProviderCausalCarrierRequiresAffirmativeProofTreeEvidence(t *testing.T) {
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\nunchanged\n")
	writeSnapshotFile(t, repo, "other.txt", "unchanged\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt", "other.txt")
	gitSnapshot(t, repo, "commit", "-m", "base")
	baseTree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
	writeSnapshotFile(t, repo, "tracked.txt", "candidate\nunchanged\n")
	gitSnapshot(t, repo, "add", "--", "tracked.txt")
	gitSnapshot(t, repo, "commit", "-m", "candidate")
	candidateTree := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
	candidate := CandidateIdentity{RepositoryID: "repo", BaseTree: baseTree, CandidateTree: candidateTree, PolicyHash: "sha256:" + hash("policy")}
	subject := hash("a")
	for _, tt := range []struct {
		name  string
		claim ProviderCausalEvidence
		want  ProviderCausalClassification
	}{
		{name: "affirmative changed line", claim: ProviderCausalEvidence{FindingID: "R3-affirmative", Location: "tracked.txt:1", ProofRefs: []string{"tracked.txt:1"}}, want: ProviderCandidateCausal},
		{name: "proof references unchanged path", claim: ProviderCausalEvidence{FindingID: "R3-mismatched", Location: "tracked.txt:1", ProofRefs: []string{"other.txt:1"}}, want: ProviderUnknown},
		{name: "range is not fully changed", claim: ProviderCausalEvidence{FindingID: "R3-partial-range", Location: "tracked.txt:1-2", ProofRefs: []string{"tracked.txt:1-2"}}, want: ProviderUnknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			carrier, err := DeriveProviderCausalCarrier(context.Background(), repo, subject, candidate, []ProviderCausalEvidence{tt.claim})
			if err != nil {
				t.Fatalf("DeriveProviderCausalCarrier: %v", err)
			}
			if got := carrier.Findings[0].Classification; got != tt.want {
				t.Fatalf("classification = %q, want %q", got, tt.want)
			}
		})
	}
}
