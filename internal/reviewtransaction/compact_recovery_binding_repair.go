package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

// The compact-v2 recovery-authorization target-drift class. A historical
// escalated recovery successor records a gentle-ai.review-recovery-authorization/v1
// binding whose target_identity is not the identity the successor actually
// persisted. The authorized identity has no persisted candidate tree, so the
// edge can never be re-derived into a reconcilable class; it is recognised
// only by the fully persisted internal disagreement and quarantined whole.
const (
	AuthorityRepairClassCompactV2RecoveryTargetDrift              AuthorityRepairClass       = "compact_v2_recovery_authorization_target_drift"
	AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted AuthorityRepairCause       = "recovery_authorization_target_identity_never_persisted"
	AuthorityRepairDispositionQuarantineDriftedRecoveryBinding    AuthorityRepairDisposition = "quarantine-drifted-recovery-binding"

	// CompactRecoveryEdgeExitRepairRecoveryBinding extends the closed exit
	// vocabulary in compact_inspect.go with the operation that admits this class.
	CompactRecoveryEdgeExitRepairRecoveryBinding = "review repair-recovery-binding"

	compactRecoveryBindingRepairRequestDigestDomain = "gentle-ai.review-recovery-binding-repair-request-digest/v1"
)

// compactRecoveryBindingRepairAfterAssessmentHook is the seam that lets a test
// mutate the store between the lock-free classification and exclusive store
// ownership, so the compare-and-swap can be observed.
var compactRecoveryBindingRepairAfterAssessmentHook = func() {}

type compactRecoveryTargetDriftClassification struct {
	Admitted                    bool
	PersistedTargetIdentity     string
	AuthorizedTargetIdentity    string
	RecordedAuthorizationSHA256 string
	ValidationError             error
	RefusalReason               error
}

// CompactRecoveryBindingRepairProof records the natively re-derived drift
// inside the quarantine audit record. The drifted authorization bytes survive
// verbatim in the quarantined residue; the proof binds them by digest only.
type CompactRecoveryBindingRepairProof struct {
	Class                       string `json:"class"`
	Cause                       string `json:"cause"`
	Disposition                 string `json:"disposition"`
	RepositoryBinding           string `json:"repository_binding"`
	AuthorizationSchema         string `json:"authorization_schema"`
	PredecessorLineageID        string `json:"predecessor_lineage_id"`
	PredecessorRevision         string `json:"predecessor_revision"`
	SuccessorRevision           string `json:"successor_revision"`
	PersistedTargetIdentity     string `json:"persisted_target_identity"`
	AuthorizedTargetIdentity    string `json:"authorized_target_identity"`
	RecordedAuthorizationSHA256 string `json:"recorded_authorization_sha256"`
	RequestDigest               string `json:"request_digest"`
	ValidationError             string `json:"validation_error"`
}

// CompactRecoveryBindingRepairCandidate publishes one selectable edge with the
// complete authorization inputs, so a maintainer never has to guess a value the
// executor re-derives under lock. Blocked names an ordering rule that would
// refuse the edge right now while still listing it.
type CompactRecoveryBindingRepairCandidate struct {
	Class                       AuthorityRepairClass       `json:"class"`
	Cause                       AuthorityRepairCause       `json:"cause"`
	Disposition                 AuthorityRepairDisposition `json:"disposition"`
	RepositoryBinding           string                     `json:"repository_binding"`
	PredecessorLineageID        string                     `json:"predecessor_lineage_id"`
	ExpectedPredecessorRevision string                     `json:"expected_predecessor_revision"`
	SuccessorLineageID          string                     `json:"successor_lineage_id"`
	ExpectedSuccessorRevision   string                     `json:"expected_successor_revision"`
	PersistedTargetIdentity     string                     `json:"persisted_target_identity"`
	AuthorizedTargetIdentity    string                     `json:"authorized_target_identity"`
	RecordedAuthorizationSHA256 string                     `json:"recorded_authorization_sha256"`
	Blocked                     string                     `json:"blocked,omitempty"`
}

// CompactRecoveryBindingRepairCounts makes "the repository does not have to be
// otherwise clean" directly observable beside the candidate list.
type CompactRecoveryBindingRepairCounts struct {
	CompactEntries       int `json:"compact_entries"`
	RecoveryEdges        int `json:"recovery_edges"`
	InvalidEdges         int `json:"invalid_edges"`
	AdmissibleCandidates int `json:"admissible_candidates"`
	BlockedCandidates    int `json:"blocked_candidates"`
}

// CompactRecoveryBindingRepairSelector narrows the published candidate list to
// one exact edge. Selection is fail-closed: every supplied value must match the
// same single classified edge or the inspection refuses.
type CompactRecoveryBindingRepairSelector struct {
	PredecessorLineageID        string
	ExpectedPredecessorRevision string
	SuccessorLineageID          string
	ExpectedSuccessorRevision   string
}

// CompactRecoveryBindingRepairRequest names one exact predecessor/successor
// edge with both expected revisions. The three drift values are deliberately
// not request fields: they are bound byte-exactly inside the authorization and
// re-derived under lock, so the two can never disagree.
type CompactRecoveryBindingRepairRequest struct {
	PredecessorLineageID        string
	ExpectedPredecessorRevision string
	SuccessorLineageID          string
	ExpectedSuccessorRevision   string
	Actor                       string
	Reason                      string
	MaintainerAuthorization     string
	RepairedAt                  time.Time
}

