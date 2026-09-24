package reviewtransaction

import (
	"errors"
	"fmt"
	"testing"
)

func TestCheckHistorySerializesBoundedAuthorityLifecycle(t *testing.T) {
	events := []HistoryEvent{
		historyEvent("start", HistoryStart, 1, 6, HistoryAbsent, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "created", "", "r1"),
		historyEvent("status", HistoryStatus, 2, 3, HistoryAbsent, HistoryAbsent, HistoryEffectNone, HistoryEffectNone, "start", "", ""),
		historyEvent("finalize", HistoryFinalize, 7, 10, HistoryReviewing, HistoryApproved, HistoryEffectNone, HistoryEffectPending, "approved", "r1", "r2"),
		historyEvent("validate", HistoryValidate, 8, 9, HistoryReviewing, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "blocked", "", "r1"),
		historyEvent("route", HistoryStatus, 11, 12, HistoryApproved, HistoryApproved, HistoryEffectPending, HistoryEffectPending, "reconcile", "", "r2"),
		historyEvent("reconcile", HistoryReconcile, 13, 14, HistoryApproved, HistoryApproved, HistoryEffectPending, HistoryEffectApplied, "applied", "", "r2"),
		historyEvent("allow", HistoryValidate, 15, 16, HistoryApproved, HistoryApproved, HistoryEffectApplied, HistoryEffectApplied, "allow", "", "r2"),
	}

	ordered, err := CheckHistory(events)
	if err != nil {
		t.Fatalf("CheckHistory() error = %v", err)
	}
	if len(ordered) != len(events) || ordered[0].InvocationID != "status" || ordered[1].InvocationID != "start" {
		t.Fatalf("linearization = %#v", ordered)
	}
}

func TestCheckHistoryRejectsImpossibleOrOversizedHistories(t *testing.T) {
	t.Run("future revision", func(t *testing.T) {
		event := historyEvent("status", HistoryStatus, 1, 2, HistoryAbsent, HistoryAbsent, HistoryEffectNone, HistoryEffectNone, "start", "", "future")
		if _, err := CheckHistory([]HistoryEvent{event}); err == nil {
			t.Fatal("CheckHistory() accepted a response observing a future revision")
		}
	})
	t.Run("finalize before start", func(t *testing.T) {
		finalize := historyEvent("finalize", HistoryFinalize, 1, 2, HistoryReviewing, HistoryApproved, HistoryEffectNone, HistoryEffectPending, "approved", "r1", "r2")
		start := historyEvent("start", HistoryStart, 3, 4, HistoryAbsent, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "created", "", "r1")
		if _, err := CheckHistory([]HistoryEvent{finalize, start}); err == nil {
			t.Fatal("CheckHistory() accepted an impossible real-time order")
		}
	})
	t.Run("input bound", func(t *testing.T) {
		if _, err := CheckHistory(make([]HistoryEvent, MaxOracleHistoryEvents+1)); !errors.Is(err, ErrOracleHistoryBound) {
			t.Fatalf("CheckHistory() error = %v, want ErrOracleHistoryBound", err)
		}
	})
}

func TestCheckHistoryKeepsLineagesIndependent(t *testing.T) {
	first := historyEvent("first", HistoryStart, 1, 2, HistoryAbsent, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "created", "", "a1")
	second := historyEvent("second", HistoryStart, 1, 2, HistoryAbsent, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "created", "", "b1")
	second.LineageID = "other-lineage"
	if _, err := CheckHistory([]HistoryEvent{first, second}); err != nil {
		t.Fatalf("CheckHistory() error = %v", err)
	}
}

func TestCheckHistoryConvergingPathsDoNotExhaustSearchBound(t *testing.T) {
	// event0 (start) and event1 (status on absent) both belong to "lineage-a".
	// If event0 runs first, "lineage-a" transitions to HistoryReviewing, making event1 impossible.
	// Therefore, any legal serialization must order event1 before event0.
	// Events 2..7 are independent and commute with event0.
	// When DFS explores event0 first, it traverses permutations of events 2..7 before backtracking.
	// Without canonical memoization of search states, traversal paths exceed MaxOracleSearchStates (4096).
	events := []HistoryEvent{
		historyEvent("start", HistoryStart, 1, 10, HistoryAbsent, HistoryReviewing, HistoryEffectNone, HistoryEffectNone, "created", "", "r1"),
	}
	events[0].LineageID = "lineage-a"
	for i := 1; i <= 6; i++ {
		ev := historyEvent(fmt.Sprintf("v%d", i), HistoryValidate, 1, 10, HistoryAbsent, HistoryAbsent, HistoryEffectNone, HistoryEffectNone, "blocked", "", "")
		ev.LineageID = fmt.Sprintf("lineage-%d", i)
		events = append(events, ev)
	}
	status := historyEvent("status", HistoryStatus, 1, 10, HistoryAbsent, HistoryAbsent, HistoryEffectNone, HistoryEffectNone, "start", "", "")
	status.LineageID = "lineage-a"
	events = append(events, status)

	ordered, err := CheckHistory(events)
	if err != nil {
		t.Fatalf("CheckHistory() error = %v, want successful serialization", err)
	}
	if len(ordered) != len(events) {
		t.Fatalf("linearization len = %d, want %d", len(ordered), len(events))
	}
	// Verify event1 (status) was ordered before event0 (start)
	statusIdx, startIdx := -1, -1
	for i, ev := range ordered {
		if ev.InvocationID == "status" {
			statusIdx = i
		}
		if ev.InvocationID == "start" {
			startIdx = i
		}
	}
	if statusIdx > startIdx {
		t.Fatalf("status at %d, start at %d; want status before start", statusIdx, startIdx)
	}
}

func historyEvent(id string, operation HistoryOperation, started, completed uint64, before, after HistoryAuthority, beforeEffect, afterEffect HistoryEffect, result, expected, observed string) HistoryEvent {
	return HistoryEvent{
		InvocationID: id, Actor: id, Operation: operation, LineageID: "lineage", IdempotencyKey: "request",
		Started: started, Completed: completed, ExpectedRevision: expected, ObservedRevision: observed, Result: result,
		BeforeAuthority: before, AfterAuthority: after, BeforeEffect: beforeEffect, AfterEffect: afterEffect,
	}
}
