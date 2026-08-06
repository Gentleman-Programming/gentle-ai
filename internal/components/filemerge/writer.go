package filemerge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// runtimeGOOS and syncDirFn are package-level vars so tests can override them
// without spawning a real Windows process.
//
// Background: Windows/NTFS does not support fsyncing a directory file
// descriptor. Calling (*os.File).Sync() on a directory handle returns
// ERROR_ACCESS_DENIED (syscall 5) regardless of user privileges — even as
// Administrator. FlushFileBuffers requires GENERIC_WRITE access on the handle,
// which Windows refuses for directories. The ErrPermission from syncDirFn is
// therefore silently tolerated when runtimeGOOS() == "windows". On Linux and
// macOS the full error is propagated so unexpected failures are still surfaced.
// See issues #293 and #294.
var runtimeGOOS = func() string { return runtime.GOOS }

// renameFn publishes the staged replacement. It is a package-level var so tests
// can simulate a rename that reports success without taking effect: the shape
// reported on Windows in #2319, where an antivirus/indexer hold turns the swap
// into a silent no-op.
var renameFn = os.Rename

// stagedFile is the staged destination of a durable write. The interface exists
// so tests can fault Chmod, Sync and Close, which a real *os.File only fails
// under disk-level conditions no unit test can produce, and those are exactly
// the conditions this package exists to survive.
type stagedFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

// createStagedFile stages a replacement in dir. Package-level var for the fault
// injection described on stagedFile.
var createStagedFile = func(dir string) (stagedFile, error) {
	return os.CreateTemp(dir, ".gentle-ai-*.tmp")
}

var syncDirFn = func(dir string) error {
	fd, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open parent directory %q: %w", dir, err)
	}
	defer fd.Close()
	return fd.Sync()
}

const maxAtomicFileSize = 16 << 20

// WriteResult reports what happened to the destination path. It is truthful on
// every return, including error returns: Changed is read back from disk after
// the replacement, never inferred from the attempt.
type WriteResult struct {
	Changed bool
	Created bool
}

// WriteFileAtomic replaces path with content and reports whether the
// replacement actually occurred.
//
// A nil error means the bytes reached stable storage (staged file synced,
// parent directory synced) and were read back from path. Any other outcome is
// an error.
//
// Callers must not read "err != nil" as "nothing happened". Replacement is
// published by a rename that can succeed while a later step fails; in that
// window the returned WriteResult reports Changed=true alongside the error, so
// rollback and journalling decisions come from the writer rather than from a
// guess at each call site (#1676).
func WriteFileAtomic(path string, content []byte, perm fs.FileMode) (WriteResult, error) {
	if perm == 0 {
		perm = 0o644
	}

	created := false
	existing, err := readComparableFile(path)
	if err == nil {
		if bytes.Equal(existing, content) {
			return WriteResult{}, nil
		}
	} else if !os.IsNotExist(err) {
		return WriteResult{}, fmt.Errorf("read existing file %q: %w", path, err)
	} else {
		created = true
	}

	landed, _, err := replaceDurably(path, bytes.NewReader(content), perm)
	result := WriteResult{Changed: landed, Created: created && landed}
	if err != nil {
		return result, err
	}
	return result, nil
}

// StreamResult describes the bytes that landed at the destination.
type StreamResult struct {
	Bytes  int64
	Digest string
}

// WriteStreamAtomic replaces path with everything readable from src and returns
// the size and SHA-256 of the bytes read back from path afterwards.
//
// The digest describes the file on disk, never the copied stream: a stream
// digest certifies its own copy and cannot detect a destination that ends up
// holding something else, which is how a truncated download used to pass
// checksum verification (#1998).
//
// A nil error means the bytes reached stable storage and were read back. The
// caller owns any size policy: check StreamResult.Bytes and remove path if the
// result is unacceptable.
func WriteStreamAtomic(path string, src io.Reader, perm fs.FileMode) (StreamResult, error) {
	_, result, err := replaceDurably(path, src, perm)
	return result, err
}

