package state

import (
	"os"
	"strings"
	"testing"
)

// TestManifestRoundTrip covers AC1 (manifest persisted), AC2 (identity
// reproducible), AC3 (resources carry stable ID, owned_extent, desired and
// observed digests).
func TestManifestRoundTrip(t *testing.T) {
	home := t.TempDir()
	original := Manifest{
		Schema:   ManifestSchema,
		Producer: Producer{BinaryVersion: "2.2.0", Commit: "abc1234"},
		Resources: []ManifestResource{{
			ID: "agents/opencode/AGENTS.md", Adapter: "full-file",
			Target:      home + "/opencode/AGENTS.md",
			OwnedExtent: OwnedExtent{Kind: ExtentFull, Ownership: OwnershipManaged},
			Desired:     "sha256:desired", Observed: "sha256:observed",
		}},
	}.WithBundleDigest()

	if err := WriteManifestAtomic(home, original); err != nil {
		t.Fatalf("WriteManifestAtomic: %v", err)
	}
	got, err := ReadManifest(home)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if got.Schema != original.Schema || got.Producer != original.Producer || len(got.Resources) != 1 {
		t.Fatalf("round-trip mismatch:\n got %+v\n want %+v", got, original)
	}
	r := got.Resources[0]
	w := original.Resources[0]
	if r.ID != w.ID || r.Adapter != w.Adapter || r.Target != w.Target || r.OwnedExtent != w.OwnedExtent || r.Desired != w.Desired || r.Observed != w.Observed {
		t.Errorf("resource identity mismatch:\n got %+v\n want %+v", r, w)
	}
}

// TestBundleDigestExcludesObserved covers AC2 — observed digest updates must
// not change the bundle identity (only desired is part of the canonical identity).
func TestBundleDigestExcludesObserved(t *testing.T) {
	base := Manifest{
		Schema: ManifestSchema, Producer: Producer{BinaryVersion: "2.2.0"},
		Resources: []ManifestResource{{
			ID: "x", Adapter: "full-file", Target: "/tmp",
			OwnedExtent: OwnedExtent{Kind: ExtentFull, Ownership: OwnershipManaged},
			Desired:     "sha256:desired", Observed: "sha256:observed-first",
		}},
	}
	first := base.WithBundleDigest()
	base.Resources[0].Observed = "sha256:observed-second"
	second := base.WithBundleDigest()

	if first.Bundle.Digest != second.Bundle.Digest {
		t.Errorf("bundle digest changed when only observed changed: %q vs %q", first.Bundle.Digest, second.Bundle.Digest)
	}
	if first.Bundle.Algo != "sha256" || first.Bundle.Digest == "" {
		t.Errorf("bundle metadata wrong: %+v", first.Bundle)
	}
}

// TestBundleDigestChangesWhenDesiredChanges covers AC2 — bundle digest MUST
// change when owned_extent or desired changes.
func TestBundleDigestChangesWhenDesiredChanges(t *testing.T) {
	base := Manifest{
		Schema: ManifestSchema, Producer: Producer{BinaryVersion: "2.2.0"},
		Resources: []ManifestResource{{
			ID: "x", Adapter: "full-file", Target: "/tmp",
			OwnedExtent: OwnedExtent{Kind: ExtentFull, Ownership: OwnershipManaged},
			Desired:     "sha256:desired-A",
		}},
	}
	first := base.WithBundleDigest()

	base.Resources[0].Desired = "sha256:desired-B"
	if first.Bundle.Digest == base.WithBundleDigest().Bundle.Digest {
		t.Error("bundle digest unchanged after desired changed")
	}

	base.Resources[0].OwnedExtent.Ownership = OwnershipUser
	if first.Bundle.Digest == base.WithBundleDigest().Bundle.Digest {
		t.Error("bundle digest unchanged after ownership changed")
	}
}

// TestJournalAppendAndRead covers AC8 — co-located journal records
// intent/complete entries for atomic-commit tracking.
func TestJournalAppendAndRead(t *testing.T) {
	home := t.TempDir()

	for _, op := range []JournalOp{OpIntent, OpComplete, OpComplete} {
		var resource string
		if op == OpIntent {
			resource = "agents/opencode/AGENTS.md"
		}
		if err := AppendJournal(home, string(op), "run-1", resource); err != nil {
			t.Fatalf("AppendJournal(%s): %v", op, err)
		}
	}

	entries, err := ReadJournal(home)
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3; got %+v", len(entries), entries)
	}
	wantOps := []string{string(OpIntent), string(OpComplete), string(OpComplete)}
	for i, want := range wantOps {
		if entries[i].Op != want || entries[i].RunID != "run-1" {
			t.Errorf("entries[%d] = %+v, want op=%s run_id=run-1", i, entries[i], want)
		}
	}
	if entries[0].Resource != "agents/opencode/AGENTS.md" {
		t.Errorf("entries[0].Resource = %q, want resource id", entries[0].Resource)
	}
}

// TestJournalCapTruncates covers AC8 — journal capped at 1 MiB with a
// single "truncated" marker replacing older entries.
func TestJournalCapTruncates(t *testing.T) {
	home := t.TempDir()
	bigBody := strings.Repeat("x", 64*1024)
	for i := 0; i < 32; i++ {
		if err := AppendJournal(home, string(OpComplete), bigBody, ""); err != nil {
			t.Fatalf("AppendJournal iteration %d: %v", i, err)
		}
	}

	info, err := os.Stat(JournalPath(home))
	if err != nil {
		t.Fatalf("stat journal: %v", err)
	}
	if info.Size() > journalCapBytes+512 {
		t.Errorf("journal size = %d, want at or below cap+%d slack", info.Size(), 512)
	}

	entries, _ := ReadJournal(home)
	for _, e := range entries {
		if e.Op == string(OpTruncated) {
			return
		}
	}
	t.Errorf("expected a truncated marker entry after overflow; got %d entries", len(entries))
}
