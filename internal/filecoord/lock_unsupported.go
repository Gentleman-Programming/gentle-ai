package filecoord

// Temporary all-platform PR1 backend: it makes no platform capability claim or filesystem change; POSIX and Windows backends are later PRs.
func unsupportedBackend() error { return &UnsupportedError{} }
