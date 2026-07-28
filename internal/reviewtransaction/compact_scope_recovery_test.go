package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
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

func TestApprovedStagedScopeRecoveryRejectsInvalidRequestsWithoutMutation(t *testing.T) {
	tests := []struct {
		name          string
		prepareIndex  func(*testing.T, string)
		baseRef       func(string) string
		afterSnapshot func(*testing.T, string, CompactStore)
		mutateRequest func(*CompactRecoveryRequest)
		want          string
	}{
		{name: "no expansion", prepareIndex: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "docs/extra.md", "# Extra\n")
		}, want: "retain the predecessor projection"},
		{name: "removed genesis path", prepareIndex: func(t *testing.T, repo string) {
			writeSnapshotFile(t, repo, "docs/extra.md", "# Extra\n")
			gitSnapshot(t, repo, "add", "docs/extra.md")
			gitSnapshot(t, repo, "rm", "--cached", "docs/candidate.md")
		}, want: "retain the predecessor projection"},
		{name: "wrong base", prepareIndex: stageStagedScopeExtra, baseRef: func(string) string { return "HEAD" }, want: "retain the predecessor projection"},
		{name: "index drift", prepareIndex: stageStagedScopeExtra, afterSnapshot: func(t *testing.T, repo string, _ CompactStore) {
			writeSnapshotFile(t, repo, "docs/race.md", "# Race\n")
			gitSnapshot(t, repo, "add", "docs/race.md")
		}, want: "live target no longer matches"},
		{name: "missing receipt", prepareIndex: stageStagedScopeExtra, afterSnapshot: func(t *testing.T, _ string, store CompactStore) {
			if err := os.Remove(store.ReceiptPath()); err != nil {
				t.Fatal(err)
			}
		}, want: "canonical published predecessor receipt"},
		{name: "noncanonical receipt", prepareIndex: stageStagedScopeExtra, afterSnapshot: func(t *testing.T, _ string, store CompactStore) {
			payload, err := os.ReadFile(store.ReceiptPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(store.ReceiptPath(), append(payload, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
		}, want: "canonical published predecessor receipt"},
		{name: "incomplete authorization", prepareIndex: stageStagedScopeExtra, mutateRequest: func(request *CompactRecoveryRequest) {
			request.MaintainerAuthorization = "authorized"
		}, want: "successor-bound maintainer authorization"},
		{name: "stale predecessor revision", prepareIndex: stageStagedScopeExtra, mutateRequest: func(request *CompactRecoveryRequest) {
			request.ExpectedPredecessorRevision = hash("stale")
		}, want: "expected predecessor revision"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, base, predecessor, store, record := approvedBaseDiffScopeRecoveryFixture(t, "approved-staged-"+strings.ReplaceAll(tt.name, " ", "-"))
			tt.prepareIndex(t, repo)
			selectedBase := base
			if tt.baseRef != nil {
				selectedBase = tt.baseRef(base)
			}
			snapshot, err := (SnapshotBuilder{Repo: repo}).BuildStagedWorkspaceOverlayRecovery(context.Background(), Target{
				Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged,
				BaseRef: selectedBase, IntendedUntracked: []string{},
			})
			if err != nil {
				t.Fatal(err)
			}
			successor := newCompactStateForStagedScopeRecovery(t, repo, predecessor, snapshot, "successor-"+strings.ReplaceAll(tt.name, " ", "-"))
			request := CompactRecoveryRequest{
				PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
				Successor: successor, Disposition: RecoveryScopeChanged, Reason: "expand staged scope", Actor: "maintainer",
			}
			request.MaintainerAuthorization = compactApprovedStagedScopeRecoveryAuthorizationBinding(
				request.PredecessorLineageID, request.ExpectedPredecessorRevision, snapshot.Identity,
				successor.LineageID, request.Actor, request.Reason,
			)
			if tt.afterSnapshot != nil {
				tt.afterSnapshot(t, repo, store)
			}
			if tt.mutateRequest != nil {
				tt.mutateRequest(&request)
			}
			stateBefore, _ := os.ReadFile(store.StatePath())
			receiptBefore, receiptBeforeErr := os.ReadFile(store.ReceiptPath())
			storesBefore, _ := DiscoverCompactStores(context.Background(), repo)
			if _, err := RecoverCompactAuthority(context.Background(), repo, request); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("recovery error = %v, want %q", err, tt.want)
			}
			stateAfter, _ := os.ReadFile(store.StatePath())
			receiptAfter, receiptAfterErr := os.ReadFile(store.ReceiptPath())
			storesAfter, _ := DiscoverCompactStores(context.Background(), repo)
			if !bytes.Equal(stateBefore, stateAfter) || !bytes.Equal(receiptBefore, receiptAfter) ||
				(receiptBeforeErr == nil) != (receiptAfterErr == nil) || len(storesBefore) != len(storesAfter) {
				t.Fatal("rejected staged scope recovery mutated authority")
			}
		})
	}
}

