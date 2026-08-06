package sddstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Issue #2547 (S1 of #2540): a multi-repository change whose tasks target
// sibling Git repositories must not report applyState ready while
// allowedEditRoots names only the planning repository. On the unfixed base
// this fixture reports ready/apply with zero blockers — the contradictory
// route: native status authorizes apply, but the sdd-apply outside-root guard
// forbids every first work unit.

// initEditAuthorityGitRepo turns dir into a Git repository. Only the planning
// repository needs a commit (the native runtime store resolves its repository
// root); sibling service repositories only need a `.git` for root detection.
func initEditAuthorityGitRepo(t *testing.T, dir string, commit bool) {
	t.Helper()
	mkdir(t, dir)
	runRuntimeLedgerGit(t, dir, "init", "-q")
	if !commit {
		return
	}
	runRuntimeLedgerGit(t, dir, "config", "user.email", "edit-authority@example.com")
	runRuntimeLedgerGit(t, dir, "config", "user.name", "Edit Authority Test")
	write(t, filepath.Join(dir, "tracked.txt"), "base\n")
	runRuntimeLedgerGit(t, dir, "add", "tracked.txt")
	runRuntimeLedgerGit(t, dir, "commit", "-qm", "base")
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestMultiRepoTaskTargetsBlockApplyWithoutEditAuthority(t *testing.T) {
	workspace := t.TempDir() // the workspace itself is NOT a Git repository
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	serviceB := filepath.Join(workspace, "service-b")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceA, false)
	initEditAuthorityGitRepo(t, serviceB, false)

	seedReadyChange(t, planning, "multi-repo-rollout", strings.Join([]string{
		"- [ ] 1.1 Update `../service-a/internal/api/handler.go` to accept the new header",
		"- [ ] 1.2 Update `../service-b/internal/worker/consume.go` to forward the header",
		"- [ ] 1.3 Record the rollout order in `openspec/changes/multi-repo-rollout/design.md`",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"})
	if err != nil {
		t.Fatal(err)
	}

	wantA := realPath(t, serviceA)
	wantB := realPath(t, serviceB)
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.ApplyState != ApplyBlocked {
		t.Fatalf("multi-repo change reported applyState = %q, nextRecommended = %q, blockedReasons = %v; want applyState blocked with blocked(edit_authority_missing) naming %s and %s",
			status.ApplyState, status.NextRecommended, status.BlockedReasons, wantA, wantB)
	}
	if status.NextRecommended == "apply" {
		t.Fatalf("nextRecommended = %q for a change whose every first work unit is outside the authorized edit roots", status.NextRecommended)
	}
	if !strings.Contains(reasons, "blocked(edit_authority_missing)") {
		t.Fatalf("blocked reasons lack the edit-authority reason code: %s", reasons)
	}
	if !strings.Contains(reasons, wantA) || !strings.Contains(reasons, wantB) {
		t.Fatalf("blocked reasons must name each unauthorized target root %q and %q: %s", wantA, wantB, reasons)
	}
}

// TestSingleRepoTasksKeepStatusByteIdentical pins the no-false-positive
// contract: for an ordinary single-repository change, detection matches
// nothing and the wiring mutates nothing, so the status an operator sees is
// byte-identical to the pre-#2547 status. The wiring only touches applyState
// and blockedReasons when at least one unauthorized root is found, so an
// empty JSON footprint here proves the whole fixture is untouched.
func TestSingleRepoTasksKeepStatusByteIdentical(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	seedReadyChange(t, repo, "single-repo-change", strings.Join([]string{
		"- [ ] 1.1 Update `internal/auth/login.go` with the new claim check",
		"- [x] 1.2 Extend `openspec/changes/single-repo-change/tasks.md` after review",
		"- [ ] 1.3 Run `go test ./...` and fix regressions",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "single-repo-change"})
	if err != nil {
		t.Fatal(err)
	}

	if status.ApplyState != ApplyReady || status.NextRecommended != "apply" {
		t.Fatalf("single-repo change lost apply readiness: applyState = %q nextRecommended = %q blockedReasons = %v",
			status.ApplyState, status.NextRecommended, status.BlockedReasons)
	}
	if len(status.BlockedReasons) != 0 {
		t.Fatalf("single-repo change gained blocked reasons: %v", status.BlockedReasons)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "edit_authority_missing") {
		t.Fatalf("single-repo status carries an edit-authority footprint: %s", encoded)
	}
}

// TestDetectUnauthorizedEditRootsHonorsAllowedRoots pins the seam a later
// slice of #2540 plugs into: persisted per-change grants extend the
// allowed-roots parameter, and detection needs no rework. It also pins the
// nearest-existing-ancestor rule (task prose names files that do not exist
// yet) and that non-path prose stays invisible.
func TestDetectUnauthorizedEditRootsHonorsAllowedRoots(t *testing.T) {
	workspace := t.TempDir()
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	initEditAuthorityGitRepo(t, planning, false)
	initEditAuthorityGitRepo(t, serviceA, false)
	tasks := strings.Join([]string{
		"- [ ] 1.1 Update `../service-a/does/not/exist/yet.go` for the rollout",
		"- [ ] 1.2 Update the billing service and run `go test ./...`",
		"- [ ] 1.3 Read https://example.com/service-b/docs first",
		"",
	}, "\n")

	wantA := realPath(t, serviceA)
	flagged := detectUnauthorizedEditRoots(tasks, planning, []string{planning})
	if len(flagged) != 1 || flagged[0] != wantA {
		t.Fatalf("detectUnauthorizedEditRoots() = %v, want exactly [%s]", flagged, wantA)
	}

	granted := detectUnauthorizedEditRoots(tasks, planning, []string{planning, serviceA})
	if len(granted) != 0 {
		t.Fatalf("granting %s must clear the block without detector rework, got %v", serviceA, granted)
	}
}

// TestEngramTasksTextBlocksApplyOnUnauthorizedTargets drives the same
// detection through resolveEngramStatus, which has no tasks.md path — only
// the tasks artifact text — so the detector must accept text plus the
// workspace root.
func TestEngramTasksTextBlocksApplyOnUnauthorizedTargets(t *testing.T) {
	workspace := t.TempDir() // the workspace itself is NOT a Git repository
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceA, false)
	mkdir(t, filepath.Join(planning, ".engram"))
	runRuntimeLedgerGit(t, planning, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")

	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/cross-repo/proposal", Content: "## Proposal\nRoll out the header", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/cross-repo/spec", Content: "## Requirements\n- SHALL forward the header", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/cross-repo/design", Content: "## Design\nSequential rollout", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/cross-repo/tasks", Content: "- [ ] 1.1 Update `../service-a/internal/api/handler.go` first\n", Project: "gentle-ai", Scope: "project"},
	})
	defer restore()

	status, err := Resolve(ResolveOptions{CWD: planning})
	if err != nil {
		t.Fatal(err)
	}
	if status.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("ArtifactStore = %q, want %q", status.ArtifactStore, ArtifactStoreEngram)
	}

	wantA := realPath(t, serviceA)
	reasons := strings.Join(status.BlockedReasons, "\n")
	if status.ApplyState != ApplyBlocked || status.NextRecommended == "apply" {
		t.Fatalf("engram-path change reported applyState = %q, nextRecommended = %q, blockedReasons = %v; want applyState blocked with blocked(edit_authority_missing) naming %s",
			status.ApplyState, status.NextRecommended, status.BlockedReasons, wantA)
	}
	if !strings.Contains(reasons, "blocked(edit_authority_missing)") || !strings.Contains(reasons, wantA) {
		t.Fatalf("blocked reasons must carry the reason code and name %q: %s", wantA, reasons)
	}
}

// TestBlockedEditAuthorityStatusCarriesConsentEnvelope is #2563's (S4b of
// #2540) end-to-end fixture at the status layer: when a multi-repository
// change reports blocked(edit_authority_missing), the projected v1 status
// carries the typed gentle-ai.sdd-integration.consent/v1 envelope whose
// granted choice names the EXACT runnable grant invocation — including the
// change-instance token that the status layer itself derives, persists in the
// change's own directory (so it dies with archive and a recreated change
// mints a fresh one), and will later use to project the granted roots back.
func TestBlockedEditAuthorityStatusCarriesConsentEnvelope(t *testing.T) {
	workspace := t.TempDir() // the workspace itself is NOT a Git repository
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	serviceB := filepath.Join(workspace, "service-b")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceA, false)
	initEditAuthorityGitRepo(t, serviceB, false)
	changeRoot := seedReadyChange(t, planning, "multi-repo-rollout", strings.Join([]string{
		"- [ ] 1.1 Update `../service-a/internal/api/handler.go` to accept the new header",
		"- [ ] 1.2 Update `../service-b/internal/worker/consume.go` to forward the header",
		"",
	}, "\n"))

	status, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	raw, ok := document["consent"]
	if !ok {
		t.Fatalf("blocked(edit_authority_missing) status carries no consent envelope: %s", payload)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var consent SDDIntegrationConsentResult
	if err := decoder.Decode(&consent); err != nil {
		t.Fatalf("consent block does not decode into the S4a contract type: %v", err)
	}
	if err := consent.Validate(); err != nil {
		t.Fatalf("emitted consent envelope fails its own contract validation: %v", err)
	}

	wantA := realPath(t, serviceA)
	wantB := realPath(t, serviceB)
	if !reflect.DeepEqual(consent.MissingRoots, []string{wantA, wantB}) {
		t.Fatalf("consent.MissingRoots = %v, want [%s %s]", consent.MissingRoots, wantA, wantB)
	}
	granted := consent.Choices[0].Invocation
	if !sddConsentGrantInvocationShape.MatchString(granted) {
		t.Fatalf("granted invocation does not match the real grant verb shape: %q", granted)
	}

	// The derivation rule: the embedded change-instance token IS the marker
	// persisted in the change's own directory, so the grant the human
	// consents to binds exactly the identity a later status read projects.
	marker, err := os.ReadFile(filepath.Join(changeRoot, ".gentle-ai-instance"))
	if err != nil {
		t.Fatalf("blocked status persisted no change-instance marker: %v", err)
	}
	token := strings.TrimSpace(string(marker))
	if token == "" || !strings.Contains(granted, " --change-instance "+token) {
		t.Fatalf("granted invocation %q does not carry the persisted instance token %q", granted, token)
	}

	// Stability within the change lifecycle: a second status resolves the
	// SAME token and renders the same invocation, so a retried conversation
	// never mints a rival identity for one change instance.
	again, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"})
	if err != nil {
		t.Fatal(err)
	}
	againProjected, err := ProjectStatusV1(again)
	if err != nil {
		t.Fatal(err)
	}
	againPayload, err := json.Marshal(againProjected)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(againPayload), " --change-instance "+token) {
		t.Fatalf("second status did not reuse the persisted instance token %q: %s", token, againPayload)
	}
}

