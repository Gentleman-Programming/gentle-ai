// Package reviewtransaction — shadow observer seam tests (Wave 1, Slice 5).
// Covers tasks.md 5.1-5.5: Advisory-Only Never Blocking, Disable Switch Is
// the Rollback Boundary, Off by Default in Live Paths, stderr-only
// divergence output, and Zero Live-Lifecycle Behavior Change.
package reviewtransaction

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestShadowObserverDisabledIsANoOp is 5.2 + 5.3: when GENTLE_AI_RDD_SHADOW
// is unset or empty, ObserveShadowRelation must execute zero shadow work —
// no in-memory row is recorded and no stderr line is emitted — even though
// the call itself is reached (proven by the call-count hook, exactly the
// "test hook counter" the orchestrator's hard safety requirements call for).
func TestShadowObserverDisabledIsANoOp(t *testing.T) {
	for _, value := range []string{"", "   "} {
		t.Run("env="+value, func(t *testing.T) {
			t.Setenv(shadowObservationEnvVar, value)
			shadowResetObserverForTest()
			var stderr bytes.Buffer
			originalStderr := shadowObserverStderr
			shadowObserverStderr = &stderr
			t.Cleanup(func() { shadowObserverStderr = originalStderr })

			ObserveShadowRelation(context.Background(), "/nonexistent/repo", GatePostApply,
				"base", "candidate", "digest", "policy", Snapshot{}, "policy", GateAllow, nil, nil)

			if got := shadowObserverCallCountForTest(); got != 1 {
				t.Fatalf("shadowObserverCallCount = %d, want 1 (the hook itself must still be reached)", got)
			}
			if rows := shadowObserverRowsForTest(); len(rows) != 0 {
				t.Fatalf("rows recorded while disabled: %#v", rows)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr written while disabled: %q", stderr.String())
			}
		})
	}
}

// TestShadowObserverAdvisoryFailureNeverBlocksLivePath is 5.1: an internal
// shadow failure (an unresolvable repository) must never panic, never
// return anything the caller could consume as a live decision, and must
// still record the failure as advisory evidence rather than silently
// disappearing.
func TestShadowObserverAdvisoryFailureNeverBlocksLivePath(t *testing.T) {
	t.Setenv(shadowObservationEnvVar, "1")
	shadowResetObserverForTest()
	var stderr bytes.Buffer
	originalStderr := shadowObserverStderr
	shadowObserverStderr = &stderr
	t.Cleanup(func() { shadowObserverStderr = originalStderr })

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("ObserveShadowRelation panicked: %v", recovered)
			}
		}()
		ObserveShadowRelation(context.Background(), t.TempDir(), GatePreCommit,
			"base", "candidate", "digest", "policy", Snapshot{}, "policy", GateAllow, nil, nil)
	}()

	rows := shadowObserverRowsForTest()
	if len(rows) != 1 {
		t.Fatalf("rows = %#v, want exactly 1 advisory row even on failure", rows)
	}
	if rows[0].Err == "" {
		t.Fatalf("row = %#v, want a recorded advisory error for an unresolvable repository", rows[0])
	}
}

// TestShadowObserverEnabledWritesStderrOnlyNeverStdout is 5.4: the
// structured divergence line goes to shadowObserverStderr only — stdout is
// reserved for contract JSON (design decision 3) and must never see it.
func TestShadowObserverEnabledWritesStderrOnlyNeverStdout(t *testing.T) {
	t.Setenv(shadowObservationEnvVar, "1")
	shadowResetObserverForTest()
	repo := initSnapshotRepo(t)

	var stderr bytes.Buffer
	originalStderr := shadowObserverStderr
	shadowObserverStderr = &stderr
	t.Cleanup(func() { shadowObserverStderr = originalStderr })

	realStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = realStdout })

	ObserveShadowRelation(context.Background(), repo, GatePostApply,
		"base", "candidate", "digest", "policy", Snapshot{}, "policy", GateAllow, nil, nil)

	writer.Close()
	os.Stdout = realStdout
	var stdoutCapture bytes.Buffer
	if _, err := stdoutCapture.ReadFrom(reader); err != nil {
		t.Fatal(err)
	}

	if stdoutCapture.Len() != 0 {
		t.Fatalf("stdout received shadow output: %q", stdoutCapture.String())
	}
	if !strings.Contains(stderr.String(), "gentle-ai.rdd-shadow/v1") {
		t.Fatalf("stderr = %q, want the structured divergence line", stderr.String())
	}
}

// TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical is 5.5: the
// same candidate evaluated through the real live gate (EvaluateNativeGate,
// which is wired to call ObserveShadowRelation in production) must produce
// a byte-identical NativeGateEvaluation whether GENTLE_AI_RDD_SHADOW is
// unset or set — for both a passing and a failing scenario (orchestrator's
// hard safety requirement).
func TestShadowObservationSwitchIsRollbackBoundaryGateByteIdentical(t *testing.T) {
	t.Run("passing scenario", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		tx, receipt, request := nativeGateFixture(t, repo, "shadow-boundary-pass")
		store, err := AuthoritativeStore(context.Background(), repo, tx.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		appendApprovedStoreChain(t, store, tx)
		bundle, err := store.ExportBundle()
		if err != nil {
			t.Fatal(err)
		}
		request.StoreRevision, request.GenesisRevision, request.ChainIdentity, request.BundleDigest =
			bundle.HeadRevision, bundle.GenesisRevision, bundle.ChainIdentity, bundle.BundleDigest

		t.Setenv(shadowObservationEnvVar, "")
		resultOff := EvaluateNativeGate(context.Background(), repo, receipt, request)
		if resultOff.Result != GateAllow {
			t.Fatalf("baseline (shadow off) passing scenario = %#v, want GateAllow", resultOff)
		}

		t.Setenv(shadowObservationEnvVar, "1")
		resultOn := EvaluateNativeGate(context.Background(), repo, receipt, request)

		if resultOff.Result != resultOn.Result || resultOff.Reason != resultOn.Reason || !reflect.DeepEqual(resultOff.Context, resultOn.Context) {
			t.Fatalf("shadow off/on diverged on a passing scenario:\noff=%#v\non=%#v", resultOff, resultOn)
		}
	})

	t.Run("failing scenario", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		tx, receipt, request := nativeGateFixture(t, repo, "shadow-boundary-fail")
		store, err := AuthoritativeStore(context.Background(), repo, tx.LineageID)
		if err != nil {
			t.Fatal(err)
		}
		appendApprovedStoreChain(t, store, tx)
		bundle, err := store.ExportBundle()
		if err != nil {
			t.Fatal(err)
		}
		request.StoreRevision, request.GenesisRevision, request.ChainIdentity, request.BundleDigest =
			bundle.HeadRevision, bundle.GenesisRevision, bundle.ChainIdentity, bundle.BundleDigest
		// Force a denial: an intended-untracked path the authoritative
		// transaction never recorded.
		request.Target.IntendedUntracked = append(append([]string{}, request.Target.IntendedUntracked...), "never-authorized.txt")

		t.Setenv(shadowObservationEnvVar, "")
		resultOff := EvaluateNativeGate(context.Background(), repo, receipt, request)
		if resultOff.Result != GateInvalidated {
			t.Fatalf("baseline (shadow off) failing scenario = %#v, want GateInvalidated", resultOff)
		}

		t.Setenv(shadowObservationEnvVar, "1")
		resultOn := EvaluateNativeGate(context.Background(), repo, receipt, request)

		if resultOff.Result != resultOn.Result || resultOff.Reason != resultOn.Reason || !reflect.DeepEqual(resultOff.Context, resultOn.Context) {
			t.Fatalf("shadow off/on diverged on a failing scenario:\noff=%#v\non=%#v", resultOff, resultOn)
		}
	})
}
