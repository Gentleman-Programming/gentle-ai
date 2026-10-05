package telemetry

// Runtime metrics have no client state. Existing telemetry policy is read-only
// here; old queue, outbox, kick and lock files are ignored without cleanup.
