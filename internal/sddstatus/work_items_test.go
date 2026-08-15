package sddstatus

import (
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
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":["build"]`, `"dependsOn":["verify"]`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":["build"]`, `"dependsOn":["missing"]`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"maxAttempts":2`, `"maxAttempts":0`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"src"`, `"../escape"`, 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":[],`, "", 1),
		strings.Replace(itemTasks("- [ ] build: Build\n- [ ] verify: Verify"), `"dependsOn":[]`, `"dependsOn":null`, 1),
		itemTasks("- [ ] build: Build\n- [ ] verify: Verify") + "\n<!-- gentle-ai.sdd-items/v1\n{",
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
