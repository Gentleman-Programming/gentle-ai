package sddstatus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots is #2540 S2's core
// round-trip: a committed grant projects its canonical absolute
// symlink-evaluated roots into RuntimeStatus.GrantedRoots, and a second
// grant chains and accumulates.
func TestRuntimeLedgerGrantCommitsAndProjectsGrantedRoots(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "grant-roots")
	if err != nil {
		t.Fatal(err)
	}
	store, err := opened.ForInstance("grant-roots-instance")
	if err != nil {
		t.Fatal(err)
	}

	sibling, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "sibling-link")
	if err := os.Symlink(sibling, link); err != nil {
		t.Fatal(err)
	}

	// The caller passes the SYMLINK path; the recorded and projected root must
	// be the canonical evaluated target, following BeginWorktree's precedent.
	granted, err := store.Grant(context.Background(), GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-1", Roots: []string{link},
		Reason: "maintainer authorized sibling repository edits", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("grant refused: %v", err)
	}
	if len(granted.GrantedRoots) != 1 || granted.GrantedRoots[0] != sibling ||
		granted.Revision == "" || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("granted status = %#v records=%d, want one chained record granting %q",
			granted, countRuntimeRecords(t, store.Dir), sibling)
	}

	// Status() replays the persisted chain: the projection round-trips.
	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GrantedRoots) != 1 || status.GrantedRoots[0] != sibling {
		t.Fatalf("replayed granted roots = %#v, want [%q]", status.GrantedRoots, sibling)
	}

	// A second grant chains PreviousRevision on the first and accumulates,
	// deduplicating the already-granted root.
	second, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accumulated, err := store.Grant(context.Background(), GrantRootsRequest{
		ExpectedRevision: granted.Revision, RequestID: "grant-2", Roots: []string{second, sibling},
		Reason: "maintainer widened the change to a second sibling", Actor: "maintainer",
	})
	if err != nil {
		t.Fatalf("second grant refused: %v", err)
	}
	if len(accumulated.GrantedRoots) != 2 || accumulated.GrantedRoots[0] != sibling ||
		accumulated.GrantedRoots[1] != second || countRuntimeRecords(t, store.Dir) != 2 {
		t.Fatalf("accumulated granted roots = %#v records=%d, want [%q %q] over 2 records",
			accumulated.GrantedRoots, countRuntimeRecords(t, store.Dir), sibling, second)
	}
}

// TestRuntimeLedgerGrantDuplicateRequestIsIdempotent proves the sibling-event
// RequestID/RequestDigest contract: an exact replay returns the committed
// revision without a new record; reuse with different inputs is refused.
func TestRuntimeLedgerGrantDuplicateRequestIsIdempotent(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "grant-idempotent")
	if err != nil {
		t.Fatal(err)
	}
	store, err := opened.ForInstance("grant-idempotent-instance")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-once", Roots: []string{root},
		Reason: "maintainer authorized sibling repository edits", Actor: "maintainer",
	}
	granted, err := store.Grant(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	replayed, err := store.Grant(context.Background(), request)
	if err != nil || replayed.Revision != granted.Revision || countRuntimeRecords(t, store.Dir) != 1 {
		t.Fatalf("grant replay = %#v err=%v records=%d, want committed revision %q and 1 record",
			replayed, err, countRuntimeRecords(t, store.Dir), granted.Revision)
	}

	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request.Roots = []string{other}
	if _, err := store.Grant(context.Background(), request); !errors.Is(err, ErrRuntimeRequestConflict) {
		t.Fatalf("conflicting grant reuse = %v, want ErrRuntimeRequestConflict", err)
	}
}