// classifyCompactRecoveryAuthorizationTargetDrift admits one recovery edge into
// the target-drift class. It is pure and read-only: no Git, no tree derivation,
// nothing rewritten. There is no default-admit branch — the first failing gate
// records a typed refusal and returns.
func classifyCompactRecoveryAuthorizationTargetDrift(predecessor, successor CompactRecord) compactRecoveryTargetDriftClassification {
	refuse := func(format string, args ...any) compactRecoveryTargetDriftClassification {
		return compactRecoveryTargetDriftClassification{RefusalReason: fmt.Errorf(format, args...)}
	}
	recovery := successor.State.Recovery
	if recovery == nil {
		return refuse("successor %q is not a recovery successor", successor.State.LineageID)
	}
	if recovery.Disposition != RecoveryEscalated {
		return refuse("recovery disposition %q is outside the escalated recovery-authorization class", recovery.Disposition)
	}
	// The accounting-only discriminator is the evidence pointer, never
	// evidence_hash: compactAccountingOnlyEscalation requires an empty hash.
	if recovery.Evidence != nil {
		return refuse("successor %q records an accounting-only escalation, which binds its target identity through recovered evidence", successor.State.LineageID)
	}
	if recovery.FinalVerificationRetry != nil {
		return refuse("successor %q records a final-verification retry, which carries its own authorization schema", successor.State.LineageID)
	}
	edgeErr := validateCompactRecoveryEdge(predecessor, successor.State)
	if edgeErr == nil {
		return refuse("recovery edge for %q validates; the successor remains authoritative", successor.State.LineageID)
	}
	if !errors.Is(edgeErr, errCompactRecoveryAuthorizationInexact) || errors.Is(edgeErr, errCompactRecoveryTargetUnchanged) {
		return refuse("recovery edge for %q fails outside the inexact-authorization sentinel: %v", successor.State.LineageID, edgeErr)
	}
	authorization := recovery.MaintainerAuthorization
	if !strings.HasPrefix(authorization, compactRecoveryAuthorizationSchema+"\n") || strings.Contains(authorization, "\r") {
		return refuse("successor %q does not record an LF-only %s binding", successor.State.LineageID, compactRecoveryAuthorizationSchema)
	}
	// Parsing is justified only here: the authorized target_identity has no
	// persisted candidate tree, so re-derivation alone cannot decide the class.
	lines := strings.Split(authorization, "\n")
	prefixes := []string{"predecessor_lineage=", "predecessor_revision=", "target_identity=", "actor=", "reason="}
	if len(lines) != len(prefixes)+1 || lines[0] != compactRecoveryAuthorizationSchema {
		return refuse("successor %q does not record the exact six-line %s shape", successor.State.LineageID, compactRecoveryAuthorizationSchema)
	}
	for index, prefix := range prefixes {
		if !strings.HasPrefix(lines[index+1], prefix) {
			return refuse("successor %q records %s keys out of order", successor.State.LineageID, compactRecoveryAuthorizationSchema)
		}
	}
	authorized := strings.TrimPrefix(lines[3], "target_identity=")
	if !validSHA256(authorized) {
		return refuse("successor %q records a target_identity that is not a sha256 identity", successor.State.LineageID)
	}
	// The sole-deviation proof: every other line must re-derive from the
	// persisted predecessor record plus the persisted provenance. Byte equality
	// makes the parser above incapable of widening the class.
	if compactRecoveryAuthorizationBinding(predecessor.State.LineageID, predecessor.Revision, authorized,
		recovery.Actor, recovery.Reason) != authorization {
		return refuse("target identity is not the sole deviation in the recorded %s binding of %q", compactRecoveryAuthorizationSchema, successor.State.LineageID)
	}
	persisted := successor.State.InitialSnapshot.Identity
	if authorized == persisted {
		return refuse("successor %q records the persisted target identity; the binding does not drift", successor.State.LineageID)
	}
	// The sole-anomaly residual: rebinding the authorization to the identity the
	// successor actually persisted must leave a completely valid edge. Any
	// residual defect keeps the edge outside this class.
	repaired := successor.State
	provenance := *recovery
	provenance.MaintainerAuthorization = compactRecoveryAuthorizationBinding(
		predecessor.State.LineageID, predecessor.Revision, persisted, recovery.Actor, recovery.Reason)
	repaired.Recovery = &provenance
	if residual := validateCompactRecoveryEdge(predecessor, repaired); residual != nil {
		return refuse("target drift is not the sole edge anomaly: %v", residual)
	}
	recorded := sha256.Sum256([]byte(authorization))
	return compactRecoveryTargetDriftClassification{
		Admitted: true, PersistedTargetIdentity: persisted, AuthorizedTargetIdentity: authorized,
		RecordedAuthorizationSHA256: "sha256:" + hex.EncodeToString(recorded[:]), ValidationError: edgeErr,
	}
}

// compactRecoveryBindingRepairAuthorizationBinding renders the exact fourteen
// LF-joined lines the maintainer must reproduce. Line one reuses the existing
// gentle-ai.review-repair-authorization/v1 head; the class-conditional lines
// below it are disjoint from the classified nine-line document, so neither can
// ever be replayed as the other. The last three lines are what make the
// attestation non-transferable: if the corrupt bytes change or a different edge
// is targeted, nothing re-derives.
func compactRecoveryBindingRepairAuthorizationBinding(repositoryBinding string, candidate CompactRecoveryBindingRepairCandidate, actor, reason string) string {
	return AuthorityRepairAuthorizationSchema +
		"\nrepository=" + repositoryBinding +
		"\nclass=" + string(AuthorityRepairClassCompactV2RecoveryTargetDrift) +
		"\ncause=" + string(AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted) +
		"\ndisposition=" + string(AuthorityRepairDispositionQuarantineDriftedRecoveryBinding) +
		"\npredecessor_lineage=" + candidate.PredecessorLineageID +
		"\npredecessor_revision=" + candidate.ExpectedPredecessorRevision +
		"\nsuccessor_lineage=" + candidate.SuccessorLineageID +
		"\nsuccessor_revision=" + candidate.ExpectedSuccessorRevision +
		"\npersisted_target_identity=" + candidate.PersistedTargetIdentity +
		"\nauthorized_target_identity=" + candidate.AuthorizedTargetIdentity +
		"\nrecorded_authorization_sha256=" + candidate.RecordedAuthorizationSHA256 +
		"\nactor=" + strings.TrimSpace(actor) +
		"\nreason=" + strings.TrimSpace(reason)
}

// CompactRecoveryBindingRepairAuthorizationLines names the ordered keys of the
// binding. It is deliberately keys only, never a fillable template: every value
// the binding binds except actor and reason is already published in the
// candidate.
func CompactRecoveryBindingRepairAuthorizationLines() []string {
	return []string{
		AuthorityRepairAuthorizationSchema, "repository", "class", "cause", "disposition",
		"predecessor_lineage", "predecessor_revision", "successor_lineage", "successor_revision",
		"persisted_target_identity", "authorized_target_identity", "recorded_authorization_sha256",
		"actor", "reason",
	}
}

// compactRecoveryBindingRepairCommandText renders the exact runnable repair for
// one admitted, unblocked edge with the persisted revisions the operation
// compare-and-swaps against and the ordered keys of the binding it verifies.
// Only a caller whose own read-only prediction admitted the edge may render it,
// so the command printed is the command that runs.
func compactRecoveryBindingRepairCommandText(repo string, candidate CompactRecoveryBindingRepairCandidate) string {
	return fmt.Sprintf("`gentle-ai %s --cwd %q --predecessor-lineage %q --expected-predecessor-revision %q --successor-lineage %q --expected-successor-revision %q --reason \"<why-it-is-repaired>\" --actor \"<actor>\" --maintainer-authorization \"<maintainer-authorization>\"`;"+
		" the repair moves the successor whole — never deleted — into the audited quarantine together with the natively re-derived proof, so the drifted authorization bytes survive exactly as persisted."+
		" --maintainer-authorization is exactly these fourteen lines, joined by LF, with no trailing newline: %s. Run `gentle-ai %s --cwd %q --preflight` first; it publishes every bound value except actor and reason.",
		CompactRecoveryEdgeExitRepairRecoveryBinding, repo, candidate.PredecessorLineageID, candidate.ExpectedPredecessorRevision,
		candidate.SuccessorLineageID, candidate.ExpectedSuccessorRevision,
		strings.Join(CompactRecoveryBindingRepairAuthorizationLines(), ", "),
		CompactRecoveryEdgeExitRepairRecoveryBinding, repo)
}

func compactRecoveryBindingRepairRequestDigest(request CompactRecoveryBindingRepairRequest) (string, error) {
	canonical := struct {
		Class                       AuthorityRepairClass       `json:"class"`
		Cause                       AuthorityRepairCause       `json:"cause"`
		Disposition                 AuthorityRepairDisposition `json:"disposition"`
		PredecessorLineageID        string                     `json:"predecessor_lineage_id"`
		ExpectedPredecessorRevision string                     `json:"expected_predecessor_revision"`
		SuccessorLineageID          string                     `json:"successor_lineage_id"`
		ExpectedSuccessorRevision   string                     `json:"expected_successor_revision"`
		Actor                       string                     `json:"actor"`
		Reason                      string                     `json:"reason"`
	}{
		Class: AuthorityRepairClassCompactV2RecoveryTargetDrift, Cause: AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted,
		Disposition:          AuthorityRepairDispositionQuarantineDriftedRecoveryBinding,
		PredecessorLineageID: request.PredecessorLineageID, ExpectedPredecessorRevision: request.ExpectedPredecessorRevision,
		SuccessorLineageID: request.SuccessorLineageID, ExpectedSuccessorRevision: request.ExpectedSuccessorRevision,
		Actor: request.Actor, Reason: request.Reason,
	}
	return classifiedAuthorityRepairDigest(compactRecoveryBindingRepairRequestDigestDomain, canonical)
}

