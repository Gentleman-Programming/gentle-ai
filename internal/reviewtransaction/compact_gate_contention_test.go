package reviewtransaction

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPrePRChainGateWaitsOutTransientContentionAndStillDecides is the reason
// the read-only gate retries automatically instead of pushing the wait to the
// caller: the whole final-authorization window is a re-derivation, so waiting
// cannot double-apply anything, and a controller that fans out gates should
// receive a verdict rather than a race report (1861).
func TestPrePRChainGateWaitsOutTransientContentionAndStillDecides(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 2)
	restoreLockTimings(t, 2*time.Second, 10*time.Millisecond)

	held, err := AcquireAuthorityFileLock(fixture.stores[0].lockPath)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = held.Release()
		close(released)
	}()

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	<-released
	if !attempted || got.Contended || got.Result != GateAllow {
		t.Fatalf("transiently contended chain gate = %#v, attempted %t; want the bounded wait to reach a verdict", got, attempted)
	}
}

// TestPrePRChainGateContentionIsANonVerdictNotInvalidated pins the exhausted
// case. A gate that never obtained a stable authority view examined nothing,
// so it must not publish `invalidated` -- that claims the receipt no longer
// covers the candidate and routes an operator to manual repair for a condition
// that clears itself.
func TestPrePRChainGateContentionIsANonVerdictNotInvalidated(t *testing.T) {
	fixture := newCompactPrePRChainFixture(t, 2)
	restoreLockTimings(t, 150*time.Millisecond, 10*time.Millisecond)

	held, err := AcquireAuthorityFileLock(fixture.stores[0].lockPath)
	if err != nil {
		t.Fatal(err)
	}

	got, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
	if !attempted || !got.Contended {
		_ = held.Release()
		t.Fatalf("exhausted chain gate = %#v, attempted %t; want an explicit non-verdict", got, attempted)
	}
	var timeout *AuthorityLockTimeoutError
	if !errors.As(got.Cause, &timeout) {
		_ = held.Release()
		t.Fatalf("exhausted chain gate cause = %v, want *AuthorityLockTimeoutError so the caller can classify it", got.Cause)
	}

	// Convergence: the identical call decides once contention ends.
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		converged, attempted := EvaluateCompactPrePRChain(context.Background(), fixture.repo, fixture.input())
		if !attempted || converged.Contended || converged.Result != GateAllow {
			t.Fatalf("sequential chain gate retry %d = %#v, attempted %t", attempt, converged, attempted)
		}
	}
}

func restoreLockTimings(t *testing.T, timeout, poll time.Duration) {
	t.Helper()
	originalTimeout, originalPoll := readOnlyStoreLockTimeout, readOnlyStoreLockPollInterval
	t.Cleanup(func() {
		readOnlyStoreLockTimeout, readOnlyStoreLockPollInterval = originalTimeout, originalPoll
	})
	readOnlyStoreLockTimeout, readOnlyStoreLockPollInterval = timeout, poll
}