// TestRuntimeLedgerGrantReplayRefusesForgedWidenedRecord is the
// runtimeRescopeEvent forgery pattern applied to grants: widened Roots no
// longer match the RequestDigest the original request bound, so replay's
// digest recompute refuses the chain.
func TestRuntimeLedgerGrantReplayRefusesForgedWidenedRecord(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "grant-forged-replay")
	if err != nil {
		t.Fatal(err)
	}
	store, err := opened.ForInstance("grant-forged-instance")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-forge-1", Roots: []string{root},
		Reason: "maintainer authorized one sibling repository", Actor: "maintainer",
	}
	granted, err := store.Grant(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	// Forged: the record claims the SAME request granted a widened root set,
	// keeping the digest the original single-root request produced.
	widened, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedRevision, request.RequestID = granted.Revision, "grant-forge-2"
	request.ChangeInstance = "grant-forged-instance"
	forgedRecord := runtimeRecord{
		Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: granted.Revision,
		Operation: runtimeOperationGrant, RequestID: request.RequestID,
		RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request),
		Grant: &runtimeGrantEvent{
			Roots: []string{root, widened}, Actor: "attacker",
			Reason: "forged widened grant", GrantedAt: "2026-08-05T00:00:00Z",
			Instance: "grant-forged-instance",
		},
	}
	revision, payload, err := runtimeRecordRevision(forgedRecord)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRecord(revision, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.publishHead(revision); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Status(); !isRuntimeRecordRejection(err, "grant_request_digest_match") {
		t.Fatalf("replay of forged widened grant = %v, want the digest-recompute rejection", err)
	}
}

// TestRuntimeLedgerLegacyChainWithoutGrantsReplaysUnchanged pins the phase-1
// compatibility constraint: a chain recorded before grant records existed
// replays exactly as before, projects no granted roots, and serializes no
// granted_roots member (omitempty is load-bearing).
func TestRuntimeLedgerLegacyChainWithoutGrantsReplaysUnchanged(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store, err := OpenRuntimeStore(context.Background(), repo, "grant-legacy-chain")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), BeginAttemptRequest{
		ExpectedRevision: "", RequestID: "legacy-begin-1", WorkUnit: "legacy-scope",
		EvidenceGoal: "prove grant-free chains replay unchanged", MaxAttempts: 2, MaxChangedLines: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := store.Finish(context.Background(), FinishAttemptRequest{
		ExpectedRevision: started.Revision, RequestID: "legacy-finish-1", Outcome: AttemptInterrupted,
		Diagnosis:          "interrupted with the workspace unchanged",
		HarnessDisposition: HarnessInvalidated, CleanupEvidence: "no executor process was ever spawned",
		ProcessEvidence: "pre-launch process scan found no descendants",
	})
	if err != nil {
		t.Fatal(err)
	}
	if finished.GrantedRoots != nil {
		t.Fatalf("grant-free chain projected granted roots: %#v", finished.GrantedRoots)
	}
	status, err := store.Status()
	if err != nil {
		t.Fatalf("grant-free chain replay = %v, want unchanged success", err)
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "granted_roots") {
		t.Fatalf("grant-free status serialized a granted_roots member: %s", payload)
	}
}

// TestRuntimeLedgerArchivedNameReuseDoesNotResurrectGrantedRoots is #2557's
// hazard fixture (S5 of #2540): archive is agent-side artifact movement that
// never touches the per-change runtime ledger, and the ledger directory is
// keyed by change name alone, so a future change reusing an archived name
// reopens the SAME chain. Its reader must not inherit the archived change's
// grants: per-change authority must not become workspace-permanent authority
// through the back door.
func TestRuntimeLedgerArchivedNameReuseDoesNotResurrectGrantedRoots(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "reused-change-name")
	if err != nil {
		t.Fatal(err)
	}
	first, err := opened.ForInstance("first-change-instance")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	granted, err := first.Grant(context.Background(), GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-first-instance", Roots: []string{root},
		Reason: "maintainer authorized sibling repository edits for the first change", Actor: "maintainer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(granted.GrantedRoots) != 1 || granted.GrantedRoots[0] != root {
		t.Fatalf("first instance projected %#v, want its own granted root %q", granted.GrantedRoots, root)
	}

	// The first change is archived: only openspec artifacts move; the ledger
	// chain stays exactly where it is. A recreated change with the same name
	// resolves the same directory and replays the same chain, and before this
	// slice its reader inherited the archived change's grants verbatim. A
	// reader with no instance identity (every current caller, including the
	// status.go embedding until S4b threads one) must project nothing.
	reused, err := OpenRuntimeStore(context.Background(), repo, "reused-change-name")
	if err != nil {
		t.Fatal(err)
	}
	status, err := reused.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GrantedRoots) != 0 {
		t.Fatalf("archived-name reuse resurrected granted roots: %#v", status.GrantedRoots)
	}

	// The recreated change carries its own instance identity: the archived
	// change's grants stay bound to the identity the human consented to.
	second, err := reused.ForInstance("second-change-instance")
	if err != nil {
		t.Fatal(err)
	}
	status, err = second.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GrantedRoots) != 0 {
		t.Fatalf("recreated change instance resurrected granted roots: %#v", status.GrantedRoots)
	}

	// The original instance keeps its authority: containment narrows the
	// projection to the consented instance, it revokes nothing.
	replayedFirst, err := reused.ForInstance("first-change-instance")
	if err != nil {
		t.Fatal(err)
	}
	status, err = replayedFirst.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GrantedRoots) != 1 || status.GrantedRoots[0] != root {
		t.Fatalf("original instance projected %#v, want its granted root %q", status.GrantedRoots, root)
	}
}

