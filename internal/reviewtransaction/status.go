package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const ReviewAuthorityStatusSchema = "gentle-ai.review-authority-status/v1"

var probeExistingStoreLock = tryLockFile

type AuthorityStatus string

const (
	AuthorityStatusClean      AuthorityStatus = "clean"
	AuthorityStatusActive     AuthorityStatus = "active"
	AuthorityStatusApproved   AuthorityStatus = "approved"
	AuthorityStatusEscalated  AuthorityStatus = "escalated"
	AuthorityStatusInvalid    AuthorityStatus = "invalid"
	AuthorityStatusIncomplete AuthorityStatus = "incomplete-store-entry"
	AuthorityStatusReset      AuthorityStatus = "reset-in-progress"
	AuthorityStatusSuperseded AuthorityStatus = "superseded"
	AuthorityStatusRecovered  AuthorityStatus = "recovered"
	AuthorityStatusCollision  AuthorityStatus = "same-lineage-mixed-collision"
	// AuthorityStatusHistorical marks a structurally valid terminal legacy-v1
	// chain that predates the receipt contract: its receipt file is absent
	// (never corrupt or mismatched). Such chains stay inventory-readable
	// without forcing the global inventory incomplete, but they are never
	// discoverable as gate or receipt authority because no receipt exists.
	AuthorityStatusHistorical AuthorityStatus = "historical-pre-receipt"
)

type AuthorityVersion string

const (
	AuthorityVersionCompact AuthorityVersion = "compact-v2"
	AuthorityVersionLegacy  AuthorityVersion = "legacy-v1"
)

type AuthorityLockStatus string

const (
	AuthorityLockOwned     AuthorityLockStatus = "owned"
	AuthorityLockReleased  AuthorityLockStatus = "released"
	AuthorityLockAmbiguous AuthorityLockStatus = "ambiguous"
)

type AuthorityInventoryEntry struct {
	Version       AuthorityVersion           `json:"version"`
	LineageID     string                     `json:"lineage_id,omitempty"`
	Path          string                     `json:"path"`
	Status        AuthorityStatus            `json:"status"`
	State         State                      `json:"state,omitempty"`
	Revision      string                     `json:"revision,omitempty"`
	ChainIdentity string                     `json:"chain_identity,omitempty"`
	Recovery      *CompactRecoveryProvenance `json:"recovery,omitempty"`
	Problems      []string                   `json:"problems"`
	compact       *CompactRecord
}

type AuthorityLockEvidence struct {
	Version   AuthorityVersion    `json:"version"`
	LineageID string              `json:"lineage_id,omitempty"`
	Path      string              `json:"path"`
	Status    AuthorityLockStatus `json:"status"`
	Owner     *storeLockOwner     `json:"owner,omitempty"`
	Problem   string              `json:"problem,omitempty"`
}

type AuthorityInventoryDiagnostic struct {
	Path    string `json:"path"`
	Problem string `json:"problem"`
}

type AuthorityStatusReport struct {
	Schema        string                         `json:"schema"`
	Operation     string                         `json:"operation"`
	Repository    string                         `json:"repository"`
	Complete      bool                           `json:"complete"`
	Authoritative bool                           `json:"authoritative"`
	Status        AuthorityStatus                `json:"status"`
	Entries       []AuthorityInventoryEntry      `json:"entries"`
	Locks         []AuthorityLockEvidence        `json:"locks"`
	Diagnostics   []AuthorityInventoryDiagnostic `json:"diagnostics"`
}

