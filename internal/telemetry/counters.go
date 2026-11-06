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
// Environment kill switches are evaluated before touching disk. Persisted
// policy is read under the state lock: otherwise a policy reader can overlap
// a Windows file replacement and silently skip an increment on a sharing
// violation. Missing state defaults to enabled; unreadable state fails safe
// and persisted opt-out leaves the state untouched. Only enabled callers
// initialize state. The lock covers policy, initialization, mutation and save.
func IncrementCounter(homeDir string, mutate func(*Counters)) error {
	if !Decide(os.Getenv, State{Enabled: true}).Enabled {
		return nil
	}
	unlock, err := lockState(homeDir)
	if err != nil {
		return err
	}
	defer unlock()

	preState, err := loadForDecision(homeDir)
	if err != nil {
		return nil
	}
	if !Decide(os.Getenv, preState).Enabled {
		return nil
	}
	// Do not call Update here: lockState is not re-entrant.
	s, err := EnsureState(homeDir)
	if err != nil {
		return err
	}
	mutate(&s.Counters)
	return Save(homeDir, s)
}

// IncrementSyncs records one successful `gentle-ai sync` run.
func IncrementSyncs(homeDir string) error {
	return IncrementCounter(homeDir, func(c *Counters) { c.Syncs++ })
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
