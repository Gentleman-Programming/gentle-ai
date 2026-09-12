package telemetry

// Runtime delivery has no flush operation. Each observation is sent once from
// stdin; stale user outbox files are intentionally neither read nor modified.
