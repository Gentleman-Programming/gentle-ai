package reviewtransaction

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func providerAdmissionFixture(t *testing.T, findingsByLens map[string][]ProviderCausalClassification) (NewLineageAuthority, ProviderCausalCarrier) {
	t.Helper()
	repo := initSnapshotRepo(t)
	treeOutput, err := runGit(context.Background(), repo, nil, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	candidate := CandidateIdentity{RepositoryID: "provider-admission", BaseTree: strings.TrimSpace(string(treeOutput)), CandidateTree: strings.TrimSpace(string(treeOutput))}
	authority := NewLineageAuthority{LineageID: "provider-admission", State: NewLineageStateReviewing, CandidateIdentity: candidate, Tier: RiskMedium, SelectedLenses: []string{"lens-a", "lens-b"}}
	for lens, classifications := range findingsByLens {
		carrier := ProviderCausalCarrier{SubjectHash: "sha256:" + strings.Repeat(string(lens[len(lens)-1]), 64), CandidateIdentity: candidate}
		for index, classification := range classifications {
			finding := ProviderCausalFinding{FindingID: "finding-" + string(lens[len(lens)-1]) + string(rune('0'+index)), Classification: classification}
			finding.EvidenceDigest = providerFindingDigest(finding)
			carrier.Findings = append(carrier.Findings, finding)
		}
		carrier.AggregateDigest = providerAggregateDigest(carrier)
		authority.CapturedResults = append(authority.CapturedResults, NewLineageCapturedResult{Lens: lens, SubjectHash: carrier.SubjectHash, Provider: carrier})
	}
	return authority, authority.CapturedResults[0].Provider
}

func TestProviderCausalAdmissionTable(t *testing.T) {
	tests := []struct {
		name        string
		class       []ProviderCausalClassification
		wantIDs     []string
		wantUnknown bool
	}{
		{name: "candidate causal is admitted", class: []ProviderCausalClassification{ProviderCandidateCausal}, wantIDs: []string{"finding-a0"}},
		{name: "proven non candidate is non blocking", class: []ProviderCausalClassification{ProviderProvenNonCandidate}},
		{name: "unknown escalates", class: []ProviderCausalClassification{ProviderUnknown}, wantUnknown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authority, _ := providerAdmissionFixture(t, map[string][]ProviderCausalClassification{"lens-a": tt.class, "lens-b": nil})
			gotIDs, gotUnknown, err := authority.ProviderCausalAdmission()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotIDs, tt.wantIDs) || gotUnknown != tt.wantUnknown {
				t.Fatalf("admission = ids %v, unknown %v; want ids %v, unknown %v", gotIDs, gotUnknown, tt.wantIDs, tt.wantUnknown)
			}
		})
	}
}

func TestProviderCausalAdmissionRejectsMissingInvalidTamperedAndConflictingCarriers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewLineageAuthority)
	}{
		{name: "missing carrier", mutate: func(authority *NewLineageAuthority) { authority.CapturedResults = authority.CapturedResults[:1] }},
		{name: "invalid carrier", mutate: func(authority *NewLineageAuthority) {
			authority.CapturedResults[0].Provider.Findings[0].EvidenceDigest = "sha256:" + strings.Repeat("f", 64)
		}},
		{name: "tampered aggregate", mutate: func(authority *NewLineageAuthority) {
			authority.CapturedResults[0].Provider.AggregateDigest = "sha256:" + strings.Repeat("e", 64)
		}},
		{name: "conflicting duplicate classification", mutate: func(authority *NewLineageAuthority) {
			duplicate := authority.CapturedResults[0].Provider.Findings[0]
			duplicate.FindingID = authority.CapturedResults[1].Provider.Findings[0].FindingID
			duplicate.Classification = ProviderProvenNonCandidate
			duplicate.EvidenceDigest = providerFindingDigest(duplicate)
			authority.CapturedResults[0].Provider.Findings = append(authority.CapturedResults[0].Provider.Findings, duplicate)
			authority.CapturedResults[0].Provider.AggregateDigest = providerAggregateDigest(authority.CapturedResults[0].Provider)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authority, _ := providerAdmissionFixture(t, map[string][]ProviderCausalClassification{"lens-a": {ProviderCandidateCausal}, "lens-b": {ProviderCandidateCausal}})
			tt.mutate(&authority)
			if _, _, err := authority.ProviderCausalAdmission(); err == nil {
				t.Fatal("invalid persisted provider authority was accepted")
			}
		})
	}
}

