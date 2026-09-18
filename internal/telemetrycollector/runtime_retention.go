package telemetrycollector

import (
	"context"
	"database/sql"
	"time"
)

// purgeRuntimeOlderThan participates in the existing raw-retention transaction.
// Child rows have no independent age: remove the whole delivery, children first.
// Deleting its identity deliberately ends dedupe; a late retry can count again.
// runtime_delivery_ids (the --runtime-store=metrics dedup table) is NOT purged
// here: it has its own, shorter cutoff and is trimmed in batches outside this
// transaction by purgeRuntimeDeliveryIDsOlderThan.
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

// runtimeDeliveryIDPurgeBatch bounds how many identities one purge
// transaction removes. Under --runtime-store=metrics the table grows by every
// accepted delivery (about a million rows a day in production), so a single
// DELETE would hold the collector's only writer for seconds and turn live
// deliveries into storage_busy. A var, not a const, so a test can lower it
// and prove the loop.
var runtimeDeliveryIDPurgeBatch = 50000

// purgeRuntimeDeliveryIDsOlderThan removes dedup identities received before
// cutoff in short, independent transactions of at most
// runtimeDeliveryIDPurgeBatch rows each, and returns how many it removed.
// Atomicity is not needed: the table has no children, and an identity that
// survives one run is removed by the next. Expiry ends dedupe on purpose; a
// replay older than the dedup window counts as a fresh delivery.
func purgeRuntimeDeliveryIDsOlderThan(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx, `DELETE FROM runtime_delivery_ids WHERE rowid IN
		 (SELECT rowid FROM runtime_delivery_ids WHERE received_at < ? LIMIT ?)`, receivedAtKey(cutoff), runtimeDeliveryIDPurgeBatch)
		if err != nil {
			return total, runtimeStorageError(err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, runtimeStorageError(err)
		}
		total += n
		if n < int64(runtimeDeliveryIDPurgeBatch) {
			return total, nil
		}
		if ctx.Err() != nil {
			return total, ctx.Err()
		}
	}
}
