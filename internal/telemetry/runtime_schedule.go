package telemetry

// Runtime delivery has no scheduler, detached native child, or process state.
// The host's event hook invokes the one-shot stdin command asynchronously.
