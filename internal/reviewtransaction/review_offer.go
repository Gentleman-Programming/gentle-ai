// Package reviewtransaction — the unwired Wave-4 offer API (design decision
// 8, Wave 3 Slice 5, task 6.5). OfferReviewAfterVerify is the exact shape
// Wave 4's post-verify sequence change will call; nothing in this wave calls
// it. Shipping the shape a wave early — rather than a temporary call site or
// deferring the API itself to Wave 4 — was decision 8's own explicit choice:
// a call site would be Wave 4's sequence change happening early, and the
// ratchet cannot see tests (it analyses reachability from ./cmd/gentle-ai
// only), so an honest baselined `.deadcode-baseline.txt` entry is the
// documented cost of shipping the shape now (task 6.6).
package reviewtransaction

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// OfferRequest names the one candidate an offer decision would be made for.
// It is deliberately minimal: Wave 4 composes the real decision (lens/tier
// selection, receipt discovery) and will widen this request shape then, not
// here — an unwired API must not pre-guess fields no caller yet needs.
type OfferRequest struct {
	LineageID string
}

// Offer is OfferReviewAfterVerify's exact result shape. Available false
// never means "denied" — it means no offer is made for this call; Wave 4's
// wiring is what turns this into a genuinely actionable choice.
type Offer struct {
	Available bool
	LineageID string
}

// OfferReviewAfterVerify is Wave 4's post-verify offer point (design decision
// 8's literal signature): after a candidate's independent verification
// evidence exists, this is where a future caller will ask whether receipt-
// driven development should now offer a review for it. It is unwired: no
// production call site in this wave reaches it.
//
// The one behavior this unwired shape is REQUIRED to get right (design
// decision 8, task 6.5): the user-owned kill switch, when recorded off at the
// GLOBAL scope, must return Offer{Available: false}, nil BEFORE any
// repository read. That ordering is provable here specifically because the
// clone-local override is off-only (SetCloneLocalRDDMode's own
// ErrRDDModeRepositoryForcedOn invariant) — a clone can never override a
// global-off switch back to on — so a global-only reading is on its own a
// complete, honest answer for the disabled case, with nothing left for a
// repository read to add. Every other case (global on, global unset)
// conservatively reports unavailable too: Wave 4 is what composes the real
// decision (receipt discovery, lens/tier composition) that could turn this
// into a genuine offer; this shape never fabricates one early.
func OfferReviewAfterVerify(ctx context.Context, repo string, request OfferRequest) (Offer, error) {
	if err := ctx.Err(); err != nil {
		return Offer{}, err
	}
	// repo is deliberately unread on every path below: the disabled-kill-
	// switch behavior this shape must prove never touches it, and no other
	// path in this wave composes a repository-dependent decision either.
	global, err := readGlobalRDDModeForOffer()
	if err != nil {
		return Offer{}, err
	}
	if mode, modeErr := normalizeRDDMode(global.Value); modeErr == nil && mode == RDDModeOff {
		return Offer{Available: false, LineageID: request.LineageID}, nil
	}
	return Offer{Available: false, LineageID: request.LineageID}, nil
}

// readGlobalRDDModeForOffer reads only the uncommitted global kill-switch
// value, never the repository. It intentionally duplicates
// internal/cli's readGlobalRDDMode rather than calling it: reviewtransaction
// is the lower layer in this wave's package boundary (design's own note on
// FreezeCandidateIdentity/authorizeReviewStart — cli-owned reuse is called
// FROM cli INTO reviewtransaction, never the reverse), so this package
// cannot import internal/cli without a cycle. Both read exactly
// internal/state's persisted RDDMode field the same way.
func readGlobalRDDModeForOffer() (RDDGlobalMode, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return RDDGlobalMode{}, err
	}
	persisted, err := state.Read(home)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RDDGlobalMode{}, nil
		}
		return RDDGlobalMode{}, err
	}
	global := RDDGlobalMode{Value: persisted.RDDMode}
	if persisted.RDDModeRecordedAt != nil {
		global.RecordedAt = persisted.RDDModeRecordedAt.UTC()
	}
	return global, nil
}
