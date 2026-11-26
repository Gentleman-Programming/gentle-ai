package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
)

const openCodeLensDispatchDirectory = "opencode-dispatch"

var (
	ErrOpenCodeLensDispatchReserved = errors.New("OpenCode lens dispatch slot is already reserved")
	ErrOpenCodeLensDispatchStale    = errors.New("OpenCode lens dispatch does not match an uncollected slot")
)

// ReserveOpenCodeLensDispatch admits one first-contact V2 dispatch per native
// slot. The resolved store path supplies the repository and lineage; the
// remaining key is the capture phase, target and selected lens. An empty
// directory is enough: atomic creation is the claim, not a PID or a lease.
// There is no release operation. Only native authority can prove capture or
// move to a different slot. A stale relay can never release another claim.
func (store CompactStore) ReserveOpenCodeLensDispatch(ctx context.Context, subject ArtifactSubject) error {
	if store.repo == "" || store.maintenanceLockPath == "" || subject.LineageID != store.lineageID ||
		validateLineageID(store.lineageID) != nil {
		return ErrOpenCodeLensDispatchStale
	}
	common := filepath.Dir(filepath.Dir(store.maintenanceLockPath))
	lineage := filepath.Join("gentle-ai", "review-transactions", "v2", store.lineageID)
	if store.Dir != filepath.Join(common, lineage) {
		return ErrOpenCodeLensDispatchStale
	}
	if err := ensureRARDirectoryChain(common, store.Dir, lineage, 4, false); err != nil {
		return err
	}

	maintenance, err := store.acquireReadMaintenance(ctx)
	if err != nil {
		return err
	}
	if maintenance != nil {
		defer maintenance.Release()
	}
	record, err := store.loadCompactRecordLocked()
	if err != nil {
		return err
	}
	state := record.State
	if record.HistoricalCompat || state.State != StateReviewing || state.CapturePhaseRevision != subject.AuthorityRevision ||
		state.InitialSnapshot.Identity != subject.TargetIdentity || subject.SelectedOrder < 0 ||
		subject.SelectedOrder >= len(state.SelectedLenses) || state.SelectedLenses[subject.SelectedOrder] != subject.Lens {
		return ErrOpenCodeLensDispatchStale
	}
	if _, captured, err := state.ActiveAdmittedLensResult(subject.SelectedOrder); err != nil {
		return err
	} else if captured {
		return ErrOpenCodeLensDispatchStale
	}

	// Both mutations are relative to directory handles verified by a no-follow
	// walk. A symlink substituted into an ancestor must not redirect mkdir.
	lineageRoot, err := openOpenCodeLensDispatchRoot(store.Dir, false)
	if err != nil {
		return err
	}
	defer lineageRoot.Close()
	dir := filepath.Join(store.Dir, openCodeLensDispatchDirectory)
	if err := lineageRoot.Mkdir(openCodeLensDispatchDirectory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	// A previous relay may have exited after mkdir but before ACL repair.
	if err := repairAndValidatePrivateRARDirectory(dir); err != nil {
		return err
	}
	root, err := openOpenCodeLensDispatchRoot(dir, true)
	if err != nil {
		return err
	}
	defer root.Close()

	key := sha256.Sum256([]byte(subject.AuthorityRevision + "\x00" + subject.TargetIdentity + "\x00" + subject.Lens))
	if err := root.Mkdir(hex.EncodeToString(key[:]), 0o700); errors.Is(err, os.ErrExist) {
		return ErrOpenCodeLensDispatchReserved
	} else if err != nil {
		return err
	}
	// Sync both entries, including when another dispatcher created the
	// reservation directory but has not yet synced its parent.
	if err := SyncReviewDirectory(dir); err != nil {
		return err
	}
	return SyncReviewDirectory(store.Dir)
}

func openOpenCodeLensDispatchRoot(dir string, private bool) (*os.Root, error) {
	checked, err := openRARPathNoFollow(dir, true)
	if err != nil {
		return nil, err
	}
	defer checked.Close()
	info, err := checked.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || private && !privateOpenRARPathSafe(checked, info) || !private && !rarRepositoryOpenDirectorySafe(checked, info) {
		return nil, errUnsafeRARAuthorityPath
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	if !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, errRARAuthorityPathReplaced
	}
	return root, nil
}
