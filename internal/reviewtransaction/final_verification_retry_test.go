package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFinalVerificationIncidentIsStrictAndCanonical(t *testing.T) {
	incident := FinalVerificationIncident{
		Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
		LineageID: "failed-final-verification", TerminalRevision: hash("1"), ValidatingRevision: hash("2"),
		TargetIdentity: hash("3"), FailedEvidenceHash: hash("4"), FinalizeRequestDigest: hash("5"),
	}
	payload, err := CanonicalFinalVerificationIncident(incident)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseFinalVerificationIncident(payload)
	if err != nil || !reflect.DeepEqual(parsed, incident) || FinalVerificationIncidentDigest(parsed) != finalVerificationPayloadDigest(payload) {
		t.Fatalf("canonical incident = %#v, digest %q, err %v", parsed, FinalVerificationIncidentDigest(parsed), err)
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"crlf":                    func(value []byte) []byte { return bytes.ReplaceAll(value, []byte("\n"), []byte("\r\n")) },
		"noncanonical whitespace": func(value []byte) []byte { return append([]byte(" "), value...) },
		"unknown field": func(value []byte) []byte {
			return bytes.Replace(value, []byte(`"schema"`), []byte(`"unknown":true,"schema"`), 1)
		},
		"wrong class": func(value []byte) []byte {
			return bytes.Replace(value, []byte(FinalVerificationIncidentProceduralToolingFailure), []byte("reviewer_empty_result"), 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFinalVerificationIncident(mutate(append([]byte(nil), payload...))); err == nil {
				t.Fatal("mutated incident was accepted")
			}
		})
	}
}

func TestRetryCompactFinalVerificationCreatesOnlyOneFrozenValidatingSuccessor(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "retry-final-source", "retry-final-successor")
	record, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	assertFinalVerificationRetrySuccessor(t, fixture, record)

	replayed, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil || replayed.Revision != record.Revision || !compactStateEqual(replayed.State, record.State) {
		t.Fatalf("exact retry replay = %#v, err %v", replayed, err)
	}
	assertFinalVerificationRetrySuccessor(t, fixture, replayed)
	changed := fixture.request
	changed.Reason = "different retry request"
	changed.MaintainerAuthorization, err = FinalVerificationRetryAuthorization(changed)
	if err != nil {
		t.Fatal(err)
	}
	beforeChangedReplay := compactAuthorityFileSnapshot(t, fixture.repo)
	if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, changed); err == nil {
		t.Fatal("different retry request replay succeeded")
	}
	if after := compactAuthorityFileSnapshot(t, fixture.repo); !reflect.DeepEqual(after, beforeChangedReplay) {
		t.Fatalf("different retry replay mutated authority: %#v != %#v", after, beforeChangedReplay)
	}
	if entries := compactAuthorityFileSnapshot(t, fixture.repo); len(entries) != len(fixture.before)+1 {
		t.Fatalf("retry materialized unexpected authority artifacts: %#v", entries)
	}
}

func TestRetryCompactFinalVerificationUsesCorrectedCurrentSnapshot(t *testing.T) {
	fixture := newCorrectedFinalVerificationRetryFixture(t, "retry-corrected-source", "retry-corrected-successor")
	if snapshotsEqual(fixture.predecessor.State.InitialSnapshot, fixture.predecessor.State.CurrentSnapshot) {
		t.Fatal("corrected fixture did not advance CurrentSnapshot")
	}
	record, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	assertFinalVerificationRetrySuccessor(t, fixture, record)
	if !snapshotsEqual(record.State.CurrentSnapshot, fixture.predecessor.State.CurrentSnapshot) ||
		!snapshotsEqual(record.State.InitialSnapshot, fixture.predecessor.State.InitialSnapshot) {
		t.Fatalf("retry snapshots = initial %#v current %#v", record.State.InitialSnapshot, record.State.CurrentSnapshot)
	}
}