func TestProviderCausalReplayRequiresExactAggregate(t *testing.T) {
	authority, carrier := providerAdmissionFixture(t, map[string][]ProviderCausalClassification{"lens-a": {ProviderCandidateCausal}, "lens-b": nil})
	transition := json.RawMessage(`{"kind":"approve"}`)
	if err := authority.RecordTransition("request-1", transition); err != nil {
		t.Fatal(err)
	}
	revision, err := NewLineageRevisionForState(authority)
	if err != nil {
		t.Fatal(err)
	}
	record := NewLineageRecord{Schema: NewLineageAuthoritySchema, Revision: revision, Authority: authority}
	aggregate, err := authority.ProviderCausalAggregateDigest(revision)
	if err != nil {
		t.Fatal(err)
	}
	got, replay, err := record.ResolveReplayWithProviderCausalDigest("request-1", aggregate)
	if err != nil || !replay || !reflect.DeepEqual(got, transition) {
		t.Fatalf("exact provider replay = %s, %v, %v", got, replay, err)
	}
	if _, replay, err := record.ResolveReplayWithProviderCausalDigest("request-1", carrier.AggregateDigest); err == nil || replay {
		t.Fatalf("mismatched provider aggregate = replay %v, err %v; want refusal", replay, err)
	}
}

func TestProviderCausalAggregateDigestBindsExactRevision(t *testing.T) {
	authority, _ := providerAdmissionFixture(t, map[string][]ProviderCausalClassification{"lens-a": {ProviderCandidateCausal}, "lens-b": nil})
	first, err := NewLineageRevisionForState(authority)
	if err != nil {
		t.Fatal(err)
	}
	secondAuthority := authority
	secondAuthority.State = NewLineageStateApproved
	second, err := NewLineageRevisionForState(secondAuthority)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, err := authority.ProviderCausalAggregateDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := authority.ProviderCausalAggregateDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("provider aggregate digest ignored the exact authority revision")
	}
}

func providerCandidateFixture(t *testing.T) (string, CandidateIdentity) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "changed.go", "package p\n\nfunc Changed() int { return 1 }\nfunc Stable() int { return 2 }\n")
	writeSnapshotFile(t, repo, "same.go", "package p\n\nfunc Same() int { return 3 }\n")
	gitSnapshot(t, repo, "add", "changed.go", "same.go")
	gitSnapshot(t, repo, "commit", "-m", "base")
	base, err := runGit(context.Background(), repo, nil, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "changed.go", "package p\n\nfunc Changed() int { return 9 }\nfunc Stable() int { return 2 }\n")
	_, err = runGit(context.Background(), repo, nil, nil, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	// The candidate tree is the workspace tree, not HEAD.
	builder := SnapshotBuilder{Repo: repo}
	_, candidate, _, err := builder.buildCurrentChanges(context.Background(), []string{}, false, ProjectionWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	return repo, CandidateIdentity{RepositoryID: "provider-test", BaseTree: strings.TrimSpace(string(base)), CandidateTree: candidate}
}

func TestDeriveProviderCausalCarrier(t *testing.T) {
	table := []struct {
		name  string
		claim ProviderCausalEvidence
		want  ProviderCausalClassification
	}{
		{name: "changed line is candidate causal", claim: ProviderCausalEvidence{FindingID: "changed", Location: "changed.go:3", Classification: ProviderProvenNonCandidate}, want: ProviderCandidateCausal},
		{name: "unchanged line in changed file is unknown", claim: ProviderCausalEvidence{FindingID: "stable", Location: "changed.go:4", Classification: ProviderProvenNonCandidate}, want: ProviderUnknown},
		{name: "unchanged path is non candidate", claim: ProviderCausalEvidence{FindingID: "same", Location: "same.go:3", Classification: ProviderCandidateCausal}, want: ProviderProvenNonCandidate},
		{name: "behavior claim without differential proof is unknown", claim: ProviderCausalEvidence{FindingID: "behavior", Location: "changed.go:4", Classification: "behavior-activated"}, want: ProviderUnknown},
	}
	repo, candidate := providerCandidateFixture(t)
	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			carrier, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{tt.claim})
			if err != nil {
				t.Fatal(err)
			}
			if got := carrier.Findings[0].Classification; got != tt.want {
				t.Fatalf("classification = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderCausalCarrierCanonicalizesClaimsAndDigest(t *testing.T) {
	repo, candidate := providerCandidateFixture(t)
	claims := []ProviderCausalEvidence{
		{FindingID: "z", Location: "same.go:3", ProofRefs: []string{" b ", "a", "a", "c"}, Classification: ProviderCandidateCausal},
		{FindingID: "z", Location: "same.go:3", ProofRefs: []string{"c", "a", "b"}, Classification: "behavior-activated"},
		{FindingID: "a", Location: "same.go:3", ProofRefs: []string{"a"}, Classification: ProviderCandidateCausal},
	}
	first, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, claims)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{claims[1], claims[2], claims[0]})
	if err != nil {
		t.Fatal(err)
	}
	if first.AggregateDigest != second.AggregateDigest || first.Findings[0].FindingID != "a" || first.Findings[1].FindingID != "z" {
		t.Fatalf("non-deterministic canonical carrier: first=%#v second=%#v", first, second)
	}
	if len(first.Findings) != 2 || len(first.Findings[1].ProofRefs) != 3 || first.Findings[1].ProofRefs[0] != "a" || first.Findings[1].ProofRefs[2] != "c" {
		t.Fatalf("deduplicated proof refs = %v, want [a b c]", first.Findings[1].ProofRefs)
	}
}

