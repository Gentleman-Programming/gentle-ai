//go:build linux || darwin

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func userMainCustodyFixture(t *testing.T) (userSnapshotArtifact, []byte) {
	t.Helper()
	commit := strings.Repeat("a", 40)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"package.json", "bin/entry.mjs"} {
		stream, err := writer.Create("gentle-shell-" + commit + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write([]byte("source DATA, never executed: " + name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	artifact, err := userMainDescriptor(commit, fmt.Sprintf("%x", sha256.Sum256(data)))
	if err != nil {
		t.Fatal(err)
	}
	return artifact, data
}

func userMainCustodyStage(t *testing.T) string {
	t.Helper()
	root := installStagePath(t)
	if err := userPrepareInstallStage(context.Background(), root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUserMainRetainedCustody(t *testing.T) {
	artifact, data := userMainCustodyFixture(t)
	for _, name := range []string{"unchanged", "archive bytes", "descriptor bytes", "archive hard link", "descriptor hard link", "archive symlink", "extra member", "missing archive", "archive mode", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := userMainCustodyStage(t)
			if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(root, "runtime/main")
			archive := filepath.Join(base, "main.zip")
			var err error
			switch name {
			case "archive bytes":
				err = os.WriteFile(archive, []byte("tampered"), 0600)
			case "descriptor bytes":
				err = os.WriteFile(filepath.Join(base, "main.json"), []byte("{}"), 0600)
			case "archive hard link":
				err = os.Link(archive, filepath.Join(filepath.Dir(root), "archive-alias"))
			case "descriptor hard link":
				err = os.Link(filepath.Join(base, "main.json"), filepath.Join(filepath.Dir(root), "descriptor-alias"))
			case "archive symlink":
				err = os.Rename(archive, filepath.Join(filepath.Dir(root), "saved-archive"))
				if err == nil {
					err = os.Symlink(filepath.Join(filepath.Dir(root), "saved-archive"), archive)
				}
			case "extra member":
				err = os.WriteFile(filepath.Join(base, "extra"), nil, 0600)
			case "missing archive":
				err = os.Remove(archive)
			case "archive mode":
				err = os.Chmod(archive, 0644)
			case "canceled":
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			if err != nil {
				t.Fatal(err)
			}
			retained, err := userReadMainArchive(ctx, root, artifact)
			if name != "unchanged" {
				if err == nil || retained != nil {
					t.Fatal("unusable retained Main archive returned", name, err)
				}
				return
			}
			if err != nil || !bytes.Equal(retained, data) {
				t.Fatal("retained bytes differ", err)
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := userReadMainArchive(ctx, root, artifact); err != nil {
				t.Fatal(err)
			}
			after, err := userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("readback mutated stage", err)
			}
			final := filepath.Join(filepath.Dir(root), "selected-final")
			if err := privateDestination(final); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(root, final); err != nil {
				t.Fatal(err)
			}
			if got, err := userReadMainArchive(ctx, final, artifact); err != nil || !bytes.Equal(got, data) {
				t.Fatal("relocated retained archive differs", err)
			}
		})
	}
}

func TestUserMainStageRefusesWithoutReplacing(t *testing.T) {
	artifact, data := userMainCustodyFixture(t)
	for _, name := range []string{"bad digest", "canceled", "existing stage"} {
		t.Run(name, func(t *testing.T) {
			root := userMainCustodyStage(t)
			ctx := context.Background()
			if name == "existing stage" {
				if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
					t.Fatal(err)
				}
			}
			before, err := userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			selected := artifact
			if name == "bad digest" {
				selected.SHA = strings.Repeat("0", 64)
			}
			if name == "canceled" {
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			if err := userStageMainArchive(ctx, root, selected, data); err == nil {
				t.Fatal("unusable Main stage accepted")
			}
			after, err := userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("refusal changed preimage", err)
			}
		})
	}
}

func TestUserCodeGraphRetainedDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "part")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := userRetainedSource(context.Background(), path, 1)
	if err == nil || !strings.Contains(err.Error(), "CodeGraph source part custody differs") {
		t.Fatal("historical CodeGraph diagnostic changed", err)
	}
}