func TestRetryCompactFinalVerificationFreezesCanonicalRepositoryIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("repository symlink repointing is not portable on Windows")
	}
	fixture := newFinalVerificationRetryFixture(t, "retry-repoint-source", "retry-repoint-successor")
	otherRepo := initSnapshotRepo(t)
	collisionStore := storeCompactStartAuthority(t, otherRepo, newCompactTestState(t, otherRepo, fixture.request.SuccessorLineageID))
	collision := mustLoadCompactRecord(t, collisionStore)
	otherBefore := compactAuthorityFileSnapshot(t, otherRepo)

	link := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(fixture.repo, link); err != nil {
		t.Fatal(err)
	}
	originalHook := finalVerificationRetryAfterRepositoryIdentity
	finalVerificationRetryAfterRepositoryIdentity = func() {
		if err := os.Remove(link); err != nil {
			t.Fatalf("remove repository link: %v", err)
		}
		if err := os.Symlink(otherRepo, link); err != nil {
			t.Fatalf("repoint repository link: %v", err)
		}
	}
	t.Cleanup(func() { finalVerificationRetryAfterRepositoryIdentity = originalHook })

	retried, err := RetryCompactFinalVerification(context.Background(), link, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	assertFinalVerificationRetrySuccessor(t, fixture, retried)
	if after := compactAuthorityFileSnapshot(t, otherRepo); !reflect.DeepEqual(after, otherBefore) {
		t.Fatalf("repointed repository authority mutated\nbefore=%#v\nafter=%#v", otherBefore, after)
	}
	stillCollision := mustLoadCompactRecord(t, collisionStore)
	if stillCollision.Revision != collision.Revision || !compactStateEqual(stillCollision.State, collision.State) {
		t.Fatalf("repointed repository successor collision was overwritten: got %#v want %#v", stillCollision, collision)
	}
}

func TestRetryCompactFinalVerificationNeverReplacesConcurrentSuccessor(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "retry-publication-source", "retry-publication-successor")
	successorStore, err := CompactAuthoritativeStore(context.Background(), fixture.repo, fixture.request.SuccessorLineageID)
	if err != nil {
		t.Fatal(err)
	}
	collisionState := newCompactTestState(t, fixture.repo, fixture.request.SuccessorLineageID)
	collisionRecord, collisionPayload, err := makeCompactRecord(collisionState)
	if err != nil {
		t.Fatal(err)
	}
	originalHook := finalVerificationRetryAfterFirstLiveValidation
	finalVerificationRetryAfterFirstLiveValidation = func() {
		if err := writeAtomic(successorStore.StatePath(), collisionPayload, 0o644); err != nil {
			t.Fatalf("publish concurrent collision: %v", err)
		}
	}
	t.Cleanup(func() { finalVerificationRetryAfterFirstLiveValidation = originalHook })

	if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request); err == nil {
		t.Fatal("retry replaced a concurrently published successor")
	}
	got := mustLoadCompactRecord(t, successorStore)
	if got.Revision != collisionRecord.Revision || !compactStateEqual(got.State, collisionRecord.State) {
		t.Fatalf("concurrent successor was overwritten: got %#v want %#v", got, collisionRecord)
	}
	predecessor := mustLoadCompactRecord(t, fixture.store)
	if predecessor.Revision != fixture.predecessor.Revision || !compactStateEqual(predecessor.State, fixture.predecessor.State) {
		t.Fatal("publication collision mutated predecessor authority")
	}
}

