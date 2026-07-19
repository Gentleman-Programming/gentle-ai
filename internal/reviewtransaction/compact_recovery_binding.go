package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
)

// CompactRecoveryBindingDomain separates every hash the composed
// scope_changed recovery binding derives from other review identities.
const CompactRecoveryBindingDomain = "gentle-ai.compact-recovery-binding/v1"

// compactRecoveryBinding composes the publication-binding inputs of one
// unbroken scope_changed recovery chain. Recovery successors always start as
// pristine reviewing authorities, so a successor created after its
// predecessor's delivery was committed can never restate the original base
// tree, genesis paths, or fix delta on its own. The binding restores exactly
// those gate inputs from the persisted, receipt-bound predecessors without
// ever mutating any authority. It is derived gate evidence only.
type compactRecoveryBinding struct {
	// Members are ordered root..leaf. Every member is a terminally approved
	// compact authority whose receipt file matches its state, and every edge
	// is a validated scope_changed recovery whose successor initial base tree
	// equals its predecessor's final candidate tree.
	Members []CompactRecord
	// BaseTree is the root member's reviewed delivery base.
	BaseTree string
	// GenesisPaths is the canonical union of every member's frozen genesis.
	GenesisPaths []string
	// FixDeltaHash commits to the ordered per-member fix delta hashes so a
	// recovered lineage inherits its predecessors' correction identity
	// instead of carrying the empty-input hash forever.
	FixDeltaHash string
}

// deriveCompactRecoveryBinding walks the scope_changed recovery chain of the
// given leaf state backwards and composes the publication binding. It returns
// ok=false when nothing can be composed: the leaf is not a scope_changed
// recovery successor, or no predecessor satisfies every fail-closed
// requirement (terminal approval, receipt bound to authority, exact revision,
// valid recovery edge, and delivery-tree continuity). A hard error reports a
// corrupt or racing authority graph; callers must then keep the original
// denial.
func deriveCompactRecoveryBinding(ctx context.Context, repo string, leaf CompactState) (compactRecoveryBinding, bool, error) {
	if leaf.Recovery == nil || leaf.Recovery.Disposition != RecoveryScopeChanged {
		return compactRecoveryBinding{}, false, nil
	}
	leafRecord, _, err := makeCompactRecord(leaf)
	if err != nil {
		return compactRecoveryBinding{}, false, err
	}
	members := []CompactRecord{leafRecord}
	current := leaf
	for current.Recovery != nil && current.Recovery.Disposition == RecoveryScopeChanged {
		store, storeErr := CompactAuthoritativeStore(ctx, repo, current.Recovery.PredecessorLineageID)
		if storeErr != nil {
			return compactRecoveryBinding{}, false, storeErr
		}
		predecessor, loadErr := store.Load()
		if loadErr != nil {
			return compactRecoveryBinding{}, false, loadErr
		}
		if predecessor.Revision != current.Recovery.PredecessorRevision {
			return compactRecoveryBinding{}, false, errors.New("recovery predecessor revision does not match successor provenance")
		}
		if edgeErr := validateCompactRecoveryEdge(predecessor, current); edgeErr != nil {
			return compactRecoveryBinding{}, false, edgeErr
		}
		if predecessor.State.State != StateApproved ||
			current.InitialSnapshot.BaseTree != predecessor.State.CurrentSnapshot.CandidateTree ||
			!compactRecoveryReceiptBound(store, predecessor) {
			break
		}
		members = append([]CompactRecord{predecessor}, members...)
		current = predecessor.State
	}
	if len(members) < 2 {
		return compactRecoveryBinding{}, false, nil
	}
	genesis := make([][]string, 0, len(members))
	fixDeltas := make([]string, 0, len(members))
	for _, member := range members {
		genesis = append(genesis, member.State.GenesisPaths)
		fixDeltas = append(fixDeltas, member.State.FixDeltaHash)
	}
	union, err := compactPrePRPathUnion(genesis...)
	if err != nil {
		return compactRecoveryBinding{}, false, err
	}
	return compactRecoveryBinding{
		Members: members, BaseTree: members[0].State.InitialSnapshot.BaseTree,
		GenesisPaths: union, FixDeltaHash: compactRecoveryBindingValuesHash("fix-delta", fixDeltas),
	}, true, nil
}

