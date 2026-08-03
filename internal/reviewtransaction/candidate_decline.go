package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	CandidateDeclineAuthorizationSchema = "gentle-ai.rdd-candidate-decline/v1"

	candidateDeclineDirectory    = "candidate-declines"
	candidateDeclineLockName     = "LOCK"
	candidateDeclineRecordPrefix = "candidate-"
	candidateDeclineRecordSuffix = ".json"
	candidateDeclineDigestDomain = "gentle-ai.rdd-candidate-decline-digest/v1"
)

var (
	// ErrCandidateDeclineCorrupt rejects a candidate-scoped decline whose durable
	// bytes cannot prove the exact snapshot the operator declined to review.
	// refusal:by-design world-action: corrupted private authority bytes must be repaired or removed before they can govern delivery
	ErrCandidateDeclineCorrupt = errors.New("candidate decline authorization is corrupt")
	// ErrCandidateDeclineAmbiguous rejects more than one candidate-scoped decline
	// that would authorize a lifecycle target. The gate must not choose between
	// independent durable authorizations.
	// refusal:by-design human-authority: only the operator can decide which independent candidate choice remains authoritative
	ErrCandidateDeclineAmbiguous = errors.New("candidate decline authorization is ambiguous")
	// ErrCandidateDeclineStale rejects a target or publication boundary that moved
	// while a candidate-scoped decline was being evaluated.
	// refusal:by-design human-authority: a changed candidate requires a fresh explicit review choice
	ErrCandidateDeclineStale = errors.New("candidate decline authorization no longer matches the live lifecycle target")
)

// CandidateDeclineAuthorization is a durable, content-addressed opt-out for one
// frozen candidate. It is intentionally not review authority: it contains no
// lineage, receipt, verdict, or approval.
type CandidateDeclineAuthorization struct {
	Schema           string   `json:"schema"`
	Snapshot         Snapshot `json:"snapshot"`
	AuthorizationRef string   `json:"authorization_ref"`
}

// CandidateDeclineGateAuthorization is the exact decline record that matched a
// lifecycle target. Its presence permits ordinary repository policy to decide the
// allowed lifecycle gate; it is never a receipt or a release authorization.
type CandidateDeclineGateAuthorization struct {
	Snapshot         Snapshot
	AuthorizationRef string
}

// RecordCandidateDecline atomically records a relayed consent decline after
// proving the candidate is still the exact live snapshot. Exact replay publishes
// identical canonical bytes and converges without creating review authority.
func RecordCandidateDecline(ctx context.Context, repo string, snapshot Snapshot) (CandidateDeclineAuthorization, error) {
	if err := validateCompactSnapshot(snapshot); err != nil {
		return CandidateDeclineAuthorization{}, fmt.Errorf("validate candidate decline snapshot: %w", err)
	}
	root, lease, err := candidateDeclineRoot(ctx, repo, true)
	if err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(root, candidateDeclineLockName))
	if err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	defer func() { _ = lock.release() }()
	if err := lease.Validate(ctx); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	if err := (SnapshotBuilder{Repo: lease.Identity().RepositoryRoot}).ValidateLiveSnapshot(ctx, snapshot); err != nil {
		return CandidateDeclineAuthorization{}, fmt.Errorf("%w: %v", ErrCandidateDeclineStale, err)
	}
	authorization := CandidateDeclineAuthorization{
		Schema:   CandidateDeclineAuthorizationSchema,
		Snapshot: snapshot,
	}
	if authorization.AuthorizationRef, err = candidateDeclineDigest(authorization); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	payload, err := canonicalCandidateDeclinePayload(authorization)
	if err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	path := filepath.Join(root, candidateDeclineRecordName(snapshot.Identity))
	if existing, readErr := readPrivateRARFile(path); readErr == nil {
		if _, parseErr := parseCandidateDeclineAuthorization(existing); parseErr != nil {
			return CandidateDeclineAuthorization{}, fmt.Errorf("%w: %v", ErrCandidateDeclineCorrupt, parseErr)
		}
		if !bytes.Equal(existing, payload) {
			return CandidateDeclineAuthorization{}, fmt.Errorf("%w: identity slot conflicts with different canonical content", ErrCandidateDeclineCorrupt)
		}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return CandidateDeclineAuthorization{}, fmt.Errorf("%w: %v", ErrCandidateDeclineCorrupt, readErr)
	}
	if err := publishPrivateRARImmutable(path, payload); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	if err := lease.Validate(ctx); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	return authorization, nil
}

