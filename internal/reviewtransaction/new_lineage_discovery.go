// Package reviewtransaction — new-lineage governing-authority discovery
// (Wave 3 Slice 4, design decision 4). This file owns exactly two things:
// DiscoverNewLineage, which answers "does a v3 authority record exist for
// this exact lineage marker, and is it readable", and
// ResolveGoverningAuthority, the pure Amendment C decision matrix that turns
// that answer (plus an independently-resolved legacy presence bit) into one
// of three outcomes — new governs, legacy governs unchanged, or deny.
//
// Both are deliberately I/O-light: DiscoverNewLineage costs one os.Stat and
// (at most) one store Load when an explicit --lineage marker is supplied,
// and nothing at all otherwise (design's corrected discovery rule: lineage
// kind is established SOLELY by v3/ record presence, so no marker means
// "new absent" by construction, not a name to go look up). The CLI-level
// caller (internal/cli/review_governing_authority.go) is the only place
// that resolves live Git evidence, and only after DiscoverNewLineage has
// already proven a record exists to compare against.
package reviewtransaction

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// NewLineageMarkerCorruptedError proves a candidate carries a new-lineage
// MARKER — an explicit --lineage reference whose v3/<lineage> directory
// exists on disk, asserting that this lineage was (or is being) reviewed
// under the new-lineage model — but the record itself could not be read:
// deleted, quarantined, corrupted, or (a foreign common dir) simply never
// written where this repository expects it.
//
// This is NOT one of Amendment C's six matrix cells (design decision 4's
// literal text: "not a matrix cell — a corruption path"). A caller that
// receives this error MUST deny and MUST NEVER fall through to legacy
// authorization, even when an otherwise-valid legacy receipt exists for the
// identical lineage id.
type NewLineageMarkerCorruptedError struct {
	LineageID string
	Cause     error
}

func (err *NewLineageMarkerCorruptedError) Error() string {
	return fmt.Sprintf("new-lineage authority marker %q is present but its record could not be read: %v", err.LineageID, err.Cause)
}

func (err *NewLineageMarkerCorruptedError) Unwrap() error { return err.Cause }

// DiscoverNewLineage resolves the v3 authority record for one candidate,
// scoped by an explicit lineage marker (the caller's --lineage flag; the
// only way a v3 record is ever named without a full-inventory scan this
// slice does not add, matching the discovery rule's own construction).
//
// found=false, err=nil means truly absent: either no marker was supplied at
// all, or the v3/<lineage> directory does not exist — the {new absent}
// matrix cell, unreachable except by this exact construction. A non-nil err
// is always *NewLineageMarkerCorruptedError: the directory (the marker)
// exists, but review-state.json could not be parsed into a valid record.
func DiscoverNewLineage(ctx context.Context, repo, lineage string) (NewLineageRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return NewLineageRecord{}, false, err
	}
	if strings.TrimSpace(lineage) == "" {
		return NewLineageRecord{}, false, nil
	}
	store, err := NewLineageAuthorityStore(ctx, repo, lineage)
	if err != nil {
		// An unresolvable store root (a malformed lineage id, an
		// unreadable Git common dir) names no marker at all — the
		// caller's own root/lineage validation reports the real cause;
		// this is not new-lineage discovery-integrity damage.
		return NewLineageRecord{}, false, nil
	}
	marker := false
	if info, statErr := os.Stat(store.Dir); statErr == nil && info.IsDir() {
		marker = true
	}
	record, loadErr := store.Load()
	if loadErr != nil {
		if os.IsNotExist(loadErr) && !marker {
			return NewLineageRecord{}, false, nil
		}
		return NewLineageRecord{}, false, &NewLineageMarkerCorruptedError{LineageID: lineage, Cause: loadErr}
	}
	return record, true, nil
}

// GoverningAuthorityKind is ResolveGoverningAuthority's exact three-value
// outcome vocabulary (design decision 4).
type GoverningAuthorityKind string

const (
	// GoverningAuthorityKindNew: the v3 authority governs; legacy is
	// ignored regardless of its own presence.
	GoverningAuthorityKindNew GoverningAuthorityKind = "new"
	// GoverningAuthorityKindLegacy: defer unchanged to legacy discovery —
	// byte-identical to today's behavior.
	GoverningAuthorityKindLegacy GoverningAuthorityKind = "legacy"
	// GoverningAuthorityKindDeny: a new-lineage candidate is never
	// authorized by a legacy receipt.
	GoverningAuthorityKindDeny GoverningAuthorityKind = "deny"
)

// ResolveGoverningAuthority is Amendment C's pure decision matrix (design
// decision 4): the full 2×2 over {new: related | unrelated | absent} ×
// {legacy: present | absent}. found and relation together encode the "new"
// axis — found=false is `absent`; found=true with relation ==
// ShadowRelationUnrelated is `unrelated`; found=true with any other
// relation (including ambiguous/unknown, which ReviewCore.validate itself
// escalates) is `related`. legacyPresent encodes the "legacy" axis and is
// only ever consulted for the `unrelated` case — it is not needed to reach
// `new` (new always governs regardless of legacy) or `legacy` (nothing new
// exists to prefer over it).
//
// This function performs no I/O and mutates nothing: every input is already
// resolved by the caller. The discovery-integrity corruption path is
// explicitly NOT one of this matrix's cells — DiscoverNewLineage's
// *NewLineageMarkerCorruptedError is handled entirely by the caller before
// this function is ever reached.
func ResolveGoverningAuthority(found bool, relation CandidateRelation, legacyPresent bool) GoverningAuthorityKind {
	if !found {
		return GoverningAuthorityKindLegacy
	}
	if relation == ShadowRelationUnrelated {
		if legacyPresent {
			// Amendment C: "a new-lineage candidate is never authorized
			// by legacy" — a v3 record exists for this exact lineage id
			// but does not relate to the live candidate, and a legacy
			// receipt independently claims the same id. Naming that
			// legacy receipt as governing would silently let a stale
			// v3 marker fall back to an authority system it was never
			// reviewed under.
			return GoverningAuthorityKindDeny
		}
		return GoverningAuthorityKindLegacy
	}
	return GoverningAuthorityKindNew
}