// compactRecoveryReceiptBound reports whether the persisted receipt file of a
// terminal predecessor matches the receipt derived from its authority.
func compactRecoveryReceiptBound(store CompactStore, record CompactRecord) bool {
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return false
	}
	receipt, err := ParseCompactReceipt(payload)
	if err != nil {
		return false
	}
	authoritative, err := record.State.Receipt()
	if err != nil {
		return false
	}
	return compactReceiptEqual(receipt, authoritative)
}

func compactRecoveryBindingValuesHash(kind string, values []string) string {
	payload, _ := json.Marshal(values)
	sum := sha256.Sum256(append([]byte(CompactRecoveryBindingDomain+"/"+kind+"\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// verifyCompactRecoveryDelivery re-derives from live Git that the composed
// recovery chain's approved content is exactly the delivery being validated:
// the publication base commit carries the composed base tree, the linear
// publication history progresses only through member final candidate trees,
// every commit inside a member's segment stays inside that member's frozen
// genesis paths, and the final tree equals the delivered candidate. Members
// whose final tree equals the already delivered tree consume no commits: they
// re-reviewed content that was already published by an earlier member.
func verifyCompactRecoveryDelivery(ctx context.Context, repo string, binding compactRecoveryBinding, finalTree, baseCommit, headCommit string) error {
	builder := SnapshotBuilder{Repo: repo}
	baseTree, err := builder.resolveTree(ctx, baseCommit)
	if err != nil {
		return err
	}
	if baseTree != binding.BaseTree {
		return errors.New("publication base does not carry the composed recovery base tree")
	}
	commits, err := compactPrePRLinearCommits(ctx, repo, baseCommit, headCommit)
	if err != nil {
		return err
	}
	previousTree, previousIndex := baseTree, -1
	for _, member := range binding.Members {
		memberFinal := member.State.CurrentSnapshot.CandidateTree
		if memberFinal == previousTree {
			continue
		}
		boundary := -1
		for index := previousIndex + 1; index < len(commits); index++ {
			if commits[index].Tree == previousTree {
				continue
			}
			if commits[index].Tree != memberFinal {
				return errors.New("publication commit does not match the recovery chain boundary")
			}
			boundary = index
			break
		}
		if boundary < 0 {
			return errors.New("recovery chain boundary is absent from publication history")
		}
		touched := []string{}
		for index := previousIndex + 1; index <= boundary; index++ {
			paths, pathsErr := builder.changedPaths(ctx, commits[index].ParentTree, commits[index].Tree)
			if pathsErr != nil {
				return pathsErr
			}
			touched = append(touched, paths...)
		}
		touched, err = compactPrePRPathUnion(touched)
		if err != nil || pathsAreSubset(touched, member.State.GenesisPaths) != nil {
			return errors.New("publication segment exceeds the recovery member's immutable genesis paths")
		}
		previousTree, previousIndex = memberFinal, boundary
	}
	for index := previousIndex + 1; index < len(commits); index++ {
		if commits[index].Tree != previousTree {
			return errors.New("publication commit remains after all recovery chain boundaries")
		}
	}
	if previousTree != finalTree {
		return errors.New("recovery chain final tree does not equal the delivered candidate")
	}
	return nil
}

// rebindCompactRecoveryDelivery derives and fully re-verifies the composed
// recovery binding for one already derived gate snapshot. It never relaxes
// the delivered candidate: the leaf receipt's final candidate tree must equal
// the live delivered tree, the live base must equal the composed base, the
// live changed paths must stay inside the composed genesis union, and the
// full publication history must be covered by the chain. Any failure keeps
// the caller's original denial.
func rebindCompactRecoveryDelivery(ctx context.Context, repo string, state CompactState, snapshot Snapshot, finalTree, baseCommit, headCommit string) (compactRecoveryBinding, bool) {
	if snapshot.CandidateTree != finalTree || baseCommit == "" || headCommit == "" {
		return compactRecoveryBinding{}, false
	}
	binding, ok, err := deriveCompactRecoveryBinding(ctx, repo, state)
	if err != nil || !ok {
		return compactRecoveryBinding{}, false
	}
	if snapshot.BaseTree != binding.BaseTree || pathsAreSubset(snapshot.Paths, binding.GenesisPaths) != nil {
		return compactRecoveryBinding{}, false
	}
	if verifyCompactRecoveryDelivery(ctx, repo, binding, finalTree, baseCommit, headCommit) != nil {
		return compactRecoveryBinding{}, false
	}
	return binding, true
}
