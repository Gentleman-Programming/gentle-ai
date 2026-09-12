package telemetrycollector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

var ErrRuntimeConflict = errors.New("runtime delivery identity conflict")
var errRuntimeStorage = errors.New("runtime storage unavailable")
var errRuntimeVersion = errors.New("unsupported collector database version")
var errRuntimeSchema = errors.New("incompatible runtime storage schema")

// This collector previously owned an unversioned database. Never downgrade an
// unknown version or adopt pre-existing runtime tables. DDL and version commit
// together; the legacy install/heartbeat tables are not rewritten.
func migrateRuntimeStorage(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return errRuntimeStorage
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return errRuntimeStorage
	}
	switch version {
	case 0:
		if _, err := tx.Exec(`
   CREATE TABLE runtime_deliveries (
    delivery_id TEXT PRIMARY KEY,
    received_at INTEGER NOT NULL,
    canonical_payload TEXT NOT NULL CHECK(json_valid(canonical_payload))
   );
   CREATE TABLE runtime_rows (
    delivery_id TEXT NOT NULL REFERENCES runtime_deliveries(delivery_id),
    ordinal INTEGER NOT NULL,
    row_json TEXT NOT NULL CHECK(json_valid(row_json)),
    PRIMARY KEY(delivery_id, ordinal)
   );
   PRAGMA user_version=1;
  `); err != nil {
			return errRuntimeStorage
		}
	case 1:
		if err := validateRuntimeStorage(tx); err != nil {
			return err
		}
	default:
		return errRuntimeVersion
	}
	// Both new database initialization and legacy upgrades are atomic. Existing
	// v1 runtime tables must pass admission before any legacy DDL is applied.
	if _, err := tx.Exec(schemaDDL); err != nil {
		return errRuntimeStorage
	}
	if tx.Commit() != nil {
		return errRuntimeStorage
	}
	return nil
}

// Validate semantic column and idempotency constraints, not sqlite_master's
// formatting or autoindex names. Table-valued PRAGMAs take bound values; no
// database-provided identifier is interpolated into SQL.
func validateRuntimeStorage(tx *sql.Tx) error {
	type column struct {
		kind     string
		required bool
	}
	for _, table := range []struct {
		name    string
		columns map[string]column
		unique  []string
	}{
		{"runtime_deliveries", map[string]column{
			"delivery_id": {"TEXT", false}, "received_at": {"INTEGER", true}, "canonical_payload": {"TEXT", true},
		}, []string{"delivery_id"}},
		{"runtime_rows", map[string]column{
			"delivery_id": {"TEXT", true}, "ordinal": {"INTEGER", true}, "row_json": {"TEXT", true},
		}, []string{"delivery_id", "ordinal"}},
	} {
		var count int
		if err := tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table.name).Scan(&count); err != nil || count != 1 {
			return errRuntimeSchema
		}
		rows, err := tx.Query(`SELECT name, upper(type), "notnull", hidden FROM pragma_table_xinfo(?)`, table.name)
		if err != nil {
			return errRuntimeSchema
		}
		seen, valid := 0, true
		for rows.Next() {
			var name, kind string
			var required, hidden int
			if err := rows.Scan(&name, &kind, &required, &hidden); err != nil {
				rows.Close()
				return errRuntimeSchema
			}
			want, ok := table.columns[name]
			valid = valid && ok && kind == want.kind && hidden == 0 && (!want.required || required == 1)
			seen++
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !valid || seen != len(table.columns) {
			return errRuntimeSchema
		}
		// Require full, non-expression BINARY uniqueness on precisely the key
		// columns. Partial indexes or extra columns cannot guarantee dedupe.
		first, last := table.unique[0], table.unique[len(table.unique)-1]
		err = tx.QueryRow(`SELECT count(*) FROM pragma_index_list(?) AS i
		 WHERE i."unique"=1 AND i.partial=0
		 AND (SELECT count(*) FROM pragma_index_xinfo(i.name) WHERE key=1)=?
		 AND (SELECT count(DISTINCT name) FROM pragma_index_xinfo(i.name)
		      WHERE key=1 AND coll='BINARY' AND name IN (?,?))=?`,
			table.name, len(table.unique), first, last, len(table.unique)).Scan(&count)
		if err != nil || count < 1 {
			return errRuntimeSchema
		}
	}
	var links int
	err := tx.QueryRow(`SELECT count(*) FROM pragma_foreign_key_list('runtime_rows')
	 WHERE "table"='runtime_deliveries' AND "from"='delivery_id' AND "to"='delivery_id'
	 AND seq=0 AND on_update='NO ACTION' AND on_delete='NO ACTION'
	 AND (SELECT count(*) FROM pragma_foreign_key_list('runtime_rows'))=1`).Scan(&links)
	if err != nil || links != 1 {
		return errRuntimeSchema
	}
	return nil
}

// InsertRuntimeEvent acknowledges only committed deliveries. All stored JSON is
// revalidated and canonicalized public data, never raw request bytes. Row JSON
// preserves exact decimal timing and independent token coverage without floats.
func (s *Storage) InsertRuntimeEvent(ctx context.Context, event telemetry.RuntimeEvent, receivedAt time.Time) (string, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return "", telemetry.ErrRuntimeEvent
	}
	event, err = telemetry.ParseRuntimeEvent(raw)
	if err != nil {
		return "", err
	}
	canonical, err := json.Marshal(event)
	if err != nil {
		return "", telemetry.ErrRuntimeEvent
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", errRuntimeStorage
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO runtime_deliveries(delivery_id,received_at,canonical_payload) VALUES(?,?,?) ON CONFLICT(delivery_id) DO NOTHING`, event.DeliveryID, receivedAtKey(receivedAt), string(canonical))
	if err != nil {
		return "", errRuntimeStorage
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return "", errRuntimeStorage
	}
	decision := "stored"
	if inserted == 0 {
		var existing string
		if err := tx.QueryRowContext(ctx, `SELECT canonical_payload FROM runtime_deliveries WHERE delivery_id=?`, event.DeliveryID).Scan(&existing); err != nil {
			return "", errRuntimeStorage
		}
		if existing != string(canonical) {
			return "", ErrRuntimeConflict
		}
		decision = "duplicate"
	} else {
		for i, row := range event.Rows {
			canonicalRow, err := json.Marshal(row)
			if err != nil {
				return "", errRuntimeStorage
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_rows(delivery_id,ordinal,row_json) VALUES(?,?,?)`, event.DeliveryID, i, string(canonicalRow)); err != nil {
				return "", errRuntimeStorage
			}
		}
	}
	if tx.Commit() != nil {
		return "", errRuntimeStorage
	}
	return decision, nil
}