// InventoryAuthority reads every recognized authority location in the repository
// Git common directory. It never creates directories, acquires locks, or repairs
// interrupted state; callers must treat Complete=false as deny-by-default.
func InventoryAuthority(ctx context.Context, repo string) (AuthorityStatusReport, error) {
	root, repository, err := reviewAuthorityRoot(ctx, repo)
	if err != nil {
		return AuthorityStatusReport{}, err
	}
	report := AuthorityStatusReport{
		Schema: ReviewAuthorityStatusSchema, Operation: "review/status", Repository: repository,
		Complete: true, Authoritative: true, Status: AuthorityStatusClean,
		Entries: []AuthorityInventoryEntry{}, Locks: []AuthorityLockEvidence{}, Diagnostics: []AuthorityInventoryDiagnostic{},
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		report.Complete, report.Authoritative, report.Status = false, false, AuthorityStatusInvalid
		report.Diagnostics = append(report.Diagnostics, AuthorityInventoryDiagnostic{Path: root, Problem: "inspect review authority root: " + err.Error()})
		return report, nil
	}

	legacy := inventoryVersion(ctx, repository, root, "v1", AuthorityVersionLegacy)
	compact := inventoryVersion(ctx, repository, root, "v2", AuthorityVersionCompact)
	report.Entries = append(report.Entries, legacy.entries...)
	report.Entries = append(report.Entries, compact.entries...)
	report.Locks = append(report.Locks, legacy.locks...)
	report.Locks = append(report.Locks, compact.locks...)
	report.Diagnostics = append(report.Diagnostics, legacy.diagnostics...)
	report.Diagnostics = append(report.Diagnostics, compact.diagnostics...)
	if !legacy.complete || !compact.complete {
		report.Complete = false
	}
	for _, lock := range report.Locks {
		if lock.Status == AuthorityLockAmbiguous {
			report.Complete = false
		}
	}
	markCompactGraph(&report)
	markMixedCollisions(&report)
	for _, entry := range report.Entries {
		if entry.Status == AuthorityStatusInvalid || entry.Status == AuthorityStatusIncomplete || entry.Status == AuthorityStatusReset || entry.Status == AuthorityStatusCollision {
			report.Complete = false
		}
	}
	sortAuthorityReport(&report)
	report.Authoritative = report.Complete
	if len(report.Entries) == 0 && report.Complete {
		report.Status = AuthorityStatusClean
	} else if !report.Complete {
		report.Status = AuthorityStatusInvalid
	} else if len(report.Entries) == 1 {
		report.Status = report.Entries[0].Status
	} else {
		report.Status = AuthorityStatusActive
	}
	return report, nil
}

type authorityVersionInventory struct {
	entries     []AuthorityInventoryEntry
	locks       []AuthorityLockEvidence
	diagnostics []AuthorityInventoryDiagnostic
	complete    bool
}

func inventoryVersion(ctx context.Context, repo, root, directory string, version AuthorityVersion) authorityVersionInventory {
	result := authorityVersionInventory{entries: []AuthorityInventoryEntry{}, locks: []AuthorityLockEvidence{}, diagnostics: []AuthorityInventoryDiagnostic{}, complete: true}
	versionRoot := filepath.Join(root, directory)
	entries, err := os.ReadDir(versionRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		return inventoryVersionProblem(result, versionRoot, err)
	}
	if version == AuthorityVersionCompact {
		if lock, exists := inventoryLock(version, "", filepath.Join(versionRoot, "LOCK")); exists {
			result.locks = append(result.locks, lock)
		}
	}
	for _, item := range entries {
		path := filepath.Join(versionRoot, item.Name())
		if version == AuthorityVersionCompact && item.Name() == "LOCK" && !item.IsDir() {
			continue
		}
		if !item.IsDir() {
			result = inventoryUnexpected(result, path, "unexpected authority-root entry")
			continue
		}
		if validateLineageID(item.Name()) != nil {
			result = inventoryUnexpected(result, path, "authority directory is not a canonical lineage identifier")
			continue
		}
		entry, locks := inventoryLineage(ctx, repo, version, path, item.Name())
		result.entries = append(result.entries, entry)
		result.locks = append(result.locks, locks...)
		if entry.Status == AuthorityStatusReset {
			result.complete = false
		}
	}
	return result
}

func inventoryVersionProblem(result authorityVersionInventory, path string, err error) authorityVersionInventory {
	result.complete = false
	result.diagnostics = append(result.diagnostics, AuthorityInventoryDiagnostic{Path: path, Problem: err.Error()})
	return result
}

func inventoryUnexpected(result authorityVersionInventory, path, problem string) authorityVersionInventory {
	result.complete = false
	result.diagnostics = append(result.diagnostics, AuthorityInventoryDiagnostic{Path: path, Problem: problem})
	return result
}

