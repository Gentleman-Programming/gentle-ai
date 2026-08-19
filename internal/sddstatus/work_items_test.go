package sddstatus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkItemsProjectIdenticallyFromOpenSpecAndEngramText(t *testing.T) {
	status := itemStatus(t)
	openSpec, present, err := projectWorkItems(itemTasks("- [ ] build: Build\n- [x] verify: Verify"), status)
	if err != nil || !present {
		t.Fatalf("OpenSpec projection = %#v, %v, %v", openSpec, present, err)
	}
	engram, present, err := projectWorkItems(itemTasks("- [ ] build: Build\n- [x] verify: Verify"), status)
	if err != nil || !present || !reflect.DeepEqual(openSpec, engram) {
		t.Fatalf("Engram projection = %#v, %v, %v", engram, present, err)
	}
	if !openSpec[0].Ready || openSpec[0].DependsOn == nil || !openSpec[1].Done {
		t.Fatalf("items = %#v", openSpec)
	}
}

func TestWorkItemsFailClosedForInvalidMetadata(t *testing.T) {
	for _, text := range []string{
		"<!-- gentle-ai.sdd-items/v1\n{\"items\":[\n-->",
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"id":"verify"`, `"id":"build"`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"workUnit":"verify"`, `"workUnit":"build"`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":["build"]`, `"dependsOn":["verify"]`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":["build"]`, `"dependsOn":["missing"]`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":[]`, `"dependsOn":["verify"]`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"maxAttempts":2`, `"maxAttempts":0`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"src"`, `"../escape"`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":[],`, "", 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":[]`, `"dependsOn":null`, 1),
		itemTasks("- [ ] build: Build\n- [ ] verify: Verify") + "\n<!-- gentle-ai.sdd-items/v1\n{",
		itemTasks("- [ ] build: Build\n- [ ] verify: Verify\n- [ ] extra: Extra"),
	} {
		if items, present, err := projectWorkItems(text, itemStatus(t)); !present || err == nil || items != nil {
			t.Fatalf("items=%#v present=%v err=%v", items, present, err)
		}
	}
	if items, present, err := projectWorkItems("- [ ] build: Build", itemStatus(t)); present || err != nil || items != nil {
		t.Fatalf("absent metadata = %#v, %v, %v", items, present, err)
	}
	status := itemStatus(t)
	applyWorkItemProjection(&status, "<!-- gentle-ai.sdd-items/v1\n{}\n-->")
	if status.Dependencies.Apply != DependencyBlocked || status.NextRecommended != "resolve-blockers" {
		t.Fatalf("invalid metadata status = %#v", status)
	}
	if len(status.BlockedReasons) != 1 || !strings.Contains(status.BlockedReasons[0], "items must not be empty") || strings.Count(status.BlockedReasons[0], "rerun `gentle-ai sdd-status") != 1 {
		t.Fatalf("invalid metadata diagnostic = %#v", status.BlockedReasons)
	}
}

