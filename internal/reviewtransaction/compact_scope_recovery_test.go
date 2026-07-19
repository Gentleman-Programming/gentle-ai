package reviewtransaction

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCorrectionRequiredScopeRecoveryCreatesFreshAuditableSuccessor(t *testing.T) {
	repo, predecessor, store, predecessorRecord := correctionScopeRecoveryFixture(t, "correction-scope-predecessor")
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	receiptBefore := []byte("preserve existing receipt bytes\n")
	if err := os.WriteFile(store.ReceiptPath(), receiptBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "process_helper.go", "package processhelper\n")
	successor := newCompactTestStateWithIntended(t, repo, "correction-scope-successor", []string{"process_helper.go"})
	successor.Generation = predecessor.Generation + 1
	recoveredAt := time.Date(2026, 7, 16, 15, 0, 0, 0, time.UTC)
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "correction requires a process helper",
		Actor: "maintainer", RecoveredAt: recoveredAt,
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State.Recovery == nil || recovered.State.Recovery.MaintainerAuthorization != request.MaintainerAuthorization ||
		recovered.State.Generation != predecessor.Generation+1 || !compactPristineReviewing(recovered.State) {
		t.Fatalf("recovered successor is not fresh: %#v", recovered.State)
	}
	if recovered.State.RiskLevel != successor.RiskLevel || recovered.State.OriginalChangedLines != successor.OriginalChangedLines ||
		recovered.State.CorrectionBudget != successor.CorrectionBudget || !equalStrings(recovered.State.GenesisPaths, successor.GenesisPaths) ||
		len(recovered.State.CorrectionAttempts) != 0 || recovered.State.CumulativeCorrectionLines != 0 {
		t.Fatalf("successor did not retain freshly derived inputs: %#v", recovered.State)
	}
	replayed, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil || replayed.Revision != recovered.Revision || !compactStateEqual(replayed.State, recovered.State) {
		t.Fatalf("exact replay = %#v, %v", replayed, err)
	}
	fork := newCompactTestStateWithIntended(t, repo, "correction-scope-fork", []string{"process_helper.go"})
	fork.Generation = predecessor.Generation + 1
	request.Successor = fork
	if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "already has successor") {
		t.Fatalf("conflicting successor error = %v", err)
	}
	stateAfter, _ := os.ReadFile(store.StatePath())
	receiptAfter, _ := os.ReadFile(store.ReceiptPath())
	if !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("recovery changed predecessor state or receipt bytes")
	}
}

func TestCorrectionRequiredScopeRecoveryRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CompactRecoveryRequest, CompactState)
		want   string
	}{
		{name: "missing authorization", mutate: func(request *CompactRecoveryRequest, _ CompactState) { request.MaintainerAuthorization = "" }, want: "maintainer authorization"},
		{name: "free-form authorization", mutate: func(request *CompactRecoveryRequest, _ CompactState) { request.MaintainerAuthorization = "authorized" }, want: "authorization binding"},
		{name: "wrong target authorization", mutate: func(request *CompactRecoveryRequest, _ CompactState) {
			request.MaintainerAuthorization = strings.Replace(request.MaintainerAuthorization, request.Successor.InitialSnapshot.Identity, hash("wrong"), 1)
		}, want: "authorization binding"},
		{name: "wrong revision", mutate: func(request *CompactRecoveryRequest, _ CompactState) {
			request.ExpectedPredecessorRevision = hash("wrong")
		}, want: "expected predecessor revision"},
		{name: "same lineage", mutate: func(request *CompactRecoveryRequest, predecessor CompactState) {
			request.Successor.LineageID = predecessor.LineageID
		}, want: "distinct successor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, predecessor, _, record := correctionScopeRecoveryFixture(t, "correction-invalid-predecessor")
			writeSnapshotFile(t, repo, "new_helper.go", "package newhelper\n")
			successor := newCompactTestStateWithIntended(t, repo, "correction-invalid-successor", []string{"new_helper.go"})
			successor.Generation = predecessor.Generation + 1
			request := CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
				Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope expanded", Actor: "maintainer"}
			request.MaintainerAuthorization = recoveryAuthorizationFixture(request)
			tt.mutate(&request, predecessor)
			if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("recovery error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("no outside-genesis path", func(t *testing.T) {
		repo, predecessor, _, record := correctionScopeRecoveryFixture(t, "correction-byte-predecessor")
		writeSnapshotFile(t, repo, "tracked.txt", "byte-only correction\n")
		successor := newCompactTestState(t, repo, "correction-byte-successor")
		successor.Generation = predecessor.Generation + 1
		request := CompactRecoveryRequest{
			PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
			Successor: successor, Disposition: RecoveryScopeChanged, Reason: "only bytes changed", Actor: "maintainer",
		}
		request.MaintainerAuthorization = recoveryAuthorizationFixture(request)
		_, err := RecoverCompactAuthority(context.Background(), repo, request)
		if err == nil || !strings.Contains(err.Error(), "path expansion") {
			t.Fatalf("byte-only recovery error = %v", err)
		}
	})
}