// replaceDurably is the single durability sequence in this package: stage beside
// the destination, apply the final metadata mutation, sync, close, publish with
// a rename, read the destination back, then sync the parent directory.
//
// The ordering is load-bearing. Chmod precedes Sync so a recovered rename cannot
// expose the file with the wrong mode, and the rename precedes the parent sync
// because publication is a namespace operation while durability is not (#2216).
//
// landed reports whether path holds the staged bytes. It is meaningful even when
// err is non-nil: publication happens before the last failure point, so a caller
// that reads "error" as "nothing happened" is wrong in exactly that window
// (#1676).
func replaceDurably(path string, src io.Reader, perm fs.FileMode) (landed bool, result StreamResult, err error) {
	if perm == 0 {
		perm = 0o644
	}

	dir := filepath.Dir(path)
	if err := ensureAtomicParentDir(dir, path); err != nil {
		return false, StreamResult{}, err
	}

	tmp, err := createStagedFile(dir)
	if err != nil {
		return false, StreamResult{}, fmt.Errorf("create temp file for %q: %w", path, err)
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	// The staged digest is not the answer; it is the oracle the answer is
	// checked against. Comparing it to the digest read back from the
	// destination is what turns "the copy succeeded" into "the right bytes are
	// at the right path".
	staged := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, staged), src)
	if err != nil {
		_ = tmp.Close()
		return false, StreamResult{}, fmt.Errorf("write temp file for %q: %w", path, err)
	}
	stagedDigest := hex.EncodeToString(staged.Sum(nil))

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return false, StreamResult{}, fmt.Errorf("set permissions on temp file for %q: %w", path, err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, StreamResult{}, fmt.Errorf("sync temp file for %q: %w", path, err)
	}

	if err := tmp.Close(); err != nil {
		return false, StreamResult{}, fmt.Errorf("close temp file for %q: %w", path, err)
	}

	if err := renameFn(tmpPath, path); err != nil {
		return false, StreamResult{}, fmt.Errorf("replace %q atomically: %w", path, err)
	}

	// Read the destination back before claiming anything about it. A rename that
	// returns nil has not necessarily taken effect. On Windows an antivirus or
	// indexer hold turns the swap into a silent no-op (#2319), and the whole
	// point of this sequence is that its result describes disk, not intent.
	diskDigest, diskBytes, err := digestFileOnDisk(path)
	if err != nil {
		return false, StreamResult{}, fmt.Errorf("read back %q after replacement: %w", path, err)
	}
	if diskBytes != written || diskDigest != stagedDigest {
		return false, StreamResult{}, fmt.Errorf(
			"replace %q atomically: the replacement did not land. The destination holds %d bytes (%s); %d bytes (%s) were written",
			path, diskBytes, diskDigest, written, stagedDigest)
	}

	// Past this point the destination holds the new bytes. Every remaining
	// failure must still say so, or callers will roll back nothing.
	cleanup = false
	result = StreamResult{Bytes: diskBytes, Digest: diskDigest}

	if err := SyncDir(dir); err != nil {
		return true, result, fmt.Errorf("sync parent directory for %q: %w", path, err)
	}

	return true, result, nil
}

// SyncDir flushes dir's entries to stable storage so a published name survives
// recovery.
//
// Windows/NTFS refuses to fsync a directory handle and returns ErrPermission
// regardless of privilege; that specific error is tolerated there and nowhere
// else, so a real failure (a full disk, say) still surfaces.
func SyncDir(dir string) error {
	err := syncDirFn(dir)
	if err == nil {
		return nil
	}
	if runtimeGOOS() == "windows" && errors.Is(err, os.ErrPermission) {
		return nil
	}
	return err
}

// FileDigest returns the SHA-256 and size of the file at path, read from disk.
// Callers comparing a published file against what they staged use this on both
// sides so the comparison is between two reads, not between a read and an
// intention.
func FileDigest(path string) (digest string, size int64, err error) {
	return digestFileOnDisk(path)
}

// digestFileOnDisk streams path and returns its SHA-256 and size. It refuses
// symlinks so a redirected destination cannot answer for the real one.
func digestFileOnDisk(path string) (digest string, size int64, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", 0, fmt.Errorf("destination %q is a symlink", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	sum := sha256.New()
	size, err = io.Copy(sum, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(sum.Sum(nil)), size, nil
}

func readComparableFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlink %q", path)
	}
	if info.Size() > maxAtomicFileSize {
		return nil, fmt.Errorf("file %q exceeds max atomic compare size %d bytes", path, maxAtomicFileSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAtomicFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAtomicFileSize {
		return nil, fmt.Errorf("file %q exceeds max atomic compare size %d bytes", path, maxAtomicFileSize)
	}
	return data, nil
}

func ensureAtomicParentDir(dir, path string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create parent directories for %q: %w", path, err)
		}
		info, err = os.Lstat(dir)
	}
	if err != nil {
		return fmt.Errorf("stat parent directory for %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Parent is a symlink (e.g. ~/.claude/agents → dotfiles repo).
		// Resolve the target and continue checks against the real directory.
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return fmt.Errorf("resolving symlink parent %q for %q: %w", dir, path, err)
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("stat symlink target %q for %q: %w", resolved, path, err)
		}
		dir = resolved
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path %q for %q is not a directory", dir, path)
	}
	if info.Mode().Perm()&0o200 == 0 {
		if err := os.Chmod(dir, 0o755); err != nil {
			return fmt.Errorf("relax parent directory permissions for %q: %w", path, err)
		}
	}
	return nil
}
