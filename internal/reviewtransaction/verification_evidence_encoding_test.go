package reviewtransaction

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newLedgerFreeEvidenceSnapshot builds a valid frozen snapshot whose target
// carries no correction ledger — the ordinary clean-path candidate every
// non-corrected review captures final verification evidence for.
func newLedgerFreeEvidenceSnapshot(t *testing.T) Snapshot {
	t.Helper()
	snapshot := Snapshot{
		Kind:                   TargetCurrentChanges,
		BaseTree:               strings.Repeat("1", 40),
		CandidateTree:          strings.Repeat("2", 40),
		Paths:                  []string{"docs/guide.md"},
		IntendedUntracked:      []string{},
		IntendedUntrackedProof: hashCanonical("gentle-ai.intended-untracked/v1"),
	}
	snapshot.PathsDigest = digestPaths(snapshot.Paths)
	snapshot.Identity = snapshotIdentityForProjection(snapshot.Kind, snapshot.Projection, snapshot.BaseTree,
		snapshot.CandidateTree, snapshot.PathsDigest, snapshot.IntendedUntrackedProof, snapshot.IntendedUntracked, snapshot.LedgerIDs)
	return snapshot
}

// TestVerificationEvidenceRecordEncodesLedgerFreeCleanPathAsEmptyArray is the
// RED-first proof for the cross-lane battery's verification-evidence finding:
// the published gentle-ai.review-verification-evidence/v2 schema requires
// ledger_ids to be an array, but the clean-path emitter (no correction, so no
// ledger) marshaled the nil slice as `"ledger_ids":null`.
func TestVerificationEvidenceRecordEncodesLedgerFreeCleanPathAsEmptyArray(t *testing.T) {
	t.Parallel()
	snapshot := newLedgerFreeEvidenceSnapshot(t)
	record, err := NewVerificationEvidenceRecord("clean-path-lineage", "sha256:"+strings.Repeat("a", 64),
		snapshot, []byte("go test ./...: pass\n"), VerificationOutcomePassed)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := CanonicalVerificationEvidenceRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte(`"ledger_ids":null`)) {
		t.Fatalf("clean-path evidence record encodes ledger_ids as null, the published schema requires an array:\n%s", payload)
	}
	if !bytes.Contains(payload, []byte(`"ledger_ids":[]`)) {
		t.Fatalf("clean-path evidence record does not encode ledger_ids as the empty array:\n%s", payload)
	}
	// A record captured with a correction ledger keeps its exact IDs.
	ledgered := snapshot
	ledgered.LedgerIDs = []string{"ledger-1"}
	ledgered.Identity = snapshotIdentityForProjection(ledgered.Kind, ledgered.Projection, ledgered.BaseTree,
		ledgered.CandidateTree, ledgered.PathsDigest, ledgered.IntendedUntrackedProof, ledgered.IntendedUntracked, ledgered.LedgerIDs)
	withLedger, err := NewVerificationEvidenceRecord("clean-path-lineage", "sha256:"+strings.Repeat("a", 64),
		ledgered, []byte("go test ./...: pass\n"), VerificationOutcomePassed)
	if err != nil {
		t.Fatal(err)
	}
	ledgeredPayload, err := CanonicalVerificationEvidenceRecord(withLedger)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ledgeredPayload, []byte(`"ledger_ids":["ledger-1"]`)) {
		t.Fatalf("ledgered evidence record lost its ledger IDs:\n%s", ledgeredPayload)
	}
}