// ResolveCandidateDeclineForGate re-derives a supported lifecycle target with
// the compact gate's own target builder, then accepts exactly one decline whose
// frozen bytes and paths still match. It never authorizes release or post-apply.
func ResolveCandidateDeclineForGate(
	ctx context.Context,
	repo string,
	input NativeGateRequestInput,
) (CandidateDeclineGateAuthorization, bool, error) {
	if input.Gate != GatePreCommit && input.Gate != GatePrePush && input.Gate != GatePrePR {
		return CandidateDeclineGateAuthorization{}, false, nil
	}
	root, lease, err := candidateDeclineRoot(ctx, repo, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return CandidateDeclineGateAuthorization{}, false, nil
		}
		return CandidateDeclineGateAuthorization{}, false, err
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(root, candidateDeclineLockName))
	if err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	defer func() { _ = lock.release() }()
	if err := lease.Validate(ctx); err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	authorizations, err := readCandidateDeclineAuthorizations(root)
	if err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	// Lifecycle projections can exclude untracked work after an intended path is
	// staged or committed. Discover it separately so a new untracked path cannot
	// hide outside a staged, push, or pre-PR candidate tree and inherit the decline.
	liveUntracked, err := (SnapshotBuilder{Repo: lease.Identity().RepositoryRoot}).DiscoverIntendedUntracked(ctx)
	if err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	if len(liveUntracked) != 0 {
		return CandidateDeclineGateAuthorization{}, false, nil
	}

	type match struct {
		authorization CandidateDeclineAuthorization
		request       GateRequest
		snapshot      Snapshot
		refs          *resolvedPrePRRefs
	}
	matches := []match{}
	for _, authorization := range authorizations {
		builder := SnapshotBuilder{Repo: lease.Identity().RepositoryRoot}
		if err := builder.ValidateEvidence(ctx, authorization.Snapshot); err != nil {
			return CandidateDeclineGateAuthorization{}, false, fmt.Errorf("%w: declined snapshot evidence: %v", ErrCandidateDeclineCorrupt, err)
		}
		request, snapshot, refs, err := candidateDeclineLifecycleSnapshot(ctx, lease.Identity().RepositoryRoot, authorization.Snapshot, input)
		if err != nil {
			return CandidateDeclineGateAuthorization{}, false, err
		}
		if candidateDeclineMatchesLifecycleSnapshot(authorization.Snapshot, snapshot, input.Gate) {
			matches = append(matches, match{authorization: authorization, request: request, snapshot: snapshot, refs: refs})
		}
	}
	if len(matches) == 0 {
		return CandidateDeclineGateAuthorization{}, false, nil
	}
	if len(matches) != 1 {
		return CandidateDeclineGateAuthorization{}, false, ErrCandidateDeclineAmbiguous
	}
	selected := matches[0]
	request, finalSnapshot, finalRefs, err := candidateDeclineLifecycleSnapshot(ctx, lease.Identity().RepositoryRoot, selected.authorization.Snapshot, input)
	if err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	if err := lease.Validate(ctx); err != nil {
		return CandidateDeclineGateAuthorization{}, false, err
	}
	if !reflect.DeepEqual(request, selected.request) || !snapshotsEqual(finalSnapshot, selected.snapshot) || !sameResolvedPrePRRefs(finalRefs, selected.refs) ||
		!candidateDeclineMatchesLifecycleSnapshot(selected.authorization.Snapshot, finalSnapshot, input.Gate) {
		return CandidateDeclineGateAuthorization{}, false, ErrCandidateDeclineStale
	}
	return CandidateDeclineGateAuthorization{
		Snapshot:         selected.authorization.Snapshot,
		AuthorizationRef: selected.authorization.AuthorizationRef,
	}, true, nil
}

func candidateDeclineLifecycleSnapshot(
	ctx context.Context,
	repo string,
	snapshot Snapshot,
	input NativeGateRequestInput,
) (GateRequest, Snapshot, *resolvedPrePRRefs, error) {
	state := CompactState{InitialSnapshot: snapshot, CurrentSnapshot: snapshot}
	request, _, err := buildCompactGateRequest(ctx, repo, state, input)
	if err != nil {
		return GateRequest{}, Snapshot{}, nil, err
	}
	live, refs, err := buildCompactLifecycleSnapshot(ctx, repo, request)
	if err != nil {
		return GateRequest{}, Snapshot{}, nil, err
	}
	return request, live, refs, nil
}

func candidateDeclineMatchesLifecycleSnapshot(expected, actual Snapshot, gate GateKind) bool {
	if expected.UnbornHead != actual.UnbornHead || expected.BaseTree != actual.BaseTree ||
		expected.CandidateTree != actual.CandidateTree || expected.PathsDigest != actual.PathsDigest ||
		!equalStrings(expected.Paths, actual.Paths) {
		return false
	}
	// Pre-commit, pre-push, and pre-PR deliberately turn an intended untracked
	// candidate path into an index or committed tree entry. Its content and mode
	// remain covered by candidate_tree and paths_digest; requiring the historical
	// untracked proof here would make the ordinary delivery transition impossible.
	if len(actual.IntendedUntracked) == 0 {
		return true
	}
	return equalStrings(expected.IntendedUntracked, actual.IntendedUntracked) &&
		expected.IntendedUntrackedProof == actual.IntendedUntrackedProof
}

