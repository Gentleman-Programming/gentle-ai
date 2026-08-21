package devjournal

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestSaveThenLoadRoundTrip proves a dispatch outcome and resume cursor
// survive a reload simulating a process restart: a brand-new Store opened
// over the same path must see exactly what a prior Store wrote.
func TestSaveThenLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root, "example-change")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	initial, err := store.Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}
	if len(initial.Record.Dispatches) != 0 {
		t.Fatalf("initial Load on a missing journal returned dispatches %+v, want none", initial.Record.Dispatches)
	}

	want := Record{
		Schema:       SchemaV1,
		Change:       "example-change",
		UpdatedAt:    time.Date(2026, 8, 21, 10, 0, 0, 0, time.UTC),
		StatusDigest: "sha256:deadbeefcafef00d",
		Cursor:       Cursor{BatchIndex: 2, RepoSlug: "repo-b"},
		Dispatches: []Dispatch{{
			RepoSlug:   "repo-a",
			Agent:      "backend-implementer",
			Attempt:    1,
			Outcome:    OutcomeDispatched,
			StartedAt:  time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC),
			FinishedAt: time.Date(2026, 8, 21, 9, 1, 0, 0, time.UTC),
		}},
	}
	if err := store.Save(want, initial.Revision); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate a process restart: a fresh Store value, not the one that wrote.
	restarted, err := Open(root, "example-change")
	if err != nil {
		t.Fatalf("Open after restart: %v", err)
	}
	got, err := restarted.Load()
	if err != nil {
		t.Fatalf("Load after restart: %v", err)
	}

	if got.Record.Cursor != want.Cursor {
		t.Fatalf("Cursor after restart = %+v, want %+v", got.Record.Cursor, want.Cursor)
	}
	if len(got.Record.Dispatches) != 1 || got.Record.Dispatches[0] != want.Dispatches[0] {
		t.Fatalf("Dispatches after restart = %+v, want %+v", got.Record.Dispatches, want.Dispatches)
	}
	if got.Record.Change != want.Change || got.Record.StatusDigest != want.StatusDigest {
		t.Fatalf("Record after restart = %+v, want Change/StatusDigest matching %+v", got.Record, want)
	}
	if got.Revision == initial.Revision {
		t.Fatalf("Revision after Save equals the pre-write revision; a real write must change it")
	}

	want2 := filepath.Join(root, ".gentle-ai", "dev-orchestrator", "v1", "example-change", "journal.json")
	if store.Path() != want2 {
		t.Fatalf("Path() = %q, want %q", store.Path(), want2)
	}
}

// TestSaveRejectsStaleRevision proves optimistic concurrency: Save with a
// stale revision is refused with ErrJournalRevisionConflict and the file on
// disk is left exactly as the last successful Save wrote it.
func TestSaveRejectsStaleRevision(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root, "example-change")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	initial, err := store.Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	first := Record{Schema: SchemaV1, Change: "example-change", Cursor: Cursor{BatchIndex: 0, RepoSlug: "repo-a"}}
	if err := store.Save(first, initial.Revision); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	afterFirst, err := store.Load()
	if err != nil {
		t.Fatalf("Load after first Save: %v", err)
	}

	// Reuse the now-stale initial revision for a second, conflicting Save.
	second := Record{Schema: SchemaV1, Change: "example-change", Cursor: Cursor{BatchIndex: 1, RepoSlug: "repo-b"}}
	if err := store.Save(second, initial.Revision); !errors.Is(err, ErrJournalRevisionConflict) {
		t.Fatalf("Save with stale revision error = %v, want ErrJournalRevisionConflict", err)
	}

	stillCurrent, err := store.Load()
	if err != nil {
		t.Fatalf("Load after rejected Save: %v", err)
	}
	if stillCurrent.Revision != afterFirst.Revision {
		t.Fatalf("file changed after a rejected Save: revision now %q, want unchanged %q", stillCurrent.Revision, afterFirst.Revision)
	}
	if stillCurrent.Record.Cursor != first.Cursor {
		t.Fatalf("file content changed after a rejected Save: cursor = %+v, want unchanged %+v", stillCurrent.Record.Cursor, first.Cursor)
	}
}