// TestRecreatedChangeNameDoesNotInheritGrantedRoots proves the S5 containment
// through S4b's wiring: a covering grant expands AllowedEditRoots and readies
// apply for THIS change instance, but recreating the change under the same
// archived name (a fresh change directory, hence a fresh minted marker)
// projects none of the old grants — the recreated change is blocked again
// with a new instance token and planning-only edit authority.
func TestRecreatedChangeNameDoesNotInheritGrantedRoots(t *testing.T) {
	workspace := t.TempDir() // the workspace itself is NOT a Git repository
	planning := filepath.Join(workspace, "planning")
	serviceA := filepath.Join(workspace, "service-a")
	initEditAuthorityGitRepo(t, planning, true)
	initEditAuthorityGitRepo(t, serviceA, false)
	tasks := strings.Join([]string{
		"- [ ] 1.1 Update `../service-a/internal/api/handler.go` to accept the new header",
		"",
	}, "\n")
	changeRoot := seedReadyChange(t, planning, "multi-repo-rollout", tasks)

	// Blocked status mints the marker; grant against exactly that identity.
	if _, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"}); err != nil {
		t.Fatal(err)
	}
	token, err := readChangeInstanceMarker(changeRoot)
	if err != nil || token == "" {
		t.Fatalf("blocked status persisted no instance marker: %q, %v", token, err)
	}
	wantA := realPath(t, serviceA)
	opened, err := OpenRuntimeStore(context.Background(), planning, "multi-repo-rollout")
	if err != nil {
		t.Fatal(err)
	}
	store, err := opened.ForInstance(token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Grant(context.Background(), GrantRootsRequest{
		RequestID: "grant-recreation-fixture", Roots: []string{wantA},
		Reason: "maintainer authorized the sibling repository", Actor: "maintainer",
	}); err != nil {
		t.Fatal(err)
	}

	// The covering grant clears detection and expands the single assignment
	// site: AllowedEditRoots = planning + granted roots, apply is ready.
	granted, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"})
	if err != nil {
		t.Fatal(err)
	}
	if granted.ApplyState != ApplyReady || granted.NextRecommended != "apply" {
		t.Fatalf("post-grant status = %q/%q with reasons %v, want ready/apply",
			granted.ApplyState, granted.NextRecommended, granted.BlockedReasons)
	}
	if !reflect.DeepEqual(granted.ActionContext.AllowedEditRoots, []string{planning, wantA}) {
		t.Fatalf("AllowedEditRoots = %v, want [%s %s]", granted.ActionContext.AllowedEditRoots, planning, wantA)
	}

	// Archive moves the change directory away; a recreated change under the
	// same name starts from a fresh directory. The old grants must not
	// resurrect: fresh marker, planning-only authority, blocked again.
	if err := os.RemoveAll(changeRoot); err != nil {
		t.Fatal(err)
	}
	seedReadyChange(t, planning, "multi-repo-rollout", tasks)
	recreated, err := Resolve(ResolveOptions{CWD: planning, ChangeName: "multi-repo-rollout"})
	if err != nil {
		t.Fatal(err)
	}
	if recreated.ApplyState != ApplyBlocked {
		t.Fatalf("recreated change inherited the archived grant: %q with roots %v",
			recreated.ApplyState, recreated.ActionContext.AllowedEditRoots)
	}
	if !reflect.DeepEqual(recreated.ActionContext.AllowedEditRoots, []string{planning}) {
		t.Fatalf("recreated AllowedEditRoots = %v, want [%s]", recreated.ActionContext.AllowedEditRoots, planning)
	}
	fresh, err := readChangeInstanceMarker(changeRoot)
	if err != nil || fresh == "" || fresh == token {
		t.Fatalf("recreated change did not mint a fresh instance token: %q vs %q (%v)", fresh, token, err)
	}
}