func TestStartCompactAuthorityRejectsStagedWorkspaceOverlayRoot(t *testing.T) {
	repo, base, predecessor, _, _ := approvedBaseDiffScopeRecoveryFixture(t, "staged-start-predecessor")
	stageStagedScopeExtra(t, repo)
	snapshot, err := (SnapshotBuilder{Repo: repo}).BuildStagedWorkspaceOverlayRecovery(context.Background(), Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged, BaseRef: base, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := newCompactStateForStagedScopeRecovery(t, repo, predecessor, snapshot, "staged-start-root")
	if _, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: state}); err == nil {
		t.Fatal("direct compact START persisted a staged workspace-overlay root")
	}
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if _, err := os.Stat(store.StatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected staged root state = %v", err)
	}
}

func TestCompactTransportRejectsStagedRecoveryFork(t *testing.T) {
	repo, base, predecessor, _, predecessorRecord := approvedBaseDiffScopeRecoveryFixture(t, "staged-import-predecessor")
	stageStagedScopeExtra(t, repo)
	snapshot, _ := (SnapshotBuilder{Repo: repo}).BuildStagedWorkspaceOverlayRecovery(context.Background(), Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged, BaseRef: base, IntendedUntracked: []string{},
	})
	successor := newCompactStateForStagedScopeRecovery(t, repo, predecessor, snapshot, "staged-import-successor")
	request := CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: predecessorRecord.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "expand staged scope", Actor: "maintainer",
	}
	request.MaintainerAuthorization = compactApprovedStagedScopeRecoveryAuthorizationBinding(
		request.PredecessorLineageID, request.ExpectedPredecessorRevision, snapshot.Identity,
		successor.LineageID, request.Actor, request.Reason,
	)
	recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
	if err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "commit", "-m", "deliver staged successor")
	fork := recovered.State
	fork.LineageID = "staged-import-fork"
	provenance := *fork.Recovery
	fork.Recovery = &provenance
	fork.Recovery.MaintainerAuthorization = compactApprovedStagedScopeRecoveryAuthorizationBinding(
		predecessor.LineageID, predecessorRecord.Revision, snapshot.Identity,
		fork.LineageID, fork.Recovery.Actor, fork.Recovery.Reason,
	)
	record, _, err := makeCompactRecord(fork)
	if err != nil {
		t.Fatal(err)
	}
	transport := CompactTransport{Schema: CompactTransportSchema, Record: record}
	transport.BundleDigest = compactTransportDigest(transport)
	if _, err := ImportCompactTransport(context.Background(), repo, transport); err == nil {
		t.Fatal("transport import created a second staged recovery child")
	}
	forkStore, _ := CompactAuthoritativeStore(context.Background(), repo, fork.LineageID)
	if _, err := os.Stat(forkStore.StatePath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected staged fork state = %v", err)
	}
}

