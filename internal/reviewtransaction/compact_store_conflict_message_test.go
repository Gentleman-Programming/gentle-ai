package reviewtransaction

import "testing"

// Issue #3935: the conflict refusal names the immutable field it refused on
// as a complete sentence instead of the truncated "at state" tail.
func TestCompactAtomicStartConflictErrorNamesImmutableField(t *testing.T) {
	err := &CompactAtomicStartConflictError{LineageID: "lineage-a", Field: "state"}
	want := `compact atomic START conflicts with active lineage "lineage-a" on immutable field "state"`
	if got := err.Error(); got != want {
		t.Fatalf("conflict message = %q, want %q", got, want)
	}
}
