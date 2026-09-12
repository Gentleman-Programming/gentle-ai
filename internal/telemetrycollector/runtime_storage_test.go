package telemetrycollector

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func TestRuntimeStorageMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO events VALUES (1, 123, 'install', 'legacy', 'v1', 'linux', 'amd64', '[]', '[]', 0, NULL)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	for i := 0; i < 2; i++ {
		s, err := OpenStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		var id string
		var version int
		if err := s.db.QueryRow(`SELECT install_id FROM events WHERE id=1`).Scan(&id); err != nil || id != "legacy" {
			t.Fatalf("legacy data: %q %v", id, err)
		}
		if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 1 {
			t.Fatalf("version %d: %v", version, err)
		}
		s.Close()
	}
}

func TestRuntimeStorageMigrationRollback(t *testing.T) {
	for _, scenario := range []string{"collision", "future version"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			setup := `CREATE TABLE runtime_rows (existing TEXT)`
			wantVersion := 0
			if scenario == "future version" {
				setup = `PRAGMA user_version=42`
				wantVersion = 42
			}
			if _, err := db.Exec(setup); err != nil {
				t.Fatal(err)
			}
			if s, err := OpenStorage(path); err == nil {
				s.Close()
				t.Fatal("migration should fail closed")
			}
			var version, count int
			if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='runtime_deliveries'`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if version != wantVersion || count != 0 {
				t.Fatalf("partial migration: version=%d tables=%d", version, count)
			}
		})
	}
}

// Refusing a database must preserve bytes, schema, and existing contents, not
// merely return an error after creating otherwise harmless legacy tables.
func TestRuntimeStorageRefusalPreservesDatabase(t *testing.T) {
	const deliveries = `CREATE TABLE runtime_deliveries (delivery_id TEXT PRIMARY KEY, received_at INTEGER NOT NULL, canonical_payload TEXT NOT NULL CHECK(json_valid(canonical_payload)));`
	const rows = `CREATE TABLE runtime_rows (delivery_id TEXT NOT NULL REFERENCES runtime_deliveries(delivery_id), ordinal INTEGER NOT NULL, row_json TEXT NOT NULL CHECK(json_valid(row_json)), PRIMARY KEY(delivery_id,ordinal));`
	for _, tc := range []struct{ name, setup string }{
		{"unknown version", `PRAGMA user_version=7; PRAGMA journal_mode=WAL;`},
		{"unversioned runtime collision", `CREATE TABLE runtime_rows (existing TEXT);`},
		{"legacy DDL failure", `CREATE TABLE events (existing TEXT);`},
		{"missing runtime tables", `PRAGMA user_version=1;`},
		{"missing rows table", `PRAGMA user_version=1;` + deliveries},
		{"missing column", `PRAGMA user_version=1;` + strings.Replace(deliveries, "received_at INTEGER NOT NULL,", "", 1) + rows},
		{"missing delivery uniqueness", `PRAGMA user_version=1;` + strings.Replace(deliveries, "TEXT PRIMARY KEY", "TEXT", 1) + rows},
		{"wrong unique columns", `PRAGMA user_version=1;` + strings.Replace(deliveries, "TEXT PRIMARY KEY", "TEXT", 1) + rows + `CREATE UNIQUE INDEX wrong_delivery ON runtime_deliveries(delivery_id,received_at);`},
		{"nonbinary uniqueness", `PRAGMA user_version=1;` + strings.Replace(deliveries, "TEXT PRIMARY KEY", "TEXT", 1) + rows + `CREATE UNIQUE INDEX folded_delivery ON runtime_deliveries(delivery_id COLLATE NOCASE);`},
		{"missing row uniqueness", `PRAGMA user_version=1;` + deliveries + strings.Replace(rows, ", PRIMARY KEY(delivery_id,ordinal)", "", 1)},
		{"partial delivery uniqueness", `PRAGMA user_version=1;` + strings.Replace(deliveries, "TEXT PRIMARY KEY", "TEXT", 1) + rows + `CREATE UNIQUE INDEX partial_delivery ON runtime_deliveries(delivery_id) WHERE received_at>0;`},
		{"wrong column type", `PRAGMA user_version=1;` + strings.Replace(deliveries, "received_at INTEGER", "received_at TEXT", 1) + rows},
		{"nullable row identity", `PRAGMA user_version=1;` + deliveries + strings.Replace(rows, "delivery_id TEXT NOT NULL", "delivery_id TEXT", 1)},
		{"missing foreign key", `PRAGMA user_version=1;` + deliveries + strings.Replace(rows, " REFERENCES runtime_deliveries(delivery_id)", "", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "refused.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE sentinel (value TEXT); INSERT INTO sentinel VALUES ('preserved');` + tc.setup); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			beforeSchema := runtimeDatabaseSnapshot(t, path)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := OpenStorage(path); err == nil {
				s.Close()
				t.Error("incompatible database was accepted")
			} else if tc.name == "unknown version" && !errors.Is(err, errRuntimeVersion) {
				t.Errorf("expected explicit unsupported version error, got %v", err)
			}
			afterSchema := runtimeDatabaseSnapshot(t, path)
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if beforeSchema != afterSchema {
				t.Error("refusal changed sqlite_master, version, or sentinel contents")
			}
			if !bytes.Equal(before, after) {
				t.Error("refusal changed database bytes")
			}
		})
	}
}

func TestRuntimeStorageEquivalentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "equivalent.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Reordered columns, lowercase declarations, and named unique indexes are
	// semantically sufficient; admission must not compare DDL strings.
	_, err = db.Exec(`PRAGMA user_version=1;
	 CREATE TABLE runtime_deliveries (canonical_payload text not null CHECK(json_valid(canonical_payload)), received_at integer not null, delivery_id text);
	 CREATE UNIQUE INDEX delivery_key ON runtime_deliveries(delivery_id);
	 CREATE TABLE runtime_rows (row_json text not null CHECK(json_valid(row_json)), ordinal integer not null, delivery_id text not null REFERENCES runtime_deliveries(delivery_id));
	 CREATE UNIQUE INDEX row_key ON runtime_rows(ordinal,delivery_id);`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ev, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stored", "duplicate"} {
		if got, err := s.InsertRuntimeEvent(context.Background(), ev, time.Unix(1, 0)); err != nil || got != want {
			t.Fatalf("equivalent schema: %q %v", got, err)
		}
	}
}

func runtimeDatabaseSnapshot(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var snapshot string
	err = db.QueryRow(`SELECT json_array(
	 (SELECT user_version FROM pragma_user_version),
	 (SELECT value FROM sentinel),
	 (SELECT json_group_array(json_array(type,name,tbl_name,sql)) FROM
	  (SELECT type,name,tbl_name,sql FROM sqlite_master ORDER BY type,name)))`).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestRuntimeStorageObservationMatrix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matrix.db")
	s, err := OpenStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertRuntimeEvent(context.Background(), ev, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if decision, err := s.InsertRuntimeEvent(context.Background(), ev, time.Unix(2, 0)); err != nil || decision != "duplicate" {
		t.Fatalf("durable retry: %q %v", decision, err)
	}
	ev.DeliveryID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := s.InsertRuntimeEvent(context.Background(), ev, time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}
	var class string
	var deliveries, responses, reported, unavailable, unsupported, sum int64
	err = s.db.QueryRow(`SELECT json_extract(row_json,'$.agent_class'), count(*),
	 sum(json_extract(row_json,'$.responses')), sum(json_extract(row_json,'$.input_tokens.reported')),
	 sum(json_extract(row_json,'$.input_tokens.unavailable')), sum(json_extract(row_json,'$.input_tokens.unsupported')),
	 sum(json_extract(row_json,'$.input_tokens.sum')) FROM runtime_rows GROUP BY json_extract(row_json,'$.agent_class')`).Scan(&class, &deliveries, &responses, &reported, &unavailable, &unsupported, &sum)
	if err != nil {
		t.Fatal(err)
	}
	if class != "sdd-apply" || deliveries != 2 || responses != 12 || reported != 2 || unavailable != 4 || unsupported != 6 || sum != 1999999999998 {
		t.Fatal("matrix lost aggregate observations or counted a retry")
	}
}

func TestRuntimeStorageCommitFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "commit.db")
	s, err := OpenStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Deferred foreign-key failure happens at COMMIT, after successful row writes.
	if _, err := s.db.Exec(`CREATE TABLE commit_probe (id TEXT REFERENCES runtime_deliveries(delivery_id) DEFERRABLE INITIALLY DEFERRED);
 CREATE TRIGGER fail_commit AFTER INSERT ON runtime_rows BEGIN INSERT INTO commit_probe VALUES ('missing'); END;`); err != nil {
		t.Fatal(err)
	}
	ev, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	if decision, err := s.InsertRuntimeEvent(context.Background(), ev, time.Now()); err == nil || decision != "" {
		t.Fatalf("failed commit acknowledged: %q %v", decision, err)
	}
	for _, table := range []string{"runtime_deliveries", "runtime_rows", "commit_probe"} {
		var count int
		if err := s.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s partial commit: %d %v", table, count, err)
		}
	}
}