func TestRetryCompactFinalVerificationIgnoresImmutablePublicationCrashResidue(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "retry-publish-residue-source", "retry-publish-residue-successor")
	successorStore, err := CompactAuthoritativeStore(context.Background(), fixture.repo, fixture.request.SuccessorLineageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(successorStore.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(successorStore.Dir, ".publish-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	retried, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil {
		t.Fatalf("retry after immutable-publication crash residue: %v", err)
	}
	assertFinalVerificationRetrySuccessor(t, fixture, retried)
}

func TestRetryCompactFinalVerificationRevalidatesEvidencePrivacyAfterOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX private-mode race coverage")
	}
	for _, tt := range []struct {
		name   string
		mutate func(string) error
	}{
		{name: "file becomes group-readable", mutate: func(path string) error { return os.Chmod(path, 0o640) }},
		{name: "directory becomes world-searchable", mutate: func(path string) error { return os.Chmod(filepath.Dir(path), 0o755) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFinalVerificationRetryFixture(t, "retry-mode-source", "retry-mode-successor")
			path := finalVerificationEvidencePath(fixture.store)
			originalHook := finalVerificationRetryEvidenceAfterLstat
			finalVerificationRetryEvidenceAfterLstat = func() {
				if err := tt.mutate(path); err != nil {
					t.Fatalf("mutate evidence mode: %v", err)
				}
			}
			t.Cleanup(func() { finalVerificationRetryEvidenceAfterLstat = originalHook })
			before := compactAuthorityFileSnapshot(t, fixture.repo)
			if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request); err == nil {
				t.Fatal("retry accepted evidence whose private mode changed after lstat")
			}
			if after := compactAuthorityFileSnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
				t.Fatalf("evidence mode denial mutated authority\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestRetryCompactFinalVerificationRequiresFinalizeRevisionContinuity(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*testing.T, *finalVerificationRetryFixture)
	}{
		{name: "first transition disconnected from request", mutate: func(t *testing.T, fixture *finalVerificationRetryFixture) {
			disconnected := hash("8")
			mutateFinalRetryAttempt(t, fixture, func(attempt *FinalizeAttempt) {
				attempt.Transitions[0].ExpectedRevision = disconnected
			})
			fixture.request.Incident.ValidatingRevision = disconnected
			fixture.request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, fixture.request)
		}},
		{name: "later transition disconnected from predecessor result", mutate: func(t *testing.T, fixture *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, fixture, func(attempt *FinalizeAttempt) {
				prior := FinalizeAttemptTransition{Operation: "review/intermediate", ExpectedRevision: attempt.Request.ExpectedRevision, Revision: hash("7")}
				attempt.Transitions = append([]FinalizeAttemptTransition{prior}, attempt.Transitions...)
			})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFinalVerificationRetryFixture(t, "retry-continuity-source", "retry-continuity-successor")
			tt.mutate(t, &fixture)
			before := compactAuthorityFileSnapshot(t, fixture.repo)
			if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request); err == nil {
				t.Fatal("retry accepted a disconnected FINALIZE transition chain")
			}
			if after := compactAuthorityFileSnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
				t.Fatalf("journal continuity denial mutated authority\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestFinalVerificationRetryAuthorizationFieldsAreCanonicalSingleLineText(t *testing.T) {
	request := FinalVerificationRetryRequest{
		PredecessorLineageID: "retry-fields-source", ExpectedPredecessorRevision: hash("1"), SuccessorLineageID: "retry-fields-successor",
		Incident: FinalVerificationIncident{Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
			LineageID: "retry-fields-source", TerminalRevision: hash("1"), ValidatingRevision: hash("2"), TargetIdentity: hash("3"),
			FailedEvidenceHash: hash("4"), FinalizeRequestDigest: hash("5")},
		Actor: "José Maintainer", Reason: "Reintentar verificación para 日本語 users",
	}
	if _, err := FinalVerificationRetryAuthorization(request); err != nil {
		t.Fatalf("valid international authorization text rejected: %v", err)
	}
	for _, tt := range []struct {
		name   string
		mutate func(*FinalVerificationRetryRequest)
	}{
		{name: "actor line separator", mutate: func(request *FinalVerificationRetryRequest) { request.Actor = "maintainer\u2028delegate" }},
		{name: "actor paragraph separator", mutate: func(request *FinalVerificationRetryRequest) { request.Actor = "maintainer\u2029delegate" }},
		{name: "actor control", mutate: func(request *FinalVerificationRetryRequest) { request.Actor = "maintainer\u0001delegate" }},
		{name: "reason line separator", mutate: func(request *FinalVerificationRetryRequest) { request.Reason = "retry\u2028verification" }},
		{name: "reason paragraph separator", mutate: func(request *FinalVerificationRetryRequest) { request.Reason = "retry\u2029verification" }},
		{name: "reason control", mutate: func(request *FinalVerificationRetryRequest) { request.Reason = "retry\u0001verification" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			invalid := request
			tt.mutate(&invalid)
			if _, err := FinalVerificationRetryAuthorization(invalid); err == nil {
				t.Fatal("non-canonical single-line authorization field was accepted")
			}
		})
	}
}

func TestRetryCompactFinalVerificationDenialsNeverMutateAuthority(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *finalVerificationRetryFixture)
	}{
		{name: "stale predecessor revision", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) {
			f.request.ExpectedPredecessorRevision = hash("9")
		}},
		{name: "invalid successor lineage", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) {
			f.request.SuccessorLineageID = "INVALID"
		}},
		{name: "multiline actor", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) {
			f.request.Actor = "maintainer\nother"
		}},
		{name: "authorization uses CRLF", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) {
			f.request.MaintainerAuthorization = strings.ReplaceAll(f.request.MaintainerAuthorization, "\n", "\r\n")
		}},
		{name: "incident lineage mismatch", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) { f.request.Incident.LineageID = "other-lineage" }},
		{name: "incident terminal mismatch", mutate: func(_ *testing.T, f *finalVerificationRetryFixture) { f.request.Incident.TerminalRevision = hash("8") }},
		{name: "incident validating mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			f.request.Incident.ValidatingRevision = hash("8")
			f.request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, f.request)
		}},
		{name: "incident target mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			f.request.Incident.TargetIdentity = hash("8")
			f.request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, f.request)
		}},
		{name: "incident evidence mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			f.request.Incident.FailedEvidenceHash = hash("8")
			f.request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, f.request)
		}},
		{name: "incident request mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			f.request.Incident.FinalizeRequestDigest = hash("8")
			f.request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, f.request)
		}},
		{name: "receipt mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			payload, err := os.ReadFile(f.store.ReceiptPath())
			if err != nil {
				t.Fatal(err)
			}
			var receipt CompactReceipt
			if err := json.Unmarshal(payload, &receipt); err != nil {
				t.Fatal(err)
			}
			receipt.EvidenceHash = hash("8")
			writeTestCompactReceipt(t, f.store.ReceiptPath(), receipt)
		}},
		{name: "journal candidate mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) {
				a.Request.CandidateDigest = hash("8")
				a.Request.RequestDigest = FinalizeAttemptRequestDigest(a.Request)
			})
		}},
		{name: "journal evidence mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) {
				a.Request.EvidenceDigest = hash("8")
				a.Request.RequestDigest = FinalizeAttemptRequestDigest(a.Request)
			})
		}},
		{name: "journal failed false", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) {
				a.Request.FailedDigest = FinalizeAttemptValueDigest("failed", false)
				a.Request.RequestDigest = FinalizeAttemptRequestDigest(a.Request)
			})
		}},
		{name: "journal request digest mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) { a.Request.RequestDigest = hash("8") })
		}},
		{name: "journal final operation mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) { a.Transitions[len(a.Transitions)-1].Operation = "review/complete-fix" })
		}},
		{name: "journal receipt marker missing", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) { a.ReceiptPublished = false })
		}},
		{name: "journal completion marker missing", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			mutateFinalRetryAttempt(t, f, func(a *FinalizeAttempt) { a.Completed = false })
		}},
		{name: "failed evidence bytes mismatch", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			if err := os.WriteFile(finalVerificationEvidencePath(f.store), []byte("different failed evidence\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "failed evidence symlink", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			path := finalVerificationEvidencePath(f.store)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.repo, "tracked.txt"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "live target drift", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			writeSnapshotFile(t, f.repo, "tracked.txt", "drifted\n")
		}},
		{name: "successor collision", mutate: func(t *testing.T, f *finalVerificationRetryFixture) {
			storeCompactStartAuthority(t, f.repo, newCompactTestState(t, f.repo, f.request.SuccessorLineageID))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newFinalVerificationRetryFixture(t, "retry-denial-source", "retry-denial-successor")
			tt.mutate(t, &fixture)
			before := compactAuthorityFileSnapshot(t, fixture.repo)
			if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request); err == nil {
				t.Fatal("denied retry succeeded")
			}
			after := compactAuthorityFileSnapshot(t, fixture.repo)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("denial mutated authority\nbefore=%#v\nafter=%#v", before, after)
			}
		})
	}
}

