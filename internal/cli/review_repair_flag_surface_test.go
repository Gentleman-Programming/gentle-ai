package cli

import "testing"

func TestReviewRepairFlagSurfaceExcludesRemovedClassifiedFlags(t *testing.T) {
	surface, ok := ReviewOperationFlagSurface("repair")
	if !ok {
		t.Fatal("repair surface missing")
	}
	for _, flag := range []string{"class", "cause", "disposition", "repository-binding", "maintainer-authorization", "expected-revision"} {
		if surface[flag] {
			t.Fatalf("repair surface still advertises --%s", flag)
		}
	}
}
