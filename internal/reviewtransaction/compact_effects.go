package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const compactEffectMarkerSchema = "gentle-ai.review-effect-marker/v1"

type compactEffectMarkerState string
type compactEffectObservation string

const (
	compactEffectPending          compactEffectMarkerState = "pending"
	compactEffectBlocked          compactEffectMarkerState = "blocked_conflict"
	compactEffectApplied          compactEffectMarkerState = "applied"
	compactEffectPendingTransient compactEffectObservation = "pending_transient"
	compactEffectBlockedConflict  compactEffectObservation = "blocked_conflict"
	compactEffectPlatformLimited  compactEffectObservation = "platform_durability_limited"
)

type compactEffectMarker struct {
	Schema            string                   `json:"schema"`
	LineageID         string                   `json:"lineage_id"`
	AuthorityRevision string                   `json:"authority_revision"`
	EventID           string                   `json:"event_id"`
	State             compactEffectMarkerState `json:"state"`
	Observation       compactEffectObservation `json:"observation"`
}

type compactEffectMarkerRepository struct{ root string }

func openCompactEffectMarkerRepository(ctx context.Context, repo string) (compactEffectMarkerRepository, error) {
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return compactEffectMarkerRepository{}, err
	}
	return compactEffectMarkerRepository{root: filepath.Join(base, "effect-markers", "v1")}, nil
}

func (repository compactEffectMarkerRepository) read(lineageID, revision, eventID string) (compactEffectMarker, error) {
	path, err := repository.path(lineageID, revision, eventID, false)
	if err != nil {
		return compactEffectMarker{}, err
	}
	payload, err := readPrivateRARFile(path)
	if err != nil {
		return compactEffectMarker{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var marker compactEffectMarker
	if decoder.Decode(&marker) != nil {
		return compactEffectMarker{}, errors.New("invalid compact effect marker JSON") // refusal:by-design world-action: private persisted marker bytes are corrupt
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return compactEffectMarker{}, errors.New("invalid compact effect marker JSON") // refusal:by-design world-action: private persisted marker bytes are corrupt
	}
	if err := validateCompactEffectMarker(marker, lineageID, revision, eventID); err != nil {
		return compactEffectMarker{}, err
	}
	return marker, nil
}

func validateCompactEffectMarker(marker compactEffectMarker, lineageID, revision, eventID string) error {
	validState := marker.State == compactEffectPending || marker.State == compactEffectBlocked || marker.State == compactEffectApplied
	validObservation := marker.Observation == compactEffectPendingTransient || marker.Observation == compactEffectBlockedConflict || marker.Observation == compactEffectPlatformLimited
	if marker.Schema != compactEffectMarkerSchema || marker.LineageID != lineageID || marker.AuthorityRevision != revision || marker.EventID != eventID || !validState || !validObservation {
		return errors.New("invalid compact effect marker") // refusal:by-design operator-knowledge: marker schema, binding, state, and observation are closed
	}
	return nil
}

func (repository compactEffectMarkerRepository) path(lineageID, revision, eventID string, create bool) (string, error) {
	if validateLineageID(lineageID) != nil || !validSHA256(revision) || !validSHA256(eventID) {
		return "", errors.New("invalid compact effect marker identity") // refusal:by-design operator-knowledge: marker identities must be validated authority components
	}
	if create {
		base := filepath.Dir(filepath.Dir(repository.root))
		if err := os.MkdirAll(base, 0o700); err != nil {
			return "", err
		}
		for _, dir := range []string{filepath.Dir(repository.root), repository.root} {
			if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return "", err
			}
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
				return "", errors.New("unsafe compact effect marker root") // refusal:by-design world-action: private marker storage was substituted or made unsafe
			}
			if err := SyncReviewDirectory(filepath.Dir(dir)); err != nil {
				return "", err
			}
		}
	}
	dir := filepath.Join(repository.root, lineageID, revision[7:])
	if err := ensurePrivateRARDirectoryTree(repository.root, dir, create); err != nil {
		return "", err
	}
	return filepath.Join(dir, eventID[7:]+".json"), nil
}