func validateCompactRecoveryBindingRepairRequestStatic(request CompactRecoveryBindingRepairRequest) error {
	if validateLineageID(request.PredecessorLineageID) != nil || validateLineageID(request.SuccessorLineageID) != nil ||
		request.PredecessorLineageID == request.SuccessorLineageID ||
		!validSHA256(request.ExpectedPredecessorRevision) || !validSHA256(request.ExpectedSuccessorRevision) ||
		strings.TrimSpace(request.Actor) != request.Actor || strings.TrimSpace(request.Reason) != request.Reason ||
		request.Actor == "" || request.Reason == "" || len(request.Actor) > 256 || len(request.Reason) > 512 ||
		strings.ContainsAny(request.Actor, "\r\n") || strings.ContainsAny(request.Reason, "\r\n") ||
		strings.Contains(request.MaintainerAuthorization, "\r") {
		return errors.New("compact recovery binding repair request is invalid: it needs two distinct lineage IDs, two sha256 revisions, a trimmed LF-only actor and reason, and an LF-only authorization; run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` to publish every value again")
	}
	return nil
}

// loadCompactRecoveryBindingRepairRecordSet reads every compact-v2 entry
// without taking the shared maintenance lease, so it is safe both lock-free and
// while this transaction owns authority maintenance exclusively.
//
// It fails closed on an entry that does not load, exactly like `review
// reconcile-authority`. This map is the ONLY graph view the leaf-first blocker,
// the removal-regression invariant and the readback proof ever see, so a
// silently truncated one is the single input that can make all three agree an
// entry is safe to move while a real, unread neighbour still depends on it —
// which would make the repair introduce the very graph defect it exists to
// clear. An unreadable neighbour is therefore a refusal, never a skipped row.
//
// The population it fails closed over is exactly the one DiscoverCompactStores
// admits: entries whose name is not a valid lineage ID, and lineage directories
// holding only unpublished atomic crash residue, are skipped rather than
// refused. Failing closed over a strictly larger population than every other
// compact reader would turn an ordinary damaged-store shape — the one most
// likely to co-occur with a drifted edge — into a repair that refuses itself.
func loadCompactRecoveryBindingRepairRecordSet(base string) (map[string]CompactRecord, int, error) {
	versionRoot := filepath.Join(base, "v2")
	entries, err := os.ReadDir(versionRoot)
	if os.IsNotExist(err) {
		return map[string]CompactRecord{}, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("inspect compact authority v2 root: %w", err)
	}
	records := make(map[string]CompactRecord, len(entries))
	compactEntries := 0
	for _, entry := range entries {
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			continue
		}
		dir := filepath.Join(versionRoot, entry.Name())
		if _, statErr := os.Stat(filepath.Join(dir, compactStateFileName)); os.IsNotExist(statErr) {
			residue, readErr := os.ReadDir(dir)
			if onlyUnpublishedCompactCrashResidue(residue, readErr) {
				continue
			}
		}
		compactEntries++
		record, loadErr := (CompactStore{Dir: dir, lineageID: entry.Name()}).Load()
		if loadErr != nil {
			return nil, 0, fmt.Errorf("%s refused: related compact authority %q does not load, so no repair can prove what still depends on it: %w",
				CompactRecoveryEdgeExitRepairRecoveryBinding, entry.Name(), loadErr)
		}
		records[record.State.LineageID] = record
	}
	return records, compactEntries, nil
}

func compactRecoverySuccessorDepth(records map[string]CompactRecord) map[string]int {
	children := make(map[string][]string, len(records))
	for lineage, record := range records {
		if record.State.Recovery == nil {
			continue
		}
		predecessor := record.State.Recovery.PredecessorLineageID
		children[predecessor] = append(children[predecessor], lineage)
	}
	depth := make(map[string]int, len(records))
	var resolve func(lineage string, seen map[string]bool) int
	resolve = func(lineage string, seen map[string]bool) int {
		if value, ok := depth[lineage]; ok {
			return value
		}
		if seen[lineage] {
			return 0
		}
		seen[lineage] = true
		best := 0
		for _, child := range children[lineage] {
			if value := resolve(child, seen) + 1; value > best {
				best = value
			}
		}
		delete(seen, lineage)
		depth[lineage] = best
		return best
	}
	for lineage := range records {
		resolve(lineage, map[string]bool{})
	}
	return depth
}

// compactRecoveryBindingRepairBlocker names the ordering rule that would refuse
// one admitted edge right now. Listing the edge with the rule keeps the surface
// honest instead of hiding a candidate the operator can reach later.
//
// Both rules are strictly ordered, never mutual. A fork — one predecessor with
// two drifted successors — must not have each sibling refuse citing the other:
// that is the shape compactAuthorityRemovalRegression documents as leaving a
// store unrecoverable forever, because neither edge can go first. The sibling
// rule therefore defers only to a lineage that sorts BEFORE this one, which is
// the same lexicographic order the candidate list publishes for equal depth, so
// the advertised order is always the repairable one.
//
// Every chain of deferrals terminates: rule one points at a descendant, so the
// remaining depth strictly decreases, and rule two points at a strictly smaller
// lineage ID under one predecessor. Neither relation can cycle.
func compactRecoveryBindingRepairBlocker(records map[string]CompactRecord, predecessorLineage, successorLineage string) string {
	lineages := make([]string, 0, len(records))
	for lineage := range records {
		lineages = append(lineages, lineage)
	}
	sort.Strings(lineages)
	for _, lineage := range lineages {
		related := records[lineage].State.Recovery
		if related == nil || lineage == successorLineage {
			continue
		}
		if related.PredecessorLineageID == successorLineage {
			return fmt.Sprintf("successor %q has its own successor %q; %s repairs one leaf edge at a time, so disposition %q first",
				successorLineage, lineage, CompactRecoveryEdgeExitRepairRecoveryBinding, lineage)
		}
		if related.PredecessorLineageID == predecessorLineage {
			if lineage >= successorLineage {
				continue
			}
			return fmt.Sprintf("predecessor %q has another successor %q; %s repairs one edge at a time, so disposition %q first",
				predecessorLineage, lineage, CompactRecoveryEdgeExitRepairRecoveryBinding, lineage)
		}
	}
	return ""
}

