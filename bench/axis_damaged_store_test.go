package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file belongs to the damaged-store axis and is deleted with it:
// `rm bench/axis_damaged_store*.go` removes the axis whole, and the core corpus
// still builds, tests and reports exactly as it did.
//
// What it covers is the ONE piece of the axis that a run cannot cover for
// itself. The fixtures' damage is proven at runtime against the product, which
// is stronger than any golden file: a golden goes stale the moment the format
// moves, and the runtime check is the thing that catches that. But the ordered
// JSON codec underneath it can be wrong for reasons that have nothing to do
// with the product — a key reordered, an escape rule missed — and those are
// worth catching without a binary in hand.

// The product marshals with encoding/json, which HTML-escapes `<`, `>` and `&`
// inside strings, preserves field declaration order, and emits numbers as
// written. A real recovery provenance carries all three: the actor is a git
// identity with angle brackets, the generation is an integer, and the state's
// key order is what the revision binds.
func TestOrderedJSONReproducesTheProductsCanonicalBytes(t *testing.T) {
	source := `{
  "schema": "gentle-ai.review-state-record/v2",
  "revision": "sha256:abc",
  "state": {
    "lineage_id": "review-successor-one",
    "generation": 2,
    "original_changed_lines": 6,
    "paths": ["docs/intro.md", "docs/widened.md"],
    "classifications": {},
    "follow_ups": [],
    "recovery": {
      "actor": "Bench <bench@example.invalid>",
      "reason": "self-recovery: approved authority, recover action available"
    }
  }
}`
	decoded, err := decodeOrderedJSON([]byte(source))
	if err != nil {
		t.Fatalf("decodeOrderedJSON: %v", err)
	}
	state, ok := orderedMember(decoded, "state")
	if !ok {
		t.Fatal("decoded record carries no state")
	}
	var buffer bytes.Buffer
	if err := encodeOrderedJSON(state, &buffer); err != nil {
		t.Fatalf("encodeOrderedJSON: %v", err)
	}

	want := `{"lineage_id":"review-successor-one","generation":2,"original_changed_lines":6,` +
		`"paths":["docs/intro.md","docs/widened.md"],"classifications":{},"follow_ups":[],` +
		// HTML-escaped, because encoding/json escapes `<` and `>` by default
		// and the product's revision binds those escaped bytes.
		`"recovery":{"actor":"Bench \u003cbench@example.invalid\u003e",` +
		`"reason":"self-recovery: approved authority, recover action available"}}`
	if buffer.String() != want {
		t.Fatalf("canonical re-encoding does not match what the product hashes:\n got %s\nwant %s", buffer.String(), want)
	}
}

// A record whose recorded revision does not re-derive is refused at load, with
// a message naming the reason. That check is this axis's format-drift tripwire,
// and a fixture must never be allowed past it.
func TestLoadStoreRecordRefusesARevisionItCannotReproduce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-state.json")
	record := `{"schema":"x","revision":"sha256:not-the-hash","state":{"lineage_id":"a"}}`
	if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadStoreRecord(path)
	if err == nil {
		t.Fatal("a record whose revision does not re-derive was accepted; every fixture after that point writes bytes the product would refuse")
	}
	if !strings.Contains(err.Error(), "can no longer reproduce") {
		t.Fatalf("the refusal must name the drift it detected, got: %v", err)
	}
}

// The axis declares itself as not black-box. That is the claim the report
// prints verbatim, so it is pinned here rather than left to a reviewer's eye.
func TestDamagedStoreAxisDeclaresItIsNotBlackBox(t *testing.T) {
	found := false
	for _, axis := range Axes() {
		if axis.Name != damagedStoreAxis {
			continue
		}
		found = true
		if axis.BlackBox {
			t.Fatal("the damaged-store axis authors store bytes; declaring it black-box would be a false claim in every report it appears in")
		}
		if len(axis.Journeys()) == 0 {
			t.Fatal("the damaged-store axis contributes no journeys")
		}
		for _, journey := range axis.Journeys() {
			if !strings.HasPrefix(journey.ID, "ds") {
				t.Fatalf("axis journey %q does not carry the axis id prefix, so a reader cannot tell it from a core journey at a glance", journey.ID)
			}
		}
	}
	if !found {
		t.Fatal("the damaged-store axis is not registered")
	}
}
