package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// sharedStoreLockReadLimit bounds how much lock-file residue inspection is
// willing to read; a healthy owner record is a few hundred bytes.
const sharedStoreLockReadLimit int64 = 64 << 10

// storeLockCoordinationBytes is the length of the byte range the platform
// lock primitive covers: store_lock_windows.go locks exactly [0, 1) through
// LockFileEx, and POSIX flock is whole-file but advisory. It is therefore the
// only region of a lock file another handle can be denied.
const storeLockCoordinationBytes int64 = 1

// storeLockOwnerDocumentPrefix is the first byte of every owner record
// acquireLocalStoreLock writes: encoding/json marshals the owner struct
// opening with '{' and never emits leading whitespace. Reading a live lock
// file past the coordination byte drops exactly this byte, so the reader can
// restore it instead of losing the whole record (see
// readStoreLockOwnerRecord). compact_shared_lock_test.go pins that invariant
// against the bytes the writer actually produces.
const storeLockOwnerDocumentPrefix = '{'

// CompactSharedStoreLockEvidence describes the live coordination state of the
// shared compact store lock (v2/LOCK) for retry-safe gate classification
// (issue #3342). Inspection is read-only: it never creates, truncates, or
// removes the lock file, and a probe that momentarily acquires the advisory
// flock releases it before returning.
type CompactSharedStoreLockEvidence struct {
	// DisplayPath names the lock for humans without leaking an absolute
	// filesystem path into gate envelopes: it is the lock's path relative to
	// the repository Git common directory.
	DisplayPath string
	Exists      bool
	// Held reports that the advisory flock is currently held, which is proof
	// of a live concurrent review operation on this store.
	Held bool
	// HolderRecorded reports that the lock file carries a readable owner
	// record; HolderPID and HolderHost are meaningful only when it is true.
	// The record is decoded tolerantly (interrupted-holder residue may leave
	// trailing bytes after the owner document), because it feeds guidance,
	// never authorization: kernel advisory ownership remains the only
	// liveness truth.
	HolderRecorded bool
	HolderPID      int
	HolderHost     string
	// HolderVerifiedDeadOnThisHost is true only when the flock is not held,
	// the recorded host is this host, and the recorded pid verifiably names
	// no running process. Only this proof may back stale-lock removal
	// guidance.
	HolderVerifiedDeadOnThisHost bool
}

// InspectCompactSharedStoreLock reports the shared compact store lock's
// coordination evidence for the repository owning repo.
func InspectCompactSharedStoreLock(ctx context.Context, repo string) (CompactSharedStoreLockEvidence, error) {
	root, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactSharedStoreLockEvidence{}, err
	}
	evidence := CompactSharedStoreLockEvidence{
		DisplayPath: path.Join("gentle-ai", "review-transactions", "v2", "LOCK"),
	}
	file, err := os.OpenFile(filepath.Join(root, "v2", "LOCK"), os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return evidence, nil
		}
		return evidence, err
	}
	defer file.Close()
	evidence.Exists = true
	if owner, recorded := readStoreLockOwnerRecord(file); recorded {
		evidence.HolderRecorded = true
		evidence.HolderPID = owner.PID
		evidence.HolderHost = owner.Host
	}
	locked, probeErr := tryLockFile(file)
	if probeErr != nil {
		return evidence, probeErr
	}
	if !locked {
		evidence.Held = true
		return evidence, nil
	}
	_ = unlockFile(file)
	if evidence.HolderRecorded {
		if host, hostErr := os.Hostname(); hostErr == nil && host == evidence.HolderHost {
			evidence.HolderVerifiedDeadOnThisHost = processVerifiedDead(evidence.HolderPID)
		}
	}
	return evidence, nil
}

// readStoreLockOwnerRecord reads the holder record from a lock file opened by
// a handle that does NOT own the lock.
//
// The whole-file read is the plain case and the only one POSIX ever needs:
// flock is advisory, so a concurrent holder never denies a reader. Windows
// byte-range locks are mandatory instead: while a holder is live, any read
// overlapping the locked coordination byte fails with a lock violation from
// every other handle, including another handle in the locking process itself.
// That is why a live-contention gate denial named the lock but not its holder
// on Windows while naming both on Linux: the holder record was never
// unreadable, only the byte range in front of it was.
//
// Retrying past the coordination byte reads the exact same bytes the holder
// wrote, minus its first one, which storeLockOwnerDocumentPrefix restores.
// The record still has to decode and validate; nothing is inferred from the
// retry itself.
func readStoreLockOwnerRecord(source io.ReaderAt) (storeLockOwner, bool) {
	if payload, err := readStoreLockRegion(source, 0); err == nil {
		return decodeStoreLockOwnerRecord(payload, 0)
	}
	payload, err := readStoreLockRegion(source, storeLockCoordinationBytes)
	if err != nil {
		return storeLockOwner{}, false
	}
	return decodeStoreLockOwnerRecord(payload, storeLockCoordinationBytes)
}

func readStoreLockRegion(source io.ReaderAt, offset int64) ([]byte, error) {
	return io.ReadAll(io.NewSectionReader(source, offset, sharedStoreLockReadLimit))
}

// decodeStoreLockOwnerRecord decodes the holder record out of lock-file bytes
// read starting at offset. It is pure so the platform-specific read geometry
// stays verifiable without a Windows runner.
func decodeStoreLockOwnerRecord(payload []byte, offset int64) (storeLockOwner, bool) {
	if owner, decoded := decodeStoreLockOwnerDocument(payload); decoded {
		return owner, true
	}
	if offset != storeLockCoordinationBytes {
		return storeLockOwner{}, false
	}
	return decodeStoreLockOwnerDocument(append([]byte{storeLockOwnerDocumentPrefix}, payload...))
}

// decodeStoreLockOwnerDocument decodes one owner document tolerantly:
// interrupted-holder residue may leave trailing bytes after it, and the
// record feeds operator guidance, never authorization, and kernel advisory
// ownership remains the only liveness truth.
func decodeStoreLockOwnerDocument(payload []byte) (storeLockOwner, bool) {
	var owner storeLockOwner
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&owner); err != nil {
		return storeLockOwner{}, false
	}
	if owner.Schema != storeLockSchema || owner.PID <= 0 || strings.TrimSpace(owner.Host) == "" {
		return storeLockOwner{}, false
	}
	return owner, true
}
