package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	// reviewDerivedStartLineageDigits is how much of the target identity the
	// derived lineage name carries. It is deliberately a prefix of the target
	// identity: the SAME candidate must resume the SAME lineage, which is what
	// makes a repeated `review start` idempotent.
	reviewDerivedStartLineageDigits = 16
)

// reviewStatusStartLineage preserves an explicitly selected free START lineage.
// An occupied selector is historical authority, never an overwrite target, so a
// fresh START falls back to the stable worktree-and-target derivation instead.
func reviewStatusStartLineage(ctx context.Context, root, targetIdentity, lineage, recoverySuccessor string) (string, error) {
	requested := strings.TrimSpace(recoverySuccessor)
	if requested == "" {
		requested = strings.TrimSpace(lineage)
	}
	if requested != "" {
		occupied, err := reviewtransaction.ExactReviewLineageOccupied(ctx, root, requested)
		if err != nil {
			return "", err
		}
		if !occupied {
			return requested, nil
		}
	}
	return reviewAtomicStartLineage(ctx, root, targetIdentity)
}

// reviewAtomicStartLineage is the pure lineage derivation for every shipped
// compact atomic START route. Its preimage is the exact worktree identity and frozen target
// identity; it reads no authority version, performs no inventory scan, and
// never searches for an available suffix. The same candidate bytes in linked
// worktrees therefore have independent lineages, while STATUS and START derive
// exactly the same lineage for one frozen worktree target.
func reviewAtomicStartLineage(ctx context.Context, root, targetIdentity string) (string, error) {
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, root)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		WorktreeIdentity string `json:"worktree_identity"`
		TargetIdentity   string `json:"target_identity"`
	}{
		WorktreeIdentity: lease.Identity().RepositoryRef,
		TargetIdentity:   strings.TrimSpace(targetIdentity),
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("gentle-ai.review-start-lineage/v3\x00"), payload...))
	return "review-" + hex.EncodeToString(sum[:])[:reviewDerivedStartLineageDigits], nil
}
