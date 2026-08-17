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
	if payload, readErr := io.ReadAll(io.LimitReader(file, sharedStoreLockReadLimit)); readErr == nil {
		var owner storeLockOwner
		if decodeErr := json.NewDecoder(bytes.NewReader(payload)).Decode(&owner); decodeErr == nil &&
			owner.Schema == storeLockSchema && owner.PID > 0 && strings.TrimSpace(owner.Host) != "" {
			evidence.HolderRecorded = true
			evidence.HolderPID = owner.PID
			evidence.HolderHost = owner.Host
		}
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
