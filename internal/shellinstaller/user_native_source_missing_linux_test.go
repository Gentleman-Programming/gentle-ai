//go:build linux && amd64

package shellinstaller

import (
	"context"
	"testing"
)

func TestUserNativeSourceMissingFactory(t *testing.T) {
	ctx := context.Background()
	artifact, data := userMainCustodyFixture(t) // No shared runtime in this DATA.
	root := userMainCustodyStage(t)
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	before, err := userTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if w, err := userNativeRuntimeFromSnapshot(ctx, root, artifact); err == nil || w != nil {
		t.Fatal("archive custody alone supplied a missing native API")
	}
	after, err := userTreeStamp(root)
	if err != nil || before != after {
		t.Fatal("missing API refusal changed preimage", err)
	}
}