func TestNewItemPlanCandidateNamesRetainedAuthorityFailure(t *testing.T) {
	retained, err := newItemPlanCandidate([]WorkItem{{ID: "build", WorkUnit: "build", EvidenceGoal: "compile", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"src"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newItemPlanCandidate([]WorkItem{{ID: "verify", WorkUnit: "verify", EvidenceGoal: "test", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"src"}}}, &retained); err == nil || !strings.Contains(err.Error(), "does not contain projected item \"verify\"") {
		t.Fatalf("missing retained item error = %v", err)
	}
	retained.Items[0].InitiallyDone = nil
	retained.Digest = itemPlanDigest(retained)
	if _, err := newItemPlanCandidate([]WorkItem{{ID: "build", WorkUnit: "build", EvidenceGoal: "compile", MaxAttempts: 1, MaxChangedLines: 1, EditRoots: []string{"src"}}}, &retained); err == nil || !strings.Contains(err.Error(), "entry \"build\" has no initial completion snapshot") {
		t.Fatalf("missing retained snapshot error = %v", err)
	}
}

func TestWorkItemStatesRespectDependenciesRuntimeScopeAndRelationships(t *testing.T) {
	status := itemStatus(t)
	status.RuntimeStatus = &RuntimeStatus{Objective: &RuntimeObjective{WorkUnit: "build", EvidenceGoal: "compile"}, ActiveAttempt: &RuntimeAttempt{WorkUnit: "build", Ordinal: 1}, EvidenceRevision: "evidence"}
	status.Relationships.DependsOn = []string{"unrelated-change"}
	items, _, err := projectWorkItems(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), status)
	if err != nil || !items[0].Active || items[0].Ready || !items[1].Blocked || items[0].RuntimeAttempt == nil || items[0].EvidenceRevision != "evidence" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	items, _, err = projectWorkItems(itemTasks("- [x] build: Build\n- [ ] verify: Verify"), status)
	if err != nil || !items[0].Done || items[0].Active || items[0].RuntimeAttempt != nil || items[0].EvidenceRevision != "" || !items[1].Blocked {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	status.RuntimeStatus = nil
	if err := os.Mkdir(filepath.Join(status.ActionContext.WorkspaceRoot, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	status.ActionContext.AllowedEditRoots = []string{filepath.Join(status.ActionContext.WorkspaceRoot, "other")}
	items, _, err = projectWorkItems(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), status)
	if err != nil || !items[0].Blocked {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	status.ActionContext.AllowedEditRoots = []string{filepath.Join(status.ActionContext.WorkspaceRoot, "allowed")}
	items, _, err = projectWorkItems(strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"src"`, `"other/future"`, 1), status)
	if err != nil || !items[0].Blocked {
		t.Fatalf("nonexistent sibling items=%#v err=%v", items, err)
	}
	items, _, err = projectWorkItems(strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"src"`, `"allowed/future"`, 1), status)
	if err != nil || !items[0].Ready {
		t.Fatalf("nonexistent descendant items=%#v err=%v", items, err)
	}
}

func TestWorkItemJSONOmitsAbsentProjection(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, t.TempDir(), nil, nil, nil, "apply", nil)
	status.Dependencies.Apply = DependencyReady
	payload, err := json.Marshal(ProjectStatusV1Must(t, status))
	if err != nil || strings.Contains(string(payload), `"items"`) {
		t.Fatalf("payload=%s err=%v", payload, err)
	}
	applyWorkItemProjection(&status, itemTasks("- [ ] build: Build\n- [x] verify: Verify"))
	payload, err = json.Marshal(ProjectStatusV1Must(t, status))
	if err != nil || !strings.Contains(string(payload), `"items"`) {
		t.Fatalf("payload=%s err=%v", payload, err)
	}
}

func TestResolveItemAcquireUsesEquivalentOpenSpecAndEngramMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	tasks := itemTasks("- [ ] build: Build\n- [ ] verify: Verify")
	changeRoot := filepath.Join(root, "openspec", "changes", "items")
	for path, content := range map[string]string{
		"proposal.md": "# Proposal\n", "design.md": "# Design\n", "tasks.md": tasks, "specs/item/spec.md": "### Requirement: Item\n#### Scenario: Acquire\n",
	} {
		write(t, filepath.Join(changeRoot, path), content)
	}
	status, statusErr := Resolve(ResolveOptions{CWD: root, ChangeName: "items", ReviewDisabled: true})
	if statusErr != nil || len(status.Items) == 0 || !status.Items[0].Ready {
		t.Fatalf("ready OpenSpec status = %#v, %v", status, statusErr)
	}
	openSpec, err := ResolveItemAcquire(ResolveOptions{CWD: root, ChangeName: "items", ReviewDisabled: true}, "build", "open-request")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(filepath.Join(root, "openspec"))
	mkdir(t, filepath.Join(root, ".engram"))
	runRuntimeLedgerGit(t, root, "init", "-q")
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	restore := stubEngramExport(t, []engramObservation{
		{Title: "sdd/items/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/items/spec", Content: "### Requirement: Item\n#### Scenario: Acquire\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/items/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/items/tasks", Content: tasks, Project: "gentle-ai", Scope: "project"},
	})
	defer restore()
	engram, err := ResolveItemAcquire(ResolveOptions{CWD: root, ChangeName: "items", ReviewDisabled: true}, "build", "engram-request")
	if err != nil {
		t.Fatal(err)
	}
	if openSpec.WorkUnit != engram.WorkUnit || openSpec.EvidenceGoal != engram.EvidenceGoal || openSpec.MaxAttempts != engram.MaxAttempts || openSpec.MaxChangedLines != engram.MaxChangedLines ||
		openSpec.ItemID != engram.ItemID || !reflect.DeepEqual(openSpec.ItemEditRoots, engram.ItemEditRoots) ||
		openSpec.itemPlan == nil || engram.itemPlan == nil || !reflect.DeepEqual(openSpec.itemPlan, engram.itemPlan) ||
		openSpec.itemPlan.Plan.Items[0].EditRoots[0] != "src" {
		t.Fatalf("OpenSpec=%#v Engram=%#v", openSpec, engram)
	}
}

func TestResolveItemAcquireReplaysEachDisjointActiveItem(t *testing.T) {
	ctx, root := context.Background(), initRuntimeLedgerRepo(t)
	for _, dir := range []string{"a", "b"} {
		mkdir(t, filepath.Join(root, dir))
	}
	const change = "replay-disjoint-active-items"
	tasks := `- [ ] a: A
- [ ] b: B
<!-- gentle-ai.sdd-items/v1
{"items":[{"id":"a","dependsOn":[],"workUnit":"a","editRoots":["a"],"maxAttempts":2,"maxChangedLines":20,"evidenceGoal":"a"},{"id":"b","dependsOn":[],"workUnit":"b","editRoots":["b"],"maxAttempts":2,"maxChangedLines":20,"evidenceGoal":"b"}]}
-->`
	changeRoot := filepath.Join(root, "openspec", "changes", change)
	for path, content := range map[string]string{
		"proposal.md":        "# Proposal\n",
		"design.md":          "# Design\n",
		"specs/item/spec.md": "### Requirement: Item\n#### Scenario: Acquire\n",
		"tasks.md":           tasks,
	} {
		write(t, filepath.Join(changeRoot, path), content)
	}
	options := ResolveOptions{CWD: root, ChangeName: change, ReviewDisabled: true}
	requests := map[string]BeginAttemptRequest{}
	results := map[string]CompactAttemptResult{}
	store := mustRuntimeStore(t, root, change)
	for _, itemID := range []string{"a", "b"} {
		request, err := ResolveItemAcquire(options, itemID, "replay-"+itemID)
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: request})
		if err != nil || result.State != CompactStateProceed {
			t.Fatalf("initial %s acquire = %#v, %v", itemID, result, err)
		}
		requests[itemID], results[itemID] = request, result
	}
	before := countRuntimeRecords(t, store.Dir)
	for _, itemID := range []string{"a", "b"} {
		replayed, err := ResolveItemAcquire(options, itemID, "replay-"+itemID)
		if err != nil || !reflect.DeepEqual(replayed, requests[itemID]) {
			t.Fatalf("%s resolved replay = %#v, %v; want %#v", itemID, replayed, err, requests[itemID])
		}
		result, err := store.Acquire(ctx, CompactAcquireRequest{BeginAttemptRequest: replayed})
		if err != nil || result.State != CompactStateProceed || result.Token != results[itemID].Token || countRuntimeRecords(t, store.Dir) != before {
			t.Fatalf("%s submitted replay = %#v, %v; records=%d want=%d", itemID, result, err, countRuntimeRecords(t, store.Dir), before)
		}
	}
}