func inventoryLineage(ctx context.Context, repo string, version AuthorityVersion, path, lineage string) (AuthorityInventoryEntry, []AuthorityLockEvidence) {
	entry := AuthorityInventoryEntry{Version: version, LineageID: lineage, Path: path, Problems: []string{}}
	items, err := os.ReadDir(path)
	if err != nil {
		entry.Status, entry.Problems = AuthorityStatusInvalid, []string{err.Error()}
		return entry, nil
	}
	locks := []AuthorityLockEvidence{}
	if version == AuthorityVersionLegacy {
		if lock, exists := inventoryLock(version, lineage, filepath.Join(path, "LOCK")); exists {
			locks = append(locks, lock)
		}
	}
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".atomic-") {
			entry.Status = AuthorityStatusReset
			entry.Problems = append(entry.Problems, "interrupted atomic authority write residue: "+item.Name())
		}
	}
	if entry.Status == AuthorityStatusReset {
		return entry, locks
	}
	if version == AuthorityVersionCompact {
		if _, statErr := os.Stat(filepath.Join(path, compactStateFileName)); os.IsNotExist(statErr) && !compactStoreHoldsAuthority(items) {
			entry.Status = AuthorityStatusIncomplete
			entry.Problems = []string{"compact store entry has no review-state.json and no authoritative artifacts; quarantine it with gentle-ai review reclaim"}
			return entry, locks
		}
		store := CompactStore{Dir: path, lineageID: lineage, repo: repo}
		record, err := store.Load()
		if err != nil {
			entry.Status, entry.Problems = AuthorityStatusInvalid, []string{err.Error()}
			return entry, locks
		}
		entry.Revision, entry.State, entry.Recovery = record.Revision, record.State.State, record.State.Recovery
		entry.compact = &record
		entry.Status = authorityStatusForState(record.State.State)
		if payload, err := os.ReadFile(store.ReceiptPath()); err == nil {
			receipt, parseErr := ParseCompactReceipt(payload)
			authoritative, authorityErr := record.State.Receipt()
			if parseErr != nil {
				entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"invalid compact receipt: " + parseErr.Error()}
			} else if authorityErr != nil || !compactReceiptEqual(receipt, authoritative) {
				entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"compact receipt does not match terminal authority"}
			}
		} else if os.IsNotExist(err) && (record.State.State == StateApproved || record.State.State == StateEscalated) {
			entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"terminal compact authority is missing its receipt"}
		} else if !os.IsNotExist(err) {
			entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"read compact receipt: " + err.Error()}
		}
		return entry, locks
	}
	store := Store{Dir: path, lineageID: lineage, repo: repo, readOnly: true}
	chain, err := store.LoadChain()
	if err != nil {
		entry.Status, entry.Problems = AuthorityStatusInvalid, []string{err.Error()}
		return entry, locks
	}
	transaction := chain.Records[len(chain.Records)-1].Transaction
	entry.State, entry.Revision, entry.ChainIdentity = transaction.State, chain.HeadRevision, chain.Identity
	entry.Status = authorityStatusForState(transaction.State)
	receiptPath := filepath.Join(path, "artifacts", "receipt.json")
	if payload, err := os.ReadFile(receiptPath); err == nil {
		receipt, parseErr := ParseReceipt(payload)
		authoritative, authorityErr := transaction.Receipt()
		if parseErr != nil {
			entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"invalid legacy receipt: " + parseErr.Error()}
		} else if authorityErr != nil || !reflect.DeepEqual(receipt, authoritative) {
			entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"legacy receipt does not match terminal authority"}
		}
	} else if os.IsNotExist(err) && (transaction.State == StateApproved || transaction.State == StateEscalated) {
		// A structurally valid terminal legacy-v1 chain whose receipt file is
		// absent predates the receipt contract. It stays inventory-readable as
		// historical context without forcing the global inventory incomplete,
		// yet remains ineligible as gate or discovery authority because every
		// receipt consumer requires the receipt file itself. A present-but-wrong
		// receipt is handled above and stays invalid.
		entry.Status = AuthorityStatusHistorical
	} else if !os.IsNotExist(err) {
		entry.Status, entry.Problems = AuthorityStatusInvalid, []string{"read legacy receipt: " + err.Error()}
	}
	return entry, locks
}

func authorityStatusForState(state State) AuthorityStatus {
	switch state {
	case StateApproved:
		return AuthorityStatusApproved
	case StateEscalated:
		return AuthorityStatusEscalated
	case StateInvalidated:
		return AuthorityStatusInvalid
	default:
		return AuthorityStatusActive
	}
}

func inventoryLock(version AuthorityVersion, lineage, path string) (AuthorityLockEvidence, bool) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return AuthorityLockEvidence{}, false
		}
		return AuthorityLockEvidence{Version: version, LineageID: lineage, Path: path, Status: AuthorityLockAmbiguous, Problem: "open existing lock inode: " + err.Error()}, true
	}
	defer file.Close()
	lock := AuthorityLockEvidence{Version: version, LineageID: lineage, Path: path, Status: AuthorityLockAmbiguous}
	locked, err := probeExistingStoreLock(file)
	if err != nil {
		lock.Problem = "probe existing lock inode: " + err.Error()
		return lock, true
	}
	if !locked {
		lock.Status = AuthorityLockOwned
		return lock, true
	}
	probeHeld := true
	defer func() {
		if probeHeld {
			_ = unlockFile(file)
		}
	}()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var owner storeLockOwner
	if err := decoder.Decode(&owner); err != nil {
		lock.Problem = "parse lock owner: " + err.Error()
		return lock, true
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		lock.Problem = "lock owner has multiple JSON values"
		return lock, true
	}
	if owner.Schema != storeLockSchema || strings.TrimSpace(owner.OwnerID) == "" || owner.PID <= 0 || strings.TrimSpace(owner.Host) == "" || owner.AcquiredAt.IsZero() {
		lock.Problem = "lock owner metadata is incomplete or invalid"
		return lock, true
	}
	if err := unlockFile(file); err != nil {
		lock.Problem = "release existing lock probe: " + err.Error()
		return lock, true
	}
	probeHeld = false
	lock.Status = AuthorityLockReleased
	return lock, true
}

