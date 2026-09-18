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
}

// Compact returns disk to the operating system after a purge. It always
// checkpoints and truncates the WAL, so the sidecar stops growing between
// maintenance runs (the collector holds the only writer and a checkpoint
// only ever happens when SQLite's automatic one gets a chance, which a
// steady stream of writes never gives it). It runs VACUUM only when at
// least compactFreePercent of a file of at least compactMinPages pages is
// free: VACUUM rewrites the whole file and holds the writer for the
// duration, which is seconds on a compact file and minutes on a bloated
// one, so the very first compaction of a bloated production file is done
// offline by the operator, not here (see docs/telemetry-collector.md).
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
		// VACUUM cannot run inside a transaction; the pool's single
		// connection has none open here.
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

// checkpointTruncate runs PRAGMA wal_checkpoint(TRUNCATE) and reports
// whether it was blocked by a reader and how many frames the WAL held.
func (s *Storage) checkpointTruncate(ctx context.Context) (busy bool, frames int64, err error) {
	var busyFlag, checkpointed int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busyFlag, &frames, &checkpointed); err != nil {
		return false, 0, fmt.Errorf("wal_checkpoint(TRUNCATE): %w", err)
	}
	if frames < 0 {
		// -1 means the database is not in WAL mode (never the case after
		// OpenStorage, but the pragma defines it).
		frames = 0
	}
	return busyFlag != 0, frames, nil
}
