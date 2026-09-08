package telemetrycollector

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTestStorage(t *testing.T) *Storage {
	t.Helper()
	path := filepath.Join(t.TempDir(), "events.sqlite")
	s, err := OpenStorage(path)
	if err != nil {
		t.Fatalf("OpenStorage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustParse(t *testing.T, body string) Event {
	t.Helper()
	ev, err := ParseEvent([]byte(body))
	if err != nil {
		t.Fatalf("ParseEvent: %v", err)
	}
	return ev
}

func TestStorage_InsertAndRetrieveEvent(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	ev := mustParse(t, validHeartbeatEvent)
	receivedAt := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if err := s.InsertEvent(ctx, ev, receivedAt); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	events, err := s.eventsOnDay(ctx, receivedAt)
	if err != nil {
		t.Fatalf("eventsOnDay: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("eventsOnDay: got %d events, want 1", len(events))
	}
	got := events[0]
	if got.InstallID != ev.NormalizedInstallID() {
		t.Errorf("InstallID = %q, want %q", got.InstallID, ev.NormalizedInstallID())
	}
	if got.Kind != EventHeartbeat {
		t.Errorf("Kind = %q, want %q", got.Kind, EventHeartbeat)
	}
	if len(got.Agents) != 2 || got.Agents[0] != "claude-code" {
		t.Errorf("Agents = %v", got.Agents)
	}
}

func TestStorage_SchemaHasNoRemoteAddressColumn(t *testing.T) {
	s := openTestStorage(t)

	rows, err := s.db.Query(`PRAGMA table_info(events)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	suspicious := []string{"ip", "addr", "remote", "host"}
	for rows.Next() {
		var (
			cid        int
			name       string
			colType    string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		lower := strings.ToLower(name)
		for _, bad := range suspicious {
			if strings.Contains(lower, bad) {
				t.Errorf("events table has a column named %q, which looks like it could hold a remote address", name)
			}
		}
	}
}

func TestStorage_InsertEventNeverPersistsAnIPShapedValue(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	ev := mustParse(t, validInstallEvent)
	receivedAt := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	if err := s.InsertEvent(ctx, ev, receivedAt); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	row := s.db.QueryRow(`SELECT received_at, event, install_id, version, os, arch, agents_json, components_json, rdd_enabled, coalesce(counters_json, '') FROM events`)
	var cols [10]string
	var rdd int
	if err := row.Scan(&cols[0], &cols[1], &cols[2], &cols[3], &cols[4], &cols[5], &cols[6], &cols[7], &rdd, &cols[9]); err != nil {
		t.Fatalf("scan row: %v", err)
	}
	const spoofedIP = "203.0.113.42"
	for i, v := range cols {
		if strings.Contains(v, spoofedIP) {
			t.Errorf("column %d unexpectedly contains an IP-shaped value: %q", i, v)
		}
	}
}

func TestStorage_PurgeOlderThanRetainsRecentRows(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), old); err != nil {
		t.Fatalf("InsertEvent(old): %v", err)
	}
	if err := s.InsertEvent(ctx, mustParse(t, validHeartbeatEvent), recent); err != nil {
		t.Fatalf("InsertEvent(recent): %v", err)
	}

	purged, err := s.PurgeOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged = %d, want 1", purged)
	}

	remainingOld, err := s.eventsOnDay(ctx, old)
	if err != nil {
		t.Fatalf("eventsOnDay(old): %v", err)
	}
	if len(remainingOld) != 0 {
		t.Errorf("eventsOnDay(old) = %d rows, want 0 after purge", len(remainingOld))
	}

	remainingRecent, err := s.eventsOnDay(ctx, recent)
	if err != nil {
		t.Fatalf("eventsOnDay(recent): %v", err)
	}
	if len(remainingRecent) != 1 {
		t.Errorf("eventsOnDay(recent) = %d rows, want 1 to survive purge", len(remainingRecent))
	}
}

func TestStorage_PurgeOlderThanIsExclusiveOnRecentBoundary(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	cutoff := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	exactlyAtCutoff := cutoff

	if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), exactlyAtCutoff); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	purged, err := s.PurgeOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	if purged != 0 {
		t.Fatalf("purged = %d, want 0: a row exactly at the cutoff is not older than it", purged)
	}
}

// TestStorage_SubSecondEventStaysOnItsOwnDayAndSurvivesRetention guards
// against comparing received_at as an RFC3339Nano string: Go's formatter
// trims trailing zero fractional digits, so a sub-second timestamp like
// "...T00:00:00.5Z" sorted as text landed BEFORE the whole-second
// "...T00:00:00Z" that precedes it in wall-clock time ('.' < 'Z'/digits),
// silently moving it to the wrong day and purging it a day early against
// a midnight-exact retention cutoff. received_at is now stored as Unix
// nanoseconds, which compares correctly regardless of formatting.
func TestStorage_SubSecondEventStaysOnItsOwnDayAndSurvivesRetention(t *testing.T) {
	s := openTestStorage(t)
	ctx := context.Background()

	day := time.Date(2026, 6, 15, 0, 0, 0, 500_000_000, time.UTC) // 00:00:00.5Z
	if err := s.InsertEvent(ctx, mustParse(t, validInstallEvent), day); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	onItsDay, err := s.eventsOnDay(ctx, day)
	if err != nil {
		t.Fatalf("eventsOnDay(2026-06-15): %v", err)
	}
	if len(onItsDay) != 1 {
		t.Errorf("eventsOnDay(2026-06-15) = %d events, want 1", len(onItsDay))
	}

	previousDay, err := s.eventsOnDay(ctx, day.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("eventsOnDay(2026-06-14): %v", err)
	}
	if len(previousDay) != 0 {
		t.Errorf("eventsOnDay(2026-06-14) = %d events, want 0: a sub-second event must not leak onto the previous day", len(previousDay))
	}

	// A retention cutoff at exactly midnight of the event's own day must
	// not purge it: 00:00:00.5 is chronologically AFTER, not before, a
	// whole-second midnight cutoff on the same day.
	purged, err := s.PurgeOlderThan(ctx, truncateToDay(day))
	if err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	if purged != 0 {
		t.Fatalf("purged = %d, want 0: the sub-second event is not older than a midnight-exact cutoff on its own day", purged)
	}

	remaining, err := s.eventsOnDay(ctx, day)
	if err != nil {
		t.Fatalf("eventsOnDay after purge: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("event was purged a day early: eventsOnDay = %d, want 1 to survive", len(remaining))
	}
}