func TestRetryCompactFinalVerificationDeniesReviewerAndValidatorIncidents(t *testing.T) {
	tests := []struct {
		name  string
		state func(*testing.T, string) (string, CompactRecord)
	}{
		{name: "reviewer empty result escalation", state: func(t *testing.T, lineage string) (string, CompactRecord) {
			repo := initSnapshotRepo(t)
			state := newCompactTestState(t, repo, lineage)
			state.State = StateEscalated
			state.ResultDispositions = []CompactResultDisposition{{Lens: LensReliability, SelectedOrder: 0, TargetIdentity: state.InitialSnapshot.Identity, ArtifactDigest: hash("2"), Class: ResultDispositionTransportSyntax, Diagnostic: "empty", Reason: "empty reviewer", Actor: "maintainer", DisposedAt: time.Unix(1, 0).UTC(), MaintainerAuthorization: "authorization"}}
			if len(state.SelectedLenses) == 0 {
				state.SelectedLenses = []string{LensReliability}
				state.RiskLevel = RiskMedium
			}
			store := storeCompactStartAuthorityForTerminalFixture(t, repo, state)
			return repo, mustLoadCompactRecord(t, store)
		}},
		{name: "historical scoped validator failure", state: func(t *testing.T, lineage string) (string, CompactRecord) {
			repo := initSnapshotRepo(t)
			state, _ := pendingCompactCorrection(t, repo, lineage)
			store := storeCompactStartAuthorityForTerminalFixture(t, repo, state)
			return repo, mustLoadCompactRecord(t, store)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, record := tt.state(t, "retry-ineligible-source")
			request := FinalVerificationRetryRequest{PredecessorLineageID: record.State.LineageID, ExpectedPredecessorRevision: record.Revision,
				SuccessorLineageID: "retry-ineligible-successor", Incident: FinalVerificationIncident{Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
					LineageID: record.State.LineageID, TerminalRevision: record.Revision, ValidatingRevision: hash("2"), TargetIdentity: record.State.CurrentSnapshot.Identity, FailedEvidenceHash: hash("3"), FinalizeRequestDigest: hash("4")},
				Actor: "maintainer", Reason: "procedural tooling failure", RecoveredAt: time.Unix(2, 0).UTC()}
			request.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, request)
			before := compactAuthorityFileSnapshot(t, repo)
			if _, err := RetryCompactFinalVerification(context.Background(), repo, request); err == nil {
				t.Fatal("ineligible incident succeeded")
			}
			if after := compactAuthorityFileSnapshot(t, repo); !reflect.DeepEqual(after, before) {
				t.Fatalf("ineligible incident mutated authority: %#v != %#v", after, before)
			}
		})
	}
}