func compactRecoveryBindingRepairCandidateFor(records map[string]CompactRecord, repositoryBinding, successorLineage string) (CompactRecoveryBindingRepairCandidate, bool) {
	successor, ok := records[successorLineage]
	if !ok || successor.State.Recovery == nil {
		return CompactRecoveryBindingRepairCandidate{}, false
	}
	predecessor, ok := records[successor.State.Recovery.PredecessorLineageID]
	if !ok {
		return CompactRecoveryBindingRepairCandidate{}, false
	}
	classification := classifyCompactRecoveryAuthorizationTargetDrift(predecessor, successor)
	if !classification.Admitted {
		return CompactRecoveryBindingRepairCandidate{}, false
	}
	return CompactRecoveryBindingRepairCandidate{
		Class: AuthorityRepairClassCompactV2RecoveryTargetDrift, Cause: AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted,
		Disposition: AuthorityRepairDispositionQuarantineDriftedRecoveryBinding, RepositoryBinding: repositoryBinding,
		PredecessorLineageID: predecessor.State.LineageID, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: successorLineage, ExpectedSuccessorRevision: successor.Revision,
		PersistedTargetIdentity: classification.PersistedTargetIdentity, AuthorizedTargetIdentity: classification.AuthorizedTargetIdentity,
		RecordedAuthorizationSHA256: classification.RecordedAuthorizationSHA256,
		Blocked:                     compactRecoveryBindingRepairBlocker(records, predecessor.State.LineageID, successorLineage),
	}, true
}

// compactRecoveryBindingRepairExitProbe answers the drift prediction for many
// successors from ONE lock-free read of the authority. Resolving the repository
// root and the record set once keeps a diagnostic surface off an
// O(edges x entries) rescan, and a probe that cannot read what it needs answers
// "not admitted" rather than turning a report that previously could not fail
// into a hard error. Refusing to predict is the fail-closed answer here: the
// worst outcome is that the surface falls back to the routing it had before
// this class existed.
type compactRecoveryBindingRepairExitProbe struct {
	records           map[string]CompactRecord
	repositoryBinding string
	readable          bool
}

func newCompactRecoveryBindingRepairExitProbe(ctx context.Context, repo string) compactRecoveryBindingRepairExitProbe {
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return compactRecoveryBindingRepairExitProbe{}
	}
	// A prepared batch owns authority maintenance, so the executor would refuse;
	// this surface must never advertise the verb inside that window.
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return compactRecoveryBindingRepairExitProbe{}
	}
	_, repositoryBinding, err := authorityRepairRoot(root)
	if err != nil {
		return compactRecoveryBindingRepairExitProbe{}
	}
	records, _, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return compactRecoveryBindingRepairExitProbe{}
	}
	return compactRecoveryBindingRepairExitProbe{records: records, repositoryBinding: repositoryBinding, readable: true}
}

func (probe compactRecoveryBindingRepairExitProbe) candidate(successorLineage string) (CompactRecoveryBindingRepairCandidate, bool) {
	if !probe.readable || validateLineageID(successorLineage) != nil {
		return CompactRecoveryBindingRepairCandidate{}, false
	}
	return compactRecoveryBindingRepairCandidateFor(probe.records, probe.repositoryBinding, successorLineage)
}

// InspectCompactRecoveryBindingRepairCandidates lists every admissible edge as
// its own candidate — never one per repository — ordered leaf-first so a
// consecutive recovery chain repairs in the order it is published. It is
// read-only, takes no maintenance or store lock, and refuses a selector that
// does not resolve to exactly one classified edge.
func InspectCompactRecoveryBindingRepairCandidates(ctx context.Context, repo string, selector CompactRecoveryBindingRepairSelector) ([]CompactRecoveryBindingRepairCandidate, CompactRecoveryBindingRepairCounts, error) {
	counts := CompactRecoveryBindingRepairCounts{}
	if err := ctx.Err(); err != nil {
		return nil, counts, err
	}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return nil, counts, err
	}
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return nil, counts, err
	}
	_, repositoryBinding, err := authorityRepairRoot(root)
	if err != nil {
		return nil, counts, err
	}
	records, compactEntries, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return nil, counts, err
	}
	counts.CompactEntries = compactEntries
	lineages := make([]string, 0, len(records))
	for lineage := range records {
		lineages = append(lineages, lineage)
	}
	sort.Strings(lineages)
	candidates := []CompactRecoveryBindingRepairCandidate{}
	for _, lineage := range lineages {
		record := records[lineage]
		if record.State.Recovery == nil {
			continue
		}
		counts.RecoveryEdges++
		predecessor, ok := records[record.State.Recovery.PredecessorLineageID]
		if !ok || validateCompactRecoveryEdge(predecessor, record.State) != nil {
			counts.InvalidEdges++
		}
		if candidate, admitted := compactRecoveryBindingRepairCandidateFor(records, repositoryBinding, lineage); admitted {
			candidates = append(candidates, candidate)
		}
	}
	depth := compactRecoverySuccessorDepth(records)
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if depth[left.SuccessorLineageID] != depth[right.SuccessorLineageID] {
			return depth[left.SuccessorLineageID] < depth[right.SuccessorLineageID]
		}
		if left.SuccessorLineageID != right.SuccessorLineageID {
			return left.SuccessorLineageID < right.SuccessorLineageID
		}
		return left.ExpectedSuccessorRevision < right.ExpectedSuccessorRevision
	})
	selected, err := applyCompactRecoveryBindingRepairSelector(candidates, selector)
	if err != nil {
		return nil, counts, err
	}
	for _, candidate := range selected {
		if candidate.Blocked == "" {
			counts.AdmissibleCandidates++
			continue
		}
		counts.BlockedCandidates++
	}
	return selected, counts, nil
}

func applyCompactRecoveryBindingRepairSelector(candidates []CompactRecoveryBindingRepairCandidate, selector CompactRecoveryBindingRepairSelector) ([]CompactRecoveryBindingRepairCandidate, error) {
	if selector == (CompactRecoveryBindingRepairSelector{}) {
		return candidates, nil
	}
	matched := []CompactRecoveryBindingRepairCandidate{}
	for _, candidate := range candidates {
		if selector.PredecessorLineageID != "" && selector.PredecessorLineageID != candidate.PredecessorLineageID {
			continue
		}
		if selector.SuccessorLineageID != "" && selector.SuccessorLineageID != candidate.SuccessorLineageID {
			continue
		}
		if selector.ExpectedPredecessorRevision != "" && selector.ExpectedPredecessorRevision != candidate.ExpectedPredecessorRevision {
			continue
		}
		if selector.ExpectedSuccessorRevision != "" && selector.ExpectedSuccessorRevision != candidate.ExpectedSuccessorRevision {
			continue
		}
		matched = append(matched, candidate)
	}
	if len(matched) != 1 {
		return nil, fmt.Errorf("%s selector matches %d classified edges; it must select exactly one. Run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` without selectors to list every admissible edge leaf-first",
			CompactRecoveryEdgeExitRepairRecoveryBinding, len(matched))
	}
	return matched, nil
}

