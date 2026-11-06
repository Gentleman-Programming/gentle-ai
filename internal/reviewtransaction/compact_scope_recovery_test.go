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
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("recovery changed predecessor state bytes")
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
	status, statusErr := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
		Target: Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}}, LineageID: predecessor.LineageID,
	})
	if statusErr != nil || status.Action != TargetStatusActionRecover || status.Replayability != ReplayabilityManualActionRequired {
		t.Fatalf("contraction status=%#v error=%v", status, statusErr)
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

	requested := newCompactFixtureStateForTarget(t, repo, "review-correction-disjoint-new", target)
	if got := requested.InitialSnapshot.Paths; len(got) != 1 || got[0] != "unrelated.go" {
		t.Fatalf("candidate paths = %v, want exactly [unrelated.go]", got)
	}
	// The shared-store gates the classifier applies before scope must still
	// pass, otherwise this would prove nothing about the retention rule.
	if requested.InitialSnapshot.BaseTree != predecessor.InitialSnapshot.BaseTree {
		t.Fatalf("fixture no longer shares a base tree with the predecessor")
	}

	started, err := createAtomicCompactAuthority(t, context.Background(), repo, requested)
	if err != nil {
		t.Fatalf("atomic START error = %v", err)
	}
	if started.Replayed {
		t.Fatal("disjoint candidate atomic START replayed an absent lineage")
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
	repo, predecessor, _, record := correctionScopeRecoveryFixture(t, "correction-replay")
	writeSnapshotFile(t, repo, "expanded.txt", "a newly scoped path\n")
	successor := newCompactTestStateWithIntended(t, repo, "correction-replay-successor", []string{"expanded.txt"})
	successor.Generation = predecessor.Generation + 1
	const actor, reason = "maintainer", "scope expanded"
	successor.Recovery = &CompactRecoveryProvenance{
		PredecessorLineageID: predecessor.LineageID, PredecessorRevision: record.Revision,
		Disposition: RecoveryScopeChanged, Reason: reason, Actor: actor,
		RecoveredAt: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC),
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(predecessor.LineageID, record.Revision,
			"sha256:"+strings.Repeat("0", 64), actor, reason),
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
	return correctionRequiredCompactAuthority(t, repo, lineage)
}
