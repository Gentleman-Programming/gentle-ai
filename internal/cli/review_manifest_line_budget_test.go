package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewManifestLineBudget is the per-entry line cost this product accepts for
// an emitted changed-path manifest. The published contract fixes the entry's
// eight required fields and forbids additional ones, so the only cost this
// product still owns is how many lines those fields are spread across. One
// entry per line is that floor; the indented-per-field form this pins against
// cost ten, which is what turned a 149-path candidate into a 1,505-line
// payload in a consumer's conversation (issue #3281).
const reviewManifestLineBudget = 1

// manifestEntryLineCost reports how many output lines the emitted
// "changed_path_manifest" array spends per entry. It is deliberately measured
// on the real emitted bytes rather than on a re-encoding, because the defect
// this pins is a property of the bytes a consumer's terminal actually receives.
func manifestEntryLineCost(t *testing.T, payload []byte, entries int) float64 {
	t.Helper()
	if entries == 0 {
		t.Fatal("manifest line cost is undefined for an empty manifest")
	}
	key := reviewManifestArrayKey
	start := bytes.Index(payload, []byte(key))
	if start < 0 {
		t.Fatalf("emitted payload carries no %q array", key)
	}
	open := start + len(key) - 1
	end := reviewJSONArrayEnd(payload, open)
	if end < 0 {
		t.Fatalf("emitted %q array is unterminated", key)
	}
	// The array body always ends with one line break before its closing
	// bracket; everything above that is the per-entry cost being measured.
	return float64(bytes.Count(payload[open:end], []byte("\n"))-1) / float64(entries)
}

// TestNegotiatedStartBoundsChangedPathManifestLineCost pins the emitted line
// cost of START's changed-path manifest. START is the route the field report
// measured: on the reporter's candidate its manifest was 89% of the payload,
// and it is emitted in full for every candidate regardless of size.
func TestNegotiatedStartBoundsChangedPathManifestLineCost(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const paths = 40
	for index := range paths {
		writeReviewStartCandidate(t, repo,
			"internal/generated/package"+strconv.Itoa(index)+"/descriptive_source_name.go",
			"package generated\n\nfunc Exported"+strconv.Itoa(index)+"() bool { return true }\n", 0o644)
	}
	var output bytes.Buffer
	if err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo, "--lineage", "manifest-line-budget",
	}), &output); err != nil {
		t.Fatal(negotiatedReviewStartFailure(err, output.String()))
	}
	result := decodeNegotiatedReviewStart(t, output.Bytes())
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.ChangedPathManifest == nil || len(*result.ChangedPathManifest) != paths {
		t.Fatalf("START manifest = %v entries, want %d; the bound must never cost the caller data",
			result.ChangedPathManifest, paths)
	}
	if cost := manifestEntryLineCost(t, output.Bytes(), paths); cost > reviewManifestLineBudget {
		t.Fatalf("START spends %.2f lines per manifest entry, want at most %d (payload = %d lines, %d bytes)",
			cost, reviewManifestLineBudget, bytes.Count(output.Bytes(), []byte("\n")), output.Len())
	}
}

