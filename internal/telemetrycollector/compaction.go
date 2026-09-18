package telemetrycollector

import (
	"context"
	"fmt"
)

// compactFreePercent is the share of free pages, in percent of the whole
// file, above which Compact rewrites the database with VACUUM. Below it the
// free pages are simply reused by new rows. The production cutover to
// --runtime-store=metrics left 91% of a 1.4 GB file free; day to day, a
// purge frees a few percent at most.
const compactFreePercent = 25

// compactMinPages is the file size, in pages, below which VACUUM is never
// worth its rewrite. A var so a test can lower it and exercise the rule on
// a small fixture.
var compactMinPages int64 = 1024

// compactMaxLivePages caps how much live data an unattended online VACUUM
// may rewrite (131,072 pages of 4 KiB is 512 MiB, seconds of rewrite).
// Above it the writer would be held long enough to lose deliveries, so
// Compact defers and the operator vacuums offline. A var for tests.
var compactMaxLivePages int64 = 131072

// CompactionReport says what Compact did, for the maintenance log line.
type CompactionReport struct {
	// PageCount and FreePages describe the file before compaction.
	PageCount, FreePages int64
	// WALFrames is how many frames the WAL held before it was checkpointed
	// and truncated (SQLite's own count, 0 when it was already empty).
	WALFrames int64
	// Busy is true when a reader outside this process (Grafana, the open-data
	// export) kept the checkpoint from completing; the WAL keeps its frames
	// until the next run.
	Busy bool
	// Vacuumed is true when the free share crossed compactFreePercent and
	// the file was rewritten.
	Vacuumed bool
	// VacuumDeferred is true when the free share qualified but the live
	// data exceeds compactMaxLivePages, so the rewrite was left to an
	// offline operator step.
	VacuumDeferred bool
}

// Compact returns disk to the operating system after a purge: it always
// checkpoints the WAL (and truncates it when no outside reader holds it,
// since the collector's steady write stream never lets SQLite's automatic
// checkpoint run), and runs VACUUM only when at least compactFreePercent of
// a file of at least compactMinPages pages is free AND the live data fits
// under compactMaxLivePages; otherwise the rewrite is reported as deferred
// for an offline operator step (see docs/telemetry-collector.md).
func (s *Storage) Compact(ctx context.Context) (CompactionReport, error) {
	var report CompactionReport
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&report.PageCount); err != nil {
		return report, fmt.Errorf("read page_count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA freelist_count`).Scan(&report.FreePages); err != nil {
		return report, fmt.Errorf("read freelist_count: %w", err)
	}

	busy, frames, err := s.checkpointTruncate(ctx)
	if err != nil {
		return report, err
	}
	report.Busy, report.WALFrames = busy, frames

	if report.PageCount >= compactMinPages && report.FreePages*100/report.PageCount >= compactFreePercent {
		if report.PageCount-report.FreePages > compactMaxLivePages {
			report.VacuumDeferred = true
			return report, nil
		}
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return report, fmt.Errorf("vacuum: %w", err)
		}
		report.Vacuumed = true
		// VACUUM writes the rewritten file through the WAL; truncate it
		// again so the sidecar does not keep a full copy of the database.
		if busy, _, err := s.checkpointTruncate(ctx); err != nil {
			return report, err
		} else if busy {
			report.Busy = true
		}
	}
	return report, nil
}

// checkpointTruncate backfills the WAL with a PASSIVE checkpoint, which
// never waits on a reader, and only when every frame was backfilled runs
// the TRUNCATE that resets the sidecar. A bare TRUNCATE would block for up
// to busy_timeout while the open-data export or Grafana holds a read
// transaction, with the maintenance goroutine holding the pool's single
// connection and stalling ingest meanwhile.
func (s *Storage) checkpointTruncate(ctx context.Context) (busy bool, frames int64, err error) {
	var busyFlag, checkpointed int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`).Scan(&busyFlag, &frames, &checkpointed); err != nil {
		return false, 0, fmt.Errorf("wal_checkpoint(PASSIVE): %w", err)
	}
	if frames < 0 { // not in WAL mode; never the case after OpenStorage
		return false, 0, nil
	}
	if busyFlag != 0 || checkpointed < frames {
		return true, frames, nil
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return false, frames, fmt.Errorf("wal_checkpoint(TRUNCATE): %w", err)
	}
	return false, frames, nil
}
