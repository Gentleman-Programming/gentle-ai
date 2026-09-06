package assets

import (
	"io/fs"
	"strings"
	"testing"
)

// #3105: the automatic gate wording said every referenced path must exist and
// that a path which does not resolve FAILS the gate, with no carve-out for the
// paths a design plans to create. A valid sdd-design artifact that names the
// files apply will create was therefore failed by the gate. Every orchestrator
// copy that carries the gate must also carry the planned-path carve-out.
func TestEveryOrchestratorGateCarvesOutPlannedPaths(t *testing.T) {
	const gate = "FAILS the gate."
	const carveOut = "A path the artifact explicitly marks as planned (to be created by a later apply) is not required to exist yet; only paths claimed as already created or read must resolve."

	checked := 0
	err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content := MustRead(path)
		if !strings.Contains(content, gate) {
			return nil
		}
		checked++
		if !strings.Contains(content, carveOut) {
			t.Errorf("%s fails unresolved paths at the gate without the planned-path carve-out", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk assets: %v", err)
	}
	if checked == 0 {
		t.Fatal("no embedded asset carries the automatic gate wording; the guard is checking nothing")
	}
}
