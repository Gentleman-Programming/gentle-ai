//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserMainSourceMaterialization(t *testing.T) {
	artifact, data := userMainCustodyFixture(t) // DATA, never executed.
	for _, name := range []string{"unchanged", "extra file", "extra directory", "changed file", "missing file", "hard link", "directory alias", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := userMainCustodyStage(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			// Protected sibling fixtures are not an authenticated distribution.
			protected := filepath.Join(root, "prefix")
			if err := os.Mkdir(protected, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(protected, "stock-marker"), []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			preimage, err := userTreeStamp(protected)
			if err != nil {
				t.Fatal(err)
			}
			if err := userStageMainSource(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "runtime/main-source")
			var mutation error
			switch name {
			case "extra file":
				mutation = os.WriteFile(filepath.Join(source, "extra"), nil, 0600)
			case "extra directory":
				mutation = os.Mkdir(filepath.Join(source, "extra"), 0700)
			case "changed file":
				mutation = os.WriteFile(filepath.Join(source, "package.json"), []byte("bad"), 0600)
			case "missing file":
				mutation = os.Remove(filepath.Join(source, "package.json"))
			case "hard link":
				mutation = os.Link(filepath.Join(source, "package.json"), filepath.Join(filepath.Dir(root), "alias"))
			case "directory alias":
				mutation = os.Rename(filepath.Join(source, "bin"), filepath.Join(filepath.Dir(root), "saved-bin"))
				if mutation == nil {
					mutation = os.Symlink(filepath.Join(filepath.Dir(root), "saved-bin"), filepath.Join(source, "bin"))
				}
			case "canceled":
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			if mutation != nil {
				t.Fatal(mutation)
			}
			err = userCompareMainSource(ctx, root, artifact)
			if (err != nil) != (name != "unchanged") {
				t.Fatal("Main source admission differs", name, err)
			}
			after, err := userTreeStamp(protected)
			if err != nil || preimage != after {
				t.Fatal("source materialization changed protected sibling", err)
			}
			if name == "unchanged" {
				before, err := userTreeStamp(root)
				if err != nil {
					t.Fatal(err)
				}
				if err := userCompareMainSource(ctx, root, artifact); err != nil {
					t.Fatal(err)
				}
				after, err := userTreeStamp(root)
				if err != nil || before != after {
					t.Fatal("readback mutated source stage", err)
				}
				final := filepath.Join(filepath.Dir(root), "selected-final")
				if err := privateDestination(final); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(root, final); err != nil {
					t.Fatal(err)
				}
				if err := userCompareMainSource(ctx, final, artifact); err != nil {
					t.Fatal("relocated source differs", err)
				}
			}
		})
	}
}

func TestUserMainSourceRefusesWithoutReplacing(t *testing.T) {
	artifact, data := userMainCustodyFixture(t)
	for _, name := range []string{"existing source", "canceled", "bad retained archive"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := userMainCustodyStage(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			if name == "existing source" {
				if err := userStageMainSource(ctx, root, artifact); err != nil {
					t.Fatal(err)
				}
			}
			if name == "bad retained archive" {
				if err := os.WriteFile(filepath.Join(root, "runtime/main/main.zip"), []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			if name == "canceled" {
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			if err := userStageMainSource(ctx, root, artifact); err == nil {
				t.Fatal("unusable source stage accepted")
			}
			after, err := userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("refusal changed preimage", err)
			}
		})
	}
}