func TestCompactAuthorityGraphLoadsHistoricalFreeFormAuthorizationWithoutRewrite(t *testing.T) {
	repo, predecessor, _, record := correctionScopeRecoveryFixture(t, "correction-graph-predecessor")
	writeSnapshotFile(t, repo, "historical-helper.go", "package helper\n")
	successor := newCompactTestStateWithIntended(t, repo, "correction-graph-successor", []string{"historical-helper.go"})
	successor.Generation = predecessor.Generation + 1
	successor.Recovery = &CompactRecoveryProvenance{PredecessorLineageID: predecessor.LineageID, PredecessorRevision: record.Revision,
		Disposition: RecoveryScopeChanged, Reason: "historical reset", Actor: "maintainer", MaintainerAuthorization: "approved issue #1257", RecoveredAt: time.Now().UTC()}
	successorStore, _ := CompactAuthoritativeStore(context.Background(), repo, successor.LineageID)
	_, payload, err := makeCompactRecord(successor)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(successorStore.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(successorStore.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	after, _ := os.ReadFile(successorStore.StatePath())
	if err != nil || len(leaves) != 1 || !bytes.Equal(payload, after) {
		t.Fatalf("historical recovery changed: leaves=%d error=%v", len(leaves), err)
	}
}

func TestCorrectionRequiredScopeRecoveryAcceptsPureGenesisContraction(t *testing.T) {
	repo, predecessor, store, predecessorRecord := correctionContractionRecoveryFixture(t, "contraction-predecessor")
	stateBefore, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
	successor := newCompactTestState(t, repo, "contraction-successor")
	if !equalStrings(successor.InitialSnapshot.Paths, []string{"tracked.txt"}) || len(predecessor.GenesisPaths) != 2 {
		t.Fatalf("fixture is not a strict contraction: live=%v genesis=%v", successor.InitialSnapshot.Paths, predecessor.GenesisPaths)
	}
	successor.Generation = predecessor.Generation + 1
	recoveredAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "remove accidentally frozen generated paths",
		Actor: "maintainer", RecoveredAt: recoveredAt,
	}
	request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("pure contraction recovery = %v", err)
	}
	if recovered.State.Recovery == nil || recovered.State.Recovery.MaintainerAuthorization != request.MaintainerAuthorization ||
		recovered.State.Generation != predecessor.Generation+1 || !compactPristineReviewing(recovered.State) {
		t.Fatalf("recovered successor is not fresh: %#v", recovered.State)
	}
	if !equalStrings(recovered.State.GenesisPaths, []string{"tracked.txt"}) {
		t.Fatalf("successor genesis paths = %v", recovered.State.GenesisPaths)
	}
	replayed, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil || replayed.Revision != recovered.Revision || !compactStateEqual(replayed.State, recovered.State) {
		t.Fatalf("exact replay = %#v, %v", replayed, err)
	}
	stateAfter, _ := os.ReadFile(store.StatePath())
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("contraction recovery changed predecessor state bytes")
	}
}

func TestCompactStartAndStatusAdvertiseRecoverForPureGenesisContraction(t *testing.T) {
	repo, predecessor, _, _ := correctionContractionRecoveryFixture(t, "contraction-start-predecessor")
	writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
	requested := newCompactTestState(t, repo, "contraction-start-probe")
	started, startErr := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	status, statusErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID,
	})
	if startErr != nil || statusErr != nil || started.Action != CompactStartRecover ||
		status.Action != TargetStatusActionRecover || status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("contraction START=%#v status=%#v errors=%v/%v", started, status, startErr, statusErr)
	}
}