func TestBuildLiveFinalVerificationSnapshotAcceptsRecoveredStagedOverlay(t *testing.T) {
	repo, base, _, _, _ := approvedBaseDiffScopeRecoveryFixture(t, "approved-staged-final-verification")
	stageStagedScopeExtra(t, repo)
	expected, err := (SnapshotBuilder{Repo: repo}).BuildStagedWorkspaceOverlayRecovery(context.Background(), Target{
		Kind: TargetBaseWorkspaceOverlay, Projection: ProjectionStaged,
		BaseRef: base, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	live, err := buildLiveFinalVerificationSnapshot(context.Background(), repo, expected)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotsEqual(live, expected) {
		t.Fatalf("live final-verification snapshot = %#v, want %#v", live, expected)
	}
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

// TestCompactStartAndStatusAgreeOnApprovedScopeChangedCandidate pins the
// convergence issue #1826's confirmation fixture exposed: an APPROVED
// predecessor whose exact frozen delivery scope is re-staged with a changed
// candidate tree. START already refuses a fresh lineage here and answers
// recover, so STATUS must classify the same predecessor as the governing
// recovery candidate — with and without explicit lineage selection — instead
// of unrelated. Otherwise negotiated status emits a START the store refuses:
// a closed loop with no executable recovery transition.
func TestCompactStartAndStatusAgreeOnApprovedScopeChangedCandidate(t *testing.T) {
	repo, predecessor, _, _ := approvedCurrentChangesScopeRecoveryFixture(t, "approved-scope-loop-predecessor")
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate, revised\n")
	requested := newCompactTestState(t, repo, "approved-scope-loop-probe")
	if !equalStrings(requested.InitialSnapshot.Paths, predecessor.GenesisPaths) ||
		requested.InitialSnapshot.CandidateTree == predecessor.CurrentSnapshot.CandidateTree {
		t.Fatalf("fixture is not the same-scope changed-candidate shape: live=%v genesis=%v",
			requested.InitialSnapshot.Paths, predecessor.GenesisPaths)
	}

	started, startErr := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	status, statusErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
	})
	if startErr != nil || statusErr != nil {
		t.Fatalf("start error = %v, status error = %v", startErr, statusErr)
	}
	if started.Action != CompactStartRecover || started.Record.State.LineageID != predecessor.LineageID {
		t.Fatalf("start = %q against %q, want %q against the approved predecessor",
			started.Action, started.Record.State.LineageID, CompactStartRecover)
	}
	if status.Applicability != TargetApplicabilityCurrent || status.LineageID != predecessor.LineageID ||
		status.Action != TargetStatusActionRecover || status.ActionDisposition != RecoveryScopeChanged ||
		status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("status routed %q/%q disposition %q lineage %q while START answers %q: the issue #1826 closed loop",
			status.Applicability, status.Action, status.ActionDisposition, status.LineageID, started.Action)
	}
	selected, selectedErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID,
	})
	if selectedErr != nil || selected.Action != TargetStatusActionRecover ||
		selected.ActionDisposition != RecoveryScopeChanged || selected.LineageID != predecessor.LineageID {
		t.Fatalf("explicit selection erased the scope_changed relationship: action=%q disposition=%q error=%v",
			selected.Action, selected.ActionDisposition, selectedErr)
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

// TestCompactRecoveryAddsGenesisPath pins the boundary between a genuine scope
// expansion of the frozen work and an entirely unrelated candidate. Expansion
// means the live scope still overlaps genesis and reaches past it. A live scope
// disjoint from genesis is not an expansion of that lineage at all: it is
// different work that happens to share a base tree, which is the ordinary case
// for two Git worktrees of the same repository. Treating it as expansion let a
// stale correction_required lineage capture an unrelated candidate, because the
// very first live path already sits outside genesis.
func TestCompactRecoveryAddsGenesisPath(t *testing.T) {
	predecessor := CompactState{GenesisPaths: []string{"a.go", "b.go", "c.go"}}
	tests := []struct {
		name string
		live []string
		want bool
	}{
		{name: "superset", live: []string{"a.go", "b.go", "c.go", "d.go"}, want: true},
		{name: "overlap reaching outside genesis", live: []string{"a.go", "x.go"}, want: true},
		{name: "disjoint paths", live: []string{"x.go", "y.go"}, want: false},
		{name: "equal set", live: []string{"a.go", "b.go", "c.go"}, want: false},
		{name: "strict subset", live: []string{"a.go", "c.go"}, want: false},
		{name: "empty live diff", live: []string{}, want: false},
		{name: "non-canonical live paths", live: []string{"x.go", "a.go"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRecoveryAddsGenesisPath(predecessor, Snapshot{Paths: tt.live}); got != tt.want {
				t.Fatalf("compactRecoveryAddsGenesisPath(%v) = %v, want %v", tt.live, got, tt.want)
			}
		})
	}
	if compactRecoveryAddsGenesisPath(CompactState{GenesisPaths: []string{"b.go", "a.go"}}, Snapshot{Paths: []string{"a.go", "x.go"}}) {
		t.Fatal("non-canonical genesis paths must not qualify as expansion")
	}
	if compactRecoveryAddsGenesisPath(CompactState{}, Snapshot{Paths: []string{"x.go"}}) {
		t.Fatal("empty genesis scope must not be expanded by an unrelated candidate")
	}
}

// TestCorrectionRequiredLineageDoesNotCaptureDisjointCandidate reproduces the
// incident that motivated the retention rule. A stale correction_required
// lineage frozen over tracked.txt must not capture a candidate that touches an
// entirely different file, even though both share a base tree and projection.
// That sharing is unavoidable in practice: Git worktrees of one repository
// share the review store under the common dir, so a store holding several stuck
// lineages would otherwise bind whichever one enumerated first to unrelated
// work, and report that lineage's frozen scope as if it were the caller's.
func TestCorrectionRequiredLineageDoesNotCaptureDisjointCandidate(t *testing.T) {
	repo, predecessor, _, _ := correctionScopeRecoveryFixture(t, "review-correction-disjoint")

	// Retire the frozen scope from the live diff and introduce unrelated work,
	// leaving live paths disjoint from genesis rather than wider than it.
	writeSnapshotFile(t, repo, "tracked.txt", "base\n")
	writeSnapshotFile(t, repo, "unrelated.go", "package unrelated\n")
	target := Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{"unrelated.go"}}

	requested := newCompactStartStateForTarget(t, repo, "review-correction-disjoint-new", target)
	if got := requested.InitialSnapshot.Paths; len(got) != 1 || got[0] != "unrelated.go" {
		t.Fatalf("candidate paths = %v, want exactly [unrelated.go]", got)
	}
	// The shared-store gates the classifier applies before scope must still
	// pass, otherwise this would prove nothing about the retention rule.
	if requested.InitialSnapshot.BaseTree != predecessor.InitialSnapshot.BaseTree {
		t.Fatalf("fixture no longer shares a base tree with the predecessor")
	}

	started, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: requested})
	if err != nil {
		t.Fatalf("StartCompactAuthority() error = %v", err)
	}
	if started.Action != CompactStartCreated {
		t.Fatalf("disjoint candidate start action = %q, want %q", started.Action, CompactStartCreated)
	}
	if started.Record.State.LineageID != requested.LineageID {
		t.Fatalf("start bound lineage %q, want the caller's %q", started.Record.State.LineageID, requested.LineageID)
	}
	if got := started.Record.State.InitialSnapshot.Identity; got != requested.InitialSnapshot.Identity {
		t.Fatalf("start reported target identity %q, want the caller's %q", got, requested.InitialSnapshot.Identity)
	}

	// The predecessor keeps its own authority: it is neither advanced nor
	// consumed by unrelated work starting alongside it.
	predecessorStore, err := CompactAuthoritativeStore(context.Background(), repo, predecessor.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := predecessorStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if after.State.State != StateCorrectionRequired || after.State.Generation != predecessor.Generation {
		t.Fatalf("predecessor authority changed: state=%q generation=%d", after.State.State, after.State.Generation)
	}
}

