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
