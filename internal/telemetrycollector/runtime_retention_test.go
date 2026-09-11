package telemetrycollector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func insertRetentionDelivery(t *testing.T, s *Storage, id int, received time.Time) telemetry.RuntimeEvent {
	t.Helper()
	ev, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	ev.DeliveryID = fmt.Sprintf("%032x", id)
	// More than one observation row proves whole deliveries are removed.
	ev.Rows = append(ev.Rows, ev.Rows[0])
	if decision, err := s.InsertRuntimeEvent(context.Background(), ev, received); err != nil || decision != "stored" {
		t.Fatalf("insert: %q %v", decision, err)
	}
	return ev
}

func assertRetentionCounts(t *testing.T, s *Storage, legacy, deliveries, rows int) {
	t.Helper()
	var gotLegacy, gotDeliveries, gotRows, orphans int
	err := s.db.QueryRow(`SELECT (SELECT count(*) FROM events),
	 (SELECT count(*) FROM runtime_deliveries), (SELECT count(*) FROM runtime_rows),
	 (SELECT count(*) FROM runtime_rows AS r LEFT JOIN runtime_deliveries AS d
	  ON r.delivery_id=d.delivery_id WHERE d.delivery_id IS NULL)`).Scan(&gotLegacy, &gotDeliveries, &gotRows, &orphans)
	if err != nil {
		t.Fatal(err)
	}
	if gotLegacy != legacy || gotDeliveries != deliveries || gotRows != rows || orphans != 0 {
		t.Fatalf("legacy/deliveries/rows/orphans = %d/%d/%d/%d, want %d/%d/%d/0", gotLegacy, gotDeliveries, gotRows, orphans, legacy, deliveries, rows)
	}
}

func TestRuntimePurgeCutoffAndDedupeExpiry(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	old := insertRetentionDelivery(t, s, 1, cutoff.Add(-time.Nanosecond))
	insertRetentionDelivery(t, s, 2, cutoff)
	insertRetentionDelivery(t, s, 3, cutoff.Add(time.Nanosecond))
	for _, received := range []time.Time{cutoff.Add(-time.Nanosecond), cutoff} {
		if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), received); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`INSERT INTO rollups_daily VALUES ('2026-06-01','retention-test','preserved',17)`); err != nil {
		t.Fatal(err)
	}
	// A retry today must not extend the original retention deadline.
	if decision, err := s.InsertRuntimeEvent(ctx, old, cutoff.Add(time.Hour)); err != nil || decision != "duplicate" {
		t.Fatalf("retry: %q %v", decision, err)
	}
	purged, err := s.PurgeOlderThan(ctx, cutoff.In(time.FixedZone("west", -7*60*60)))
	if err != nil || purged != 1 {
		t.Fatalf("legacy-only purge count: %d %v", purged, err)
	}
	assertRetentionCounts(t, s, 1, 2, 4)
	var value, version int
	if err := s.db.QueryRow(`SELECT value FROM rollups_daily WHERE metric='retention-test'`).Scan(&value); err != nil || value != 17 {
		t.Fatalf("rollup changed: %d %v", value, err)
	}
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
		t.Fatalf("schema changed: %d %v", version, err)
	}
	if count, err := s.PurgeOlderThan(ctx, cutoff); err != nil || count != 0 {
		t.Fatalf("repeat purge: %d %v", count, err)
	}
	// No infinite tombstones: an expired identity is no longer recognized.
	if decision, err := s.InsertRuntimeEvent(ctx, old, cutoff.Add(time.Hour)); err != nil || decision != "stored" {
		t.Fatalf("expired retry: %q %v", decision, err)
	}
	assertRetentionCounts(t, s, 1, 3, 6)
}

