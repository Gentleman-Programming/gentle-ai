package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCompactEffectMarkerTransitionsAreMonotonic(t *testing.T) {
	states := []compactEffectMarkerState{compactEffectPending, compactEffectBlocked, compactEffectApplied}
	allowed := map[[2]compactEffectMarkerState]bool{
		{compactEffectPending, compactEffectPending}: true, {compactEffectPending, compactEffectBlocked}: true, {compactEffectPending, compactEffectApplied}: true,
		{compactEffectBlocked, compactEffectBlocked}: true, {compactEffectBlocked, compactEffectApplied}: true,
		{compactEffectApplied, compactEffectApplied}: true,
	}
	for _, from := range append([]compactEffectMarkerState{""}, states...) {
		for _, to := range states {
			name := string(from) + "_to_" + string(to)
			t.Run(name, func(t *testing.T) {
				repository, marker := newCompactEffectMarkerFixture(t, "case"+strings.ReplaceAll(name, "_", "-"))
				if from != "" {
					marker.State, marker.Observation = from, observationFor(from)
					if _, err := repository.write(marker); err != nil {
						t.Fatal(err)
					}
				}
				path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
				before, _ := os.ReadFile(path)
				marker.State, marker.Observation = to, observationFor(to)
				_, err := repository.write(marker)
				wantAllowed := from == "" || allowed[[2]compactEffectMarkerState{from, to}]
				if (err == nil) != wantAllowed {
					t.Fatalf("write error = %v; allowed = %v", err, wantAllowed)
				}
				if !wantAllowed {
					after, _ := os.ReadFile(path)
					if !bytes.Equal(before, after) {
						t.Fatal("rejected regression rewrote marker")
					}
				}
			})
		}
	}
}

func TestCompactEffectMarkerStrictValidation(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "strict-validation")
	if _, err := repository.write(marker); err != nil {
		t.Fatal(err)
	}
	path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
	valid, _ := os.ReadFile(path)
	cases := []struct {
		name    string
		payload []byte
	}{
		{"malformed", []byte("{")},
		{"trailing JSON", append(append([]byte(nil), valid...), []byte("{}")...)},
		{"unknown field", []byte(`{"schema":"gentle-ai.review-effect-marker/v1","lineage_id":"strict-validation","authority_revision":"` + hash("a") + `","event_id":"` + hash("b") + `","state":"pending","observation":"pending_transient","extra":true}`)},
		{"wrong schema", markerPayload(marker, func(value *compactEffectMarker) { value.Schema = "wrong" })},
		{"wrong lineage", markerPayload(marker, func(value *compactEffectMarker) { value.LineageID = "wrong" })},
		{"wrong revision", markerPayload(marker, func(value *compactEffectMarker) { value.AuthorityRevision = hash("c") })},
		{"wrong event", markerPayload(marker, func(value *compactEffectMarker) { value.EventID = hash("d") })},
		{"invalid state", markerPayload(marker, func(value *compactEffectMarker) { value.State = "unknown" })},
		{"invalid observation", markerPayload(marker, func(value *compactEffectMarker) { value.Observation = "unknown" })},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, tt.payload, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); err == nil {
				t.Fatal("accepted invalid marker")
			}
		})
	}
}

func TestCompactEffectMarkerRejectsUnsafeStorageAndIdentity(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "unsafe-storage")
	for _, identity := range []struct{ lineage, revision, event string }{{"../escape", marker.AuthorityRevision, marker.EventID}, {marker.LineageID, "bad", marker.EventID}, {marker.LineageID, marker.AuthorityRevision, "bad"}} {
		if _, err := repository.path(identity.lineage, identity.revision, identity.event, true); err == nil {
			t.Fatal("accepted invalid path component")
		}
	}
	if err := os.MkdirAll(filepath.Dir(repository.root), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), repository.root); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.write(marker); err == nil {
		t.Fatal("accepted symlink root")
	}

	repository, marker = newCompactEffectMarkerFixture(t, "non-regular")
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); err == nil {
		t.Fatal("accepted non-regular marker")
	}
}

func TestCompactEffectMarkerIsPrivateSeparateAndStable(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "private-stable")
	authority := filepath.Join(filepath.Dir(filepath.Dir(repository.root)), "v2", "authority.json")
	if err := os.MkdirAll(filepath.Dir(authority), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authority, []byte("authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeAuthority, _ := os.ReadFile(authority)
	marker.State, marker.Observation = compactEffectApplied, compactEffectAppliedDurable
	if _, err := repository.write(marker); err != nil {
		t.Fatal(err)
	}
	path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
	before, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if _, err := repository.write(marker); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	afterInfo, _ := os.Stat(path)
	afterAuthority, _ := os.ReadFile(authority)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(afterInfo.ModTime()) || !bytes.Equal(beforeAuthority, afterAuthority) {
		t.Fatal("exact replay or separate authority bytes changed")
	}
	for dir := filepath.Dir(path); dir != filepath.Dir(filepath.Dir(repository.root)); dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("directory %q is not private", dir)
		}
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode = %o", info.Mode().Perm())
	}
}

