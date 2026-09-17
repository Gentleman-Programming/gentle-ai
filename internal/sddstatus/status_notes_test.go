package sddstatus

import (
	"encoding/json"
	"strings"
	"testing"
)

// This file pins the #4372 contract split: `blockedReasons` carries only genuine
// blockers, and informational diagnostics live in the separate, always-present
// `notes` array. The contradiction it removes was real: the producer appended
// `note(future_edit_roots)` to the same channel it used for blocking reasons
// while deliberately keeping `applyState: ready` and `nextRecommended: apply`,
// and every consumer contract says a non-empty `blockedReasons` forbids apply.
// A consumer that obeyed its own documented gate therefore stopped on a note the
// producer had already declared non-blocking.
//
// The routing split itself is pinned end to end by
// TestFutureWorkUnitEditAuthorityRootIsInformationalNotBlocking, which owns the
// fixture; the tests here cover the serialized shape and the human-facing
// renderers that consumers rely on.

// TestStatusNotesIsAlwaysASerializedArray keeps the new field's shape stable for
// consumers: an empty note set must project as `[]`, never as a missing key or a
// JSON null a consumer would have to special-case.
func TestStatusNotesIsAlwaysASerializedArray(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	if status.Notes == nil {
		t.Fatal("baseStatus left Notes nil, want an empty array")
	}
	projected, err := ProjectStatusV2(status)
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
	raw, ok := document["notes"]
	if !ok {
		t.Fatalf("v2 document omitted the notes field: %s", payload)
	}
	if string(raw) != "[]" {
		t.Fatalf("empty notes projected as %s, want []", raw)
	}
}

// TestRenderersReportNotesOutsideBlockedReasons keeps the human-facing surfaces
// honest: a reader must see the note, and must not read it under a heading that
// says blocked.
func TestRenderersReportNotesOutsideBlockedReasons(t *testing.T) {
	status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
	status.Notes = []string{"note(future_edit_roots): a later work unit targets /elsewhere"}

	for name, rendered := range map[string]string{
		"markdown":   RenderMarkdown(status),
		"dispatcher": RenderDispatcherMarkdown(status),
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(rendered, "### Notes") {
				t.Fatalf("%s omitted the notes section:\n%s", name, rendered)
			}
			if !strings.Contains(rendered, "note(future_edit_roots)") {
				t.Fatalf("%s omitted the note text:\n%s", name, rendered)
			}
			if strings.Contains(rendered, "### Blocked Reasons") {
				t.Fatalf("%s reported an informational note under blocked reasons:\n%s", name, rendered)
			}
		})
	}
}