func markCompactGraph(report *AuthorityStatusReport) {
	byLineage := map[string]int{}
	children := map[string][]int{}
	for index := range report.Entries {
		entry := &report.Entries[index]
		if entry.Version == AuthorityVersionCompact && entry.Status != AuthorityStatusReset && entry.Status != AuthorityStatusIncomplete &&
			(entry.Status != AuthorityStatusInvalid || entry.State == StateInvalidated && len(entry.Problems) == 0) {
			byLineage[entry.LineageID] = index
		}
	}
	for index := range report.Entries {
		entry := &report.Entries[index]
		if entry.Version != AuthorityVersionCompact || entry.Recovery == nil || entry.Status == AuthorityStatusInvalid || entry.Status == AuthorityStatusReset {
			continue
		}
		predecessor, ok := byLineage[entry.Recovery.PredecessorLineageID]
		if !ok || entry.compact == nil || report.Entries[predecessor].compact == nil {
			entry.Status = AuthorityStatusInvalid
			entry.Problems = append(entry.Problems, "recovery predecessor is missing")
			continue
		}
		if err := validateCompactRecoveryEdge(*report.Entries[predecessor].compact, entry.compact.State); err != nil {
			entry.Status = AuthorityStatusInvalid
			entry.Problems = append(entry.Problems, "invalid recovery edge: "+err.Error())
			continue
		}
		children[entry.Recovery.PredecessorLineageID] = append(children[entry.Recovery.PredecessorLineageID], index)
		entry.Status = AuthorityStatusRecovered
	}
	for lineage, descendants := range children {
		if len(descendants) != 1 {
			for _, index := range descendants {
				report.Entries[index].Status = AuthorityStatusInvalid
				report.Entries[index].Problems = append(report.Entries[index].Problems, "recovery graph forks from "+lineage)
			}
			continue
		}
		if predecessor, ok := byLineage[lineage]; ok && (report.Entries[predecessor].Status != AuthorityStatusInvalid ||
			report.Entries[predecessor].State == StateInvalidated && len(report.Entries[predecessor].Problems) == 0) {
			report.Entries[predecessor].Status = AuthorityStatusSuperseded
		}
	}
	for lineage, start := range byLineage {
		seen := map[string]bool{}
		for current := lineage; current != ""; {
			if seen[current] {
				for cycleLineage := range seen {
					if index, ok := byLineage[cycleLineage]; ok {
						report.Entries[index].Status = AuthorityStatusInvalid
						report.Entries[index].Problems = append(report.Entries[index].Problems, "recovery graph contains a cycle")
					}
				}
				break
			}
			seen[current] = true
			entry := report.Entries[start]
			if current != lineage {
				index, ok := byLineage[current]
				if !ok {
					break
				}
				entry = report.Entries[index]
			}
			if entry.Recovery == nil {
				break
			}
			current = entry.Recovery.PredecessorLineageID
		}
	}
}

func markMixedCollisions(report *AuthorityStatusReport) {
	versions := map[string]map[AuthorityVersion]bool{}
	for _, entry := range report.Entries {
		if entry.LineageID == "" {
			continue
		}
		if versions[entry.LineageID] == nil {
			versions[entry.LineageID] = map[AuthorityVersion]bool{}
		}
		versions[entry.LineageID][entry.Version] = true
	}
	for index := range report.Entries {
		entry := &report.Entries[index]
		if versions[entry.LineageID][AuthorityVersionCompact] && versions[entry.LineageID][AuthorityVersionLegacy] {
			entry.Status = AuthorityStatusCollision
			entry.Problems = append(entry.Problems, "lineage exists in both compact-v2 and legacy-v1 authority stores")
		}
	}
}

func sortAuthorityReport(report *AuthorityStatusReport) {
	sort.Slice(report.Entries, func(i, j int) bool {
		if report.Entries[i].LineageID != report.Entries[j].LineageID {
			return report.Entries[i].LineageID < report.Entries[j].LineageID
		}
		return report.Entries[i].Version < report.Entries[j].Version
	})
	sort.Slice(report.Locks, func(i, j int) bool { return report.Locks[i].Path < report.Locks[j].Path })
	sort.Slice(report.Diagnostics, func(i, j int) bool { return report.Diagnostics[i].Path < report.Diagnostics[j].Path })
	for index := range report.Entries {
		sort.Strings(report.Entries[index].Problems)
	}
}
