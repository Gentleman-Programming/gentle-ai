package storage

// AvailableBytes returns the number of bytes available to an unprivileged user
// on the volume containing path. path may be an existing file or directory,
// or a path that does not yet exist — in which case the nearest existing
// ancestor is used.
func AvailableBytes(path string) (int64, error) {
	return availableBytes(path)
}
