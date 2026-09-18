package filecoord

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const maxSnapshotFileSize = 16 << 20

var (
	ErrSymlinkTarget    = errors.New("file coordination snapshot target is a symlink or reparse point")
	ErrNonRegularTarget = errors.New("file coordination snapshot target is not a regular file")
	ErrOversizedTarget  = errors.New("file coordination snapshot target exceeds size limit")
)

// FileIdentity captures a point-in-time filesystem identity token.
type FileIdentity struct {
	Dev     uint64
	Ino     uint64
	Size    int64
	ModTime int64
	Hash    [32]byte
}

// Matches returns true if both identities are equal in all attributes.
func (id FileIdentity) Matches(other FileIdentity) bool {
	return id.Dev == other.Dev &&
		id.Ino == other.Ino &&
		id.Size == other.Size &&
		id.ModTime == other.ModTime &&
		id.Hash == other.Hash
}

// Snapshot is a point-in-time capture of a file's content and metadata.
type Snapshot struct {
	Path     string
	Bytes    []byte
	Mode     fs.FileMode
	Identity FileIdentity
	Exists   bool
}

// ReadSnapshot captures a side-effect-free point-in-time snapshot of target
// without following a final symlink or reparse point.
func ReadSnapshot(target string) (*Snapshot, error) {
	cleaned, err := cleanTarget(target)
	if err != nil {
		return nil, err
	}

	info, statErr := os.Lstat(cleaned)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return &Snapshot{Path: cleaned, Exists: false}, nil
		}
		return nil, fmt.Errorf("lstat target %q: %w", cleaned, statErr)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrSymlinkTarget
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNonRegularTarget
	}
	if info.Size() > maxSnapshotFileSize {
		return nil, ErrOversizedTarget
	}

	return readPlatformSnapshot(cleaned, info)
}

func computeHash(b []byte) [32]byte {
	return sha256.Sum256(b)
}
