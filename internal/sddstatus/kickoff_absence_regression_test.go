package sddstatus

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestKickoffAbsenceRegressionMatchesPreGovernanceGolden is INC-21's P3
// control gate (design.md S1.3, D-05; tasks.md Phase 11). It is not a
// conventional RED test: tasks.md 11.1 requires it to capture the
// StatusV2Projection JSON a change with NO kickoff.yaml already produces
// today, confirmed green against the unmodified tree BEFORE governance.go
// exists or status.go/status_v2.go are touched (Phases 12-14). Every one of
// those later phases re-runs this exact test and must keep it green,
// unchanged, as its V-E evidence: D-05 requires the unsealed case to be
// byte-for-byte indistinguishable from before this increment, including the
// structural absence of the "governance" key.
//
// The workspace root varies every run (t.TempDir()), so it is the only
// substring scrubbed from both the actual and the frozen golden text before
// comparing; every other byte -- key order, field presence, indentation --
// is compared literally.
func TestKickoffAbsenceRegressionMatchesPreGovernanceGolden(t *testing.T) {
	root := t.TempDir()
	changeName := "regression-no-kickoff"
	seedReadyChange(t, root, changeName, "- [x] 1.1 Done\n- [ ] 1.2 Pending\n")

	status, err := Resolve(ResolveOptions{CWD: root, ChangeName: changeName, IncludeInstructions: true})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	projection, err := ProjectStatusV2(status)
	if err != nil {
		t.Fatalf("ProjectStatusV2() error = %v", err)
	}
	// A plain Encoder with HTML escaping disabled, rather than
	// json.MarshalIndent, keeps the frozen golden below readable as literal
	// "<unresolved>" text instead of "<unresolved>": the escaping
	// choice is cosmetic (Marshal's default HTML-safe mode versus this
	// encoder's literal mode), never a change to which bytes -- keys, values,
	// field presence, indentation -- the comparison covers.
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(projection); err != nil {
		t.Fatalf("json encode error = %v", err)
	}
	actual := bytes.TrimRight(buffer.Bytes(), "\n")

	got := scrubKickoffAbsenceWorkspaceRoot(string(actual), root)
	want := kickoffAbsenceRegressionGolden
	if got != want {
		t.Fatalf(
			"StatusV2Projection JSON for a change with no kickoff.yaml drifted from the pre-INC-21 golden (D-05 requires byte-for-byte identity).\n--- got ---\n%s\n--- want ---\n%s",
			got, want,
		)
	}
	if strings.Contains(got, `"governance"`) {
		t.Fatal(`StatusV2Projection names "governance" for a change with no kickoff.yaml; D-05 requires structural absence (nil, omitempty)`)
	}
}

// scrubKickoffAbsenceWorkspaceRoot replaces every occurrence of the
// t.TempDir()-generated workspace root with a fixed placeholder, in both its
// raw and its JSON-string-escaped form (Windows paths carry backslashes,
// which json.Marshal escapes as "\\"), so the comparison is stable across
// runs and platforms without weakening it: every byte the workspace root
// does not touch is still compared literally.
func scrubKickoffAbsenceWorkspaceRoot(jsonText, root string) string {
	escaped := strings.ReplaceAll(root, `\`, `\\`)
	scrubbed := strings.ReplaceAll(jsonText, escaped, "<WORKSPACE_ROOT>")
	return strings.ReplaceAll(scrubbed, root, "<WORKSPACE_ROOT>")
}

// kickoffAbsenceRegressionGolden was captured by running this test against
// the unmodified tree, before governance.go existed and before status.go or
// status_v2.go were touched (tasks.md 11.1). It is frozen pre-INC-21 truth:
// no line of it may change while this control gate is in force.
const kickoffAbsenceRegressionGolden = `{
  "schemaName": "gentle-ai.sdd-status",
  "schemaVersion": 2,
  "changeName": "regression-no-kickoff",
  "artifactStore": "openspec",
  "planningHome": {
    "mode": "repo-local",
    "path": "<WORKSPACE_ROOT>\\openspec"
  },
  "changeRoot": "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff",
  "artifactPaths": {
    "proposal": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\proposal.md"
    ],
    "specs": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\specs\\auth\\spec.md"
    ],
    "design": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\design.md"
    ],
    "tasks": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\tasks.md"
    ],
    "applyProgress": [],
    "verifyReport": []
  },
  "contextFiles": {
    "proposal": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\proposal.md"
    ],
    "specs": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\specs\\auth\\spec.md"
    ],
    "design": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\design.md"
    ],
    "tasks": [
      "<WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\tasks.md"
    ],
    "applyProgress": [],
    "verifyReport": []
  },
  "artifacts": {
    "applyProgress": "missing",
    "design": "done",
    "proposal": "done",
    "specs": "done",
    "tasks": "done",
    "verifyReport": "missing"
  },
  "taskProgress": {
    "total": 2,
    "completed": 1,
    "pending": 1,
    "allComplete": false
  },
  "dependencies": {
    "proposal": "all_done",
    "specs": "all_done",
    "design": "all_done",
    "tasks": "all_done",
    "apply": "ready",
    "verify": "ready",
    "archive": "ready"
  },
  "applyState": "ready",
  "actionContext": {
    "mode": "repo-local",
    "workspaceRoot": "<WORKSPACE_ROOT>",
    "allowedEditRoots": [
      "<WORKSPACE_ROOT>"
    ]
  },
  "relationships": {
    "dependsOn": [],
    "supersedes": [],
    "amends": [],
    "conflictsWith": [],
    "sameDomainActiveChanges": []
  },
  "phaseInstructions": {
    "apply": [
      "Change: regression-no-kickoff",
      "State: ready",
      "Read proposal, specs, design, and tasks before editing.",
      "Artifact store: openspec; read the file at that path.",
      "Tasks locator: <WORKSPACE_ROOT>\\openspec\\changes\\regression-no-kickoff\\tasks.md",
      "Apply-progress locator: <unresolved>",
      "Resume from the apply-progress locator when it resolves; implement only unchecked tasks and mark each complete at the tasks locator as work completes."
    ],
    "verify": [
      "Change: regression-no-kickoff",
      "State: ready",
      "Verification is optional: when requested, inspect the implementation, including partial work, against the proposal, specs, design, and tasks.",
      "Run applicable practical checks; report actual results, unfinished tasks, findings, and unavailable checks without inventing a pass.",
      "A missing, stale, malformed, or failed report does not block archive. Verification grants no edit authority."
    ],
    "archive": [
      "Change: regression-no-kickoff",
      "State: ready",
      "Verify-report locator: <unresolved>",
      "Archive records the actual task state and any available verification findings; neither a report nor task completion is an admission requirement.",
      "Preserve historical report and task bytes. Retain edit permissions, safe copy/move and collision checks, and native delta-spec validation."
    ]
  },
  "nextRecommended": "apply",
  "blockedReasons": [],
  "notes": []
}`
