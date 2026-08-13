package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
)

type CompactRepositoryContextOutcome string

const (
	CompactRepositoryContextApplied           CompactRepositoryContextOutcome = "applied"
	CompactRepositoryContextPending           CompactRepositoryContextOutcome = "pending"
	CompactRepositoryContextBlocked           CompactRepositoryContextOutcome = "blocked_conflict"
	CompactRepositoryContextDurabilityLimited CompactRepositoryContextOutcome = "durability_limited"
)

type CompactRepositoryContextResult struct {
	Handle  string
	EventID string
	Outcome CompactRepositoryContextOutcome
}

func compactRepositoryContextIntent(ctx context.Context, repo string, state CompactState) (CompactEffectIntent, error) {
	statePayload, err := json.Marshal(state)
	if err != nil {
		return CompactEffectIntent{}, err
	}
	binding := ReviewRepositoryContextBinding{LineageID: state.LineageID, TargetIdentity: state.InitialSnapshot.Identity, Revision: compactStateRevision(statePayload)}
	identity, err := reviewRepositoryIdentity(ctx, repo)
	if err != nil {
		return CompactEffectIntent{}, err
	}
	handle := reviewRepositoryContextHandle(binding, identity)
	payload, err := json.Marshal(reviewRepositoryContextFile{
		Schema: ReviewRepositoryContextSchema, Handle: handle, LineageID: binding.LineageID,
		TargetIdentity: binding.TargetIdentity, Revision: binding.Revision, RepositoryIdentity: identity.RepositoryIdentity,
		RepositoryRoot: identity.RepositoryRoot, GitCommonDir: identity.GitCommonDir, GitDir: identity.GitDir,
	})
	if err != nil {
		return CompactEffectIntent{}, err
	}
	return CompactEffectIntent{Class: "repository_context", Destination: handle, PayloadHash: hashPayloadBytes(payload)}, nil
}

func ReconcileCompactRepositoryContext(ctx context.Context, store CompactStore, record CompactRecord) (CompactRepositoryContextResult, error) {
	var selected *CompactEffectIntent
	for index := range record.EffectIntents {
		if record.EffectIntents[index].Class == "repository_context" {
			if selected != nil {
				return CompactRepositoryContextResult{}, errors.New("compact authority has multiple repository context effects") // refusal:by-design world-action: persisted authority violates the one-event schema and no operator command may rewrite it
			}
			selected = &record.EffectIntents[index]
		}
	}
	if selected == nil {
		return CompactRepositoryContextResult{}, errors.New("compact authority has no repository context effect") // refusal:by-design operator-knowledge: this reconciler only accepts authority created by negotiated START
	}
	err := reconcileCompactRepositoryContext(ctx, store, record)
	markers, openErr := openCompactEffectMarkerRepository(ctx, store.repo)
	if openErr != nil {
		return CompactRepositoryContextResult{}, openErr
	}
	marker, readErr := markers.read(record.State.LineageID, record.Revision, selected.EventID)
	if readErr != nil {
		return CompactRepositoryContextResult{}, errors.Join(err, readErr)
	}
	return CompactRepositoryContextResult{Handle: selected.Destination, EventID: selected.EventID, Outcome: CompactRepositoryContextOutcome(marker.State)}, err
}

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
				return errors.New("repository context effect is blocked by an identity conflict") // refusal:by-design world-action: an immutable marker proves committed effect identity conflict
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
				errors.New("repository context effect binding or payload does not match committed intent")) // refusal:by-design world-action: committed effect identity cannot be rewritten by an operator
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
		return errors.New("repository context marker durability is limited") // refusal:by-design world-action: the platform could not prove directory durability; retrying the same event may promote it
	}
	return nil
}

func hashPayloadBytes(payload []byte) string {
	return "sha256:" + identityHash(string(payload))
}
