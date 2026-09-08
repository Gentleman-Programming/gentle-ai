package telemetry

import "os"

// IncrementCounter loads the telemetry state, applies mutate to its
// counters, and persists the result. It is used by call sites that only
// record activity and never attempt a send themselves (the review lifecycle
// records outcomes this way; the actual send happens later, opportunistically,
// from install, update, or sync). Callers treat a returned error as
// non-fatal: recording telemetry must never fail the operation that
// triggered it.
//
// The kill switches are evaluated here, centrally, before anything is
// touched on disk: a missing state file means nothing has opted out
// locally, an unreadable one fails safe (does nothing), and an opted-out
// host (env or persisted enabled:false) returns nil without ever calling
// EnsureState. This makes every call site gated by construction, including
// ones (e.g. `gentle-ai sync`) that call IncrementCounter directly instead
// of going through a CLI-side gate first.
func IncrementCounter(homeDir string, mutate func(*Counters)) error {
	preState, err := loadForDecision(homeDir)
	if err != nil {
		return nil
	}
	if !Decide(os.Getenv, preState).Enabled {
		return nil
	}
	return Update(homeDir, func(s *State) { mutate(&s.Counters) })
}

// IncrementSyncs records one successful `gentle-ai sync` run.
func IncrementSyncs(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.Syncs++ })
}

// IncrementSDDPhaseRuns records one successful `sdd-attempt finish|settle`.
func IncrementSDDPhaseRuns(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.SDDPhaseRuns++ })
}

// IncrementReviewsApproved records one review reaching the approved
// terminal state.
func IncrementReviewsApproved(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.ReviewsApproved++ })
}

// IncrementReviewsCorrection records one review opening a bounded
// correction.
func IncrementReviewsCorrection(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.ReviewsCorrection++ })
}

// IncrementReviewsEscalated records one review reaching the escalated
// terminal state.
func IncrementReviewsEscalated(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.ReviewsEscalated++ })
}
