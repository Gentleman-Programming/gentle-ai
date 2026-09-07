package telemetrycollector

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // cgo-free sqlite driver, registers as "sqlite"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	received_at INTEGER NOT NULL, -- Unix nanoseconds, UTC (see receivedAtKey)
	event TEXT NOT NULL,
	install_id TEXT NOT NULL,
	version TEXT NOT NULL,
	os TEXT NOT NULL,
	arch TEXT NOT NULL,
	agents_json TEXT NOT NULL,
	components_json TEXT NOT NULL,
	rdd_enabled INTEGER NOT NULL,
	counters_json TEXT
);
CREATE INDEX IF NOT EXISTS idx_events_received_at ON events(received_at);
CREATE INDEX IF NOT EXISTS idx_events_install_id ON events(install_id);

CREATE TABLE IF NOT EXISTS rollups_daily (
	day TEXT NOT NULL,
	metric TEXT NOT NULL,
	key TEXT NOT NULL,
	value INTEGER NOT NULL,
	PRIMARY KEY (day, metric, key)
);
`

const dayLayout = "2006-01-02"

// Storage owns the SQLite database backing the collector: the append-only
// events table and the rollups_daily table filled by the daily rollup job.
// It never stores or exposes the remote address of a request; that value
// simply never reaches this type.
type Storage struct {
	db *sql.DB
}

// OpenStorage opens (creating if necessary) the SQLite database at path
// and applies the schema. All writes are serialized through a single
// connection: this collector's throughput does not justify the complexity
// of concurrent SQLite writers, and a single connection sidesteps
// SQLITE_BUSY entirely.
//
// journal_mode is DELETE, not WAL: WAL's main benefit is letting readers
// and a writer proceed concurrently, which this process cannot use anyway
// since SetMaxOpenConns(1) already serializes every read and write of its
// own onto one connection. DELETE mode also never creates -wal/-shm
// sidecar files, which keeps the read-only Grafana deployment (see
// deploy/telemetry/install.sh's install_grafana) down to granting access
// to one file instead of three.
func OpenStorage(path string) (*Storage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=DELETE;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Storage{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// InsertEvent appends one validated event, stamped with receivedAt (server
// time, never derived from the client-supplied sent_at). It never persists
// a remote address because Event carries none.
func (s *Storage) InsertEvent(ctx context.Context, ev Event, receivedAt time.Time) error {
	agentsJSON, err := json.Marshal(ev.Agents)
	if err != nil {
		return fmt.Errorf("marshal agents: %w", err)
	}
	componentsJSON, err := json.Marshal(ev.Components)
	if err != nil {
		return fmt.Errorf("marshal components: %w", err)
	}

	var countersJSON sql.NullString
	if ev.Counters != nil {
		raw, err := json.Marshal(ev.Counters)
		if err != nil {
			return fmt.Errorf("marshal counters: %w", err)
		}
		countersJSON = sql.NullString{String: string(raw), Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO events (received_at, event, install_id, version, os, arch, agents_json, components_json, rdd_enabled, counters_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		receivedAtKey(receivedAt),
		ev.Kind,
		ev.NormalizedInstallID(),
		ev.Version,
		ev.OS,
		ev.Arch,
		string(agentsJSON),
		string(componentsJSON),
		boolToInt(ev.RDDEnabled),
		countersJSON,
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

// storedEvent is one row of the events table, decoded back into Go types
// for rollup computation.
type storedEvent struct {
	ReceivedAt time.Time
	Kind       string
	InstallID  string
	Version    string
	OS         string
	Arch       string
	Agents     []string
	Components []string
	RDDEnabled bool
}

// eventsOnDay returns every event whose received_at falls within the given
// UTC calendar day.
func (s *Storage) eventsOnDay(ctx context.Context, day time.Time) ([]storedEvent, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	return s.eventsBetween(ctx, start, end)
}

func (s *Storage) eventsBetween(ctx context.Context, start, end time.Time) ([]storedEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT received_at, event, install_id, version, os, arch, agents_json, components_json, rdd_enabled
		FROM events
		WHERE received_at >= ? AND received_at < ?
		ORDER BY received_at ASC
	`, receivedAtKey(start), receivedAtKey(end))
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var out []storedEvent
	for rows.Next() {
		var (
			receivedAt int64
			rddEnabled int
			agentsJSON string
			compJSON   string
			ev         storedEvent
		)
		if err := rows.Scan(&receivedAt, &ev.Kind, &ev.InstallID, &ev.Version, &ev.OS, &ev.Arch, &agentsJSON, &compJSON, &rddEnabled); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		ev.ReceivedAt = parseReceivedAtKey(receivedAt)
		ev.RDDEnabled = rddEnabled != 0
		if err := json.Unmarshal([]byte(agentsJSON), &ev.Agents); err != nil {
			return nil, fmt.Errorf("unmarshal agents: %w", err)
		}
		if err := json.Unmarshal([]byte(compJSON), &ev.Components); err != nil {
			return nil, fmt.Errorf("unmarshal components: %w", err)
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// execer is satisfied by both *sql.DB and *sql.Tx, so upsertRollup and
// deleteRollupsForDay can run either standalone or, as RunDailyRollup does,
// inside one transaction shared with the delete that precedes them.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// upsertRollup writes or replaces one (day, metric, key) -> value row.
func upsertRollup(ctx context.Context, exec execer, day, metric, key string, value int64) error {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO rollups_daily (day, metric, key, value)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(day, metric, key) DO UPDATE SET value = excluded.value
	`, day, metric, key, value)
	if err != nil {
		return fmt.Errorf("upsert rollup %s/%s/%s: %w", day, metric, key, err)
	}
	return nil
}

// deleteRollupsForDay clears any previously computed telemetry rollup for
// one day, so re-running the daily job (e.g. after a crash) does not leave
// stale rows behind from partial data. It excludes downloadMetrics: those
// rows are written by FetchAndStoreDownloads, not by RunDailyRollup, and
// must survive a (re-)rollup of the same day.
func deleteRollupsForDay(ctx context.Context, exec execer, day string) error {
	_, err := exec.ExecContext(ctx, `
		DELETE FROM rollups_daily WHERE day = ? AND metric NOT IN (?, ?)
	`, day, metricNpmDownloadsDay, metricGithubReleaseDownloads)
	if err != nil {
		return fmt.Errorf("delete rollups for %s: %w", day, err)
	}
	return nil
}

// lastRolledDay returns the most recent day with a telemetry rollup row in
// rollups_daily, or ok=false if no day has ever been rolled up. It excludes
// downloadMetrics so an external download snapshot (which can land under
// today or yesterday before any telemetry rollup ever runs) never poisons
// the watermark RunMaintenance uses to resume its catch-up loop.
func (s *Storage) lastRolledDay(ctx context.Context) (day time.Time, ok bool, err error) {
	var maxDay sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT MAX(day) FROM rollups_daily WHERE metric NOT IN (?, ?)
	`, metricNpmDownloadsDay, metricGithubReleaseDownloads).Scan(&maxDay); err != nil {
		return time.Time{}, false, fmt.Errorf("query last rolled day: %w", err)
	}
	if !maxDay.Valid {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(dayLayout, maxDay.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse last rolled day %q: %w", maxDay.String, err)
	}
	return parsed, true, nil
}

// oldestEventDay returns the UTC calendar day of the oldest raw event, or
// ok=false if the events table is empty.
func (s *Storage) oldestEventDay(ctx context.Context) (day time.Time, ok bool, err error) {
	var minReceivedAt sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(received_at) FROM events`).Scan(&minReceivedAt); err != nil {
		return time.Time{}, false, fmt.Errorf("query oldest event: %w", err)
	}
	if !minReceivedAt.Valid {
		return time.Time{}, false, nil
	}
	return truncateToDay(parseReceivedAtKey(minReceivedAt.Int64)), true, nil
}

// rollupRows fetches metric rows for a set of days, used by the summary
// aggregation.
func (s *Storage) rollupRows(ctx context.Context, metric string, fromDay, toDay string) ([]rollupRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, key, value FROM rollups_daily
		WHERE metric = ? AND day >= ? AND day <= ?
	`, metric, fromDay, toDay)
	if err != nil {
		return nil, fmt.Errorf("query rollup rows for %s: %w", metric, err)
	}
	defer rows.Close()

	var out []rollupRow
	for rows.Next() {
		var r rollupRow
		if err := rows.Scan(&r.Day, &r.Key, &r.Value); err != nil {
			return nil, fmt.Errorf("scan rollup row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type rollupRow struct {
	Day   string
	Key   string
	Value int64
}

// PurgeOlderThan deletes raw events received before cutoff and reports how
// many rows were removed. Rollups are never purged by this call: they are
// the durable historical record once raw events age out.
func (s *Storage) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE received_at < ?`, receivedAtKey(cutoff))
	if err != nil {
		return 0, fmt.Errorf("purge events before %s: %w", cutoff, err)
	}
	return res.RowsAffected()
}

// receivedAtKey and parseReceivedAtKey convert between time.Time and the
// events.received_at column: Unix nanoseconds since the epoch, UTC. This
// replaces an earlier RFC3339Nano text encoding, whose width varies with
// the number of significant fractional-second digits (Go's formatter
// trims trailing zeros) — a fractional timestamp like
// "2026-01-02T00:00:00.5Z" sorts as a STRING before the whole-second
// "2026-01-02T00:00:00Z" that precedes it in wall-clock time, because '.'
// (0x2e) sorts below 'Z'/digits. That silently misplaced events across
// eventsOnDay/rollup/PurgeOlderThan's day-boundary comparisons. Integers
// compare correctly regardless of formatting, by construction.
func receivedAtKey(t time.Time) int64 {
	return t.UTC().UnixNano()
}

func parseReceivedAtKey(key int64) time.Time {
	return time.Unix(0, key).UTC()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