// approvedCurrentChangesScopeRecoveryFixture builds the recovery shape none of
// RecoverCompactAuthority's authorization comparisons used to cover: an
// APPROVED predecessor over current changes in the workspace projection, whose
// successor keeps that same projection. It is neither the approved *staged*
// scope recovery, nor a correction-required predecessor, nor a projection
// change, so before the supplied-authorization gate existed the caller's
// --maintainer-authorization was recorded verbatim without ever being compared.
func approvedCurrentChangesScopeRecoveryFixture(t *testing.T, lineage string) (string, CompactState, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "approved candidate\n")
	state := newCompactTestState(t, repo, lineage)
	store := storeCompactStartAuthority(t, repo, state)
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	results := make([]LensResult, len(state.SelectedLenses))
	for index, lens := range state.SelectedLenses {
		results[index] = LensResult{Lens: lens, Findings: []Finding{}, Evidence: []string{"reviewed"}}
	}
	if err := state.CompleteReview(CompactReviewInput{
		LensResults: results, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{},
	}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, state)); err != nil {
		t.Fatal(err)
	}
	if state.State != StateApproved {
		t.Fatalf("fixture state = %s, want approved", state.State)
	}
	record, err = store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return repo, state, store, record
}

// approvedCurrentChangesScopeRecoveryRequest expands the approved fixture's
// scope with one new intended-untracked path and returns the matching request,
// with no maintainer authorization set.
func approvedCurrentChangesScopeRecoveryRequest(t *testing.T, repo string, predecessor CompactState, record CompactRecord, successorLineage string) CompactRecoveryRequest {
	t.Helper()
	writeSnapshotFile(t, repo, "expanded.txt", "a newly scoped path\n")
	successor := newCompactTestStateWithIntended(t, repo, successorLineage, []string{"expanded.txt"})
	successor.Generation = predecessor.Generation + 1
	return CompactRecoveryRequest{
		PredecessorLineageID: predecessor.LineageID, ExpectedPredecessorRevision: record.Revision,
		Successor: successor, Disposition: RecoveryScopeChanged, Reason: "scope expanded", Actor: "maintainer",
		RecoveredAt: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC),
	}
}

