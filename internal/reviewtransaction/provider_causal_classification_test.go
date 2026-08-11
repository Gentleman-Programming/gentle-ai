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
		{name: "behavior activated without differential proof", claim: ProviderCausalEvidence{FindingID: "R3-behavior", Location: "tracked.txt:1", ProofRefs: []string{"tracked.txt:1"}, CausalDisposition: CausalBehaviorActivated}, want: ProviderUnknown},
		{name: "behavior activated with differential proof", claim: ProviderCausalEvidence{FindingID: "R3-behavior-diff", Location: "tracked.txt:1", ProofRefs: []string{"tracked.txt:1"}, BaseProofRefs: []string{"tracked.txt:1"}, CandidateProofRefs: []string{"tracked.txt:1"}, CausalDisposition: CausalBehaviorActivated}, want: ProviderCandidateCausal},
		{name: "pre-existing with affirmative both-tree proof", claim: ProviderCausalEvidence{FindingID: "R3-pre-existing", Location: "other.txt:1", ProofRefs: []string{"other.txt:1"}, BaseProofRefs: []string{"other.txt:1"}, CandidateProofRefs: []string{"other.txt:1"}, CausalDisposition: CausalPreExisting}, want: ProviderProvenNonCandidate},
		{name: "base-only with changed paired proof", claim: ProviderCausalEvidence{FindingID: "R3-base-only", Location: "tracked.txt:1", ProofRefs: []string{"tracked.txt:1"}, BaseProofRefs: []string{"tracked.txt:1"}, CandidateProofRefs: []string{"tracked.txt:1"}, CausalDisposition: CausalBaseOnly}, want: ProviderProvenNonCandidate},
		{name: "pre-existing with mismatched proof paths", claim: ProviderCausalEvidence{FindingID: "R3-mismatched-pair", Location: "other.txt:1", ProofRefs: []string{"other.txt:1"}, BaseProofRefs: []string{"other.txt:1"}, CandidateProofRefs: []string{"tracked.txt:1"}, CausalDisposition: CausalPreExisting}, want: ProviderUnknown},
		{name: "pre-existing with missing proof", claim: ProviderCausalEvidence{FindingID: "R3-missing-proof", Location: "other.txt:1", ProofRefs: []string{"other.txt:1"}, BaseProofRefs: []string{"missing.txt:1"}, CandidateProofRefs: []string{"missing.txt:1"}, CausalDisposition: CausalPreExisting}, want: ProviderUnknown},
		{name: "pre-existing with unpaired proof", claim: ProviderCausalEvidence{FindingID: "R3-unpaired-proof", Location: "other.txt:1", ProofRefs: []string{"other.txt:1"}, BaseProofRefs: []string{"other.txt:1"}, CandidateProofRefs: []string{}, CausalDisposition: CausalPreExisting}, want: ProviderUnknown},
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

func TestProviderCausalCarrierRejectsMalformedFrozenProofReferences(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*ProviderCausalFinding)
	}{
		{name: "malformed base proof", mutate: func(finding *ProviderCausalFinding) { finding.BaseProofRefs = []string{"not-a-location"} }},
		{name: "malformed candidate proof", mutate: func(finding *ProviderCausalFinding) { finding.CandidateProofRefs = []string{"not-a-location"} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			finding := ProviderCausalFinding{
				FindingID: "R3-001", BaseProofRefs: []string{"tracked.txt:1"}, CandidateProofRefs: []string{"tracked.txt:1"}, Classification: ProviderProvenNonCandidate,
			}
			tt.mutate(&finding)
			finding.EvidenceDigest = providerFindingDigest(finding)
			carrier := ProviderCausalCarrier{
				SubjectHash: hash("a"), CandidateIdentity: CandidateIdentity{RepositoryID: "repo"}, Findings: []ProviderCausalFinding{finding},
			}
			carrier.AggregateDigest = providerAggregateDigest(carrier)
			if err := carrier.Validate(); err == nil || !strings.Contains(err.Error(), "proof references are not parseable") {
				t.Fatalf("Validate() error = %v, want malformed proof refusal", err)
			}
		})
	}
}
