package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// A filesystem root is never an SDD workspace, and answering as if it might be
// is worse than refusing.
//
// #2790's Windows reporter had a failure continuation resolve its working
// directory to the drive root. `sdd-status` did not refuse it. It reported a
// successful, empty, entirely plausible status — `changeName: null`,
// `artifactStore: openspec`, `planningHome.path: "/openspec"` — so the operator
// read "SDD lost my project" instead of "that command was pointed at the wrong
// directory". The Engram reporter on the same issue read the same empty answer
// as their change having vanished.
//
// The emitted continuation is the plugin's defect and our contract already
// forbids it. This is the other half, and it is ours: our binary produced that
// answer. No project lives at `/` or at `C:\`, so refusing costs nothing and
// there is no false positive to weigh.

func TestResolveWorkspaceRootRefusesAFilesystemRoot(t *testing.T) {
	root := filepath.VolumeName(mustAbs(t, string(filepath.Separator))) + string(filepath.Separator)

	_, err := resolveWorkspaceRoot(ResolveOptions{CWD: root})
	if err == nil {
		t.Fatalf("resolveWorkspaceRoot(%q) succeeded; a filesystem root is never a workspace, and answering with an empty status reads as the project having vanished (#2790)", root)
	}
	// The operator has to learn what actually happened, which is that something
	// handed this command the wrong directory.
	for _, want := range []string{root, "--cwd"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal does not name %q:\n%v", want, err)
		}
	}
}

func TestResolveWorkspaceRootRefusesAFilesystemRootAsExplicitWorkspace(t *testing.T) {
	root := filepath.VolumeName(mustAbs(t, string(filepath.Separator))) + string(filepath.Separator)

	if _, err := resolveWorkspaceRoot(ResolveOptions{WorkspaceRoot: root}); err == nil {
		t.Fatal("an explicit workspace root pointing at the filesystem root was accepted")
	}
}

// TestResolveWorkspaceRootStillAcceptsOrdinaryDirectories keeps the guard
// narrow. It must reject exactly one shape, not "directories that look
// unpromising": a temp dir with no openspec tree is a perfectly ordinary
// starting point and every existing caller depends on it resolving.
func TestResolveWorkspaceRootStillAcceptsOrdinaryDirectories(t *testing.T) {
	dir := t.TempDir()
	resolved, err := resolveWorkspaceRoot(ResolveOptions{CWD: dir})
	if err != nil {
		t.Fatalf("resolveWorkspaceRoot(%q) = %v, want an ordinary directory to resolve", dir, err)
	}
	if resolved != mustAbs(t, dir) {
		t.Fatalf("resolved = %q, want %q", resolved, mustAbs(t, dir))
	}
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