func TestRetryCompactFinalVerificationIsPermanentlyOneShotAcrossAncestry(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "retry-once-source", "retry-once-successor")
	first, err := RetryCompactFinalVerification(context.Background(), fixture.repo, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, first.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte("retry also failed\n")
	request := finalVerificationAttemptRequest(first, evidence, true)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	terminal := first.State
	if err := terminal.CompleteVerification(evidence, false); err != nil {
		t.Fatal(err)
	}
	planned, err := store.PlanFinalizeAttemptTransition(request.RequestDigest, "review/complete-verification", first.Revision, terminal)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace(first.Revision, "review/complete-verification", terminal)
	if err != nil || revision != planned {
		t.Fatalf("retry terminal = %q, %v", revision, err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, terminal)); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	writeFinalVerificationEvidence(t, store, evidence)
	secondIncident := FinalVerificationIncident{Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
		LineageID: terminal.LineageID, TerminalRevision: revision, ValidatingRevision: first.Revision, TargetIdentity: terminal.CurrentSnapshot.Identity,
		FailedEvidenceHash: payloadDigest(evidence), FinalizeRequestDigest: request.RequestDigest}
	second := FinalVerificationRetryRequest{PredecessorLineageID: terminal.LineageID, ExpectedPredecessorRevision: revision, SuccessorLineageID: "retry-twice-successor",
		Incident: secondIncident, Actor: "maintainer", Reason: "second procedural failure", RecoveredAt: time.Unix(3, 0).UTC()}
	second.MaintainerAuthorization = mustFinalVerificationRetryAuthorization(t, second)
	before := compactAuthorityFileSnapshot(t, fixture.repo)
	if _, err := RetryCompactFinalVerification(context.Background(), fixture.repo, second); err == nil {
		t.Fatal("second final-verification retry succeeded")
	}
	if after := compactAuthorityFileSnapshot(t, fixture.repo); !reflect.DeepEqual(after, before) {
		t.Fatalf("second retry mutated authority: %#v != %#v", after, before)
	}
}

