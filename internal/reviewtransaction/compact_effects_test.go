package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCompactEffectMarkerTransitionsAreMonotonic(t *testing.T) {
	states := []compactEffectMarkerState{compactEffectPending, compactEffectBlocked, compactEffectApplied}
	allowed := map[[2]compactEffectMarkerState]bool{{compactEffectPending, compactEffectPending}: true, {compactEffectPending, compactEffectBlocked}: true, {compactEffectPending, compactEffectApplied}: true, {compactEffectBlocked, compactEffectBlocked}: true, {compactEffectBlocked, compactEffectApplied}: true, {compactEffectApplied, compactEffectApplied}: true}
	for _, from := range append([]compactEffectMarkerState{""}, states...) {
		for _, to := range states {
			name := string(from) + "_to_" + string(to)
			t.Run(name, func(t *testing.T) {
				repository, marker := newCompactEffectMarkerFixture(t, "case"+strings.ReplaceAll(name, "_", "-"))
				if from != "" {
					marker.State, marker.Observation = from, observationFor(from)
					if _, err := repository.write(context.Background(), marker); err != nil {
						t.Fatal(err)
					}
				}
				path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
				before, _ := os.ReadFile(path)
				marker.State, marker.Observation = to, observationFor(to)
				_, err := repository.write(context.Background(), marker)
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

func TestCompactEffectMarkerIsPrivateSeparateAndStable(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "private-stable")
	sharedRoot := filepath.Dir(filepath.Dir(repository.root))
	if err := os.MkdirAll(sharedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sharedRoot, 0o700) })
	authority := filepath.Join(sharedRoot, "v2", "authority.json")
	if err := os.MkdirAll(filepath.Dir(authority), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authority, []byte("authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeAuthority, _ := os.ReadFile(authority)
	marker.State, marker.Observation = compactEffectApplied, compactEffectPlatformLimited
	if _, err := repository.write(context.Background(), marker); err != nil {
		t.Fatal(err)
	}
	if err := validateRARRepositoryParent(sharedRoot); err != nil {
		t.Fatalf("valid shared authority ancestor was rejected: %v", err)
	}
	if err := validatePrivateRARDirectory(filepath.Join(sharedRoot, "effect-markers")); err != nil {
		t.Fatalf("marker subtree is not private: %v", err)
	}
	if err := validatePrivateRARDirectory(repository.root); err != nil {
		t.Fatalf("marker root is not private: %v", err)
	}
	path, _ := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, false)
	if err := validatePrivateRARFile(path); err != nil {
		t.Fatalf("marker file is not private: %v", err)
	}
	before, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	time.Sleep(20 * time.Millisecond)
	if _, err := repository.write(context.Background(), marker); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	afterInfo, _ := os.Stat(path)
	afterAuthority, _ := os.ReadFile(authority)
	if !bytes.Equal(before, after) || !info.ModTime().Equal(afterInfo.ModTime()) || !bytes.Equal(beforeAuthority, afterAuthority) {
		t.Fatal("exact replay or separate authority bytes changed")
	}
	// Windows cannot represent 0600: Chmod honors only the write bit and
	// Stat reports 0666; ownership privacy is enforced by the RAR security
	// descriptor checks instead (#3231).
	wantMarkerMode := fs.FileMode(0o600)
	if runtime.GOOS == "windows" {
		wantMarkerMode = 0o666
	}
	if info.Mode().Perm() != wantMarkerMode {
		t.Fatalf("marker mode = %o, want %o", info.Mode().Perm(), wantMarkerMode)
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
			_, _ = repository.write(context.Background(), candidate)
		}()
	}
	wait.Wait()
	marker.State, marker.Observation = compactEffectApplied, compactEffectDurable
	_, _ = repository.write(context.Background(), marker)
	if got, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); err != nil || got.State != compactEffectApplied {
		t.Fatalf("marker = %#v, %v", got, err)
	}
}

func TestCompactEffectMarkerRejectsCallerBeforeMutationAndReportsLimitedDurability(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "publication")
	marker.Observation = "unknown"
	if _, err := repository.write(context.Background(), marker); err == nil {
		t.Fatal("accepted invalid caller marker")
	}
	if _, err := os.Stat(filepath.Dir(filepath.Dir(repository.root))); !os.IsNotExist(err) {
		t.Fatalf("storage mutated: %v", err)
	}
	marker.Observation = compactEffectPendingTransient
	if _, err := repository.write(context.Background(), marker); err != nil {
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
	result, err := repository.write(context.Background(), marker)
	if err != nil || !result.DurabilityLimited {
		t.Fatalf("publication = %#v, %v", result, err)
	}
	if got, readErr := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); readErr != nil || got != marker {
		t.Fatalf("published marker = %#v, %v", got, readErr)
	}
}