func TestRuntimePurgeRollback(t *testing.T) {
	for _, stage := range []string{"legacy", "child", "parent", "commit"} {
		t.Run(stage, func(t *testing.T) {
			s := openTestStorage(t)
			ctx := context.Background()
			cutoff := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
			insertRetentionDelivery(t, s, 1, cutoff.Add(-time.Hour))
			insertRetentionDelivery(t, s, 2, cutoff)
			if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), cutoff.Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			var setup string
			switch stage {
			case "legacy":
				setup = `CREATE TRIGGER fail_purge BEFORE DELETE ON events BEGIN SELECT RAISE(ABORT,'test failure'); END`
			case "child":
				setup = `CREATE TRIGGER fail_purge BEFORE DELETE ON runtime_rows BEGIN SELECT RAISE(ABORT,'test failure'); END`
			case "parent":
				setup = `CREATE TRIGGER fail_purge BEFORE DELETE ON runtime_deliveries BEGIN SELECT RAISE(ABORT,'test failure'); END`
			case "commit":
				setup = `CREATE TABLE retention_probe (id TEXT REFERENCES runtime_deliveries(delivery_id) DEFERRABLE INITIALLY DEFERRED);
				 CREATE TRIGGER fail_purge AFTER DELETE ON runtime_deliveries BEGIN INSERT INTO retention_probe VALUES ('missing'); END`
			}
			if _, err := s.db.Exec(setup); err != nil {
				t.Fatal(err)
			}
			if count, err := s.PurgeOlderThan(ctx, cutoff); err == nil || count != 0 {
				t.Fatalf("failed purge reported committed count: %d %v", count, err)
			}
			assertRetentionCounts(t, s, 1, 2, 4)
		})
	}
}

func TestRuntimeMaintenanceWithoutLegacyEvents(t *testing.T) {
	// Existing policy performs date arithmetic without a special zero/negative
	// sentinel. Preserve it rather than introducing runtime-only validation.
	for _, tc := range []struct{ days, cutoffDay int }{{2, 12}, {0, 14}, {-1, 15}} {
		t.Run(fmt.Sprintf("days=%d", tc.days), func(t *testing.T) {
			s := openTestStorage(t)
			now := time.Date(2026, 6, 15, 1, 30, 0, 0, time.FixedZone("east", 2*60*60))
			cutoff := time.Date(2026, 6, tc.cutoffDay, 0, 0, 0, 0, time.UTC)
			insertRetentionDelivery(t, s, 1, cutoff.Add(-time.Nanosecond))
			insertRetentionDelivery(t, s, 2, cutoff)
			if err := RunMaintenance(context.Background(), s, now, tc.days); err != nil {
				t.Fatal(err)
			}
			assertRetentionCounts(t, s, 0, 1, 2)
		})
	}
}

func TestRuntimeMaintenanceRollupFailureCanRetry(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -3)
	insertRetentionDelivery(t, s, 1, old)
	if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), old); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TABLE maintenance_gate (enabled INTEGER);
	 INSERT INTO maintenance_gate VALUES (1);
	 CREATE TRIGGER fail_rollup BEFORE INSERT ON rollups_daily
	 WHEN (SELECT enabled FROM maintenance_gate)=1 BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := RunMaintenance(ctx, s, now, 2); err == nil {
		t.Fatal("rollup failure was ignored")
	}
	assertRetentionCounts(t, s, 1, 1, 2)
	if _, err := s.db.Exec(`UPDATE maintenance_gate SET enabled=0`); err != nil {
		t.Fatal(err)
	}
	if err := RunMaintenance(ctx, s, now, 2); err != nil {
		t.Fatal(err)
	}
	assertRetentionCounts(t, s, 0, 0, 0)
}

func TestRuntimeMaintenanceCancellationCanRetry(t *testing.T) {
	s := openTestStorage(t)
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	insertRetentionDelivery(t, s, 1, now.AddDate(0, 0, -3))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RunMaintenance(ctx, s, now, 2); err == nil {
		t.Fatal("cancelled runtime-only maintenance succeeded")
	}
	assertRetentionCounts(t, s, 0, 1, 2)
	if err := RunMaintenance(context.Background(), s, now, 2); err != nil {
		t.Fatal(err)
	}
	assertRetentionCounts(t, s, 0, 0, 0)
}