func TestGenericRecoveryStillRejectsUnchangedEscalatedTarget(t *testing.T) {
	fixture := newFinalVerificationRetryFixture(t, "generic-unchanged-source", "generic-unchanged-successor")
	successor := newCompactTestState(t, fixture.repo, fixture.request.SuccessorLineageID)
	successor.Generation = fixture.predecessor.State.Generation + 1
	_, err := RecoverCompactAuthority(context.Background(), fixture.repo, CompactRecoveryRequest{
		PredecessorLineageID: fixture.predecessor.State.LineageID, ExpectedPredecessorRevision: fixture.predecessor.Revision,
		Successor: successor, Disposition: RecoveryEscalated, Reason: fixture.request.Reason, Actor: fixture.request.Actor,
		MaintainerAuthorization: compactRecoveryAuthorizationBinding(fixture.predecessor.State.LineageID, fixture.predecessor.Revision, successor.InitialSnapshot.Identity, fixture.request.Actor, fixture.request.Reason),
	})
	if !errors.Is(err, errCompactRecoveryTargetUnchanged) {
		t.Fatalf("generic unchanged-target recovery error = %v", err)
	}
}

type finalVerificationRetryFixture struct {
	repo        string
	store       CompactStore
	predecessor CompactRecord
	request     FinalVerificationRetryRequest
	evidence    []byte
	before      map[string]string
}

func newFinalVerificationRetryFixture(t *testing.T, predecessorLineage, successorLineage string) finalVerificationRetryFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	state := newCompactTestState(t, repo, predecessorLineage)
	store := storeCompactStartAuthority(t, repo, state)
	entry := mustLoadCompactRecord(t, store)
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{}, Classifications: []FindingEvidence{}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	validatingRevision, err := store.Replace(entry.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte("failed final verification evidence\n")
	validating := CompactRecord{Schema: compactRecordSchema, Revision: validatingRevision, State: state}
	request := finalVerificationAttemptRequest(validating, evidence, true)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	terminal := state
	if err := terminal.CompleteVerification(evidence, false); err != nil {
		t.Fatal(err)
	}
	planned, err := store.PlanFinalizeAttemptTransition(request.RequestDigest, "review/complete-verification", validatingRevision, terminal)
	if err != nil {
		t.Fatal(err)
	}
	terminalRevision, err := store.Replace(validatingRevision, "review/complete-verification", terminal)
	if err != nil || terminalRevision != planned {
		t.Fatalf("terminal transition = %q, %v", terminalRevision, err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, terminal)); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	writeFinalVerificationEvidence(t, store, evidence)
	predecessor := mustLoadCompactRecord(t, store)
	incident := FinalVerificationIncident{Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
		LineageID: predecessorLineage, TerminalRevision: predecessor.Revision, ValidatingRevision: validatingRevision,
		TargetIdentity: terminal.CurrentSnapshot.Identity, FailedEvidenceHash: payloadDigest(evidence), FinalizeRequestDigest: request.RequestDigest}
	retry := FinalVerificationRetryRequest{PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: successorLineage, Incident: incident, Actor: "maintainer", Reason: "redo final verification after procedural tooling failure", RecoveredAt: time.Unix(123, 0).UTC()}
	retry.MaintainerAuthorization, err = FinalVerificationRetryAuthorization(retry)
	if err != nil {
		t.Fatal(err)
	}
	return finalVerificationRetryFixture{repo: repo, store: store, predecessor: predecessor, request: retry, evidence: evidence, before: compactAuthorityFileSnapshot(t, repo)}
}

