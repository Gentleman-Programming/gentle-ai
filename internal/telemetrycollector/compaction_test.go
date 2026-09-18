package telemetrycollector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openCompactionStorage(t *testing.T) (*Storage, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	s, err := OpenStorage(path)
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func pragmaInt(t *testing.T, s *Storage, pragma string) int64 {
	t.Helper()
	var n int64
	if err := s.db.QueryRow("PRAGMA " + pragma).Scan(&n); err != nil {
		t.Fatalf("PRAGMA %s: %v", pragma, err)
	}
	return n
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return info.Size()
}

// After the daily purge removes most of the file, Compact must hand the space
// back: the WAL sidecar is truncated to nothing and the free pages are
// reclaimed by a VACUUM, which only runs when the free share is large enough
// to matter (the threshold is lowered here so a small fixture crosses it).
func TestCompact_TruncatesWALAndVacuumsAfterLargePurge(t *testing.T) {
	s, path := openCompactionStorage(t)
	ctx := context.Background()
	previous := compactMinPages
	compactMinPages = 8
	t.Cleanup(func() { compactMinPages = previous })

	cutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3000; i++ {
		insertDeliveryIDAt(t, s, i, cutoff.Add(-time.Duration(i+1)*time.Second))
	}
	if _, err := s.PurgeOlderThan(ctx, cutoff, cutoff); err != nil {
		t.Fatal(err)
	}
	pagesBefore := pragmaInt(t, s, "page_count")
	if free := pragmaInt(t, s, "freelist_count"); free*100/pagesBefore < compactFreePercent {
		t.Fatalf("fixture too small to exercise the vacuum rule: %d free of %d pages", free, pagesBefore)
	}
	if fileSize(t, path+"-wal") == 0 {
		t.Fatal("fixture did not leave a WAL behind")
	}

	report, err := s.Compact(ctx)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if !report.Vacuumed {
		t.Fatalf("report = %+v, want Vacuumed", report)
	}
	if report.Busy {
		t.Fatalf("report = %+v, want a checkpoint that was not blocked", report)
	}
	if free := pragmaInt(t, s, "freelist_count"); free != 0 {
		t.Fatalf("freelist_count after Compact = %d, want 0", free)
	}
	if pagesAfter := pragmaInt(t, s, "page_count"); pagesAfter >= pagesBefore {
		t.Fatalf("page_count after Compact = %d, want fewer than %d", pagesAfter, pagesBefore)
	}
	if size := fileSize(t, path+"-wal"); size != 0 {
		t.Fatalf("WAL size after Compact = %d, want 0", size)
	}
	// The database is still usable through the same connection afterwards.
	insertDeliveryIDAt(t, s, 90000, cutoff)
}

// A database that is small, or dense (few free pages), is checkpointed but
// never rewritten: VACUUM is the expensive step and must stay rare.
func TestCompact_DenseOrSmallDatabaseIsNotVacuumed(t *testing.T) {
	s, path := openCompactionStorage(t)
	ctx := context.Background()

	for i := 0; i < 200; i++ {
		insertDeliveryIDAt(t, s, i, time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC))
	}
	report, err := s.Compact(ctx)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if report.Vacuumed {
		t.Fatalf("report = %+v, want no VACUUM on a dense database", report)
	}
	if size := fileSize(t, path+"-wal"); size != 0 {
		t.Fatalf("WAL size after Compact = %d, want 0 (checkpoint still truncates)", size)
	}
	if report.PageCount == 0 {
		t.Fatalf("report = %+v, want the page count filled in", report)
	}
}
