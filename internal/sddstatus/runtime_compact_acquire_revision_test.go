package sddstatus

import (
	"context"
	"strings"
	"testing"
)

// TestCompactAcquireExpectedRevisionValidatesCAS is the RED reproduction for
// #4160: reset/status guidance may point a caller at acquire's own
// --expected-revision after a mutation that moved the ledger, but acquire
// previously never even looked at ExpectedRevision -- it silently overwrote
// whatever the caller declared with the live revision before every fresh
// begin, so nothing distinguished a caller who declared the correct revision
// from one who declared a stale one.
func TestCompactAcquireExpectedRevisionValidatesCAS(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "acquire-cas-revision")
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}

	status, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("matching expected revision proceeds", func(t *testing.T) {
		result, err := store.Acquire(context.Background(), CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				ExpectedRevision: status.Revision, RequestID: "acquire-cas-match", WorkUnit: "cas-unit",
				EvidenceGoal: "prove a matching expected revision proceeds", MaxAttempts: 2, MaxChangedLines: 20,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != CompactStateProceed || result.Token == "" {
			t.Fatalf("matching expected-revision acquire = %#v", result)
		}
	})

	t.Run("stale expected revision blocks without mutation", func(t *testing.T) {
		store := mustRuntimeStore(t, repo, "acquire-cas-stale")
		before, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		beforeRecords := countRuntimeRecords(t, store.Dir)
		stale := runtimeTestHash('9')

		result, err := store.Acquire(context.Background(), CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				ExpectedRevision: stale, RequestID: "acquire-cas-stale", WorkUnit: "cas-unit",
				EvidenceGoal: "prove a stale expected revision is refused", MaxAttempts: 2, MaxChangedLines: 20,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != CompactStateBlocked || result.Reason != CompactBlockInvalidContinuation {
			t.Fatalf("stale expected-revision acquire = %#v", result)
		}
		// The same typed stale-revision shape every legacy verb's own
		// store.mutate returns, so the message names --expected-revision and
		// the live current revision rather than an opaque authority failure.
		if !strings.Contains(result.Detail, ErrRuntimeRevisionConflict.Error()) {
			t.Fatalf("stale expected-revision acquire detail = %q, want the typed revision-conflict shape %q", result.Detail, ErrRuntimeRevisionConflict.Error())
		}
		if !strings.Contains(result.Detail, "--expected-revision") || !strings.Contains(result.Detail, before.Revision) {
			t.Fatalf("stale expected-revision acquire detail = %q, want it to name --expected-revision and the current revision %q", result.Detail, before.Revision)
		}

		after, err := store.Status()
		if err != nil {
			t.Fatal(err)
		}
		if after.Revision != before.Revision || countRuntimeRecords(t, store.Dir) != beforeRecords {
			t.Fatalf("stale expected-revision acquire mutated the ledger: before=%q after=%q records before=%d after=%d",
				before.Revision, after.Revision, beforeRecords, countRuntimeRecords(t, store.Dir))
		}
	})

	t.Run("empty expected revision is unchanged (backward compatible)", func(t *testing.T) {
		store := mustRuntimeStore(t, repo, "acquire-cas-empty")
		result, err := store.Acquire(context.Background(), CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				RequestID: "acquire-cas-empty", WorkUnit: "cas-unit",
				EvidenceGoal: "prove an absent expected revision is still admitted", MaxAttempts: 2, MaxChangedLines: 20,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != CompactStateProceed || result.Token == "" {
			t.Fatalf("empty expected-revision acquire = %#v", result)
		}
	})
}

// TestCompactAcquireTokenAndExpectedRevisionMustAgree covers #4160's second
// clause: acquire now accepts both --token (ownership-continuation proof) and
// --expected-revision (CAS input), and a caller naming two different ledger
// states at once gets a refusal instead of an ambiguous silent pick between
// them.
func TestCompactAcquireTokenAndExpectedRevisionMustAgree(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	store := mustRuntimeStore(t, repo, "acquire-token-revision-agree")
	if err := store.ensureDirectories(); err != nil {
		t.Fatal(err)
	}
	parent, err := store.Begin(context.Background(), BeginAttemptRequest{
		RequestID: "acquire-agree-parent", WorkUnit: "agree-unit",
		EvidenceGoal: "prove token and expected-revision must agree", MaxAttempts: 2, MaxChangedLines: 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("matching token and expected revision proceed", func(t *testing.T) {
		result, err := store.Acquire(context.Background(), CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				ExpectedRevision: parent.Revision, RequestID: "acquire-agree-actor", WorkUnit: "agree-unit",
				EvidenceGoal: "prove token and expected-revision must agree", MaxAttempts: 2, MaxChangedLines: 20,
			},
			Token: parent.Revision,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != CompactStateProceed || result.Token != parent.Revision {
			t.Fatalf("agreeing token/expected-revision acquire = %#v", result)
		}
	})

	t.Run("disagreeing token and expected revision are refused before any ledger read", func(t *testing.T) {
		_, err := store.Acquire(context.Background(), CompactAcquireRequest{
			BeginAttemptRequest: BeginAttemptRequest{
				ExpectedRevision: runtimeTestHash('7'), RequestID: "acquire-agree-conflict", WorkUnit: "agree-unit",
				EvidenceGoal: "prove token and expected-revision must agree", MaxAttempts: 2, MaxChangedLines: 20,
			},
			Token: parent.Revision,
		})
		if err == nil {
			t.Fatal("disagreeing token/expected-revision acquire = nil error, want a refusal")
		}
		if !strings.Contains(err.Error(), "token") || !strings.Contains(err.Error(), "expected") {
			t.Fatalf("disagreeing token/expected-revision refusal = %q, want it to name both flags", err.Error())
		}
	})
}
