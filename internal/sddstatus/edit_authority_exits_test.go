package sddstatus

import (
	"path/filepath"
	"strings"
	"testing"
)

// #2934: a checkbox line that names a sibling repository path as a read-only
// input is not an edit target. The `(read-only)` marker is the deterministic
// exit; the same line without it keeps blocking, and the blocked reason names
// the marker as the third exit next to the two it already names.
func TestReadOnlyMarkedSiblingPathDoesNotBlockApply(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	serviceB := filepath.Join(workspace, "service-b")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceB, false)

	seedReadyChange(t, planning, "release-consumer", strings.Join([]string{
		"- [ ] 1. Read `../service-b/dist/release-1.0.tgz` (read-only) as an unchanged input",
		"- [ ] 2. Compare `../service-b/CHANGELOG.md` (READ-ONLY) against the pinned version",
		"- [ ] 3. Record the pinned version in `openspec/changes/release-consumer/design.md`",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "release-consumer"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ApplyState != ApplyReady || len(status.BlockedReasons) != 0 {
		t.Fatalf("read-only marked sibling paths blocked apply: applyState = %q, blockedReasons = %v", status.ApplyState, status.BlockedReasons)
	}
}

func TestUnmarkedSiblingPathStillBlocksAndReasonNamesReadOnlyMarker(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	serviceB := filepath.Join(workspace, "service-b")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceB, false)

	seedReadyChange(t, planning, "release-consumer", strings.Join([]string{
		"- [ ] 1. Read `../service-b/dist/release-1.0.tgz` as an unchanged input",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "release-consumer"})
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.ApplyState != ApplyBlocked || !strings.Contains(reasons, "blocked(edit_authority_missing)") {
		t.Fatalf("unmarked sibling path did not block: applyState = %q, blockedReasons = %v", status.ApplyState, status.BlockedReasons)
	}
	if !strings.Contains(reasons, realPath(t, serviceB)) {
		t.Fatalf("blocked reason does not name the sibling root: %s", reasons)
	}
	if !strings.Contains(reasons, "mark a read-only input with (read-only) right after its backticked path") {
		t.Fatalf("blocked reason does not name the read-only marker as an exit: %s", reasons)
	}
}

// The marker is token-scoped: a line that mixes a marked read-only input with
// an unmarked out-of-root write target keeps blocking on the unmarked path,
// and a marker placed elsewhere on the line annotates nothing.
func TestReadOnlyMarkerIsTokenScoped(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	serviceB := filepath.Join(workspace, "service-b")
	serviceC := filepath.Join(workspace, "service-c")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceB, false)
	initEditAuthorityGitRepo(t, serviceC, false)

	seedReadyChange(t, planning, "release-consumer", strings.Join([]string{
		"- [ ] 1. Read `../service-b/dist/release-1.0.tgz` (read-only) and update `../service-c/config/pin.yaml`",
		"- [ ] 2. Update `../service-b/CHANGELOG.md` with the pinned version (read-only)",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "release-consumer"})
	if err != nil {
		t.Fatal(err)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.ApplyState != ApplyBlocked || !strings.Contains(reasons, "blocked(edit_authority_missing)") {
		t.Fatalf("mixed line with an unmarked write target did not block: applyState = %q, blockedReasons = %v", status.ApplyState, status.BlockedReasons)
	}
	for _, root := range []string{realPath(t, serviceB), realPath(t, serviceC)} {
		if !strings.Contains(reasons, root) {
			t.Fatalf("blocked reason does not name the unmarked target root %s: %s", root, reasons)
		}
	}
}

// #3504: a resolved absolute path outside every allowed edit root is an
// unauthorized root whether or not it belongs to a Git repository.
func TestNonGitDirectoryOutsideAllowedRootsBlocksApply(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	external := filepath.Join(workspace, "external-config")
	initEditAuthorityGitRepo(t, planning, true)
	mkdir(t, external)

	seedReadyChange(t, planning, "external-rollout", strings.Join([]string{
		"- [ ] 1.1 Update `" + filepath.Join(external, "app", "settings.yaml") + "` with the new endpoint",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "external-rollout"})
	if err != nil {
		t.Fatal(err)
	}
	want := realPath(t, external)
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.ApplyState != ApplyBlocked || !strings.Contains(reasons, "blocked(edit_authority_missing)") || !strings.Contains(reasons, want) {
		t.Fatalf("non-Git external directory reported applyState = %q, blockedReasons = %v; want blocked(edit_authority_missing) naming %s", status.ApplyState, status.BlockedReasons, want)
	}
	if status.Consent == nil || len(status.Consent.MissingRoots) != 1 || status.Consent.MissingRoots[0] != want {
		t.Fatalf("consent envelope does not name the external directory: %#v", status.Consent)
	}
}

func TestDetectUnauthorizedEditRootsNamesNonGitOutsidePathsOnly(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	external := filepath.Join(workspace, "external-config")
	initEditAuthorityGitRepo(t, planning, false)
	mkdir(t, external)
	mkdir(t, filepath.Join(planning, "internal", "api"))

	outside := detectUnauthorizedEditRoots("- [ ] 1.1 Update `"+filepath.Join(external, "app", "settings.yaml")+"`\n", planning, []string{planning})
	if want := realPath(t, external); len(outside) != 1 || outside[0] != want {
		t.Fatalf("detectUnauthorizedEditRoots() = %v, want exactly [%s]", outside, want)
	}
	inside := detectUnauthorizedEditRoots("- [ ] 1.1 Update `internal/api/handler.go`\n- [ ] 1.2 Update `"+filepath.Join(planning, "internal", "api", "router.go")+"`\n", planning, []string{planning})
	if len(inside) != 0 {
		t.Fatalf("paths inside the allowed root were flagged: %v", inside)
	}
}