func TestCompactEffectMarkerWriteHonorsCancellationAndPreservesPublicationCause(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "write-failures")
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := acquireStoreLock(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := repository.write(ctx, marker); !errors.Is(err, context.Canceled) {
		t.Fatalf("write error = %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected publication failure")
	original := syncReviewDirectory
	syncReviewDirectory = func(dir string) error {
		if dir == filepath.Dir(path) {
			// Deliberate hostile corruption injected mid-publication: the raw
			// write is the point here, and it must not re-enter this hook the
			// way the private channel would.
			if err := os.WriteFile(path, []byte("corrupt\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return injected
		}
		return original(dir)
	}
	t.Cleanup(func() { syncReviewDirectory = original })
	result, err := repository.write(context.Background(), marker)
	if !result.DurabilityLimited || !errors.Is(err, injected) || !strings.Contains(err.Error(), "read-back mismatch") {
		t.Fatalf("publication = %#v, %v", result, err)
	}
}

func observationFor(state compactEffectMarkerState) compactEffectObservation {
	if state == compactEffectBlocked {
		return compactEffectBlockedConflict
	}
	if state == compactEffectApplied {
		return compactEffectDurable
	}
	return compactEffectPendingTransient
}

func TestCompactEffectMarkerStrictValidation(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "strict-validation")
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		t.Fatal(err)
	}
	valid := markerPayload(marker, func(*compactEffectMarker) {})
	cases := []struct {
		name    string
		payload []byte
	}{
		{"valid", valid},
		{"malformed", []byte("{")},
		{"trailing JSON", append(append([]byte(nil), valid...), []byte("{}")...)},
		{"unknown field", []byte(`{"schema":"gentle-ai.review-effect-marker/v1","lineage_id":"strict-validation","authority_revision":"` + hash("a") + `","event_id":"` + hash("b") + `","state":"pending","observation":"pending_transient","extra":true}`)},
		{"wrong schema", markerPayload(marker, func(value *compactEffectMarker) { value.Schema = "wrong" })},
		{"wrong lineage", markerPayload(marker, func(value *compactEffectMarker) { value.LineageID = "wrong" })},
		{"wrong revision", markerPayload(marker, func(value *compactEffectMarker) { value.AuthorityRevision = hash("c") })},
		{"wrong event", markerPayload(marker, func(value *compactEffectMarker) { value.EventID = hash("d") })},
		{"invalid state", markerPayload(marker, func(value *compactEffectMarker) { value.State = "unknown" })},
		{"invalid observation", markerPayload(marker, func(value *compactEffectMarker) { value.Observation = "unknown" })},
		{"mismatched state and observation", markerPayload(marker, func(value *compactEffectMarker) { value.State = compactEffectApplied })},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// Seeding must use the private channel: a raw os.WriteFile
			// carries the inherited default DACL, which the Windows private
			// validators correctly refuse (#3231).
			if err := writePrivateRARAtomic(path, tt.payload); err != nil {
				t.Fatal(err)
			}
			got, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID)
			if tt.name == "valid" {
				if err != nil || got != marker {
					t.Fatalf("marker = %#v, %v", got, err)
				}
			} else if err == nil {
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
	if err := os.Symlink(t.TempDir(), repository.root); err == nil {
		if _, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true); err == nil {
			t.Fatal("accepted symlink root")
		}
	} else if !errors.Is(err, os.ErrPermission) && !errors.Is(err, errors.ErrUnsupported) {
		t.Fatal(err)
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

func TestWritePrivateRARAtomicRejectsSymlinkParent(t *testing.T) {
	realParent := t.TempDir()
	linkedParent := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, errors.ErrUnsupported) {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := writePrivateRARAtomic(filepath.Join(linkedParent, "marker.json"), []byte("marker\n")); err == nil {
		t.Fatal("accepted symlink private parent")
	}
}

func TestWritePrivateRARAtomicRejectsInvalidPayloadSize(t *testing.T) {
	dir := t.TempDir()
	if err := writePrivateRARAtomic(filepath.Join(dir, "empty.json"), nil); err == nil {
		t.Fatal("accepted empty atomic payload")
	}
	if err := writePrivateRARAtomic(filepath.Join(dir, "oversized.json"), make([]byte, rarAuthorityMaxBytes+1)); err == nil {
		t.Fatal("accepted oversized atomic payload")
	}
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

func historicalCompactRepositoryContextIntent(t *testing.T, repo string, state CompactState) CompactEffectIntent {
	t.Helper()
	statePayload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	binding := ReviewRepositoryContextBinding{LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: compactStateRevision(statePayload)}
	identity, err := reviewRepositoryIdentity(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	handle := reviewRepositoryContextHandle(binding, identity)
	payload, err := json.Marshal(reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: handle, LineageID: binding.LineageID,
		TargetIdentity: binding.TargetIdentity, Revision: binding.Revision, RepositoryIdentity: identity.RepositoryIdentity,
		RepositoryRoot: identity.RepositoryRoot, GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	return CompactEffectIntent{Class: CompactEffectClassRepositoryContext, Destination: handle, PayloadHash: hashPayloadBytes(payload)}
}

func TestCompactRepositoryContextIntentAndReconcilerContract(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, "repository-context-contract")
	intent := historicalCompactRepositoryContextIntent(t, repo, state)
	if intent.Class != CompactEffectClassRepositoryContext || intent.Destination == "" || !validSHA256(intent.PayloadHash) {
		t.Fatalf("intent = %#v", intent)
	}
	record, payload, err := makeCompactRecordWithIntents(state, []CompactEffectIntent{intent})
	if err != nil {
		t.Fatal(err)
	}
	store, _ := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileCompactRepositoryContext(context.Background(), store, record)
	if err != nil || result.Handle != intent.Destination || result.EventID != record.EffectIntents[0].EventID || result.Outcome != CompactRepositoryContextApplied {
		t.Fatalf("result = %#v, %v", result, err)
	}
	path, err := reviewRepositoryContextPath(intent.Destination)
	if err != nil {
		t.Fatal(err)
	}
	// The locator subsystem owns its own storage contract ("not authority"),
	// so the published context reads through the locator reader; the strict
	// private-RAR reader is the wrong bar for it on Windows (#3231).
	published, err := readReviewRepositoryContext(path)
	if err != nil || hashPayloadBytes(bytes.TrimSuffix(published, []byte{'\n'})) != intent.PayloadHash {
		t.Fatalf("published context = %q, %v", published, err)
	}

	blockedState := newCompactTestState(t, repo, "repository-context-blocked")
	blockedIntent := historicalCompactRepositoryContextIntent(t, repo, blockedState)
	blockedRecord, _, err := makeCompactRecordWithIntents(blockedState, []CompactEffectIntent{blockedIntent})
	if err != nil {
		t.Fatal(err)
	}
	blockedRecord.EffectIntents[0].PayloadHash = hash("mismatch")
	blockedStore, _ := CompactAuthoritativeStore(context.Background(), repo, blockedState.LineageID)
	blocked, blockedErr := ReconcileCompactRepositoryContext(context.Background(), blockedStore, blockedRecord)
	if blocked.Outcome != CompactRepositoryContextBlocked || blockedErr == nil {
		t.Fatalf("blocked result = %#v, %v", blocked, blockedErr)
	}

	conflictState := newCompactTestState(t, repo, "repository-context-conflict")
	conflictIntent := historicalCompactRepositoryContextIntent(t, repo, conflictState)
	conflictRecord, _, err := makeCompactRecordWithIntents(conflictState, []CompactEffectIntent{conflictIntent})
	if err != nil {
		t.Fatal(err)
	}
	conflictPath, err := prepareReviewRepositoryContextPath(conflictIntent.Destination)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := reviewRepositoryIdentity(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	conflictingPayload, err := json.Marshal(reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: conflictIntent.Destination, LineageID: conflictState.LineageID,
		TargetIdentity: conflictState.InitialSnapshot.Identity, Revision: conflictRecord.EffectIntents[0].BindingRevision,
		RepositoryIdentity: identity.RepositoryIdentity, RepositoryRoot: filepath.Join(identity.RepositoryRoot, "other"),
		GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conflictPath, append(conflictingPayload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	conflictStore, _ := CompactAuthoritativeStore(context.Background(), repo, conflictState.LineageID)
	conflict, conflictErr := ReconcileCompactRepositoryContext(context.Background(), conflictStore, conflictRecord)
	if conflict.Outcome != CompactRepositoryContextBlocked || conflictErr == nil {
		t.Fatalf("conflict result = %#v, %v", conflict, conflictErr)
	}

	pendingState := newCompactTestState(t, repo, "repository-context-pending")
	pendingIntent := historicalCompactRepositoryContextIntent(t, repo, pendingState)
	pendingRecord, pendingPayload, err := makeCompactRecordWithIntents(pendingState, []CompactEffectIntent{pendingIntent})
	if err != nil {
		t.Fatal(err)
	}
	pendingStore, _ := CompactAuthoritativeStore(context.Background(), repo, pendingState.LineageID)
	if err := writeAtomic(pendingStore.StatePath(), pendingPayload, 0o644); err != nil {
		t.Fatal(err)
	}
	pendingPath, err := prepareReviewRepositoryContextPath(pendingIntent.Destination)
	if err != nil {
		t.Fatal(err)
	}
	originalSync := syncReviewDirectory
	syncReviewDirectory = func(dir string) error {
		if dir == filepath.Dir(pendingPath) {
			return errors.New("injected transient publication failure")
		}
		return originalSync(dir)
	}
	pending, pendingErr := ReconcileCompactRepositoryContext(context.Background(), pendingStore, pendingRecord)
	syncReviewDirectory = originalSync
	if pending.Outcome != CompactRepositoryContextPending || pendingErr == nil {
		t.Fatalf("pending result = %#v, %v", pending, pendingErr)
	}
	if _, err := ReconcileCompactRepositoryContext(context.Background(), store, CompactRecord{State: state}); err == nil {
		t.Fatal("record without repository context intent reconciled")
	}
}

func markerPayload(marker compactEffectMarker, mutate func(*compactEffectMarker)) []byte {
	mutate(&marker)
	payload, _ := json.Marshal(marker)
	return payload
}
