package main

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateJourneyManifest = flag.Bool("update-journey-manifest", false,
	"rewrite bench/testdata/journeys.manifest from the registered corpus")

const journeyManifestPath = "testdata/journeys.manifest"

// TestRegisteredJourneysMatchTheManifest replaces the hand-written total that
// used to guard this corpus.
//
// The property is unchanged and is still the point: a journey must not appear
// or vanish unnoticed. What changed is how it is recorded. A single integer
// merges badly by construction, because two branches that each add one journey
// each write the same next number, and git resolves that identical one-line
// change by silently taking one side. Both are green alone and the second to
// land leaves the count describing a tree that does not exist. That is not
// hypothetical: it happened on 2026-08-11, when #3037 and #3038 each moved the
// count 93 -> 94 and main went red at 95 journeys under a baseline of 94, and
// it nearly happened again the same day with #3039 and #2139.
//
// A sorted manifest of IDs fixes the dangerous half and is honest about the
// rest. Measured on a staged three-way merge rather than assumed:
//
//	two branches, each adding one journey     manifest        counter
//	ids that sort apart (j10 and j70)         merges cleanly  silently 4, truth 5
//	ids that sort adjacent (jAA and jBB)      CONFLICTS       silently 4, truth 5
//
// So the manifest does not always merge cleanly, and claiming otherwise would
// be wrong. What it always does is refuse to be silently wrong: adjacent
// additions produce a conflict, which is the correct outcome because it stops
// and asks. The counter cannot do that, because both branches write the same
// bytes and git has nothing to disagree about.
//
// The rest is preserved and improved: a vanished journey is still caught, and
// the failure now names which one instead of leaving a reader to diff two
// numbers.
func TestRegisteredJourneysMatchTheManifest(t *testing.T) {
	registered := make([]string, 0, len(Journeys()))
	seen := map[string]bool{}
	for _, journey := range Journeys() {
		if seen[journey.ID] {
			t.Errorf("journey ID %q collides in the corpus", journey.ID)
		}
		seen[journey.ID] = true
		registered = append(registered, journey.ID)
	}
	sort.Strings(registered)

	if *updateJourneyManifest {
		if err := os.WriteFile(journeyManifestPath, []byte(strings.Join(registered, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s with %d journeys", journeyManifestPath, len(registered))
		return
	}

	raw, err := os.ReadFile(journeyManifestPath)
	if err != nil {
		t.Fatalf("read %s: %v\nRun: go test ./bench -run TestRegisteredJourneysMatchTheManifest -update-journey-manifest",
			filepath.ToSlash(journeyManifestPath), err)
	}
	expected := make([]string, 0, len(registered))
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			expected = append(expected, trimmed)
		}
	}

	inManifest := map[string]bool{}
	for _, id := range expected {
		inManifest[id] = true
	}
	var added, removed []string
	for _, id := range registered {
		if !inManifest[id] {
			added = append(added, id)
		}
	}
	for _, id := range expected {
		if !seen[id] {
			removed = append(removed, id)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		if !sort.StringsAreSorted(expected) {
			t.Errorf("%s is not sorted; a sorted file is what makes concurrent additions merge cleanly", journeyManifestPath)
		}
		return
	}
	for _, id := range added {
		t.Errorf("journey %q is registered but absent from %s", id, journeyManifestPath)
	}
	for _, id := range removed {
		t.Errorf("journey %q is in %s but no longer registered", id, journeyManifestPath)
	}
	t.Logf("Adding or removing a journey is deliberate. Record it with:\n  go test ./bench -run TestRegisteredJourneysMatchTheManifest -update-journey-manifest")
}