// TestVerificationEvidenceRecordWithNullLedgerHistoryStaysReadable pins the
// compatibility half of the fix: records persisted by an older binary encoded
// the missing ledger as `null`, and those immutable bytes must keep parsing
// (canonical-form validation re-marshals the decoded record, so the historical
// null round-trips through the nil slice unchanged).
func TestVerificationEvidenceRecordWithNullLedgerHistoryStaysReadable(t *testing.T) {
	t.Parallel()
	snapshot := newLedgerFreeEvidenceSnapshot(t)
	record, err := NewVerificationEvidenceRecord("clean-path-lineage", "sha256:"+strings.Repeat("a", 64),
		snapshot, []byte("go test ./...: pass\n"), VerificationOutcomePassed)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := CanonicalVerificationEvidenceRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	historical := bytes.Replace(payload, []byte(`"ledger_ids":[]`), []byte(`"ledger_ids":null`), 1)
	if bytes.Equal(historical, payload) {
		t.Fatal("historical payload substitution did not apply")
	}
	// The digest domain is the marshaled record, so the historical bytes carry
	// the digest an older binary computed over the null encoding.
	legacy := record
	legacy.LedgerIDs = nil
	legacy.RecordDigest = verificationEvidenceRecordDigest(legacy)
	historical = bytes.Replace(historical,
		[]byte(`"record_digest":"`+record.RecordDigest+`"`),
		[]byte(`"record_digest":"`+legacy.RecordDigest+`"`), 1)
	parsed, err := ParseVerificationEvidenceRecord(historical)
	if err != nil {
		t.Fatalf("historical null-ledger record no longer parses: %v\n%s", err, historical)
	}
	if parsed.LedgerIDs != nil {
		t.Fatalf("historical record decoded ledger IDs = %#v, want nil", parsed.LedgerIDs)
	}
}

// TestVerificationEvidenceDigestDomainIsPinnedToEachRecordsOwnEncoding pins
// the documented digest-domain shift: the same logical ledger-free record
// digests differently under the historical `null` encoding and the published
// `[]` encoding, and the digest-keyed evidence store accepts each form
// validated against its own persisted canonical bytes, never the other's.
func TestVerificationEvidenceDigestDomainIsPinnedToEachRecordsOwnEncoding(t *testing.T) {
	t.Parallel()
	snapshot, revision := newLedgerFreeEvidenceSnapshot(t), "sha256:"+strings.Repeat("a", 64)
	lineage, payload := "clean-path-lineage", []byte("go test ./...: pass\n")
	newStore, legacyStore := t.TempDir(), t.TempDir()
	captured, err := PublishCapturedVerificationEvidence(CaptureVerificationEvidenceRequest{
		StoreDir: newStore, LineageID: lineage, AuthorityRevision: revision,
		Target: snapshot, Payload: payload, Outcome: VerificationOutcomePassed,
	})
	if err != nil {
		t.Fatal(err)
	}
	legacy := captured.Record
	legacy.LedgerIDs = nil
	legacy.RecordDigest = verificationEvidenceRecordDigest(legacy)
	if legacy.RecordDigest == captured.Record.RecordDigest {
		t.Fatal("digest domain shift: null and empty-array encodings must digest differently")
	}
	legacyBytes, err := CanonicalVerificationEvidenceRecord(legacy)
	if err != nil || !bytes.Contains(legacyBytes, []byte(`"ledger_ids":null`)) {
		t.Fatalf("null-form record does not validate against its own digest: %v\n%s", err, legacyBytes)
	}
	dir, err := compactFinalEvidenceCandidateDir(legacyStore, revision, snapshot.Identity)
	if err != nil || os.MkdirAll(dir, 0o700) != nil {
		t.Fatal(err)
	}
	if os.WriteFile(filepath.Join(dir, CompactFinalEvidenceFile), payload, 0o600) != nil ||
		os.WriteFile(filepath.Join(dir, CompactFinalEvidenceRecordFile), legacyBytes, 0o600) != nil {
		t.Fatal("write legacy evidence artifacts")
	}
	for storeDir, digest := range map[string]string{newStore: captured.Record.RecordDigest, legacyStore: legacy.RecordDigest} {
		loaded, err := ReadCapturedVerificationEvidence(storeDir, lineage, revision, snapshot)
		if err != nil || loaded.Record.RecordDigest != digest {
			t.Fatalf("evidence store read = %#v, %v; want record digest %s", loaded.Record, err, digest)
		}
	}
}
