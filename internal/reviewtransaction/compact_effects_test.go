package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCompactEffectMarkerStrictValidation(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "strict-validation")
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		t.Fatal(err)
	}
	valid := markerPayload(marker, func(*compactEffectMarker) {})
	cases := []struct {
		name    string
		payload []byte
	}{
		{"valid", valid},
		{"malformed", []byte("{")},
		{"trailing JSON", append(append([]byte(nil), valid...), []byte("{}")...)},
		{"unknown field", []byte(`{"schema":"gentle-ai.review-effect-marker/v1","lineage_id":"strict-validation","authority_revision":"` + hash("a") + `","event_id":"` + hash("b") + `","state":"pending","observation":"pending_transient","extra":true}`)},
		{"wrong schema", markerPayload(marker, func(value *compactEffectMarker) { value.Schema = "wrong" })},
		{"wrong lineage", markerPayload(marker, func(value *compactEffectMarker) { value.LineageID = "wrong" })},
		{"wrong revision", markerPayload(marker, func(value *compactEffectMarker) { value.AuthorityRevision = hash("c") })},
		{"wrong event", markerPayload(marker, func(value *compactEffectMarker) { value.EventID = hash("d") })},
		{"invalid state", markerPayload(marker, func(value *compactEffectMarker) { value.State = "unknown" })},
		{"invalid observation", markerPayload(marker, func(value *compactEffectMarker) { value.Observation = "unknown" })},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(path, tt.payload, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID)
			if tt.name == "valid" {
				if err != nil || got != marker {
					t.Fatalf("marker = %#v, %v", got, err)
				}
			} else if err == nil {
				t.Fatal("accepted invalid marker")
			}
		})
	}
}

func TestCompactEffectMarkerRejectsUnsafeStorageAndIdentity(t *testing.T) {
	repository, marker := newCompactEffectMarkerFixture(t, "unsafe-storage")
	for _, identity := range []struct{ lineage, revision, event string }{{"../escape", marker.AuthorityRevision, marker.EventID}, {marker.LineageID, "bad", marker.EventID}, {marker.LineageID, marker.AuthorityRevision, "bad"}} {
		if _, err := repository.path(identity.lineage, identity.revision, identity.event, true); err == nil {
			t.Fatal("accepted invalid path component")
		}
	}
	if err := os.MkdirAll(filepath.Dir(repository.root), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), repository.root); err == nil {
		if _, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true); err == nil {
			t.Fatal("accepted symlink root")
		}
	} else if !errors.Is(err, os.ErrPermission) && !errors.Is(err, errors.ErrUnsupported) {
		t.Fatal(err)
	}

	repository, marker = newCompactEffectMarkerFixture(t, "non-regular")
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); err == nil {
		t.Fatal("accepted non-regular marker")
	}
}

func newCompactEffectMarkerFixture(t *testing.T, lineage string) (compactEffectMarkerRepository, compactEffectMarker) {
	t.Helper()
	repo := initSnapshotRepo(t)
	repository, err := openCompactEffectMarkerRepository(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	return repository, compactEffectMarker{Schema: compactEffectMarkerSchema, LineageID: lineage, AuthorityRevision: hash("a"), EventID: hash("b"), State: compactEffectPending, Observation: compactEffectPendingTransient}
}

func markerPayload(marker compactEffectMarker, mutate func(*compactEffectMarker)) []byte {
	mutate(&marker)
	payload, _ := json.Marshal(marker)
	return payload
}