// RepairCompactRecoveryBinding quarantines one compact-v2 recovery successor
// whose recorded gentle-ai.review-recovery-authorization/v1 binding names a
// target identity the successor never persisted. The predecessor and every
// unrelated authority stay untouched, the successor entry moves whole — never
// deleted — into the audited quarantine together with the re-derived proof, and
// an exact replay converges on the same committed record.
func RepairCompactRecoveryBinding(ctx context.Context, repo string, request CompactRecoveryBindingRepairRequest) (CompactReclaimRecord, error) {
	if err := ctx.Err(); err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := validateCompactRecoveryBindingRepairRequestStatic(request); err != nil {
		return CompactReclaimRecord{}, err
	}
	base, root, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	_, repositoryBinding, err := authorityRepairRoot(root)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	initiallyMatched := compactRecoveryBindingRepairInitiallyMatched(base, repositoryBinding, request)
	compactRecoveryBindingRepairAfterAssessmentHook()
	// acquireStoreLock is the single maintenance/CAS boundary this repair takes:
	// it leases authority maintenance shared — which excludes batch
	// reconciliation and every exclusive maintenance owner — and then takes the
	// exclusive compact-v2 store lock. Acquiring the exclusive maintenance lock
	// on top of it would deadlock against its own shared lease, so it must not
	// be added.
	//
	// The wait is bounded rather than immediate because two maintainers issuing
	// the same authorized request must converge on one committed record instead
	// of one of them losing to transient contention: the loser then discovers
	// the replay under the same lock. Every gate below still re-derives, so
	// waiting can never double-apply anything.
	//
	// It goes through acquireStoreLockForConvergentCompletion rather than the
	// raw bounded helper so this caller joins that guard's declared population
	// explicitly instead of silently widening it: store_lock.go keeps the
	// instant refusal for pre-commit mutation paths, and a waiting caller that
	// is not named in the declaration would leave the registered guard green
	// while its real legitimate population had changed.
	versionRoot := filepath.Join(base, "v2")
	lock, err := acquireStoreLockForConvergentCompletion(ctx, filepath.Join(versionRoot, "LOCK"))
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	defer lock.release()
	if err := ensureNoPreparedCompactBatchReconciliation(base); err != nil {
		return CompactReclaimRecord{}, err
	}
	requestDigest, err := compactRecoveryBindingRepairRequestDigest(request)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	// Idempotent replay is resolved before anything else under lock: `review
	// reconcile-authority` refuses an exact replay after quarantine, but this
	// verb must converge, so it follows the classified-repair semantics.
	if replay, found, err := discoverCompactRecoveryBindingRepairRecord(ctx, base, request, repositoryBinding, requestDigest); err != nil {
		return CompactReclaimRecord{}, err
	} else if found {
		return reconcileCompactRecoveryBindingRepairRecord(ctx, base, root, repositoryBinding, request, replay)
	}
	dir := filepath.Join(versionRoot, request.SuccessorLineageID)
	successor, err := (CompactStore{Dir: dir, lineageID: request.SuccessorLineageID}).Load()
	if err != nil {
		if os.IsNotExist(err) {
			return CompactReclaimRecord{}, fmt.Errorf("%s refused: successor %q holds no compact authority state; if a prior repair was interrupted after moving the entry, the prepared reclaim-record.json under %s locates the moved residue: %w",
				CompactRecoveryEdgeExitRepairRecoveryBinding, request.SuccessorLineageID, filepath.Join(base, "quarantine"), err)
		}
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: successor %q holds a compact state record that cannot be loaded (%v); inspection classifies it %s."+
			" This repair quarantines only an edge it can natively re-derive from readable state, so it cannot admit this record."+
			" Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` and escalate that report",
			CompactRecoveryEdgeExitRepairRecoveryBinding, request.SuccessorLineageID, err, compactRecoveryEntryProblem(err), root)
	}
	recovery := successor.State.Recovery
	if recovery == nil {
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: successor %q is not a recovery successor; capture the current classification with `gentle-ai review inspect-authority --cwd %q`",
			CompactRecoveryEdgeExitRepairRecoveryBinding, request.SuccessorLineageID, root)
	}
	if recovery.PredecessorLineageID != request.PredecessorLineageID {
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: successor %q names predecessor %q; run `gentle-ai review repair-recovery-binding --preflight --cwd %q` to publish the edge it actually records",
			CompactRecoveryEdgeExitRepairRecoveryBinding, request.SuccessorLineageID, recovery.PredecessorLineageID, root)
	}
	predecessor, err := (CompactStore{Dir: filepath.Join(versionRoot, request.PredecessorLineageID), lineageID: request.PredecessorLineageID}).Load()
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("load recovery binding repair predecessor: %w", err)
	}
	if drift := compactRecoveryBindingRepairRevisionDrift(request, predecessor.Revision, successor.Revision); drift != nil {
		if initiallyMatched {
			return CompactReclaimRecord{}, fmt.Errorf("compact recovery binding repair changed before exclusive maintenance ownership: %w", drift)
		}
		return CompactReclaimRecord{}, drift
	}
	classification := classifyCompactRecoveryAuthorizationTargetDrift(predecessor, successor)
	if !classification.Admitted {
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: %v. Run `gentle-ai review repair-recovery-binding --preflight --cwd %q` to see which edges this class still admits.%s",
			CompactRecoveryEdgeExitRepairRecoveryBinding, classification.RefusalReason,
			root, compactNonReconcilableContinuation(ctx, root, successor))
	}
	candidate := CompactRecoveryBindingRepairCandidate{
		Class: AuthorityRepairClassCompactV2RecoveryTargetDrift, Cause: AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted,
		Disposition: AuthorityRepairDispositionQuarantineDriftedRecoveryBinding, RepositoryBinding: repositoryBinding,
		PredecessorLineageID: request.PredecessorLineageID, ExpectedPredecessorRevision: predecessor.Revision,
		SuccessorLineageID: request.SuccessorLineageID, ExpectedSuccessorRevision: successor.Revision,
		PersistedTargetIdentity: classification.PersistedTargetIdentity, AuthorizedTargetIdentity: classification.AuthorizedTargetIdentity,
		RecordedAuthorizationSHA256: classification.RecordedAuthorizationSHA256,
	}
	if request.MaintainerAuthorization != compactRecoveryBindingRepairAuthorizationBinding(repositoryBinding, candidate, request.Actor, request.Reason) {
		return CompactReclaimRecord{}, fmt.Errorf("%s requires an exact maintainer authorization binding; run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` to publish every bound value again (schema %s, class %s, cause %s, disposition %s, repository %s, over predecessor %s@%s, successor %s@%s, persisted target %s, authorized target %s, recorded authorization %s)",
			CompactRecoveryEdgeExitRepairRecoveryBinding, AuthorityRepairAuthorizationSchema,
			AuthorityRepairClassCompactV2RecoveryTargetDrift, AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted,
			AuthorityRepairDispositionQuarantineDriftedRecoveryBinding, repositoryBinding,
			candidate.PredecessorLineageID, candidate.ExpectedPredecessorRevision,
			candidate.SuccessorLineageID, candidate.ExpectedSuccessorRevision,
			candidate.PersistedTargetIdentity, candidate.AuthorizedTargetIdentity, candidate.RecordedAuthorizationSHA256)
	}
	records, _, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if blocked := compactRecoveryBindingRepairBlocker(records, request.PredecessorLineageID, request.SuccessorLineageID); blocked != "" {
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: %s. Capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q` before ordering the repairs",
			CompactRecoveryEdgeExitRepairRecoveryBinding, blocked, root)
	}
	remaining := compactRecordsWithout(records, request.SuccessorLineageID)
	if err := compactAuthorityRemovalRegression(records, remaining); err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("%s refused: %w", CompactRecoveryEdgeExitRepairRecoveryBinding, err)
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		return CompactReclaimRecord{}, fmt.Errorf("inspect recovery binding repair target: %w", err)
	}
	residue := make([]string, 0, len(items))
	for _, item := range items {
		residue = append(residue, item.Name())
	}
	sort.Strings(residue)
	if request.RepairedAt.IsZero() {
		request.RepairedAt = time.Now().UTC()
	}
	committed, err := quarantineCompactStoreEntry(ctx, base, dir, CompactReclaimRecord{
		Schema: CompactReclaimRecordSchema, Status: CompactReclaimPrepared, LineageID: request.SuccessorLineageID,
		Reason: request.Reason, Actor: request.Actor, ReclaimedAt: request.RepairedAt.UTC(),
		SourcePath: dir, Residue: residue,
		RecoveryBindingRepair: &CompactRecoveryBindingRepairProof{
			Class: string(AuthorityRepairClassCompactV2RecoveryTargetDrift), Cause: string(AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted),
			Disposition: string(AuthorityRepairDispositionQuarantineDriftedRecoveryBinding), RepositoryBinding: repositoryBinding,
			AuthorizationSchema: AuthorityRepairAuthorizationSchema, PredecessorLineageID: request.PredecessorLineageID,
			PredecessorRevision: predecessor.Revision, SuccessorRevision: successor.Revision,
			PersistedTargetIdentity: classification.PersistedTargetIdentity, AuthorizedTargetIdentity: classification.AuthorizedTargetIdentity,
			RecordedAuthorizationSHA256: classification.RecordedAuthorizationSHA256, RequestDigest: requestDigest,
			ValidationError: classification.ValidationError.Error(),
		},
	})
	if err != nil {
		return committed, err
	}
	if err := verifyCompactRecoveryBindingRepairReadback(base, dir, request.SuccessorLineageID, records); err != nil {
		return committed, err
	}
	return committed, nil
}

func compactRecoveryBindingRepairRevisionDrift(request CompactRecoveryBindingRepairRequest, predecessorRevision, successorRevision string) error {
	if predecessorRevision != request.ExpectedPredecessorRevision {
		return fmt.Errorf("%w: expected predecessor revision %q, current %q", ErrConcurrentUpdate, request.ExpectedPredecessorRevision, predecessorRevision)
	}
	if successorRevision != request.ExpectedSuccessorRevision {
		return fmt.Errorf("%w: expected successor revision %q, current %q", ErrConcurrentUpdate, request.ExpectedSuccessorRevision, successorRevision)
	}
	return nil
}

func compactRecoveryBindingRepairInitiallyMatched(base, repositoryBinding string, request CompactRecoveryBindingRepairRequest) bool {
	records, _, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return false
	}
	candidate, admitted := compactRecoveryBindingRepairCandidateFor(records, repositoryBinding, request.SuccessorLineageID)
	return admitted && candidate.Blocked == "" &&
		candidate.PredecessorLineageID == request.PredecessorLineageID &&
		candidate.ExpectedPredecessorRevision == request.ExpectedPredecessorRevision &&
		candidate.ExpectedSuccessorRevision == request.ExpectedSuccessorRevision
}

// verifyCompactRecoveryBindingRepairReadback re-reads the store under the
// still-held locks and proves the repair removed exactly one graph defect and
// introduced none, rather than trusting the move it just performed.
func verifyCompactRecoveryBindingRepairReadback(base, dir, successorLineage string, before map[string]CompactRecord) error {
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		return compactRecoveryBindingRepairReadbackRefusal(fmt.Sprintf("still observes source %q: %v", dir, err))
	}
	after, _, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return fmt.Errorf("read back compact recovery binding repair: %w", err)
	}
	if _, present := after[successorLineage]; present {
		return compactRecoveryBindingRepairReadbackRefusal(fmt.Sprintf("still observes successor %q", successorLineage))
	}
	priorViolations, _ := compactAuthorityGraphViolations(before)
	remainingViolations, _ := compactAuthorityGraphViolations(after)
	if len(remainingViolations) != len(priorViolations)-1 {
		return compactRecoveryBindingRepairReadbackRefusal(fmt.Sprintf("observes %d authority graph defects, want %d",
			len(remainingViolations), len(priorViolations)-1))
	}
	return compactAuthorityRemovalRegression(before, after)
}

// compactRecoveryBindingRepairReadbackRefusal frames every readback failure
// with the one command that answers it. The audited quarantine record is
// already committed at this point, so the operator needs the machine-readable
// diagnosis, not another repair.
func compactRecoveryBindingRepairReadbackRefusal(detail string) error {
	return fmt.Errorf("compact recovery binding repair readback %s; capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd <repo>` beside the committed reclaim-record.json", detail)
}

