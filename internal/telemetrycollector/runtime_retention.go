package telemetrycollector

import (
	"context"
	"database/sql"
	"time"
)

// purgeRuntimeOlderThan participates in the existing raw-retention transaction.
// Child rows have no independent age: remove the whole delivery, children first.
// Deleting its identity deliberately ends dedupe; a late retry can count again.
// runtime_delivery_ids (the --runtime-store=metrics dedup table) is small and
// has no children, but shares the same retention cutoff and the same
// end-of-dedupe behavior on expiry.
func purgeRuntimeOlderThan(ctx context.Context, tx *sql.Tx, cutoff time.Time) error {
	for _, query := range []string{
		`DELETE FROM runtime_rows WHERE delivery_id IN
		 (SELECT delivery_id FROM runtime_deliveries WHERE received_at < ?)`,
		`DELETE FROM runtime_deliveries WHERE received_at < ?`,
		`DELETE FROM runtime_delivery_ids WHERE received_at < ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, receivedAtKey(cutoff)); err != nil {
			return errRuntimeStorage
		}
	}
	return nil
}
