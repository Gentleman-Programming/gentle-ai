package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// The four properties in this file are one claim about scope: authority
// validation is per lineage, never per repository. Every worktree of a
// repository shares one Git common directory and therefore one review store,
// so a repository-global verdict over that store is a verdict over every
// worktree at once. One record nobody is operating on could refuse every
// operation anybody performed.
//
// These are removals, not additions. No state is introduced: a record that
// cannot be read is simply absent from the graph, and absence already has a
// meaning the graph knows (a successor naming it dangles). What goes away is
// the assumption that the inventory is a single atomic object which is either
// wholly trustworthy or wholly refused.

// damageCompactStateStructurally truncates a persisted record to half its
// bytes, which is what a write interrupted between open and close leaves. The
// result cannot be parsed at all, so it is a STRUCTURAL failure: never the
// semantic-validation class issue-1813's quarantine already admits.
func damageCompactStateStructurally(t *testing.T, store CompactStore) {
	t.Helper()
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.StatePath(), payload[:len(payload)/2], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("fixture claims an unreadable record but Load() succeeded")
	}
}

// writeHistoricalCompactStateField rewrites a persisted record the way a build
// that still knew the named field would have written it: the field is present
// in the state object AND the recorded revision is the hash of those exact
// bytes. That is the #2399 shape precisely — the record is internally
// consistent and self-verifying, and the only thing wrong with it is that the
// current schema no longer declares one of its fields.
//
// The revision preimage is reproduced here rather than imported from the
// production writer on purpose: what a historical record binds is the
// persisted marshalling, so a fixture that asked the current writer to produce
// it would prove nothing about a record the current writer never wrote.
func writeHistoricalCompactStateField(t *testing.T, store CompactStore, field, value string) []byte {
	t.Helper()
	payload, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Schema   string          `json:"schema"`
		Revision string          `json:"revision"`
		State    json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	marker := []byte(`"state": {` + "\n")
	index := bytes.Index(payload, marker)
	if index < 0 {
		t.Fatalf("fixture did not find the state object in %s", store.StatePath())
	}
	insertion := []byte(`    "` + field + `": "` + value + `",` + "\n")
	historical := make([]byte, 0, len(payload)+len(insertion))
	historical = append(historical, payload[:index+len(marker)]...)
	historical = append(historical, insertion...)
	historical = append(historical, payload[index+len(marker):]...)

	var reread struct {
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(historical, &reread); err != nil {
		t.Fatal(err)
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, reread.State); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(append([]byte(CompactStateSchema+"\x00"), compacted.Bytes()...))
	revision := "sha256:" + hex.EncodeToString(sum[:])
	historical = bytes.Replace(historical, []byte(`"revision": "`+envelope.Revision+`"`), []byte(`"revision": "`+revision+`"`), 1)
	if !bytes.Contains(historical, []byte(revision)) {
		t.Fatal("fixture could not restate the historical revision")
	}
	if err := os.WriteFile(store.StatePath(), historical, 0o644); err != nil {
		t.Fatal(err)
	}
	return historical
}

// TestUnreadableCompactRecordServesEveryUnrelatedLineage is property 1. One
// record nobody named must not remove every other lineage from enumeration.
func TestUnreadableCompactRecordServesEveryUnrelatedLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	healthyA := quarantineFixtureHealthyLineage(t, repo, "scoped-healthy-a", "healthy candidate a\n")
	damagedLineage := quarantineFixtureHealthyLineage(t, repo, "scoped-unreadable", "damaged candidate\n")
	healthyB := quarantineFixtureHealthyLineage(t, repo, "scoped-healthy-b", "healthy candidate b\n")

	damagedStore, err := CompactAuthoritativeStore(context.Background(), repo, damagedLineage)
	if err != nil {
		t.Fatal(err)
	}
	damageCompactStateStructurally(t, damagedStore)

	t.Run("leaf enumeration serves the healthy lineages", func(t *testing.T) {
		leaves, err := CompactAuthorityLeaves(context.Background(), repo)
		if err != nil {
			t.Fatalf("one unreadable record poisoned leaf enumeration: %v", err)
		}
		names := map[string]bool{}
		for _, leaf := range leaves {
			names[leaf.lineageID] = true
		}
		if !names[healthyA] || !names[healthyB] {
			t.Fatalf("healthy lineages excluded by an unrelated unreadable record: %#v", names)
		}
		if names[damagedLineage] {
			t.Fatalf("the unreadable lineage was enumerated as authority: %#v", names)
		}
	})

	t.Run("selector-free target status serves the healthy lineages", func(t *testing.T) {
		candidates, err := loadCompactTargetStatusCandidates(context.Background(), repo, "")
		if err != nil {
			t.Fatalf("one unreadable record poisoned selector-free target status: %v", err)
		}
		if _, ok := candidates[healthyA]; !ok {
			t.Fatalf("healthy lineage %q excluded: %#v", healthyA, candidates)
		}
		if _, ok := candidates[healthyB]; !ok {
			t.Fatalf("healthy lineage %q excluded: %#v", healthyB, candidates)
		}
		if _, ok := candidates[damagedLineage]; ok {
			t.Fatalf("the unreadable lineage was offered as a candidate: %#v", candidates)
		}
	})

	t.Run("the inventory stays authoritative for the healthy lineages", func(t *testing.T) {
		report, err := InventoryAuthority(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if !report.Complete || !report.Authoritative {
			t.Fatalf("one unreadable record made the whole repository non-authoritative: complete=%t authoritative=%t status=%s",
				report.Complete, report.Authoritative, report.Status)
		}
		if !hasAuthorityInventoryStatus(report.Entries, healthyA, AuthorityStatusApproved) ||
			!hasAuthorityInventoryStatus(report.Entries, healthyB, AuthorityStatusApproved) {
			t.Fatalf("healthy lineages lost their entries: %#v", report.Entries)
		}
	})
}

// TestUnreadableCompactRecordStillRefusesItselfAndNamesAnExit is property 2.
// Scoping the refusal must not make the damaged record loadable, and the
// refusal must name the entry and a runnable continuation.
func TestUnreadableCompactRecordStillRefusesItselfAndNamesAnExit(t *testing.T) {
	repo := initSnapshotRepo(t)
	quarantineFixtureHealthyLineage(t, repo, "refuses-healthy", "healthy candidate\n")
	damagedLineage := quarantineFixtureHealthyLineage(t, repo, "refuses-unreadable", "damaged candidate\n")

	damagedStore, err := CompactAuthoritativeStore(context.Background(), repo, damagedLineage)
	if err != nil {
		t.Fatal(err)
	}
	damageCompactStateStructurally(t, damagedStore)

	t.Run("an explicit selector on the damaged lineage fails closed", func(t *testing.T) {
		if _, err := loadCompactTargetStatusCandidates(context.Background(), repo, damagedLineage); err == nil {
			t.Fatal("an explicit selector on the unreadable lineage was admitted")
		}
	})

	t.Run("the inventory names the entry, the reason, and a runnable exit", func(t *testing.T) {
		report, err := InventoryAuthority(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		named := false
		for _, entry := range report.Entries {
			if entry.LineageID != damagedLineage {
				continue
			}
			named = true
			if entry.Status != AuthorityStatusInvalid {
				t.Fatalf("the unreadable entry is not reported invalid: %#v", entry)
			}
			if len(entry.Problems) == 0 || strings.TrimSpace(entry.Problems[0]) == "" {
				t.Fatalf("the unreadable entry names no reason: %#v", entry)
			}
			if !anyProblemNamesAnExit(entry.Problems) {
				t.Fatalf("the unreadable entry names no runnable exit: %#v", entry.Problems)
			}
		}
		if !named {
			t.Fatalf("the unreadable entry is not named at all: %#v", report.Entries)
		}
	})
}

func anyProblemNamesAnExit(problems []string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, "gentle-ai review ") {
			return true
		}
	}
	return false
}

