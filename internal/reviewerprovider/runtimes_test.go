package reviewerprovider

import (
	"slices"
	"testing"
)

// TestRegisteredRuntimeIdentitiesStableLexicalOrder pins the registered
// reviewer provider runtime identities to a non-empty, strictly sorted
// lexical list so consumers can rely on deterministic order.
func TestRegisteredRuntimeIdentitiesStableLexicalOrder(t *testing.T) {
	identities := RegisteredRuntimeIdentities()
	if len(identities) == 0 {
		t.Fatal("RegisteredRuntimeIdentities() returned empty slice, want non-empty supported runtimes")
	}
	for index := 1; index < len(identities); index++ {
		if identities[index-1] >= identities[index] {
			t.Fatalf("RegisteredRuntimeIdentities() is not strictly sorted: %q comes before %q", identities[index-1], identities[index])
		}
	}
	sorted := slices.Clone(identities)
	slices.Sort(sorted)
	if !slices.Equal(identities, sorted) {
		t.Fatalf("RegisteredRuntimeIdentities() = %q, want strictly sorted %q", identities, sorted)
	}
}