func TestCompactEffectMarkerConcurrentWritersConverge(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "convergence")
	var wait sync.WaitGroup
	for i := range 30 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			candidate := marker
			candidate.State = []compactEffectMarkerState{compactEffectPending, compactEffectBlocked, compactEffectApplied}[i%3]
			candidate.Observation = observationFor(candidate.State)
			_, _ = repository.write(candidate)
		}()
	}
	wait.Wait()
	marker.State, marker.Observation = compactEffectApplied, compactEffectAppliedDurable
	if _, err := repository.write(marker); err != nil {
		t.Fatal(err)
	}
	if got, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); err != nil || got != marker {
		t.Fatalf("marker = %#v, %v", got, err)
	}
}

func TestCompactEffectMarkerRejectsCallerBeforeMutationAndReportsLimitedDurability(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "publication")
	marker.Observation = "unknown"
	if _, err := repository.write(marker); err == nil {
		t.Fatal("accepted invalid caller marker")
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(repository.root))); !os.IsNotExist(err) {
		t.Fatalf("storage mutated: %v", err)
	}
	marker.Observation = compactEffectPendingTransient
	if _, err := repository.write(marker); err != nil {
		t.Fatal(err)
	}
	marker.State, marker.Observation = compactEffectBlocked, compactEffectBlockedConflict
	path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
	original := syncReviewDirectory
	syncReviewDirectory = func(dir string) error {
		if dir == filepath.Dir(path) {
			return errors.New("injected")
		}
		return original(dir)
	}
	t.Cleanup(func() { syncReviewDirectory = original })
	result, err := repository.write(marker)
	if err != nil || !result.DurabilityLimited {
		t.Fatalf("publication = %#v, %v", result, err)
	}
	if got, readErr := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); readErr != nil || got != marker {
		t.Fatalf("published marker = %#v, %v", got, readErr)
	}
}

func TestCompactRepositoryContextReconciliationPublishesExactIntentOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-reconcile")
	record, payload := compactRepositoryContextIntentFixture(t, repo, state)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reconcileCompactRepositoryContext(context.Background(), store, record); err != nil {
		t.Fatal(err)
	}
	intent := record.EffectIntents[0]
	path, _ := reviewRepositoryContextPath(intent.Destination)
	before, err := os.ReadFile(path)
	if err != nil || hashPayloadBytes(bytes.TrimSuffix(before, []byte{'\n'})) != intent.PayloadHash {
		t.Fatalf("published payload mismatch: %v", err)
	}
	info, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if err := reconcileCompactRepositoryContext(context.Background(), store, record); err != nil {
		t.Fatal(err)
	}
	afterInfo, _ := os.Stat(path)
	if !info.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("applied retry rewrote repository context")
	}
	markers, _ := openCompactEffectMarkerRepository(context.Background(), repo)
	marker, err := markers.read(state.LineageID, record.Revision, intent.EventID)
	if err != nil || marker.State != compactEffectApplied {
		t.Fatalf("marker = %#v, %v", marker, err)
	}
}

func TestCompactRepositoryContextLimitedDurabilityPromotesOnRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-limited")
	record, _ := compactRepositoryContextIntentFixture(t, repo, state)
	markers, _ := openCompactEffectMarkerRepository(context.Background(), repo)
	path, _ := markers.path(state.LineageID, record.Revision, record.EffectIntents[0].EventID, true)
	original := syncReviewDirectory
	syncReviewDirectory = func(dir string) error {
		if dir == filepath.Dir(path) {
			return errors.New("injected")
		}
		return original(dir)
	}
	if err := reconcileCompactRepositoryContext(context.Background(), CompactStore{repo: repo}, record); err == nil {
		t.Fatal("limited durability reconciled")
	}
	syncReviewDirectory = original
	t.Cleanup(func() { syncReviewDirectory = original })
	marker, err := markers.read(state.LineageID, record.Revision, record.EffectIntents[0].EventID)
	if err != nil || marker.State != compactEffectDurabilityLimited || marker.Observation != compactEffectPlatformLimited {
		t.Fatalf("marker = %#v, %v", marker, err)
	}
	if err := reconcileCompactRepositoryContext(context.Background(), CompactStore{repo: repo}, record); err != nil {
		t.Fatalf("retry did not promote limited marker: %v", err)
	}
	marker, err = markers.read(state.LineageID, record.Revision, record.EffectIntents[0].EventID)
	if err != nil || marker.State != compactEffectApplied || marker.Observation != compactEffectAppliedDurable {
		t.Fatalf("promoted marker = %#v, %v", marker, err)
	}
}