func TestRuntimeStorageDelivery(t *testing.T) {
	s, err := OpenStorage(filepath.Join(t.TempDir(), "runtime.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ev, err := telemetry.ParseRuntimeEvent(runtimeFixture())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Unix(123, 456)
	var wg sync.WaitGroup
	results := make(chan string, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			decision, err := s.InsertRuntimeEvent(ctx, ev, now)
			results <- decision
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	stored, duplicate := 0, 0
	for decision := range results {
		switch decision {
		case "stored":
			stored++
		case "duplicate":
			duplicate++
		default:
			t.Fatalf("decision %q", decision)
		}
	}
	if stored != 1 || duplicate != 7 {
		t.Fatalf("stored=%d duplicate=%d", stored, duplicate)
	}
	ev.Host = "codex"
	if _, err := s.InsertRuntimeEvent(ctx, ev, now); !errors.Is(err, ErrRuntimeConflict) {
		t.Fatalf("conflict: %v", err)
	}
	var deliveries, rows int
	var received int64
	if err := s.db.QueryRow(`SELECT count(*), received_at FROM runtime_deliveries`).Scan(&deliveries, &received); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM runtime_rows`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if deliveries != 1 || rows != 1 || received != receivedAtKey(now) {
		t.Fatalf("delivery=%d rows=%d time=%d", deliveries, rows, received)
	}
	for _, metric := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		var reported, unavailable, unsupported, sum int64
		q := `SELECT json_extract(row_json,'$.` + metric + `.reported'),json_extract(row_json,'$.` + metric + `.unavailable'),json_extract(row_json,'$.` + metric + `.unsupported'),json_extract(row_json,'$.` + metric + `.sum') FROM runtime_rows`
		if err := s.db.QueryRow(q).Scan(&reported, &unavailable, &unsupported, &sum); err != nil {
			t.Fatal(err)
		}
		if reported != 1 || unavailable != 2 || unsupported != 3 || sum != 999999999999 {
			t.Fatalf("lost %s coverage", metric)
		}
	}
	var duration string
	if err := s.db.QueryRow(`SELECT row_json FROM runtime_rows`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(duration, `"sum_ms":125e-2`) {
		t.Fatal("decimal timing not preserved")
	}
	// Force the row write to fail after the delivery header has been inserted.
	if _, err := s.db.Exec(`CREATE TRIGGER reject_runtime BEFORE INSERT ON runtime_rows BEGIN SELECT RAISE(ABORT, 'private-canary'); END`); err != nil {
		t.Fatal(err)
	}
	ev.DeliveryID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if decision, err := s.InsertRuntimeEvent(ctx, ev, now); err == nil || decision != "" {
		t.Fatalf("failed write acknowledged: %q %v", decision, err)
	}
	if err := s.db.QueryRow(`SELECT count(*) FROM runtime_deliveries`).Scan(&deliveries); err != nil || deliveries != 1 {
		t.Fatalf("partial write: %d %v", deliveries, err)
	}
}