func TestDeriveProviderCausalCarrierRejectsConflictingDuplicateIDs(t *testing.T) {
	repo, candidate := providerCandidateFixture(t)
	for _, tt := range []struct {
		name  string
		claim ProviderCausalEvidence
	}{
		{name: "location", claim: ProviderCausalEvidence{FindingID: "duplicate", Location: "changed.go:3"}},
		{name: "proof refs", claim: ProviderCausalEvidence{FindingID: "duplicate", Location: "same.go:3", ProofRefs: []string{"other-proof"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{
				{FindingID: "duplicate", Location: "same.go:3"}, tt.claim,
			})
			if err == nil || !strings.Contains(err.Error(), "conflicting duplicate finding id") {
				t.Fatalf("conflicting duplicate IDs error = %v", err)
			}
		})
	}
}

func TestProviderReviewerClassificationDoesNotChangeCarrier(t *testing.T) {
	repo, candidate := providerCandidateFixture(t)
	base := ProviderCausalEvidence{FindingID: "same", Location: "same.go:3", ProofRefs: []string{"same.go:3"}}
	first, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{{FindingID: base.FindingID, Location: base.Location, ProofRefs: base.ProofRefs, Classification: ProviderCandidateCausal}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{{FindingID: base.FindingID, Location: base.Location, ProofRefs: base.ProofRefs, Classification: "behavior-activated"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.AggregateDigest != second.AggregateDigest || first.Findings[0].Classification != ProviderProvenNonCandidate || second.Findings[0].Classification != ProviderProvenNonCandidate {
		t.Fatalf("reviewer classification changed provider result: first=%#v second=%#v", first, second)
	}
	changedProof, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{{FindingID: base.FindingID, Location: base.Location, ProofRefs: []string{"other-proof"}}})
	if err != nil {
		t.Fatal(err)
	}
	if first.AggregateDigest == changedProof.AggregateDigest {
		t.Fatal("aggregate digest did not change after a canonical proof change")
	}
	changedIdentity := candidate
	changedIdentity.PolicyHash = "changed-policy"
	changedCandidate, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), changedIdentity, []ProviderCausalEvidence{base})
	if err != nil {
		t.Fatal(err)
	}
	if first.AggregateDigest == changedCandidate.AggregateDigest {
		t.Fatal("aggregate digest did not change after a candidate identity change")
	}
}

func TestDeriveProviderCausalCarrierRejectsMalformedLocationAsUnknown(t *testing.T) {
	repo, candidate := providerCandidateFixture(t)
	carrier, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{{FindingID: "malformed", Location: "../changed.go:3", Classification: ProviderCandidateCausal}})
	if err != nil {
		t.Fatal(err)
	}
	if carrier.Findings[0].Classification != ProviderUnknown {
		t.Fatalf("classification = %q, want unknown", carrier.Findings[0].Classification)
	}
}

func TestDeriveProviderCausalCarrierUnknownFailClosed(t *testing.T) {
	repo, candidate := providerCandidateFixture(t)
	carrier, err := DeriveProviderCausalCarrier(context.Background(), repo, "sha256:"+strings.Repeat("a", 64), candidate, []ProviderCausalEvidence{{FindingID: "missing", Location: "missing.go:99", Classification: ProviderCandidateCausal}})
	if err != nil {
		t.Fatal(err)
	}
	if carrier.Findings[0].Classification != ProviderUnknown {
		t.Fatalf("classification = %q, want unknown", carrier.Findings[0].Classification)
	}
}
