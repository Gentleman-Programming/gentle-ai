//go:build linux && amd64

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserNativeSourceBinding(t *testing.T) {
	artifact, data := userNativeSourceFixture(t)
	for _, name := range []string{"unchanged", "descriptor mismatch", "retained archive", "import changed", "extra", "missing", "hardlink", "symlink", "canceled"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
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
			bound := artifact
			if name == "descriptor mismatch" {
				bound.SHA = strings.Repeat("b", 64)
			}
			w, err := userNativeRuntimeFromSnapshot(ctx, root, bound)
			if name == "descriptor mismatch" {
				if err == nil || w != nil {
					t.Fatal("mismatched descriptor reached native runtime")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			after, err := userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("source binding changed the unpublished stage", err)
			}
			if name == "unchanged" {
				bound.SHA = strings.Repeat("b", 64)
				if err := w.nativeSourceCheck(ctx); err != nil {
					t.Fatal("caller mutation rebound sealed source", err)
				}
				return // Fixture bytes are DATA, never executable evidence.
			}
			if name == "descriptor mismatch" {
				return
			}
			member := filepath.Join(root, "runtime/main-source/scripts/dependency.mjs")
			var mutation error
			switch name {
			case "retained archive":
				mutation = os.WriteFile(filepath.Join(root, "runtime/main/main.zip"), []byte("bad"), 0600)
			case "import changed":
				mutation = os.WriteFile(member, []byte("changed"), 0600)
			case "extra":
				mutation = os.WriteFile(filepath.Join(root, "runtime/main-source/extra"), nil, 0600)
			case "missing":
				mutation = os.Remove(member)
			case "hardlink":
				mutation = os.Link(member, filepath.Join(filepath.Dir(root), "alias"))
			case "symlink":
				mutation = os.Remove(member)
				if mutation == nil {
					mutation = os.Symlink(filepath.Join(root, "runtime/main-source/package.json"), member)
				}
			case "canceled":
				stopped, cancel := context.WithCancel(ctx)
				cancel()
				ctx = stopped
			}
			if mutation != nil {
				t.Fatal(mutation)
			}
			before, err = userTreeStamp(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Prepare(ctx, true); err == nil {
				t.Fatal("source refusal did not precede prerequisites")
			}
			if _, err := os.Lstat(filepath.Join(root, "runtime/native-bootstrap")); !os.IsNotExist(err) {
				t.Fatal("source refusal created bootstrap tooling", err)
			}
			_, collectErr := w.invokeWithEnvironment(ctx, "collect", nil, false, nil)
			userAssertNativeSourceRefusal(t, collectErr, name == "canceled")
			// This inert plan tests source refusal, never installation/plan health.
			_, _, runErr := w.RunWithLocation(ctx, NativeInstallerPlan{owner: w, data: json.RawMessage(`{}`)}, true)
			userAssertNativeSourceRefusal(t, runErr, name == "canceled")
			after, err = userTreeStamp(root)
			if err != nil || before != after {
				t.Fatal("source refusal changed preimage", err)
			}
		})
	}
}

func userAssertNativeSourceRefusal(t *testing.T, err error, canceled bool) {
	t.Helper()
	if canceled {
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation cause lost", err)
		}
		return
	}
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "refused" || !errors.Is(err, errNativeSourceSnapshot) {
		t.Fatal("expected complete-source refusal, not downstream runtime failure", err)
	}
}

func userNativeSourceFixture(t *testing.T) (userSnapshotArtifact, []byte) {
	t.Helper()
	commit := strings.Repeat("a", 40)
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, name := range []string{"package.json", "bin/entry.mjs", "scripts/installer-runtime.mjs", "scripts/dependency.mjs"} {
		stream, err := writer.Create("gentle-shell-" + commit + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stream.Write([]byte("fixture DATA only")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	artifact, err := userMainDescriptor(commit, fmt.Sprintf("%x", sha256.Sum256(data.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	return artifact, data.Bytes()
}
