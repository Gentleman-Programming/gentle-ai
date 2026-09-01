package sddstatus

import (
	"reflect"
	"testing"
)

// This file is the Wave 4 S2 guard for design.md decision 9 (maintainer-
// confirmed 2026-08-02): RuntimeObjective / BeginAttemptRequest is the sole
// owner of work-unit scope (WorkUnit, EvidenceGoal, MaxAttempts,
// MaxChangedLines). Before this slice, CompactAcquireRequest independently
// redeclared the same four fields — exactly the "two request structs over
// one concept" shape that produced issue #2133/#2151 (change-level Complete
// contradicting a work-unit-scoped objective). This test proves the
// duplication cannot silently reappear: CompactAcquireRequest must be a thin
// projection (an embedded BeginAttemptRequest plus nothing else of scope
// substance), never a parallel struct.

// TestRuntimeObjectiveIsSoleWorkUnitScopeOwner fails if CompactAcquireRequest
// redeclares any of BeginAttemptRequest's work-unit-scope fields
// (WorkUnit/EvidenceGoal/MaxAttempts/MaxChangedLines/RequestID/
// ExpectedRevision) outside the embed. A sibling field that is NOT part of
// that scope — like Token, #2291's optional zero-mutation ownership proof —
// is legitimate: it is orthogonal continuation metadata, not a second
// work-unit-scope owner, and CompactSettleRequest already carries the same
// Token concept alongside its own non-scope fields.
func TestRuntimeObjectiveIsSoleWorkUnitScopeOwner(t *testing.T) {
	acquireType := reflect.TypeOf(CompactAcquireRequest{})
	beginType := reflect.TypeOf(BeginAttemptRequest{})
	beginFieldNames := make(map[string]bool, beginType.NumField())
	for i := 0; i < beginType.NumField(); i++ {
		beginFieldNames[beginType.Field(i).Name] = true
	}

	embedded := false
	for i := 0; i < acquireType.NumField(); i++ {
		field := acquireType.Field(i)
		if field.Anonymous && field.Type == beginType {
			embedded = true
			continue
		}
		if beginFieldNames[field.Name] {
			t.Fatalf("CompactAcquireRequest.%s redeclares BeginAttemptRequest's work-unit scope outside the embed — BeginAttemptRequest is the single work-unit-scope owner per decision 9", field.Name)
		}
	}
	if !embedded {
		t.Fatal("CompactAcquireRequest must embed BeginAttemptRequest anonymously — BeginAttemptRequest is the single work-unit-scope owner per decision 9")
	}
}