func TestResolveEngramStatusRetainedItemPlanBlocksVerifyAndArchiveUntilJoined(t *testing.T) {
	root := initRuntimeLedgerRepo(t)
	for _, dir := range []string{"build", "verify"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const change = "engram-item-join"
	store := mustRuntimeStore(t, root, change)
	plan, err := newItemPlanCandidate([]WorkItem{
		{ID: "build", WorkUnit: "build", EvidenceGoal: "compile", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"build"}},
		{ID: "verify", WorkUnit: "verify", EvidenceGoal: "test", MaxAttempts: 1, MaxChangedLines: 20, EditRoots: []string{"verify"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.Begin(context.Background(), runtimePlanRequest(t, store, plan, "build", "engram-build"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Finish(context.Background(), FinishAttemptRequest{ExpectedRevision: started.Revision, RequestID: "engram-build-finish", Outcome: AttemptPassed, EvidenceRevision: runtimeTestHash('a'), Diagnosis: "passed", HarnessDisposition: HarnessReused, CleanupEvidence: "clean", ProcessEvidence: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".engram"), 0o755); err != nil {
		t.Fatal(err)
	}
	runRuntimeLedgerGit(t, root, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
	tasks := itemTasks("- [x] build: Build\n- [x] verify: Verify")
	base := []engramObservation{
		{Title: "sdd/engram-item-join/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-item-join/spec", Content: "### Requirement: Item\n#### Scenario: Join\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-item-join/design", Content: "# Design\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/engram-item-join/tasks", Content: tasks, Project: "gentle-ai", Scope: "project"},
	}
	for _, report := range []string{"", boundedVerifyEnvelope(shaID("a"), "pass")} {
		observations := append([]engramObservation{}, base...)
		if report != "" {
			observations = append(observations, engramObservation{Title: "sdd/engram-item-join/verify-report", Content: report, Project: "gentle-ai", Scope: "project"})
		}
		restore := stubEngramExport(t, observations)
		status, ok, err := resolveEngramStatus(root, change, false, true)
		restore()
		if err != nil || !ok || status.NextRecommended != string(PhaseApply) || status.Dependencies.Verify != DependencyBlocked || status.Dependencies.Archive != DependencyBlocked {
			t.Fatalf("report=%q Engram join routing = %#v ok=%v err=%v", report, status, ok, err)
		}
	}
}

func itemStatus(t *testing.T) Status {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	status := baseStatus(ArtifactStoreOpenSpec, root, nil, nil, nil, "apply", nil)
	status.Dependencies.Apply = DependencyReady
	return status
}

func itemTasks(checkboxes string) string {
	return checkboxes + `
<!-- gentle-ai.sdd-items/v1
{"items":[{"id":"build","dependsOn":[],"workUnit":"build","editRoots":["src"],"maxAttempts":2,"maxChangedLines":100,"evidenceGoal":"compile"},{"id":"verify","dependsOn":["build"],"workUnit":"verify","editRoots":["src"],"maxAttempts":1,"maxChangedLines":50,"evidenceGoal":"test"}]}
-->`
}

func ProjectStatusV1Must(t *testing.T, status Status) StatusV1Projection {
	t.Helper()
	projected, err := ProjectStatusV1(status)
	if err != nil {
		t.Fatal(err)
	}
	return projected
}
