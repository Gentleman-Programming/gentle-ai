package sddstatus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedNestedSubprojectRepo builds #2891's exact layout: ONE Git repository
// whose planning workspace is a nested subproject, with a sibling service
// directory next to it.
//
//	<repo>/.git
//	<repo>/<subproject>/openspec/changes/<change>/...
//	<repo>/<service-dir>/...
func seedNestedSubprojectRepo(t *testing.T, tasks string) (repo string, planning string) {
	t.Helper()
	repo = initRuntimeLedgerRepo(t)
	planning = filepath.Join(repo, "subproject")
	service := filepath.Join(repo, "service")
	for _, dir := range []string{planning, service} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(service, "handler.go"), []byte("package service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	seedReadyChange(t, planning, "nested-change", tasks)
	return repo, planning
}

// TestSiblingDirectoryInsideOneRepositoryBlocksApply is #2891.
//
// detectUnauthorizedEditRoots was written for the cross-repository case and
// says so in its own comment: it flags a token only when the token "resolves
// to a real Git repository different from the planning repository". That
// treats "outside the workspace" and "a different repository" as the same
// thing. They are not. When the planning workspace is a nested subproject,
// a sibling directory is outside every authorized edit root while sharing the
// planning repository's Git root, so `target == planningGitRoot` skipped it
// and apply stayed ready for a plan sdd-apply's outside-root guard would
// refuse.
//
// #2540's edit-authority chain does not cover this: its targets resolve to a
// different Git root. Neither does #3001's filesystem-root guard.
func TestSiblingDirectoryInsideOneRepositoryBlocksApply(t *testing.T) {
	_, planning := seedNestedSubprojectRepo(t, strings.Join([]string{
		"- [ ] 1.1 Add the claim check to `../service/handler.go`",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "nested-change"})
	if err != nil {
		t.Fatal(err)
	}

	if status.ApplyState != ApplyBlocked {
		t.Fatalf("applyState = %q, want blocked: the plan edits a directory outside every authorized edit root", status.ApplyState)
	}
	reasons := strings.Join(status.BlockedReasons, "\n")
	if !strings.Contains(reasons, "edit_authority_missing") {
		t.Fatalf("blocked reasons do not name the missing edit authority: %v", status.BlockedReasons)
	}
	// The refusal has to name the directory the operator would grant, not the
	// file and not the whole repository.
	if !strings.Contains(reasons, filepath.Join(filepath.Dir(planning), "service")) {
		t.Fatalf("blocked reason does not name the unauthorized directory: %v", status.BlockedReasons)
	}
	// Both exits stay stated verbatim; this issue widens what is detected, it
	// does not change what the operator can do about it.
	for _, exit := range []string{"edit tasks.md so every work unit stays inside the authorized edit roots", "grant this change edit authority"} {
		if !strings.Contains(reasons, exit) {
			t.Fatalf("blocked reason dropped the exit %q: %v", exit, status.BlockedReasons)
		}
	}
}

// TestNestedSubprojectTargetsInsideItsOwnWorkspaceStayReady is the
// false-positive guard. Widening detection must not block the ordinary case:
// a nested subproject editing its own files is entirely inside its authorized
// root and has nothing to grant.
func TestNestedSubprojectTargetsInsideItsOwnWorkspaceStayReady(t *testing.T) {
	_, planning := seedNestedSubprojectRepo(t, strings.Join([]string{
		"- [ ] 1.1 Update `internal/auth/login.go` with the new claim check",
		"- [ ] 1.2 Run `go test ./...` and fix regressions",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "nested-change"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ApplyState != ApplyReady || len(status.BlockedReasons) != 0 {
		t.Fatalf("a subproject editing its own files lost readiness: applyState = %q blockedReasons = %v",
			status.ApplyState, status.BlockedReasons)
	}
}

// TestGrantedSiblingDirectoryRestoresReadiness proves the new detection plugs
// into the SAME grant seam as the cross-repository case: an authorized root
// covering the sibling makes the plan admissible, with no separate mechanism.
func TestGrantedSiblingDirectoryRestoresReadiness(t *testing.T) {
	repo, planning := seedNestedSubprojectRepo(t, strings.Join([]string{
		"- [ ] 1.1 Add the claim check to `../service/handler.go`",
		"",
	}, "\n"))

	tasks := "- [ ] 1.1 Add the claim check to `../service/handler.go`\n"
	roots := detectUnauthorizedEditRoots(tasks, planning, []string{planning, filepath.Join(repo, "service")})
	if len(roots) != 0 {
		t.Fatalf("granting the sibling directory left it unauthorized: %v", roots)
	}
}
