package state

import (
	"os"
	"strings"
	"testing"
)

// TestManifestRoundTrip: manifest persists, identity is reproducible, resources carry all fields.
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

// TestBundleDigestExcludesObserved: observed updates must not change bundle identity.
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

// TestBundleDigestChangesWhenDesiredChanges: digest MUST change when desired changes.
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
}

// TestJournalAppendAndRead: journal records intent/complete for atomic-commit tracking.
func TestJournalAppendAndRead(t *testing.T) {
	home := t.TempDir()

	for _, op := range []string{OpIntent, OpComplete, OpComplete} {
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

// TestJournalCapTruncates: journal capped at 1 MiB; older entries replaced by one "truncated" marker.
func TestJournalCapTruncates(t *testing.T) {
	home := t.TempDir()
	for i := 0; i < 32; i++ {
		if err := AppendJournal(home, string(OpComplete), strings.Repeat("x", 64*1024), ""); err != nil {
			t.Fatalf("AppendJournal iteration %d: %v", i, err)
		}
	}

	info, _ := os.Stat(JournalPath(home))
	if info.Size() > journalCapBytes+512 {
		t.Errorf("journal size = %d, want at or below cap+512", info.Size())
	}

	entries, err := ReadJournal(home)
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	for _, e := range entries {
		if e.Op == string(OpTruncated) {
			return
		}
	}
	t.Errorf("expected a truncated marker entry after overflow")
}