// TestGraphViolationIsScopedToItsOwnLineage is the second removal: a violation
// carried by one edge must not be a verdict on lineages that edge does not
// reach. The forged-authorization pair is the exact shape #1892/#2014 report.
func TestGraphViolationIsScopedToItsOwnLineage(t *testing.T) {
	repo := initSnapshotRepo(t)
	unrelated := quarantineFixtureHealthyLineage(t, repo, "scoped-graph-unrelated", "unrelated candidate\n")
	_, successor, _ := forgedRecoveryPair(t, repo, "scoped-graph", "forged scoped target\n")

	leaves, err := CompactAuthorityLeaves(context.Background(), repo)
	if err != nil {
		t.Fatalf("one forged recovery edge poisoned leaf enumeration: %v", err)
	}
	names := map[string]bool{}
	for _, leaf := range leaves {
		names[leaf.lineageID] = true
	}
	if !names[unrelated] {
		t.Fatalf("the unrelated healthy lineage was excluded by a forged edge it has no relation to: %#v", names)
	}
	if names[successor.State.LineageID] {
		t.Fatalf("the forged successor was enumerated as authority: %#v", names)
	}

	blocked := CompactAuthorityLineageBlocked(context.Background(), repo, successor.State.LineageID)
	if blocked == nil {
		t.Fatal("the forged successor reports no lineage-scoped block")
	}
	if !strings.Contains(blocked.Error(), successor.State.LineageID) {
		t.Fatalf("the lineage-scoped block does not name the entry: %v", blocked)
	}
	if err := CompactAuthorityLineageBlocked(context.Background(), repo, unrelated); err != nil {
		t.Fatalf("the unrelated lineage reports a block it does not carry: %v", err)
	}
}