func TestCorrectionRequiredScopeRecoveryContractionGuards(t *testing.T) {
	t.Run("missing authorization", func(t *testing.T) {
		repo, predecessor, _, record := correctionContractionRecoveryFixture(t, "contraction-noauth-predecessor")
		writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
		successor := newCompactTestState(t, repo, "contraction-noauth-successor")
		successor.Generation = predecessor.Generation + 1
		request := CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
			Successor: successor, Disposition: RecoveryScopeChanged, Reason: "contract scope", Actor: "maintainer"}
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "maintainer authorization") {
			t.Fatalf("missing authorization error = %v", err)
		}
	})
	t.Run("mismatched authorization", func(t *testing.T) {
		repo, predecessor, _, record := correctionContractionRecoveryFixture(t, "contraction-badauth-predecessor")
		writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
		successor := newCompactTestState(t, repo, "contraction-badauth-successor")
		successor.Generation = predecessor.Generation + 1
		request := CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
			Successor: successor, Disposition: RecoveryScopeChanged, Reason: "contract scope", Actor: "maintainer"}
		request.MaintainerAuthorization = strings.Replace(recoveryAuthorizationFixture(request), successor.InitialSnapshot.Identity, hash("wrong"), 1)
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "authorization binding") {
			t.Fatalf("mismatched authorization error = %v", err)
		}
	})
	t.Run("empty live diff", func(t *testing.T) {
		repo, predecessor, _, record := correctionContractionRecoveryFixture(t, "contraction-empty-predecessor")
		writeSnapshotFile(t, repo, "tracked.txt", "base\n")
		writeSnapshotFile(t, repo, "deleted.txt", "delete me\n")
		snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Paths) != 0 {
			t.Fatalf("fixture live diff is not empty: %v", snapshot.Paths)
		}
		lines := 0
		successor, err := NewCompactState(Start{LineageID: "contraction-empty-successor", Mode: ModeOrdinaryBounded,
			Generation: predecessor.Generation + 1, Snapshot: snapshot, PolicyHash: hash("1"), RiskLevel: RiskLow,
			SelectedLenses: []string{}, OriginalChangedLines: &lines})
		if err != nil {
			return // an empty live diff cannot even form a recovery successor: fail closed
		}
		request := CompactRecoveryRequest{PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
			Successor: successor, Disposition: RecoveryScopeChanged, Reason: "empty diff", Actor: "maintainer"}
		request.MaintainerAuthorization = recoveryAuthorizationFixture(request)
		if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), "path expansion") {
			t.Fatalf("empty live diff recovery error = %v", err)
		}
	})
}

func TestCompactRecoveryContractsGenesisPaths(t *testing.T) {
	predecessor := CompactState{GenesisPaths: []string{"a.go", "b.go", "c.go"}}
	tests := []struct {
		name string
		live []string
		want bool
	}{
		{name: "strict subset", live: []string{"a.go", "c.go"}, want: true},
		{name: "single retained path", live: []string{"b.go"}, want: true},
		{name: "equal set", live: []string{"a.go", "b.go", "c.go"}, want: false},
		{name: "empty live diff", live: []string{}, want: false},
		{name: "disjoint paths", live: []string{"x.go", "y.go"}, want: false},
		{name: "superset", live: []string{"a.go", "b.go", "c.go", "d.go"}, want: false},
		{name: "overlap with outside path", live: []string{"a.go", "x.go"}, want: false},
		{name: "non-canonical live paths", live: []string{"c.go", "a.go"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRecoveryContractsGenesisPaths(predecessor, Snapshot{Paths: tt.live}); got != tt.want {
				t.Fatalf("compactRecoveryContractsGenesisPaths(%v) = %v, want %v", tt.live, got, tt.want)
			}
		})
	}
	if compactRecoveryContractsGenesisPaths(CompactState{GenesisPaths: []string{"b.go", "a.go"}}, Snapshot{Paths: []string{"a.go"}}) {
		t.Fatal("non-canonical genesis paths must not qualify as contraction")
	}
}

func recoveryAuthorizationFixture(request CompactRecoveryRequest) string {
	return "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + request.PredecessorLineageID +
		"\npredecessor_revision=" + request.ExpectedPredecessorRevision + "\ntarget_identity=" + request.Successor.InitialSnapshot.Identity +
		"\nactor=" + strings.TrimSpace(request.Actor) + "\nreason=" + strings.TrimSpace(request.Reason)
}

func correctionScopeRecoveryFixture(t *testing.T, lineage string) (string, CompactState, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	state, store, record := correctionRequiredAuthorityFixture(t, repo, lineage)
	return repo, state, store, record
}

func correctionContractionRecoveryFixture(t *testing.T, lineage string) (string, CompactState, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nwrong\n")
	writeSnapshotFile(t, repo, "deleted.txt", "accidentally frozen generated noise\n")
	state, store, record := correctionRequiredAuthorityFixture(t, repo, lineage)
	return repo, state, store, record
}

func correctionRequiredAuthorityFixture(t *testing.T, repo, lineage string) (CompactState, CompactStore, CompactRecord) {
	t.Helper()
	state := newCompactTestState(t, repo, lineage)
	store := storeCompactStartAuthority(t, repo, state)
	started, _ := store.Load()
	finding := Finding{ID: "R3-001", Lens: "reliability", Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if len(results) == 0 {
		t.Fatal("correction fixture unexpectedly selected no lenses")
	}
	results[0].Findings = []Finding{finding}
	if err := state.CompleteReview(CompactReviewInput{LensResults: results,
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	if state.State != StateCorrectionRequired {
		t.Fatalf("fixture state = %s", state.State)
	}
	if _, err := store.Replace(started.Revision, "review/complete-review", state); err != nil {
		t.Fatal(err)
	}
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return state, store, record
}
