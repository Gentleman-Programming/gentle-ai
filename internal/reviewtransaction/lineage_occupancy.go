package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ExactReviewLineageOccupied reports whether one exact lineage path is occupied
// in any persisted authority schema. It observes no sibling lineage and never
// loads, validates, or mutates the authority stored at the occupied path.
func ExactReviewLineageOccupied(ctx context.Context, repo, lineageID string) (bool, error) {
	if err := validateLineageID(lineageID); err != nil {
		return false, err
	}
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return false, err
	}
	for _, version := range []string{"v1", "v2", "v3"} {
		_, statErr := os.Lstat(filepath.Join(base, version, lineageID))
		switch {
		case statErr == nil:
			return true, nil
		case errors.Is(statErr, fs.ErrNotExist):
			continue
		default:
			return false, fmt.Errorf("inspect exact review lineage %q in %s: %w", lineageID, version, statErr)
		}
	}
	return false, nil
}

// ExactReviewLineageForeignWorktree reports whether one exact occupied lineage
// froze its compact authority from a worktree other than repo. Issue #4023:
// the shared authority store lives under the Git common dir, so a linked
// worktree of the same repository sees the lineage as occupied even though it
// never froze it. ExactReviewLineageOccupied only proves occupancy, never
// which worktree owns it, so a caller resuming an explicitly named lineage
// must additionally confirm this before treating a bound STATUS as a resume
// instead of a fresh preflight of the wrong working tree.
//
// A record written before the worktree-bound atomic START binding existed
// carries no InitialAtomicStart and cannot be compared; it is never reported
// foreign, preserving today's behavior for historical authority.
func ExactReviewLineageForeignWorktree(ctx context.Context, repo, lineageID string) (bool, error) {
	store, err := CompactAuthoritativeStore(ctx, repo, lineageID)
	if err != nil {
		return false, err
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		if IsCompactAuthorityOperationalFailure(err) {
			return false, err
		}
		return false, nil
	}
	binding := record.State.InitialAtomicStart
	if binding == nil {
		return false, nil
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return false, err
	}
	return binding.WorktreeIdentity != lease.Identity().RepositoryRef, nil
}
