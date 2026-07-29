package cli

import "github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"

// reviewSelfRecoveryShape is a closed enumeration of the deterministic
// triggering states a self-derived recovery reason may name. It is
// deliberately not a free-text string: only the constants below exist, so
// the CAS audit trail can never carry operator-invented prose in this field.
type reviewSelfRecoveryShape string

const (
	// reviewSelfRecoveryShapeRecoverInvalidated names a StateInvalidated
	// predecessor: recovery always requires a fresh successor lineage.
	reviewSelfRecoveryShapeRecoverInvalidated reviewSelfRecoveryShape = "recover_invalidated"
	// reviewSelfRecoveryShapeRecoverCorrectionRequired names a
	// StateCorrectionRequired predecessor with a recover action available.
	reviewSelfRecoveryShapeRecoverCorrectionRequired reviewSelfRecoveryShape = "recover_correction_required"
	// reviewSelfRecoveryShapeRecoverApproved names a StateApproved
	// predecessor with a recover action available (e.g. an approved staged
	// scope expansion).
	reviewSelfRecoveryShapeRecoverApproved reviewSelfRecoveryShape = "recover_approved"
	// reviewSelfRecoveryShapeRecoverEscalatedChangedTarget names a
	// StateEscalated predecessor being recovered against a genuinely
	// changed target.
	reviewSelfRecoveryShapeRecoverEscalatedChangedTarget reviewSelfRecoveryShape = "recover_escalated_changed_target"
	// reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly names a
	// StateEscalated predecessor being recovered against its unchanged
	// target, eligible only when the escalation was a pure budget crossing.
	reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly reviewSelfRecoveryShape = "recover_escalated_accounting_only"
)

// reviewSelfRecoveryReason renders the fixed, machine-generated reason for a
// closed self-recovery shape. Free text is banned by construction: the only
// inputs this function accepts are the named constants above, and an
// unrecognized shape resolves to empty rather than echoing its caller's
// input, so a caller can never smuggle unregistered prose into the CAS audit
// trail through this seam.
func reviewSelfRecoveryReason(shape reviewSelfRecoveryShape) string {
	switch shape {
	case reviewSelfRecoveryShapeRecoverInvalidated:
		return "self-recovery: invalidated authority, fresh successor lineage required"
	case reviewSelfRecoveryShapeRecoverCorrectionRequired:
		return "self-recovery: correction-required authority, recover action available"
	case reviewSelfRecoveryShapeRecoverApproved:
		return "self-recovery: approved authority, recover action available"
	case reviewSelfRecoveryShapeRecoverEscalatedChangedTarget:
		return "self-recovery: escalated authority, target changed"
	case reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly:
		return "self-recovery: escalated authority, accounting-only budget crossing"
	default:
		return ""
	}
}

// reviewSelfRecoveryShapeForRecover derives the self-recovery shape for a
// `review recover` predecessor from data already loaded by the caller: its
// current state, and whether the freshly built live target differs from the
// predecessor's original target identity. It never gates legality — that
// remains entirely reviewtransaction.RecoverCompactAuthority's job, unchanged
// by self-derivation — it only picks which fixed reason names the state for
// the CAS audit trail. A predecessor state outside this closed set (for
// example an ACTIVE StateReviewing attempt) is not a derivable shape: ok is
// false and the caller must fall back to requiring explicit --actor,
// --reason, and --maintainer-authorization, exactly as before self-derivation
// existed.
func reviewSelfRecoveryShapeForRecover(predecessor reviewtransaction.State, targetChanged bool) (reviewSelfRecoveryShape, bool) {
	// The closed set of self-deriving predecessor states is owned by
	// reviewtransaction.RecoverySelfDerivedInputs, which denial diagnostics
	// also consult when they decide what to ask an operator for. Consulting it
	// here keeps the promise and the demand from drifting apart: a denial can
	// never omit an input this command then insists on.
	if len(reviewtransaction.RecoverySelfDerivedInputs(predecessor)) == 0 {
		return "", false
	}
	switch predecessor {
	case reviewtransaction.StateInvalidated:
		return reviewSelfRecoveryShapeRecoverInvalidated, true
	case reviewtransaction.StateCorrectionRequired:
		return reviewSelfRecoveryShapeRecoverCorrectionRequired, true
	case reviewtransaction.StateApproved:
		return reviewSelfRecoveryShapeRecoverApproved, true
	case reviewtransaction.StateEscalated:
		if targetChanged {
			return reviewSelfRecoveryShapeRecoverEscalatedChangedTarget, true
		}
		return reviewSelfRecoveryShapeRecoverEscalatedAccountingOnly, true
	default:
		return "", false
	}
}
