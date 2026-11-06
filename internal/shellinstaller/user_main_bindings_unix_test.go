//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Refuse unbound Main before assets/Node/process setup. No fake stock verifier.
func TestUserMainBindingsRefusesBeforeProcess(t *testing.T) {
	for _, name := range []string{"missing source", "source changed", "wrong snapshot", "canceled"} {
		t.Run(name, func(t *testing.T) {
			artifact, data := userMainCustodyFixture(t)
			root := userMainCustodyStage(t)
			ctx := context.Background()
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			if name != "missing source" {
				if err := userStageMainSource(ctx, root, artifact); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "source changed":
				if err := os.WriteFile(filepath.Join(root, "runtime/main-source/package.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "wrong snapshot":
				artifact.SHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			case "canceled":
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, verify := range []func(context.Context, string, string, userSnapshotArtifact) error{
				userVerifyMainBindings, userVerifyMainSDKPreviewBindings,
			} {
				if err := verify(ctx, root, filepath.Join(filepath.Dir(root), "selected-final"), artifact); err == nil {
					t.Fatal("unbound Main accepted")
				}
			}
			after, err := userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("refusal changed private stage", err)
			}
		})
	}
}