// TestApprovedScopeRecoveryRefusesSuppliedAuthorizationThatBindsNothing pins
// the audit invariant a recovery provenance record depends on: a supplied
// maintainer authorization is an attestation, so it must bind to this exact
// edge or the recovery is refused. A binding whose target_identity is all
// zeros binds nothing, yet it used to be accepted and written verbatim into
// CompactRecoveryProvenance next to the real target identity, where any later
// audit reads it as genuine. Nothing may be persisted on refusal.
func TestApprovedScopeRecoveryRefusesSuppliedAuthorizationThatBindsNothing(t *testing.T) {
	zeroIdentity := "sha256:" + strings.Repeat("0", 64)
	tests := []struct {
		name          string
		authorization func(CompactRecoveryRequest) string
	}{
		{name: "all-zeros target identity", authorization: func(request CompactRecoveryRequest) string {
			return compactRecoveryAuthorizationBinding(request.PredecessorLineageID, request.ExpectedPredecessorRevision,
				zeroIdentity, request.Actor, request.Reason)
		}},
		{name: "empty target identity", authorization: func(request CompactRecoveryRequest) string {
			return compactRecoveryAuthorizationBinding(request.PredecessorLineageID, request.ExpectedPredecessorRevision,
				"", request.Actor, request.Reason)
		}},
		{name: "another lineage", authorization: func(request CompactRecoveryRequest) string {
			return compactRecoveryAuthorizationBinding("some-other-lineage", request.ExpectedPredecessorRevision,
				request.Successor.InitialSnapshot.Identity, request.Actor, request.Reason)
		}},
		{name: "free-form prose", authorization: func(CompactRecoveryRequest) string {
			return "a maintainer said yes over chat"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lineage := "approved-forged-" + strings.ReplaceAll(tt.name, " ", "-")
			repo, predecessor, _, record := approvedCurrentChangesScopeRecoveryFixture(t, lineage)
			request := approvedCurrentChangesScopeRecoveryRequest(t, repo, predecessor, record, lineage+"-successor")
			request.MaintainerAuthorization = tt.authorization(request)

			_, err := RecoverCompactAuthority(context.Background(), repo, request)
			if err == nil || !strings.Contains(err.Error(), "authorization binding") {
				t.Fatalf("recovery error = %v, want an authorization binding refusal", err)
			}
			successorStore, storeErr := CompactAuthoritativeStore(context.Background(), repo, request.Successor.LineageID)
			if storeErr != nil {
				t.Fatal(storeErr)
			}
			if _, statErr := os.Stat(successorStore.StatePath()); !errors.Is(statErr, os.ErrNotExist) {
				persisted, _ := successorStore.Load()
				t.Fatalf("refused recovery still persisted provenance %#v (stat=%v)", persisted.State.Recovery, statErr)
			}
		})
	}
}