// TestCapturePreflightBoundsChangedPathManifestLineCost pins the same bound on
// the capture preflight readback. That route stays reachable for candidates the
// provider-materialized reviewer route refuses, so it must stay emittable and
// merely stop spending ten lines on every entry.
func TestCapturePreflightBoundsChangedPathManifestLineCost(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const paths = 40
	for index := range paths {
		writeReviewStartCandidate(t, repo,
			"internal/generated/package"+strconv.Itoa(index)+"/descriptive_source_name.go",
			"package generated\n\nfunc Exported"+strconv.Itoa(index)+"() bool { return true }\n", 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "manifest-preflight-line-budget")
	var output bytes.Buffer
	if err := RunReviewCaptureResult([]string{
		"--repository-context", started.RepositoryContext.Handle,
		"--lineage", started.LineageID, "--target", started.RepositoryContext.TargetIdentity,
		"--expected-revision", started.RepositoryContext.Revision,
		"--lens", started.SelectedLenses[0], "--order", "0", "--preflight",
	}, &output); err != nil {
		t.Fatalf("capture-result preflight: %v\n%s", err, output.String())
	}
	var preflight reviewCapturePreflightResult
	decodeStrictReviewJSON(t, output.Bytes(), &preflight)
	if len(preflight.ChangedPathManifest) != paths {
		t.Fatalf("preflight manifest = %d entries, want %d", len(preflight.ChangedPathManifest), paths)
	}
	if cost := manifestEntryLineCost(t, output.Bytes(), paths); cost > reviewManifestLineBudget {
		t.Fatalf("capture preflight spends %.2f lines per manifest entry, want at most %d (payload = %d lines, %d bytes)",
			cost, reviewManifestLineBudget, bytes.Count(output.Bytes(), []byte("\n")), output.Len())
	}
}

// TestNegotiatedStatusBoundsChangedPathManifestLineCost pins the bound on the
// negotiated collection transition, which repeats the identical manifest once
// per uncaptured lens. The repetition itself is fixed by the published schema
// and is not what this pins; the per-entry line cost of each copy is.
func TestNegotiatedStatusBoundsChangedPathManifestLineCost(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	// Forty paths, matching the START and preflight fixtures above. This used
	// to be pinned to the retired 32-entry evidence cap, which was the largest
	// candidate STATUS would answer at all before issue #3367.
	const paths = 40
	for index := range paths {
		writeReviewStartCandidate(t, repo,
			"internal/generated/package"+strconv.Itoa(index)+"/descriptive_source_name.go",
			"package generated\n\nfunc Exported"+strconv.Itoa(index)+"() bool { return true }\n", 0o644)
	}
	started := runNegotiatedReviewStart(t, repo, "manifest-status-line-budget")
	var output bytes.Buffer
	if err := RunReview([]string{
		"status", "--cwd", repo, "--contract", ReviewIntegrationContractV2, "--next-transition",
		"--lineage", started.LineageID,
	}, &output); err != nil {
		t.Fatalf("negotiated status: %v\n%s", err, output.String())
	}
	var status ReviewTargetStatusResult
	decodeStrictReviewJSON(t, output.Bytes(), &status)
	if status.NextTransition == nil || status.NextTransition.Collect == nil || len(status.NextTransition.Collect.Inputs) == 0 {
		t.Fatalf("negotiated status did not offer a collection transition: %s", output.String())
	}
	for _, input := range status.NextTransition.Collect.Inputs {
		if input.ChangedPathManifest == nil || len(*input.ChangedPathManifest) != paths {
			t.Fatalf("collection input manifest = %v, want %d entries", input.ChangedPathManifest, paths)
		}
	}
	inputs := len(status.NextTransition.Collect.Inputs)
	if cost := manifestEntryLineCost(t, output.Bytes(), paths); cost > reviewManifestLineBudget {
		t.Fatalf("negotiated status spends %.2f lines per manifest entry across %d inputs, want at most %d (payload = %d lines, %d bytes)",
			cost, inputs, reviewManifestLineBudget, bytes.Count(output.Bytes(), []byte("\n")), output.Len())
	}
}

// TestEncodeReviewJSONLeavesNonManifestPayloadsByteIdentical is the safety pin
// for the emitter change: every envelope this product emits that carries no
// changed-path manifest must still be byte-identical to the indented encoder
// it has always used. Without this, a whitespace rewrite aimed at one field
// could silently reshape every other published envelope.
func TestEncodeReviewJSONLeavesNonManifestPayloadsByteIdentical(t *testing.T) {
	payloads := []any{
		nil,
		map[string]any{},
		[]any{},
		map[string]any{"empty_object": map[string]any{}, "empty_array": []any{}, "nested": map[string]any{"deep": []any{1, 2, 3}}},
		map[string]any{"escaped": "quote \" backslash \\ newline \n tag <b>&</b>", "unicode": "árbol ✅"},
		ReviewIntegrationFailure{Schema: "gentle-ai.review-integration.failure/v2", Operation: "review.start", Code: "invalid_request"},
	}
	for index, payload := range payloads {
		var emitted, reference bytes.Buffer
		if err := encodeReviewJSON(&emitted, payload); err != nil {
			t.Fatalf("payload %d: %v", index, err)
		}
		encoder := json.NewEncoder(&reference)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(payload); err != nil {
			t.Fatalf("payload %d reference: %v", index, err)
		}
		if !bytes.Equal(emitted.Bytes(), reference.Bytes()) {
			t.Fatalf("payload %d emitted bytes drifted from the indented encoder:\n got: %q\nwant: %q",
				index, emitted.String(), reference.String())
		}
	}
}

// TestEncodeReviewJSONPreservesManifestValue proves the bound is a whitespace
// property only: the emitted bytes must decode to exactly the manifest that
// was handed to the emitter, because the artifact subject's digest is taken
// over that value and a single altered entry would break every binding on it.
func TestEncodeReviewJSONPreservesManifestValue(t *testing.T) {
	manifest := []reviewtransaction.ChangedPathManifestEntry{
		{Path: "docs/removed.md", Status: reviewtransaction.CandidatePathDeleted, OldMode: "100644", NewMode: "000000", Deleted: true},
		{Path: "internal/auth/session.go", Status: reviewtransaction.CandidatePathAdded, OldMode: "000000", NewMode: "100644"},
		{Path: "link", Status: reviewtransaction.CandidatePathTypeChanged, OldMode: "120000", NewMode: "100644", TypeChanged: true, IntendedUntracked: true},
		{Path: "scripts/deploy \"quoted\".sh", Status: reviewtransaction.CandidatePathModified, OldMode: "100644", NewMode: "100755", ModeOnly: true},
	}
	digest, err := reviewtransaction.ChangedPathManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	source := struct {
		Schema              string                                       `json:"schema"`
		ChangedPathManifest []reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest"`
		Trailing            string                                       `json:"trailing"`
	}{Schema: "gentle-ai.review-capture-preflight/v1", ChangedPathManifest: manifest, Trailing: "after"}

	var emitted bytes.Buffer
	if err := encodeReviewJSON(&emitted, source); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Schema              string                                       `json:"schema"`
		ChangedPathManifest []reviewtransaction.ChangedPathManifestEntry `json:"changed_path_manifest"`
		Trailing            string                                       `json:"trailing"`
	}
	if err := json.Unmarshal(emitted.Bytes(), &decoded); err != nil {
		t.Fatalf("emitted manifest payload is not decodable: %v\n%s", err, emitted.String())
	}
	roundTripped, err := reviewtransaction.ChangedPathManifestDigest(decoded.ChangedPathManifest)
	if err != nil {
		t.Fatal(err)
	}
	if roundTripped != digest {
		t.Fatalf("emitted manifest digest = %q, want %q", roundTripped, digest)
	}
	if decoded.Schema != source.Schema || decoded.Trailing != source.Trailing {
		t.Fatalf("emitted payload lost sibling fields: %#v", decoded)
	}
	if cost := manifestEntryLineCost(t, emitted.Bytes(), len(manifest)); cost > reviewManifestLineBudget {
		t.Fatalf("emitter spends %.2f lines per manifest entry, want at most %d", cost, reviewManifestLineBudget)
	}
	// The compaction must not leak into a sibling array that merely follows it.
	if !strings.Contains(emitted.String(), "\n  \"trailing\": \"after\"") {
		t.Fatalf("emitted payload stopped indenting after the manifest array:\n%s", emitted.String())
	}
}

// TestEncodeReviewJSONIgnoresManifestKeyInsideStringValues proves the rewrite
// keys off real object keys rather than off matching text, so a repository path
// that happens to spell the key never reshapes a payload.
func TestEncodeReviewJSONIgnoresManifestKeyInsideStringValues(t *testing.T) {
	source := map[string]any{"note": "\"changed_path_manifest\": [ is only text here", "count": 2}
	var emitted, reference bytes.Buffer
	if err := encodeReviewJSON(&emitted, source); err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(&reference)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(source); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(emitted.Bytes(), reference.Bytes()) {
		t.Fatalf("a string value spelling the manifest key reshaped the payload:\n got: %q\nwant: %q",
			emitted.String(), reference.String())
	}
}
