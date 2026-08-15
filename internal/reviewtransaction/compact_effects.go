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
	compactEffectDurable          compactEffectObservation = "durable"
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
type compactEffectPublication struct{ DurabilityLimited bool }

func openCompactEffectMarkerRepository(ctx context.Context, repo string) (compactEffectMarkerRepository, error) {
	base, _, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return compactEffectMarkerRepository{}, err
	}
	return compactEffectMarkerRepository{root: filepath.Join(base, "effect-markers", "v1")}, nil
}

func (repository compactEffectMarkerRepository) write(ctx context.Context, marker compactEffectMarker) (compactEffectPublication, error) {
	if err := validateCompactEffectMarker(marker, marker.LineageID, marker.AuthorityRevision, marker.EventID); err != nil {
		return compactEffectPublication{}, err
	}
	path, err := repository.path(marker.LineageID, marker.AuthorityRevision, marker.EventID, true)
	if err != nil {
		return compactEffectPublication{}, err
	}
	lock, err := acquireStoreLockForConvergentCompletion(ctx, path+".lock")
	if err != nil {
		return compactEffectPublication{}, err
	}
	defer lock.release()
	old, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID)
	if err == nil {
		if old.State == compactEffectApplied {
			if old == marker {
				return compactEffectPublication{}, nil
			}
			return compactEffectPublication{}, errors.New("applied compact effect marker is immutable") // refusal:by-design operator-knowledge: a caller may only replay the exact applied value
		}
		if old.State == compactEffectBlocked && marker.State == compactEffectPending {
			return compactEffectPublication{}, errors.New("compact effect marker transition regressed") // refusal:by-design operator-knowledge: callers must submit a monotonic state
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return compactEffectPublication{}, err
	}
	payload, _ := json.Marshal(marker)
	publication := compactEffectPublication{}
	var publicationErr error
	if err := writePrivateRARAtomic(path, append(payload, '\n')); err != nil {
		var syncErr *directorySyncError
		if !errors.As(err, &syncErr) {
			return publication, err
		}
		publication.DurabilityLimited = true
		if marker.State == compactEffectApplied {
			marker.Observation = compactEffectPlatformLimited
			payload, _ = json.Marshal(marker)
			if rewriteErr := writePrivateRARAtomic(path, append(payload, '\n')); rewriteErr != nil && !errors.As(rewriteErr, &syncErr) {
				return publication, errors.Join(err, rewriteErr)
			}
		}
		publicationErr = err
	}
	got, err := repository.read(marker.LineageID, marker.AuthorityRevision, marker.EventID)
	if err != nil || got != marker {
		return publication, errors.Join(publicationErr, err, errors.New("compact effect marker read-back mismatch")) // refusal:by-design world-action: storage did not retain the exact atomic replacement
	}
	return publication, nil
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
	validPair := marker.State == compactEffectPending && marker.Observation == compactEffectPendingTransient ||
		marker.State == compactEffectBlocked && marker.Observation == compactEffectBlockedConflict ||
		marker.State == compactEffectApplied && (marker.Observation == compactEffectDurable || marker.Observation == compactEffectPlatformLimited)
	if marker.Schema != compactEffectMarkerSchema || marker.LineageID != lineageID || marker.AuthorityRevision != revision || marker.EventID != eventID || !validPair {
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
		markerRoot := filepath.Dir(repository.root)
		created, err := createPrivateRARDirectory(markerRoot)
		if err != nil {
			return "", err
		}
		if created {
			if err := SyncReviewDirectory(base); err != nil {
				return "", err
			}
		}
		if err := validatePrivateRARDirectory(markerRoot); err != nil {
			return "", err
		}
		created, err = createPrivateRARDirectory(repository.root)
		if err != nil {
			return "", err
		}
		if created {
			if err := SyncReviewDirectory(markerRoot); err != nil {
				return "", err
			}
		}
		if err := validatePrivateRARDirectory(repository.root); err != nil {
			return "", err
		}
	}
	dir := filepath.Join(repository.root, lineageID, revision[7:])
	if err := ensurePrivateRARDirectoryTree(repository.root, dir, create); err != nil {
		return "", err
	}
	return filepath.Join(dir, eventID[7:]+".json"), nil
}
