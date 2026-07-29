package releasepolicy

import (
	"os"
	"path/filepath"
	"testing"
)

// TestValidateRunMarkerRejectsAMarkerInsideASymlinkedDist pins the same
// asymmetric-canonicalization class 1773 reports elsewhere. The marker was
// resolved with filepath.EvalSymlinks and the snapshot directory was not, so
// a dist that is itself a symlink made a marker living inside the snapshot
// directory look like a marker living outside it, and the guard that keeps the
// run identity out of the directory GoReleaser regenerates stopped guarding.
func TestValidateRunMarkerRejectsAMarkerInsideASymlinkedDist(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realDist := filepath.Join(root, "real-dist")
	if err := os.MkdirAll(realDist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDist, filepath.Join(root, "dist")); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(realDist, "run-marker")
	if err := os.WriteFile(marker, []byte("run-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := validateRunMarker(root, marker, "run-1"); err == nil {
		t.Fatal("a marker inside the snapshot directory was accepted because dist is a symlink")
	}
}

// TestValidateRunMarkerAcceptsAMarkerOutsideDist keeps the guard a real
// decision: refusing a marker inside dist must not refuse every marker.
func TestValidateRunMarkerAcceptsAMarkerOutsideDist(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "run-marker")
	if err := os.WriteFile(marker, []byte("run-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := validateRunMarker(root, marker, "run-1"); err != nil {
		t.Fatalf("a marker outside the snapshot directory was refused: %v", err)
	}
}

// TestValidateRunMarkerRejectsAMarkerDirectlyInsideDist is the case the
// lexical comparator already caught, kept so the identity rewrite cannot lose
// it.
func TestValidateRunMarkerRejectsAMarkerDirectlyInsideDist(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(root, "dist")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dist, "run-marker")
	if err := os.WriteFile(marker, []byte("run-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := validateRunMarker(root, marker, "run-1"); err == nil {
		t.Fatal("a marker directly inside the snapshot directory was accepted")
	}
}