// TestApprovedScopeRecoveryKeepsAbsentAndExactAuthorization is the other half
// of the asymmetry: absent stays allowed, because RunReviewRecover legitimately
// self-mints actor, reason and binding for this shape, and an exact binding is
// accepted and recorded verbatim.
func TestApprovedScopeRecoveryKeepsAbsentAndExactAuthorization(t *testing.T) {
	t.Run("absent authorization is still accepted", func(t *testing.T) {
		repo, predecessor, _, record := approvedCurrentChangesScopeRecoveryFixture(t, "approved-absent")
		request := approvedCurrentChangesScopeRecoveryRequest(t, repo, predecessor, record, "approved-absent-successor")

		recovered, err := RecoverCompactAuthority(context.Background(), repo, request)
		if err != nil {
			t.Fatalf("absent authorization was refused: %v", err)
		}
		if recovered.State.Recovery == nil || recovered.State.Recovery.MaintainerAuthorization != "" {
			t.Fatalf("recovered provenance = %#v, want an empty maintainer authorization", recovered.State.Recovery)
		}
		successorStore, _ := CompactAuthoritativeStore(context.Background(), repo, request.Successor.LineageID)
		persisted, loadErr := successorStore.Load()
		if loadErr != nil || persisted.State.Recovery == nil || persisted.State.Recovery.MaintainerAuthorization != "" {
			t.Fatalf("persisted provenance = %#v, error %v", persisted.State.Recovery, loadErr)
		}
	})

	t.Run("exact authorization is accepted and recorded", func(t *testing.T) {
		repo, predecessor, _, record := approvedCurrentChangesScopeRecoveryFixture(t, "approved-exact")
		request := approvedCurrentChangesScopeRecoveryRequest(t, repo, predecessor, record, "approved-exact-successor")
		request.MaintainerAuthorization = recoveryAuthorizationFixture(request)

		if _, err := RecoverCompactAuthority(context.Background(), repo, request); err != nil {
			t.Fatalf("exact authorization was refused: %v", err)
		}
		successorStore, _ := CompactAuthoritativeStore(context.Background(), repo, request.Successor.LineageID)
		persisted, loadErr := successorStore.Load()
		if loadErr != nil || persisted.State.Recovery == nil ||
			persisted.State.Recovery.MaintainerAuthorization != request.MaintainerAuthorization {
			t.Fatalf("persisted provenance = %#v, error %v", persisted.State.Recovery, loadErr)
		}
	})
}

// TestCompactRecoverySuppliedAuthorizationBinds pins the predicate itself,
// including the one value that is a recognized sentinel rather than a binding:
// ReleaseScopeRecoveryAuthorization must never be refused by this gate, since
// the release-scope branches prove it their own way.
func TestCompactRecoverySuppliedAuthorizationBinds(t *testing.T) {
	const lineage, revision, identity, successor = "pred", "sha256:rev", "sha256:target", "succ"
	const actor, reason = "maintainer", "scope expanded"
	exact := compactRecoveryAuthorizationBinding(lineage, revision, identity, actor, reason)
	successorBound := compactApprovedStagedScopeRecoveryAuthorizationBinding(lineage, revision, identity, successor, actor, reason)
	tests := []struct {
		name          string
		authorization string
		want          bool
	}{
		{"exact plain binding", exact, true},
		{"exact successor-bound binding", successorBound, true},
		{"release scope sentinel", ReleaseScopeRecoveryAuthorization, true},
		{"wrong target identity", compactRecoveryAuthorizationBinding(lineage, revision, "sha256:other", actor, reason), false},
		{"wrong actor", compactRecoveryAuthorizationBinding(lineage, revision, identity, "someone else", reason), false},
		{"free-form prose", "approved out of band", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compactRecoverySuppliedAuthorizationBinds(tt.authorization, lineage, revision, identity, successor, actor, reason); got != tt.want {
				t.Fatalf("compactRecoverySuppliedAuthorizationBinds(%q) = %t, want %t", tt.authorization, got, tt.want)
			}
		})
	}
}