// compactRecoveryBindingRepairReplayRefusal frames the replay-record refusals
// that are evidence about the AUDIT RECORD itself — tampering, an unsafe path,
// an interrupted move — and therefore have no command-shaped resolution.
func compactRecoveryBindingRepairReplayRefusal(detail string) error {
	// refusal:by-design world-action: a quarantined audit record that no longer re-derives is evidence of tampering or an interrupted move, and no review command may rewrite an audit record — that is the guarantee quarantine exists to give — so the exit is a maintainer inspecting the audited quarantine directory beside the authority root
	return fmt.Errorf("compact recovery binding repair refused %s", detail)
}

// compactRecoveryBindingRepairReplayInputRefusal frames the replay refusals the
// operator can fix by re-issuing the request: the recorded audit or the
// authorization describes a different edge than the request does. Those are
// input mismatches, not tampering, so they name the command that republishes
// every bound value instead of declaring a dead end.
func compactRecoveryBindingRepairReplayInputRefusal(detail string) error {
	return fmt.Errorf("compact recovery binding repair refused %s; run `gentle-ai review repair-recovery-binding --preflight --cwd <repo>` to publish every bound value for the edge this request names", detail)
}

type compactRecoveryBindingRepairRecordMatch struct {
	record CompactReclaimRecord
	path   string
}

