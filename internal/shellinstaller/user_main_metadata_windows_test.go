//go:build windows

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newMainMetadataStage(t *testing.T) string {
	t.Helper()
	stage := t.TempDir()
	if err := userWindowsPrivate(stage); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stage, "runtime", "archives"), 0700); err != nil {
		t.Fatal(err)
	}
	return stage
}

// Issue #6239: a Main component update that reuses the SDK already carries
// main.json and main.zip from the retired installation; staging them again
// failed with "file exists" (and would have written zero-value metadata).
func TestUserWindowsStageMainMetadataKeepsReusedSnapshot(t *testing.T) {
	stage := newMainMetadataStage(t)
	reused := map[string][]byte{"main.json": []byte(`{"reused":"descriptor"}`), "runtime/archives/main.zip": []byte("reused snapshot")}
	for name, data := range reused {
		if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(name)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := userWindowsStageMainMetadata(stage, true, userWindowsArtifact{}, nil); err != nil {
		t.Fatalf("reused Main snapshot was staged again: %v", err)
	}
	for name, want := range reused {
		got, err := os.ReadFile(filepath.Join(stage, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("reused %s changed: %q %v", name, got, err)
		}
	}
}

// A Main install or update that acquires the snapshot stages its descriptor
// and archive.
func TestUserWindowsStageMainMetadataWritesAcquiredSnapshot(t *testing.T) {
	stage := newMainMetadataStage(t)
	artifact, data := userWindowsOverlayMainFixture(t, "0d181e872b06c6067741513fd6d2f562deaad5c1")
	if err := userWindowsStageMainMetadata(stage, false, artifact, data); err != nil {
		t.Fatal(err)
	}
	descriptor, _ := json.Marshal(artifact)
	if got, err := os.ReadFile(filepath.Join(stage, "main.json")); err != nil || !bytes.Equal(got, descriptor) {
		t.Fatalf("staged main.json differs: %q %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(stage, "runtime", "archives", "main.zip")); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("staged main.zip differs: %v", err)
	}
}

// The real reuse-then-stage sequence of a Main component update that reuses
// the SDK keeps the carried snapshot pair for every non-SDK scope.
func TestUserWindowsMainComponentUpdateKeepsReusedSnapshotAfterReuse(t *testing.T) {
	for _, component := range []string{UserComponentSupervisor, UserComponentNode, UserComponentCodeGraph, UserComponentContext7} {
		t.Run(component, func(t *testing.T) {
			backup, _ := writeReuseBackup(t, "node.zip", "fd.zip", "rg.zip")
			carried := map[string][]byte{"main.json": []byte(`{"reused":"descriptor"}`), "runtime/archives/main.zip": []byte("reused main zip")}
			for name, data := range carried {
				if err := os.WriteFile(filepath.Join(backup, filepath.FromSlash(name)), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			stage := filepath.Join(t.TempDir(), "stage")
			for _, dir := range []string{"runtime/archives", "config"} {
				if err := os.MkdirAll(filepath.Join(stage, filepath.FromSlash(dir)), 0700); err != nil {
					t.Fatal(err)
				}
			}
			scope, err := userInstallScopeOf(UserInstallRequest{Scope: component})
			if err != nil {
				t.Fatal(err)
			}
			installed := &userWindowsManifest{MainCommit: "0d181e872b06c6067741513fd6d2f562deaad5c1", MainArchiveSHA: "installed-main-sha"}
			if err := userWindowsReuseComponents(context.Background(), stage, backup, installed, UserInstallRequest{}, scope, false); err != nil {
				t.Fatal(err)
			}
			if err := userWindowsStageMainMetadata(stage, scope.reused[UserComponentSDK], userWindowsArtifact{}, nil); err != nil {
				t.Fatalf("reused Main snapshot was staged again: %v", err)
			}
			for name, want := range carried {
				if got, err := os.ReadFile(filepath.Join(stage, filepath.FromSlash(name))); err != nil || !bytes.Equal(got, want) {
					t.Fatalf("carried %s differs: %q %v", name, got, err)
				}
			}
		})
	}
}