func TestCompactRepositoryContextReconciliationFailsClosedOnIntentMismatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-conflict")
	record, _ := compactRepositoryContextIntentFixture(t, repo, state)
	record.EffectIntents[0].PayloadHash = hash("different")
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err := reconcileCompactRepositoryContext(context.Background(), store, record); err == nil {
		t.Fatal("mismatched intent reconciled")
	}
	intent := record.EffectIntents[0]
	if path, _ := reviewRepositoryContextPath(intent.Destination); func() bool { _, err := os.Stat(path); return !os.IsNotExist(err) }() {
		t.Fatal("mismatched intent published repository context")
	}
	markers, _ := openCompactEffectMarkerRepository(context.Background(), repo)
	marker, err := markers.read(state.LineageID, record.Revision, intent.EventID)
	if err != nil || marker.State != compactEffectBlocked {
		t.Fatalf("marker = %#v, %v", marker, err)
	}
}

func TestCompactRepositoryContextMustReconcileBeforeSuccessor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-successor")
	record, payload := compactRepositoryContextIntentFixture(t, repo, state)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	intent := record.EffectIntents[0]
	markers, _ := openCompactEffectMarkerRepository(context.Background(), repo)
	if _, err := markers.write(compactEffectMarker{Schema: compactEffectMarkerSchema, LineageID: state.LineageID,
		AuthorityRevision: record.Revision, EventID: intent.EventID, State: compactEffectBlocked, Observation: compactEffectBlockedConflict}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Replace(record.Revision, "review/test", state); err == nil || !strings.Contains(err.Error(), "reconcile compact predecessor effects") {
		t.Fatalf("successor error = %v", err)
	}
}

func TestCompactRepositoryContextMustReconcileBeforeRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-recovery")
	record, payload := compactRepositoryContextIntentFixture(t, repo, state)
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	intent := record.EffectIntents[0]
	markers, _ := openCompactEffectMarkerRepository(context.Background(), repo)
	if _, err := markers.write(compactEffectMarker{Schema: compactEffectMarkerSchema, LineageID: state.LineageID,
		AuthorityRevision: record.Revision, EventID: intent.EventID, State: compactEffectBlocked, Observation: compactEffectBlockedConflict}); err != nil {
		t.Fatal(err)
	}
	successor := state
	successor.LineageID = "repository-context-recovery-successor"
	_, err := RecoverCompactAuthority(context.Background(), repo, CompactRecoveryRequest{
		PredecessorLineageID: state.LineageID, ExpectedPredecessorRevision: record.Revision, Successor: successor,
	})
	if err == nil || !strings.Contains(err.Error(), "reconcile recovery predecessor effects") {
		t.Fatalf("recovery error = %v", err)
	}
}

func compactRepositoryContextIntentFixture(t *testing.T, repo string, state CompactState) (CompactRecord, []byte) {
	t.Helper()
	statePayload, _ := json.Marshal(state)
	binding := ReviewRepositoryContextBinding{LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: compactStateRevision(statePayload)}
	identity, err := reviewRepositoryIdentity(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	handle := reviewRepositoryContextHandle(binding, identity)
	contextPayload, _ := json.Marshal(reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: handle, LineageID: binding.LineageID,
		TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
		RepositoryIdentity: identity.RepositoryIdentity, RepositoryRoot: identity.RepositoryRoot,
		GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	})
	record, payload, err := makeCompactRecordWithIntents(state, []CompactEffectIntent{{Class: "repository_context", Destination: handle, PayloadHash: hashPayloadBytes(contextPayload)}})
	if err != nil {
		t.Fatal(err)
	}
	return record, payload
}

func newCompactEffectMarkerFixture(t *testing.T, lineage string) (compactEffectMarkerRepository, compactEffectMarker) {
	t.Helper()
	repo := initSnapshotRepo(t)
	repository, err := openCompactEffectMarkerRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return repository, compactEffectMarker{Schema: compactEffectMarkerSchema, LineageID: lineage, AuthorityRevision: hash("a"), EventID: hash("b"), State: compactEffectPending, Observation: compactEffectPendingTransient}
}

func observationFor(state compactEffectMarkerState) compactEffectObservation {
	if state == compactEffectBlocked {
		return compactEffectBlockedConflict
	}
	if state == compactEffectApplied {
		return compactEffectAppliedDurable
	}
	return compactEffectPendingTransient
}

func markerPayload(marker compactEffectMarker, mutate func(*compactEffectMarker)) []byte {
	mutate(&marker)
	payload, _ := json.Marshal(marker)
	return payload
}