// TestHistoricalUnknownFieldLoadsReadOnlyWithoutRewriting is property 4 and
// the #2399 shape: a historical top-level field whose record still verifies
// against its original bytes loads read-only, and reading it changes nothing
// on disk.
func TestHistoricalUnknownFieldLoadsReadOnlyWithoutRewriting(t *testing.T) {
	repo := initSnapshotRepo(t)
	lineage := quarantineFixtureHealthyLineage(t, repo, "historical-risk-source", "historical candidate\n")
	store, err := CompactAuthoritativeStore(context.Background(), repo, lineage)
	if err != nil {
		t.Fatal(err)
	}
	historical := writeHistoricalCompactStateField(t, store, "risk_source", "automatic")
	recordedRevision := recordedCompactRevision(t, historical)

	record, err := store.Load()
	if err != nil {
		t.Fatalf("a historical record whose revision still binds its own bytes was refused: %v", err)
	}
	if !record.HistoricalCompat {
		t.Fatalf("the historical record did not load through the read-only compatibility path: %#v", record)
	}
	if record.Revision != recordedRevision {
		t.Fatalf("the historical record's revision moved: %q != %q", record.Revision, recordedRevision)
	}

	current, err := os.ReadFile(store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, historical) {
		t.Fatal("reading a historical record rewrote its persisted bytes")
	}
	if _, err := store.Replace(record.Revision, "review/start", record.State); !errors.Is(err, ErrHistoricalCompatReadOnly) {
		t.Fatalf("historical authority accepted an ordinary mutation: %v", err)
	}

	// An unrelated unknown field is still refused: the allowlist is narrow.
	other := quarantineFixtureHealthyLineage(t, repo, "historical-unknown-other", "other candidate\n")
	otherStore, err := CompactAuthoritativeStore(context.Background(), repo, other)
	if err != nil {
		t.Fatal(err)
	}
	writeHistoricalCompactStateField(t, otherStore, "not_a_historical_field", "whatever")
	if _, err := otherStore.Load(); err == nil {
		t.Fatal("an unrelated unknown field was tolerated")
	}
}

func recordedCompactRevision(t *testing.T, payload []byte) string {
	t.Helper()
	var envelope struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Revision == "" {
		t.Fatal("fixture record carries no revision")
	}
	return envelope.Revision
}
