package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sort"
)

// ManagedDigest returns a deterministic content digest ("sha256:<hex>") over
// every managed asset embedded in FS. Two binaries built from the same source
// embed byte-identical assets and therefore produce the same digest,
// regardless of build metadata such as vcs.revision, build timestamp, or
// module version. Reviewer asset-provenance checks compare this digest
// instead of a build identity, so a rebuild that changes nothing about the
// embedded assets is never treated as stale (#2685).
func ManagedDigest() (string, error) {
	return managedDigest(FS)
}

// managedDigest computes the digest over an arbitrary fs.FS. Factored out of
// ManagedDigest so tests can exercise the walk/hash logic against a small
// in-memory fixture instead of the full embedded asset tree.
//
// io/fs guarantees forward-slash-separated paths regardless of host
// platform, and paths are explicitly sorted before hashing, so the result is
// deterministic across operating systems and across repeated calls.
func managedDigest(fsys fs.FS) (string, error) {
	var paths []string
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)

	hash := sha256.New()
	for _, path := range paths {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return "", err
		}
		// NUL-delimit path and content so no concatenation of different
		// (path, content) pairs can collide onto the same byte stream.
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