// TestValidateCompactRecoveryEdgeRefusesForgedSchemaAuthorization proves the
// replay side agrees with the write side on the one claim that can lie: a
// recorded authorization that names the recovery-authorization schema asserts
// a maintainer bound this exact edge, so replay refuses one that binds nothing.
// Pre-contract free-form text makes no such claim and keeps the tolerance
// TestCompactAuthorityGraphLoadsHistoricalFreeFormAuthorizationWithoutRewrite
// depends on.
func TestValidateCompactRecoveryEdgeRefusesForgedSchemaAuthorization(t *testing.T) {
	repo, predecessor, _, record := approvedCurrentChangesScopeRecoveryFixture(t, "approved-replay")
	request := approvedCurrentChangesScopeRecoveryRequest(t, repo, predecessor, record, "approved-replay-successor")
	successor := request.Successor
	successor.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: record.Revision,
		Disposition: RecoveryScopeChanged, Reason: request.Reason, Actor: request.Actor,
		RecoveredAt: request.RecoveredAt,
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision,
			"sha256:"+strings.Repeat("0", 64), request.Actor, request.Reason),
	}
	if err := validateCompactRecoveryEdge(record, successor); err == nil ||
		!strings.Contains(err.Error(), "authorization binding") {
		t.Fatalf("replay of a forged schema authorization = %v, want an authorization binding refusal", err)
	}

	successor.Recovery.MaintainerAuthorization = "approved issue #1257"
	if err := validateCompactRecoveryEdge(record, successor); err != nil {
		t.Fatalf("replay of a pre-contract free-form authorization = %v, want it tolerated", err)
	}
}

func recoveryAuthorizationFixture(request CompactRecoveryRequest) string {
	return "gentle-ai.review-recovery-authorization/v1\npredecessor_lineage=" + request.PredecessorLineageID +
		"\npredecessor_revision=" + request.ExpectedPredecessorRevision + "\ntarget_identity=" + request.Successor.InitialSnapshot.Identity +
		"\nactor=" + strings.TrimSpace(request.Actor) + "\nreason=" + strings.TrimSpace(request.Reason)
}

func approvedBaseDiffScopeRecoveryFixture(t *testing.T, lineage string) (string, string, CompactState, CompactStore, CompactRecord) {
	t.Helper()
	repo := initSnapshotRepo(t)
	base := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(t, repo, "docs/candidate.md", "# Candidate\n")
	gitSnapshot(t, repo, "add", "docs/candidate.md")
	gitSnapshot(t, repo, "commit", "-m", "add candidate")
	state := newCompactStartStateForTarget(t, repo, lineage, Target{Kind: TargetBaseDiff, BaseRef: base, IntendedUntracked: []string{}})
	store := storeCompactStartAuthority(t, repo, state)
	record, _ := store.Load()
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{}, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(record.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteVerification([]byte("verified\n"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(revision, "review/complete-verification", state); err != nil {
		t.Fatal(err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, state)); err != nil {
		t.Fatal(err)
	}
	record, _ = store.Load()
	return repo, base, state, store, record
}

func stageStagedScopeExtra(t *testing.T, repo string) {
	t.Helper()
	writeSnapshotFile(t, repo, "docs/extra.md", "# Extra\n")
	gitSnapshot(t, repo, "add", "docs/extra.md")
}

func newCompactStateForStagedScopeRecovery(t *testing.T, repo string, predecessor CompactState, snapshot Snapshot, lineage string) CompactState {
	t.Helper()
	assessment, err := (SnapshotBuilder{Repo: repo}).AssessSnapshotRisk(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewCompactState(Start{
		LineageID: lineage, Mode: ModeOrdinaryBounded, Generation: predecessor.Generation + 1,
		Snapshot: snapshot, PolicyHash: predecessor.PolicyHash, RiskLevel: assessment.Level,
		SelectedLenses: []string{}, OriginalChangedLines: &assessment.ChangedLines,
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
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