func newCorrectedFinalVerificationRetryFixture(t *testing.T, predecessorLineage, successorLineage string) finalVerificationRetryFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfour\n")
	state := newCompactTestState(t, repo, predecessorLineage)
	store := storeCompactStartAuthority(t, repo, state)
	entry := mustLoadCompactRecord(t, store)
	finding := Finding{ID: "R3-final-retry", Lens: strings.TrimPrefix(state.SelectedLenses[0], "review-"), Location: "tracked.txt:5", Severity: "CRITICAL", Claim: "wrong value", ProofRefs: []string{"candidate-only failure"}}
	if err := state.CompleteReview(CompactReviewInput{LensResults: []LensResult{{Lens: state.SelectedLenses[0], Findings: []Finding{finding}, Evidence: []string{"reviewed exact candidate"}}},
		Classifications: []FindingEvidence{{FindingID: finding.ID, Class: EvidenceDeterministic, Causality: CausalIntroduced, Proof: "changed hunk"}}, RefuterOutcomes: []EvidenceResult{}}); err != nil {
		t.Fatal(err)
	}
	reviewedRevision, err := store.Replace(entry.Revision, "review/complete-review", state)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.BeginCorrection(2); err != nil {
		t.Fatal(err)
	}
	forecastRevision, err := store.Replace(reviewedRevision, "review/begin-fix", state)
	if err != nil {
		t.Fatal(err)
	}
	writeSnapshotFile(t, repo, "tracked.txt", "base\none\ntwo\nthree\nfixed\n")
	fix, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetFixDiff, BaseRef: state.CurrentSnapshot.CandidateTree, IntendedUntracked: state.InitialSnapshot.IntendedUntracked, LedgerIDs: state.FixFindingIDs})
	if err != nil {
		t.Fatal(err)
	}
	fixHash := FixDeltaHashForSnapshot(fix)
	validation := bindTargetedValidationForTest(ScopedValidationResult{LedgerIDs: append([]string(nil), state.FixFindingIDs...), FixCausedFindings: []Finding{}, FollowUps: []FollowUp{},
		OriginalCriteria: ValidationCheck{EvidenceHash: hash("2"), FixDeltaHash: fixHash, Passed: true}, CorrectionRegression: ValidationCheck{EvidenceHash: hash("3"), FixDeltaHash: fixHash, Passed: true}}, fix)
	actual, err := (SnapshotBuilder{Repo: repo}).ChangedLines(context.Background(), fix)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.CompleteCorrection(fix, actual, validation); err != nil {
		t.Fatal(err)
	}
	validatingRevision, err := store.Replace(forecastRevision, "review/complete-fix", state)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte("corrected candidate final verification failed procedurally\n")
	validating := CompactRecord{Schema: compactRecordSchema, Revision: validatingRevision, State: state}
	request := finalVerificationAttemptRequest(validating, evidence, true)
	if _, _, err := store.BeginFinalizeAttempt(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	terminal := state
	if err := terminal.CompleteVerification(evidence, false); err != nil {
		t.Fatal(err)
	}
	planned, err := store.PlanFinalizeAttemptTransition(request.RequestDigest, "review/complete-verification", validatingRevision, terminal)
	if err != nil {
		t.Fatal(err)
	}
	terminalRevision, err := store.Replace(validatingRevision, "review/complete-verification", terminal)
	if err != nil || terminalRevision != planned {
		t.Fatalf("corrected terminal = %q, %v", terminalRevision, err)
	}
	if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, terminal)); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkFinalizeAttemptReceiptPublished(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFinalizeAttempt(request.RequestDigest); err != nil {
		t.Fatal(err)
	}
	writeFinalVerificationEvidence(t, store, evidence)
	predecessor := mustLoadCompactRecord(t, store)
	incident := FinalVerificationIncident{Schema: FinalVerificationIncidentSchema, Class: FinalVerificationIncidentProceduralToolingFailure,
		LineageID: predecessorLineage, TerminalRevision: predecessor.Revision, ValidatingRevision: validatingRevision, TargetIdentity: terminal.CurrentSnapshot.Identity,
		FailedEvidenceHash: payloadDigest(evidence), FinalizeRequestDigest: request.RequestDigest}
	retry := FinalVerificationRetryRequest{PredecessorLineageID: predecessorLineage, ExpectedPredecessorRevision: predecessor.Revision, SuccessorLineageID: successorLineage,
		Incident: incident, Actor: "maintainer", Reason: "retry corrected final verification", RecoveredAt: time.Unix(124, 0).UTC()}
	retry.MaintainerAuthorization, err = FinalVerificationRetryAuthorization(retry)
	if err != nil {
		t.Fatal(err)
	}
	return finalVerificationRetryFixture{repo: repo, store: store, predecessor: predecessor, request: retry, evidence: evidence, before: compactAuthorityFileSnapshot(t, repo)}
}

func finalVerificationAttemptRequest(record CompactRecord, evidence []byte, failed bool) FinalizeAttemptRequest {
	request := FinalizeAttemptRequest{LineageID: record.State.LineageID, ExpectedRevision: record.Revision,
		CandidateDigest: FinalizeAttemptValueDigest("candidate", record.State.CurrentSnapshot), ReviewerResultsDigest: FinalizeAttemptValueDigest("reviewer-results", []string{}),
		CorrectionForecastDigest: FinalizeAttemptValueDigest("correction-forecast", 0), ValidationDigest: FinalizeAttemptValueDigest("validation", nil),
		RefuterDigest: FinalizeAttemptValueDigest("refuter", nil), EvidenceDigest: FinalizeAttemptValueDigest("evidence", evidence), FailedDigest: FinalizeAttemptValueDigest("failed", failed)}
	request.RequestDigest = FinalizeAttemptRequestDigest(request)
	return request
}