func candidateDeclineRoot(ctx context.Context, repo string, create bool) (string, *RepositoryIdentityLease, error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return "", nil, fmt.Errorf("resolve candidate decline repository identity: %w", err)
	}
	identity := reviewRepositoryIdentityRecordFromLease(lease)
	base := filepath.Join(identity.GitCommonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory, rarAuthorityVersion)
	if err := ensureRARRepositoryRoot(identity.GitCommonDir, base, create); err != nil {
		return "", nil, err
	}
	root := filepath.Join(base, candidateDeclineDirectory)
	if err := ensurePrivateRARDirectoryTree(base, root, create); err != nil {
		return "", nil, err
	}
	return root, lease, nil
}

func readCandidateDeclineAuthorizations(root string) ([]CandidateDeclineAuthorization, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCandidateDeclineCorrupt, err)
	}
	authorizations := []CandidateDeclineAuthorization{}
	for _, entry := range entries {
		name := entry.Name()
		if name == candidateDeclineLockName || strings.HasPrefix(name, ".rar-") {
			continue
		}
		identity, ok := candidateDeclineRecordIdentity(name)
		if !ok || entry.IsDir() {
			return nil, fmt.Errorf("%w: unexpected candidate decline entry %q", ErrCandidateDeclineCorrupt, name)
		}
		payload, err := readPrivateRARFile(filepath.Join(root, name))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCandidateDeclineCorrupt, err)
		}
		authorization, err := parseCandidateDeclineAuthorization(payload)
		if err != nil || authorization.Snapshot.Identity != identity {
			if err == nil {
				// refusal:by-design world-action: mismatched private authority bytes must be repaired or removed before delivery
				err = errors.New("candidate decline filename does not match the frozen target")
			}
			return nil, fmt.Errorf("%w: %v", ErrCandidateDeclineCorrupt, err)
		}
		authorizations = append(authorizations, authorization)
	}
	return authorizations, nil
}

func candidateDeclineRecordName(identity string) string {
	return candidateDeclineRecordPrefix + strings.TrimPrefix(identity, "sha256:") + candidateDeclineRecordSuffix
}

func candidateDeclineRecordIdentity(name string) (string, bool) {
	if !strings.HasPrefix(name, candidateDeclineRecordPrefix) || !strings.HasSuffix(name, candidateDeclineRecordSuffix) {
		return "", false
	}
	digest := strings.TrimSuffix(strings.TrimPrefix(name, candidateDeclineRecordPrefix), candidateDeclineRecordSuffix)
	identity := "sha256:" + digest
	return identity, validSHA256(identity)
}

func (authorization CandidateDeclineAuthorization) validate() error {
	if authorization.Schema != CandidateDeclineAuthorizationSchema {
		// refusal:by-design world-action: an unknown durable schema cannot safely authorize delivery
		return errors.New("invalid candidate decline authorization schema")
	}
	if err := validateCompactSnapshot(authorization.Snapshot); err != nil {
		return fmt.Errorf("invalid candidate decline snapshot: %w", err)
	}
	if !validSHA256(authorization.AuthorizationRef) {
		// refusal:by-design world-action: an invalid content address cannot bind durable authorization bytes
		return errors.New("invalid candidate decline authorization ref")
	}
	want, err := candidateDeclineDigest(authorization)
	if err != nil {
		return err
	}
	if authorization.AuthorizationRef != want {
		// refusal:by-design world-action: modified durable bytes cannot retain authority from their previous content address
		return errors.New("candidate decline authorization ref does not match canonical content")
	}
	return nil
}

func candidateDeclineDigest(authorization CandidateDeclineAuthorization) (string, error) {
	authorization.AuthorizationRef = ""
	payload, err := json.Marshal(authorization)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	_, _ = sum.Write([]byte(candidateDeclineDigestDomain))
	_, _ = sum.Write([]byte{0})
	_, _ = sum.Write(payload)
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}

func canonicalCandidateDeclinePayload(authorization CandidateDeclineAuthorization) ([]byte, error) {
	if err := authorization.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(authorization)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseCandidateDeclineAuthorization(payload []byte) (CandidateDeclineAuthorization, error) {
	var authorization CandidateDeclineAuthorization
	if err := decodeStrictRARJSON(payload, &authorization); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	if err := authorization.validate(); err != nil {
		return CandidateDeclineAuthorization{}, err
	}
	canonical, err := canonicalCandidateDeclinePayload(authorization)
	if err != nil || !bytes.Equal(payload, canonical) {
		// refusal:by-design world-action: non-canonical durable bytes must be repaired or removed before delivery
		return CandidateDeclineAuthorization{}, errors.New("candidate decline authorization is not canonical")
	}
	return authorization, nil
}
