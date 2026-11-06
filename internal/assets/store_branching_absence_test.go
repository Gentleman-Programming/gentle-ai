package assets

import (
	"io/fs"
	"strings"
	"testing"
)

// #3814: the shipped prose used to document the defect as the contract. It told
// actors that the native dispatcher "always emits artifactStore: openspec" and
// that for an Engram store they must skip the dispatcher and resolve artifacts
// by hand. That instruction is the origin of the store branching in every phase
// agent, and the native surface now resolves the declared store itself.
//
// This guard exists because the same contract lives in twelve hand-maintained
// orchestrator copies plus the per-runtime agent sets: the first correction of
// this prose reached one runtime and left ten carrying the contradicted text.
func TestNoAssetInstructsActorsToBypassTheDispatcher(t *testing.T) {
	forbidden := []string{
		"do NOT invoke the dispatcher",
		"do not invoke the dispatcher",
		"always emits `artifactStore: openspec`",
		"cannot observe Engram-backed",
	}

	err := fs.WalkDir(FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content := MustRead(path)
		for _, phrase := range forbidden {
			if strings.Contains(content, phrase) {
				t.Errorf("%s instructs actors to bypass or distrust the native dispatcher: %q", path, phrase)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk assets: %v", err)
	}
}