func writeFinalVerificationEvidence(t *testing.T, store CompactStore, evidence []byte) {
	t.Helper()
	path := finalVerificationEvidencePath(store)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, evidence, 0o600); err != nil {
		t.Fatal(err)
	}
}

func finalVerificationEvidencePath(store CompactStore) string {
	return filepath.Join(store.Dir, CompactFinalEvidenceDir, CompactFinalEvidenceFile)
}

func payloadDigest(payload []byte) string {
	return finalVerificationPayloadDigest(payload)
}

func mustFinalVerificationRetryAuthorization(t *testing.T, request FinalVerificationRetryRequest) string {
	t.Helper()
	authorization, err := FinalVerificationRetryAuthorization(request)
	if err != nil {
		t.Fatal(err)
	}
	return authorization
}

func mutateFinalRetryAttempt(t *testing.T, fixture *finalVerificationRetryFixture, mutate func(*FinalizeAttempt)) {
	t.Helper()
	payload, err := os.ReadFile(fixture.store.FinalizeAttemptJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := parseFinalizeAttemptJournal(payload, fixture.predecessor.State.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&journal.Attempts[len(journal.Attempts)-1])
	encoded, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.store.FinalizeAttemptJournalPath(), append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFinalVerificationRetrySuccessor(t *testing.T, fixture finalVerificationRetryFixture, record CompactRecord) {
	t.Helper()
	predecessor := fixture.predecessor.State
	state := record.State
	if state.LineageID != fixture.request.SuccessorLineageID || state.Generation != predecessor.Generation+1 || state.State != StateValidating || state.EvidenceHash != "" || state.Recovery == nil ||
		state.Recovery.Disposition != RecoveryFinalVerificationRetry || state.Recovery.FinalVerificationRetry == nil {
		t.Fatalf("retry successor identity/state = %#v", state)
	}
	want := predecessor
	want.LineageID, want.Generation, want.State, want.EvidenceHash, want.Recovery = state.LineageID, state.Generation, StateValidating, "", state.Recovery
	if !compactStateEqual(state, want) {
		t.Fatalf("retry successor changed frozen authority\ngot=%#v\nwant=%#v", state, want)
	}
	proof := state.Recovery.FinalVerificationRetry
	if proof.FailedEvidenceHash != predecessor.EvidenceHash || proof.FinalizeRequestDigest != fixture.request.Incident.FinalizeRequestDigest || proof.IncidentDigest != FinalVerificationIncidentDigest(fixture.request.Incident) ||
		!reflect.DeepEqual(proof.Incident, fixture.request.Incident) || proof.SourceFinalizeAttempt.Request.RequestDigest != fixture.request.Incident.FinalizeRequestDigest {
		t.Fatalf("retry source proof = %#v", proof)
	}
	store, err := CompactAuthoritativeStore(context.Background(), fixture.repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{store.ReceiptPath(), store.FinalizeAttemptJournalPath(), finalVerificationEvidencePath(store)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retry successor started an extra budget/artifact at %q: %v", path, err)
		}
	}
}

func compactAuthorityFileSnapshot(t *testing.T, repo string) map[string]string {
	t.Helper()
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]string{}
	if err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() == "LOCK" || strings.HasPrefix(entry.Name(), ".atomic-") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(base, path)
		result[filepath.ToSlash(relative)] = finalVerificationPayloadDigest(payload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func mustLoadCompactRecord(t *testing.T, store CompactStore) CompactRecord {
	t.Helper()
	record, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func storeCompactStartAuthorityForTerminalFixture(t *testing.T, repo string, state CompactState) CompactStore {
	t.Helper()
	store, err := CompactAuthoritativeStore(context.Background(), repo, state.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	record, payload, err := makeCompactRecord(state)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision == "" {
		t.Fatal("empty fixture revision")
	}
	if err := writeAtomic(store.StatePath(), payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if state.State == StateEscalated {
		if err := WriteCompactReceiptAtomic(store.ReceiptPath(), stateReceipt(t, state)); err != nil {
			t.Fatal(err)
		}
	}
	return store
}
