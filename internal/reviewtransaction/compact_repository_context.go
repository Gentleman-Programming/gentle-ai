package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
)

func reconcileCompactRepositoryContext(ctx context.Context, store CompactStore, record CompactRecord) error {
	for _, intent := range record.EffectIntents {
		if intent.Class != "repository_context" {
			continue
		}
		markers, err := openCompactEffectMarkerRepository(ctx, store.repo)
		if err != nil {
			return err
		}
		marker := compactEffectMarker{Schema: compactEffectMarkerSchema, LineageID: record.State.LineageID,
			AuthorityRevision: record.Revision, EventID: intent.EventID}
		if existing, readErr := markers.read(marker.LineageID, marker.AuthorityRevision, marker.EventID); readErr == nil {
			if existing.State == compactEffectApplied {
				continue
			}
			if existing.State == compactEffectBlocked {
				return errors.New("repository context effect is blocked by an identity conflict")
			}
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return readErr
		}

		binding := ReviewRepositoryContextBinding{LineageID: record.State.LineageID,
			TargetIdentity: record.State.InitialSnapshot.Identity, Revision: intent.BindingRevision}
		identity, err := reviewRepositoryIdentity(ctx, store.repo)
		if err != nil {
			return writeCompactRepositoryContextMarker(markers, marker, compactEffectPending, compactEffectPendingTransient, err)
		}
		handle := reviewRepositoryContextHandle(binding, identity)
		contextRecord := reviewRepositoryContextFile{
			Schema: ReviewRepositoryContextSchema, Handle: handle, LineageID: binding.LineageID,
			TargetIdentity: binding.TargetIdentity, Revision: binding.Revision,
			RepositoryIdentity: identity.RepositoryIdentity, RepositoryRoot: identity.RepositoryRoot,
			GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
		}
		payload, err := json.Marshal(contextRecord)
		if err != nil {
			return err
		}
		if handle != intent.Destination || hashPayloadBytes(payload) != intent.PayloadHash {
			return writeCompactRepositoryContextMarker(markers, marker, compactEffectBlocked, compactEffectBlockedConflict,
				errors.New("repository context effect binding or payload does not match committed intent"))
		}
		path, err := reviewRepositoryContextPath(handle)
		if err == nil {
			var home string
			home, err = reviewRepositoryContextHome()
			if err == nil {
				var root string
				root, err = ensureReviewRepositoryContextStorageRoot(home, true)
				if err == nil {
					err = ensurePrivateLocatorDirectory(root, filepath.Dir(path))
				}
			}
		}
		if err == nil {
			err = publishReviewRepositoryContext(path, append(payload, '\n'))
		}
		if err != nil {
			return writeCompactRepositoryContextMarker(markers, marker, compactEffectPending, compactEffectPendingTransient, err)
		}
		if err := writeCompactRepositoryContextMarker(markers, marker, compactEffectApplied, compactEffectAppliedDurable, nil); err != nil {
			return err
		}
	}
	return nil
}

func writeCompactRepositoryContextMarker(repository compactEffectMarkerRepository, marker compactEffectMarker, state compactEffectMarkerState, observation compactEffectObservation, cause error) error {
	marker.State, marker.Observation = state, observation
	publication, err := repository.write(marker)
	if err != nil {
		return err
	}
	if cause != nil {
		return cause
	}
	if publication.DurabilityLimited {
		return errors.New("repository context marker durability is limited")
	}
	return nil
}

func hashPayloadBytes(payload []byte) string {
	return "sha256:" + identityHash(string(payload))
}
