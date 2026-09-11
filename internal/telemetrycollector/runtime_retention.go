package telemetrycollector

import (
	"context"
	"database/sql"
	"time"
)

// purgeRuntimeOlderThan participates in the existing raw-retention transaction.
// Child rows have no independent age: remove the whole delivery, children first.
// Deleting its identity deliberately ends dedupe; a late retry can count again.
func purgeRuntimeOlderThan(ctx context.Context, tx *sql.Tx, cutoff time.Time) error {
	for _, query := range []string{
		`DELETE FROM runtime_rows WHERE delivery_id IN
		 (SELECT delivery_id FROM runtime_deliveries WHERE received_at < ?)`,
		`DELETE FROM runtime_deliveries WHERE received_at < ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, receivedAtKey(cutoff)); err != nil {
			return errRuntimeStorage
		}
	}
	return nil
}