// TestRuntimeLedgerGrantRequiresChangeInstance contains the write path: a
// store that never declared which change instance it serves cannot record
// authority at all, and an incoherent caller-supplied instance is refused
// rather than silently rebound.
func TestRuntimeLedgerGrantRequiresChangeInstance(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "grant-instance-required")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-no-instance", Roots: []string{root},
		Reason: "maintainer authorized sibling repository edits", Actor: "maintainer",
	}
	if _, err := opened.Grant(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "requires a change-instance identity") {
		t.Fatalf("instance-less grant = %v, want the ForInstance refusal", err)
	}
	if _, err := opened.ForInstance(""); err == nil {
		t.Fatal("empty change-instance identity was accepted")
	}

	store, err := opened.ForInstance("declared-instance")
	if err != nil {
		t.Fatal(err)
	}
	request.ChangeInstance = "some-other-instance"
	if _, err := store.Grant(context.Background(), request); err == nil ||
		!strings.Contains(err.Error(), "does not equal the store's instance identity") {
		t.Fatalf("incoherent grant instance = %v, want the mismatch refusal", err)
	}
	if countRuntimeRecords(t, store.Dir) != 0 {
		t.Fatalf("refused grants recorded %d records, want none", countRuntimeRecords(t, store.Dir))
	}
}

// TestRuntimeLedgerGrantReplayRefusesForgedInstanceMarker applies the sibling
// forgery discipline to the instance identity: a record whose identity was
// rebound or stripped after publication no longer matches the digest the
// original request bound, so replay refuses the chain instead of projecting
// the grant into another instance.
func TestRuntimeLedgerGrantReplayRefusesForgedInstanceMarker(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	opened, err := OpenRuntimeStore(context.Background(), repo, "grant-instance-forgery")
	if err != nil {
		t.Fatal(err)
	}
	store, err := opened.ForInstance("consented-instance")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := GrantRootsRequest{
		ExpectedRevision: "", RequestID: "grant-instance-1", Roots: []string{root},
		Reason: "maintainer authorized one sibling repository", Actor: "maintainer",
		ChangeInstance: "consented-instance",
	}
	granted, err := store.Grant(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	// Forged: the digest still binds the consented instance, but the record
	// claims the grant belongs to a different one — the exact rebinding a
	// name-reuse attacker would need to resurrect authority.
	forge := func(t *testing.T, requestID, instance string) error {
		t.Helper()
		forgedRequest := request
		forgedRequest.ExpectedRevision, forgedRequest.RequestID = granted.Revision, requestID
		forgedRecord := runtimeRecord{
			Schema: runtimeRecordSchema, Change: store.Change, PreviousRevision: granted.Revision,
			Operation: runtimeOperationGrant, RequestID: requestID,
			RequestDigest: runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", forgedRequest),
			Grant: &runtimeGrantEvent{
				Roots: []string{root}, Actor: "maintainer",
				Reason: "maintainer authorized one sibling repository", GrantedAt: "2026-08-05T00:00:00Z",
				Instance: instance,
			},
		}
		revision, payload, err := runtimeRecordRevision(forgedRecord)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.publishRecord(revision, payload); err != nil {
			t.Fatal(err)
		}
		if err := store.publishHead(revision); err != nil {
			t.Fatal(err)
		}
		_, statusErr := store.Status()
		if headErr := store.publishHead(granted.Revision); headErr != nil {
			t.Fatal(headErr)
		}
		return statusErr
	}

	if err := forge(t, "grant-instance-rebound", "second-change-instance"); !isRuntimeRecordRejection(err, "grant_request_digest_match") {
		t.Fatalf("replay of rebound instance marker = %v, want the digest-recompute rejection", err)
	}
	if err := forge(t, "grant-instance-stripped", ""); !isRuntimeRecordRejection(err, "invalid_grant_change_instance") {
		t.Fatalf("replay of stripped instance marker = %v, want the invalid-identity rejection", err)
	}
}