// discoverCompactRecoveryBindingRepairRecord finds the one quarantine record
// this exact request already produced. Records of any other class are never
// replayable through this verb.
func discoverCompactRecoveryBindingRepairRecord(ctx context.Context, base string, request CompactRecoveryBindingRepairRequest, repositoryBinding, requestDigest string) (compactRecoveryBindingRepairRecordMatch, bool, error) {
	if err := ctx.Err(); err != nil {
		return compactRecoveryBindingRepairRecordMatch{}, false, err
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	if err := ensureCanonicalReviewQuarantineRoot(base, quarantineRoot); err != nil {
		return compactRecoveryBindingRepairRecordMatch{}, false, err
	}
	entries, err := readAuthorityRepairDirectory(quarantineRoot, authorityRepairMaxQuarantineRecords)
	if os.IsNotExist(err) {
		return compactRecoveryBindingRepairRecordMatch{}, false, nil
	}
	if err != nil {
		return compactRecoveryBindingRepairRecordMatch{}, false, fmt.Errorf("inspect compact recovery binding repair quarantine: %w", err)
	}
	var matched *compactRecoveryBindingRepairRecordMatch
	// The same bounded read the classified replay discovery performs: a per-record
	// cap alone still leaves a records x cap read budget, so the cumulative
	// accumulator is what actually bounds it.
	totalBytes := int64(0)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return compactRecoveryBindingRepairRecordMatch{}, false, err
		}
		if !strings.HasPrefix(entry.Name(), request.SuccessorLineageID+"-") {
			continue
		}
		path := filepath.Join(quarantineRoot, entry.Name())
		info, statErr := os.Lstat(path)
		canonical, canonicalErr := filepath.EvalSymlinks(path)
		if statErr != nil || canonicalErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || filepath.Clean(canonical) != path {
			return compactRecoveryBindingRepairRecordMatch{}, false, compactRecoveryBindingRepairReplayRefusal("unsafe quarantine directory")
		}
		recordPath := filepath.Join(path, "reclaim-record.json")
		recordInfo, statErr := os.Lstat(recordPath)
		if statErr != nil || recordInfo.Mode()&os.ModeSymlink != 0 || !recordInfo.Mode().IsRegular() ||
			recordInfo.Size() < 0 || recordInfo.Size() > authorityRepairMaxEventBytes {
			return compactRecoveryBindingRepairRecordMatch{}, false, compactRecoveryBindingRepairReplayRefusal("unsafe replay record")
		}
		totalBytes += recordInfo.Size()
		if totalBytes > authorityRepairMaxTotalBytes {
			return compactRecoveryBindingRepairRecordMatch{}, false, errAuthorityRepairTruncated
		}
		payload, readErr := os.ReadFile(recordPath)
		if readErr != nil || int64(len(payload)) != recordInfo.Size() {
			return compactRecoveryBindingRepairRecordMatch{}, false, compactRecoveryBindingRepairReplayRefusal("unreadable replay record")
		}
		record, decodeErr := decodeClassifiedAuthorityRepairRecord(payload)
		if decodeErr != nil {
			return compactRecoveryBindingRepairRecordMatch{}, false, fmt.Errorf("compact recovery binding repair refused corrupt replay record: %w", decodeErr)
		}
		if record.RecoveryBindingRepair == nil {
			// Records of every other class remain replayable only through the
			// operation that created them; they can never prove this class.
			continue
		}
		if record.LineageID != request.SuccessorLineageID {
			// The directory prefix is only a cheap pre-filter: quarantine
			// directories are named "<lineage>-<random>", so repairing lineage
			// "x" also scans the completed quarantine of sibling lineage "x-y".
			// Records are never deleted, so treating another lineage's record as
			// a malformed one for this request would permanently dead-end every
			// hyphen-sharing sibling — the sequential multi-edge repair this verb
			// exists to provide. The record's own lineage is the authority.
			continue
		}
		if err := validateCompactRecoveryBindingRepairRecordShape(record, path, base, request, repositoryBinding, requestDigest); err != nil {
			return compactRecoveryBindingRepairRecordMatch{}, false, err
		}
		if matched != nil {
			return compactRecoveryBindingRepairRecordMatch{}, false, compactRecoveryBindingRepairReplayRefusal("duplicate replay records")
		}
		found := compactRecoveryBindingRepairRecordMatch{record: record, path: path}
		matched = &found
	}
	if matched == nil {
		return compactRecoveryBindingRepairRecordMatch{}, false, nil
	}
	return *matched, true, nil
}

func validateCompactRecoveryBindingRepairRecordShape(record CompactReclaimRecord, quarantinePath, base string, request CompactRecoveryBindingRepairRequest, repositoryBinding, requestDigest string) error {
	proof := record.RecoveryBindingRepair
	if record.Schema != CompactReclaimRecordSchema ||
		(record.Status != CompactReclaimPrepared && record.Status != CompactReclaimCommitted) ||
		record.LineageID != request.SuccessorLineageID || record.ReclaimedAt.IsZero() ||
		record.InvalidRecoveryEdge != nil || record.MalformedRecoveryAuthorization != nil || record.PristineAbandonment != nil ||
		record.IncompleteAbandonment != nil || record.MalformedLegacyFreeze != nil || record.LegacyAliasRepair != nil ||
		record.ClassifiedAuthorityRepair != nil || record.LegacyFixScopeQuarantine != nil {
		return compactRecoveryBindingRepairReplayRefusal("incomplete replay record")
	}
	if record.SourcePath != filepath.Join(base, "v2", request.SuccessorLineageID) || record.QuarantinePath != quarantinePath ||
		record.Reason != request.Reason || record.Actor != request.Actor {
		return compactRecoveryBindingRepairReplayInputRefusal("mismatched replay audit")
	}
	if proof.Class != string(AuthorityRepairClassCompactV2RecoveryTargetDrift) ||
		proof.Cause != string(AuthorityRepairCauseRecoveryAuthorizationTargetNeverPersisted) ||
		proof.Disposition != string(AuthorityRepairDispositionQuarantineDriftedRecoveryBinding) ||
		proof.AuthorizationSchema != AuthorityRepairAuthorizationSchema || proof.RepositoryBinding != repositoryBinding ||
		proof.PredecessorLineageID != request.PredecessorLineageID || proof.PredecessorRevision != request.ExpectedPredecessorRevision ||
		proof.SuccessorRevision != request.ExpectedSuccessorRevision || proof.RequestDigest != requestDigest {
		return compactRecoveryBindingRepairReplayRefusal("mismatched replay proof")
	}
	candidate := CompactRecoveryBindingRepairCandidate{
		PredecessorLineageID: proof.PredecessorLineageID, ExpectedPredecessorRevision: proof.PredecessorRevision,
		SuccessorLineageID: record.LineageID, ExpectedSuccessorRevision: proof.SuccessorRevision,
		PersistedTargetIdentity: proof.PersistedTargetIdentity, AuthorizedTargetIdentity: proof.AuthorizedTargetIdentity,
		RecordedAuthorizationSHA256: proof.RecordedAuthorizationSHA256,
	}
	if request.MaintainerAuthorization != compactRecoveryBindingRepairAuthorizationBinding(repositoryBinding, candidate, request.Actor, request.Reason) {
		return compactRecoveryBindingRepairReplayInputRefusal("a replay whose authorization does not re-derive")
	}
	previous := ""
	for _, name := range record.Residue {
		if name == "" || name <= previous || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			return compactRecoveryBindingRepairReplayRefusal("noncanonical residue inventory")
		}
		previous = name
	}
	return nil
}

