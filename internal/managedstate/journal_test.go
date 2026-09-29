package managedstate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestJournalRoundTripPreservesRecoveryTruth(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "claude", "gentleman.md")
	j := &Journal{
		Schema:        JournalSchema,
		TransactionID: "tx-roundtrip",
		HomeDir:       home,
		FromSchema:    state.ManifestSchema,
		Resources: []Resource{{
			ID:      "persona/output-style/gentleman",
			Adapter: "claude",
			Target:  target,
			Extent:  state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged},
			Before:  DigestMode{Exists: true, SHA256: "before-hex", Mode: 0o644},
			Desired: DigestMode{Exists: true, SHA256: "desired-hex", Mode: 0o644},
		}},
		LastPhase: PhasePrepared,
	}
	if err := j.persist(home); err != nil {
		t.Fatalf("persist: %v", err)
	}

	path := filepath.Join(TransactionsDir(home), "tx-roundtrip", "journal.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("journal not at transactions/<id>/journal.json: %v", err)
	}

	got, err := LoadJournal(home, "tx-roundtrip")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Schema != JournalSchema || got.TransactionID != "tx-roundtrip" {
		t.Fatalf("identity lost: %+v", got)
	}
	if got.LastPhase != PhasePrepared {
		t.Fatalf("last phase lost: %q", got.LastPhase)
	}
	if len(got.Resources) != 1 || got.Resources[0].Before.SHA256 != "before-hex" {
		t.Fatalf("resources lost: %+v", got.Resources)
	}
	if got.Resources[0].Extent.Kind != state.ExtentFull || got.Resources[0].Extent.Ownership != state.OwnershipManaged {
		t.Fatalf("extent lost: %+v", got.Resources[0].Extent)
	}
}

func TestLoadActiveJournalNoneMultipleSingle(t *testing.T) {
	home := t.TempDir()

	if _, err := LoadActiveJournal(home); err != ErrNoActiveJournal {
		t.Fatalf("empty home must return ErrNoActiveJournal, got %v", err)
	}

	mk := func(id string) *Journal {
		return &Journal{Schema: JournalSchema, TransactionID: id, HomeDir: home, LastPhase: PhasePrepared}
	}
	for _, id := range []string{"tx-a", "tx-b"} {
		if err := mk(id).persist(home); err != nil {
			t.Fatalf("persist %s: %v", id, err)
		}
	}
	if _, err := LoadActiveJournal(home); err == nil || !strings.Contains(err.Error(), "two") {
		t.Fatalf("two active journals must never guess, got %v", err)
	}

	if err := os.RemoveAll(filepath.Join(TransactionsDir(home), "tx-b")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, err := LoadActiveJournal(home)
	if err != nil {
		t.Fatalf("single active journal must load: %v", err)
	}
	if got.TransactionID != "tx-a" {
		t.Fatalf("wrong journal: %s", got.TransactionID)
	}
}

func TestJournalRevisionsAreReReadableAfterPersist(t *testing.T) {
	home := t.TempDir()
	j := &Journal{Schema: JournalSchema, TransactionID: "tx-rev", HomeDir: home, LastPhase: PhaseDiscovered}
	for _, phase := range []Phase{PhaseDiscovered, PhaseClassified, PhaseSnapshotted, PhasePrepared, PhaseApplying} {
		j.LastPhase = phase
		if err := j.persist(home); err != nil {
			t.Fatalf("persist %s: %v", phase, err)
		}
	}
	got, err := LoadJournal(home, "tx-rev")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.LastPhase != PhaseApplying {
		t.Fatalf("latest revision must win: %q", got.LastPhase)
	}
}