// reconcileCompactRecoveryBindingRepairRecord converges an exact replay. A
// committed record returns unchanged; a prepared one resumes from whichever
// durable phase it stopped at, discriminated by the residue directory.
//
// The source and residue discriminator uses the canonical-directory check
// rather than a bare stat: a symlinked residue must never read as a genuine
// one, and a permission error must never be coerced into "does not exist",
// because that boolean is what decides whether this call performs the durable
// move.
func reconcileCompactRecoveryBindingRepairRecord(ctx context.Context, base, root, repositoryBinding string, request CompactRecoveryBindingRepairRequest, match compactRecoveryBindingRepairRecordMatch) (CompactReclaimRecord, error) {
	record := match.record
	residuePath := filepath.Join(match.path, "residue")
	residueExists, residueErr := canonicalClassifiedRepairDirectory(residuePath)
	if residueErr != nil {
		return CompactReclaimRecord{}, compactRecoveryBindingRepairReplayRefusal("an unsafe or unreadable replay residue directory")
	}
	sourceExists, sourceErr := canonicalClassifiedRepairDirectory(record.SourcePath)
	if sourceErr != nil {
		return CompactReclaimRecord{}, compactRecoveryBindingRepairReplayRefusal("an unsafe or unreadable replay source directory")
	}
	if record.Status == CompactReclaimCommitted && (sourceExists || !residueExists) {
		return CompactReclaimRecord{}, compactRecoveryBindingRepairReplayRefusal("incomplete committed residue")
	}
	if record.Status == CompactReclaimPrepared && sourceExists == residueExists {
		return CompactReclaimRecord{}, compactRecoveryBindingRepairReplayRefusal("ambiguous prepared residue")
	}
	proofRoot := residuePath
	if !residueExists {
		proofRoot = record.SourcePath
	}
	if err := validateCompactRecoveryBindingRepairResidue(record, proofRoot); err != nil {
		return CompactReclaimRecord{}, err
	}
	if record.Status == CompactReclaimCommitted {
		return record, nil
	}
	var before map[string]CompactRecord
	if sourceExists {
		// The durable move has not happened yet, so this call is a mutation, not a
		// convergence: it must clear the same live admission the fresh path clears.
		// A crash between the prepared audit record and the rename leaves the
		// authority writable, and resuming on the recorded proof alone would move
		// an entry the CURRENT graph no longer admits.
		admitted, err := compactRecoveryBindingRepairStillAdmits(base, root, repositoryBinding, request)
		if err != nil {
			return CompactReclaimRecord{}, err
		}
		before = admitted
		if err := reclaimQuarantineResidue(record.SourcePath, residuePath); err != nil {
			return record, fmt.Errorf("resume compact recovery binding repair residue: %w", err)
		}
		if err := SyncReviewDirectory(filepath.Dir(record.SourcePath)); err != nil {
			return record, err
		}
		if err := SyncReviewDirectory(match.path); err != nil {
			return record, err
		}
		if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseRenamed, record); err != nil {
			return record, err
		}
	}
	committed := record
	committed.Status = CompactReclaimCommitted
	if err := persistReclaimRecord(committed); err != nil {
		return record, fmt.Errorf("resume compact recovery binding repair audit: %w", err)
	}
	if err := compactReclaimPhaseHook(ctx, compactReclaimPhaseCommitted, committed); err != nil {
		return committed, err
	}
	if before != nil {
		// A resumed move earns the same readback proof as a fresh one: it removed
		// exactly one graph defect and introduced none.
		if err := verifyCompactRecoveryBindingRepairReadback(base, record.SourcePath, record.LineageID, before); err != nil {
			return committed, err
		}
	}
	return committed, nil
}

// compactRecoveryBindingRepairStillAdmits re-derives the live admission before
// a prepared replay performs the durable move it never reached, and returns the
// record set that admission was proven against so the resumed move can be read
// back against it. The neighbouring classified route closes the same window by
// re-validating its live assessment whenever the prepared source still exists.
func compactRecoveryBindingRepairStillAdmits(base, root, repositoryBinding string, request CompactRecoveryBindingRepairRequest) (map[string]CompactRecord, error) {
	records, _, err := loadCompactRecoveryBindingRepairRecordSet(base)
	if err != nil {
		return nil, err
	}
	candidate, admitted := compactRecoveryBindingRepairCandidateFor(records, repositoryBinding, request.SuccessorLineageID)
	if !admitted {
		return nil, compactRecoveryBindingRepairLiveAdmissionRefusal(root, "the live authority graph no longer classifies that edge into this class")
	}
	if candidate.PredecessorLineageID != request.PredecessorLineageID ||
		candidate.ExpectedPredecessorRevision != request.ExpectedPredecessorRevision ||
		candidate.ExpectedSuccessorRevision != request.ExpectedSuccessorRevision {
		return nil, fmt.Errorf("%w: a prepared compact recovery binding repair expected predecessor %s@%s and successor @%s, current %s@%s and @%s",
			ErrConcurrentUpdate, request.PredecessorLineageID, request.ExpectedPredecessorRevision, request.ExpectedSuccessorRevision,
			candidate.PredecessorLineageID, candidate.ExpectedPredecessorRevision, candidate.ExpectedSuccessorRevision)
	}
	if candidate.Blocked != "" {
		return nil, compactRecoveryBindingRepairLiveAdmissionRefusal(root, candidate.Blocked)
	}
	if err := compactAuthorityRemovalRegression(records, compactRecordsWithout(records, request.SuccessorLineageID)); err != nil {
		return nil, fmt.Errorf("%s refused to resume a prepared repair: %w", CompactRecoveryEdgeExitRepairRecoveryBinding, err)
	}
	return records, nil
}

// compactRecoveryBindingRepairLiveAdmissionRefusal frames a prepared repair the
// live authority no longer admits. It is operator-fixable by construction — the
// preflight republishes what the class still admits — so it names that command
// instead of declaring a dead end.
func compactRecoveryBindingRepairLiveAdmissionRefusal(root, detail string) error {
	return fmt.Errorf("%s refused to resume a prepared repair: %s. Run `gentle-ai review repair-recovery-binding --preflight --cwd %q` to publish the edges this class still admits, and capture the complete machine-readable diagnosis with `gentle-ai review inspect-authority --cwd %q`",
		CompactRecoveryEdgeExitRepairRecoveryBinding, detail, root, root)
}

// validateCompactRecoveryBindingRepairResidue re-derives the drift proof from
// the quarantined bytes themselves and requires it to equal the recorded proof,
// so a fabricated reclaim-record.json can never replay as a genuine repair.
func validateCompactRecoveryBindingRepairResidue(record CompactReclaimRecord, proofRoot string) error {
	items, err := os.ReadDir(proofRoot)
	if err != nil {
		return fmt.Errorf("compact recovery binding repair refused unreadable replay residue: %w", err)
	}
	observed := make([]string, 0, len(items))
	for _, item := range items {
		observed = append(observed, item.Name())
	}
	sort.Strings(observed)
	if !reflect.DeepEqual(record.Residue, observed) {
		return compactRecoveryBindingRepairReplayRefusal("forged replay proof")
	}
	payload, err := os.ReadFile(filepath.Join(proofRoot, compactStateFileName))
	if err != nil {
		return compactRecoveryBindingRepairReplayRefusal("forged replay proof")
	}
	quarantined, err := parseCompactRecord(payload, record.LineageID)
	if err != nil {
		return compactRecoveryBindingRepairReplayRefusal("forged replay proof")
	}
	recovery := quarantined.State.Recovery
	if recovery == nil {
		return compactRecoveryBindingRepairReplayRefusal("forged replay proof")
	}
	recorded := sha256.Sum256([]byte(recovery.MaintainerAuthorization))
	proof := record.RecoveryBindingRepair
	rederived := CompactRecoveryBindingRepairProof{
		Class: proof.Class, Cause: proof.Cause, Disposition: proof.Disposition, RepositoryBinding: proof.RepositoryBinding,
		AuthorizationSchema: proof.AuthorizationSchema, PredecessorLineageID: recovery.PredecessorLineageID,
		PredecessorRevision: recovery.PredecessorRevision, SuccessorRevision: proof.SuccessorRevision,
		PersistedTargetIdentity: quarantined.State.InitialSnapshot.Identity, AuthorizedTargetIdentity: proof.AuthorizedTargetIdentity,
		RecordedAuthorizationSHA256: "sha256:" + hex.EncodeToString(recorded[:]), RequestDigest: proof.RequestDigest,
		ValidationError: proof.ValidationError,
	}
	if compactRecoveryAuthorizationBinding(recovery.PredecessorLineageID, recovery.PredecessorRevision,
		proof.AuthorizedTargetIdentity, recovery.Actor, recovery.Reason) != recovery.MaintainerAuthorization ||
		!reflect.DeepEqual(rederived, *proof) {
		return compactRecoveryBindingRepairReplayRefusal("forged replay proof")
	}
	return nil
}
