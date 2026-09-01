package reviewtransaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Clone-scoped review store reset.
//
// Review authority accumulates without bound under <git-common-dir>/gentle-ai:
// every candidate leaves a lineage behind, every lineage stays after delivery,
// and nothing removes a delivered one. `review abandon` refuses terminal
// states and `review reclaim` refuses any entry holding authoritative
// artifacts, so a store that has degraded has no exit at all. This is that
// exit, and it is deliberately the bluntest operation in the package: it does
// not repair, migrate, or reconcile anything. It removes the lineage state for
// one clone so the next review starts from nothing.
//
// Three rules shape the whole file.
//
// It is an allowlist, never a denylist. Only paths named in
// storeResetRemovableTargets are ever removed. Anything else -- including a
// directory a future release adds -- is reported and left exactly where it is.
// The failure mode of a denylist here is destroying state nobody remembered to
// exclude, and that failure is silent.
//
// The kill switch is not lineage state. It lives in two places, and one of them
// (review-transactions/rar-authority/v1/rdd-mode) is *inside* the authority
// tree, still written for gentle-ai builds installed before #2882 that read
// only that location. A reset that removed review-transactions/ wholesale would
// switch reviews back on for a user who had turned them off, which is the worst
// thing this command could possibly do.
//
// Honest accounting beats a clean-looking report. Bytes are counted as freed
// only after they are actually gone. A category that could not be moved is
// reported with its reason, is not counted, and makes the whole run incomplete.
//
// A fourth rule follows from the third. The safety this command advertises --
// the exclusive maintenance lease and the in-flight refusal -- only covers the
// state gentle-ai itself writes. A category outside that coverage cannot be
// removed by a default run no matter how dead it looks, because "we could not
// find a live writer" is not the same claim as "there is no live writer". Such
// a category is spared, reported under Preserved with the coverage it lacks,
// and removed only when the caller names it (storeResetTarget.optIn).

// StoreResetReportSchema identifies the clone-scoped review store reset
// projection.
const StoreResetReportSchema = "gentle-ai.review-store-reset/v1"

const (
	// StoreResetPreview names a read-only survey that removes nothing.
	StoreResetPreview = "preview"
	// StoreResetApplied names a run that attempted removal.
	StoreResetApplied = "reset"
)

// storeResetLockTimeout is the context deadline this command hands to
// AcquireReviewMaintenanceExclusive. It is not an independent waiting budget,
// and reading it as one is how the previous 30s literal came to describe
// behavior that never happened: the acquisition applies maintenanceLockTimeout
// internally, so that shorter bound is what a contended lease actually expires
// on. The only reason this is larger is that the same context also covers the
// Git identity resolution the acquisition performs first, and pinning it to the
// lease bound would let a slow repository probe cancel an uncontended lease.
//
// A reset that cannot get the lease promptly is competing with a live review,
// and waiting longer only makes the collision worse -- which is exactly what
// the 2s bound already enforces, so this constant tracks it rather than
// competing with it.
const storeResetLockTimeout = maintenanceLockTimeout + storeResetIdentitySlack

// storeResetIdentitySlack is the headroom storeResetLockTimeout adds for the
// repository identity resolution that runs inside the same context, before the
// lease bound applies.
const storeResetIdentitySlack = 8 * time.Second

// storeResetStagingPrefix names the directory a run moves categories into
// before deleting them. Staging is what makes the store consistent at one
// instant instead of during a long recursive walk: a concurrent reader either
// sees a category or does not, never half of one.
const storeResetStagingPrefix = ".store-reset-"

// storeResetRename is the rename seam, mirroring reclaimQuarantineResidue in
// compact_reclaim.go so partial-failure paths stay testable.
var storeResetRename = os.Rename

// storeResetRemoveTree is the deletion seam, and it exists for the same reason
// the rename seam does. The interesting deletion failure is the one that leaves
// the tree behind -- ENOSPC, EIO, a busy mount -- because that is when the byte
// accounting has to subtract what survived, and no portable arrangement of
// permissions reproduces it: every permission failure this package can stage on
// a developer's disk is either retried into success or leaves a subtree the
// accounting cannot read and therefore counts as zero.
var storeResetRemoveTree = os.RemoveAll

// storeResetAcquireLease is the lease seam. Production always takes the real
// exclusive maintenance lease; a test substitutes a wrapper around it to prove,
// from a barrier rather than from timing, that the store is classified only
// after the lease is held.
var storeResetAcquireLease = AcquireReviewMaintenanceExclusive

// storeResetTarget names one path under <git-common-dir>/gentle-ai and why it
// is in the list it is in. The reason is not decoration: this command destroys
// data, and every entry it touches or spares has to be able to justify itself
// to the person reading the preview.
type storeResetTarget struct {
	name   string
	parts  []string
	reason string
	// candidateViews marks the one category that is not merely files.
	// Candidate views are registered Git worktrees written read-only, so the
	// administrative directories that register them go with the checkouts and
	// the permissions have to be relaxed before anything can be unlinked.
	candidateViews bool
	// optIn marks a category no default run removes. Such a category is
	// reported under Preserved carrying withheldReason until the caller names
	// it explicitly, and only then does it appear under Removable.
	optIn bool
	// withheldReason explains, to the person reading the preview, why the
	// default run spares an optIn category. It is a different sentence from
	// reason: reason says what the category is, withheldReason says what this
	// command cannot promise about removing it.
	withheldReason string
}

// storeResetRemovableTargets is the complete inclusion list. Every entry is
// state derived from reviewing candidates; none of it is a user preference, and
// none of it can be authored by hand.
var storeResetRemovableTargets = []storeResetTarget{
	{
		name: "candidate-views", parts: []string{"candidate-views"}, candidateViews: true,
		reason: "per-candidate repository checkouts materialized for reviewer inspection; regenerated on demand and the largest consumer of disk",
	},
	{
		name: "review-transactions/v1", parts: []string{"review-transactions", "v1"},
		reason: "legacy per-lineage authority event chains; read-only in current builds",
	},
	{
		name: "review-transactions/v2", parts: []string{"review-transactions", "v2"},
		reason: "compact per-lineage authority records, receipts, and captured reviewer results",
	},
	{
		name: "review-transactions/quarantine", parts: []string{"review-transactions", "quarantine"},
		reason: "audited residue of lineages already discarded by reclaim or abandon",
	},
	{
		name: "review-transactions/effect-markers", parts: []string{"review-transactions", "effect-markers"},
		reason: "per-lineage delivery idempotence markers; meaningless once their lineage is gone",
	},
	{
		name: "reviews", parts: []string{"reviews"}, optIn: true,
		reason: "review graph store written by the gentle-pi adapter, outside this command's lease and in-flight coverage; removed only because --include-adapter-reviews was given",
		withheldReason: "review graph store the gentle-pi adapter writes and this command cannot vouch for: " +
			"REVIEW-MAINTENANCE.lock does not cover it and the in-flight refusal never reads it, so a default run cannot tell a dead graph from a live review; pass --include-adapter-reviews to remove it anyway",
	},
}

// storeResetPreservedTargets is the complete exclusion list. Two of these are
// the kill switch, two are state another component owns, and one is a path no
// gentle-ai code has ever written -- which is itself the reason.
var storeResetPreservedTargets = []storeResetTarget{
	{
		name: "review-mode", parts: []string{"review-mode"},
		reason: "the receipt-driven-development kill switch and its consent latch: a user preference, not lineage state",
	},
	{
		name: "review-transactions/rar-authority", parts: []string{"review-transactions", "rar-authority"},
		reason: "holds the pre-#2882 mirror of that same kill switch, still written for installed builds that read only this location",
	},
	{
		name: "sdd-runtime", parts: []string{"sdd-runtime"},
		reason: "SDD attempt state keyed by change name rather than by review lineage; removing it would lose in-flight SDD work",
	},
	{
		name: "defect-reports", parts: []string{"defect-reports"},
		reason: "diagnostic reports that may not have been filed upstream yet; evidence this command cannot regenerate",
	},
	{
		name: "review-artifacts", parts: []string{"review-artifacts"},
		reason: "no gentle-ai code writes or reads this path, so its contents were placed by hand; a reset never removes what the product did not create",
	},
	{
		name: "REVIEW-MAINTENANCE.lock", parts: []string{"REVIEW-MAINTENANCE.lock"},
		reason: "the advisory maintenance lease this operation itself holds; it carries no lineage state and is recreated on demand",
	},
}

// StoreResetEntry is one category's line in the report.
type StoreResetEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Reason  string `json:"reason"`
	Present bool   `json:"present"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Removed bool   `json:"removed"`
	// Skipped carries why a present removable category was not removed. It is
	// only ever set on a run that tried.
	Skipped string `json:"skipped,omitempty"`
}

// StoreResetLineage names one review that is still open, or one whose record
// this command could not read.
type StoreResetLineage struct {
	LineageID string `json:"lineage_id"`
	State     State  `json:"state,omitempty"`
	// Unreadable carries the parse failure for a record that could not be
	// classified. Such a record is treated as in-flight, because guessing the
	// other way destroys work.
	Unreadable string `json:"unreadable,omitempty"`
}

// StoreResetReport is what both the preview and the reset return. The two carry
// the same shape on purpose: the preview is not a different report, it is the
// same report with nothing removed yet.
type StoreResetReport struct {
	Schema     string `json:"schema"`
	Operation  string `json:"operation"`
	Repository string `json:"repository"`
	StoreRoot  string `json:"store_root"`
	// Removable, Preserved, and Unrecognized together account for every entry
	// found under the store root. Nothing is left out of the report because
	// this command did not recognize it -- that is precisely what Unrecognized
	// is for.
	Removable    []StoreResetEntry   `json:"removable"`
	Preserved    []StoreResetEntry   `json:"preserved"`
	Unrecognized []StoreResetEntry   `json:"unrecognized"`
	InFlight     []StoreResetLineage `json:"in_flight"`
	Settled      int                 `json:"settled_lineages"`
	RemovedFiles int                 `json:"removed_files"`
	RemovedBytes int64               `json:"removed_bytes"`
	// Residue names a staging directory that outlived the run. Its contents
	// are already out of the store, but the bytes are still on disk and are
	// not counted as freed. It is set both when the deletion failed and when
	// the run deliberately kept the directory to hold UnrestoredAdminDirs.
	Residue string `json:"residue,omitempty"`
	// UnrestoredAdminDirs names every Git worktree administrative directory
	// this run moved aside for a category it then did not remove, and could
	// not put back. They are the one case where a category reported SKIPPED is
	// not in the state the run found it: the checkouts are still there, but
	// their registrations are parked under Residue instead of at Original.
	UnrestoredAdminDirs []StoreResetUnrestoredAdminDir `json:"unrestored_admin_dirs,omitempty"`
	// Complete is false whenever any present removable category was not
	// removed, for any reason. A partial run never reports success.
	Complete bool `json:"complete"`
}

// StoreResetUnrestoredAdminDir names one Git worktree administrative directory
// a run moved aside and could not move back, and where it ended up.
type StoreResetUnrestoredAdminDir struct {
	// Original is where the directory belongs, and where it has to be put back
	// before the candidate view it registers works again.
	Original string `json:"original"`
	// Staged is where the run left it.
	Staged string `json:"staged"`
}

// StoreResetRequest carries the decisions the caller owns.
type StoreResetRequest struct {
	// IncludeInFlight permits removing lineages that have not reached a
	// terminal state. It exists so the refusal has an exit, not so callers can
	// set it by default.
	IncludeInFlight bool
	// IncludeAdapterReviews permits removing the adapter-written reviews/
	// graph store. It is separate from IncludeInFlight because it overrides a
	// different guarantee: IncludeInFlight discards reviews this command can
	// see and named, while this one discards a category it cannot see into at
	// all, and so cannot list, count as in flight, or lock out a writer from.
	IncludeAdapterReviews bool
}

// StoreResetInFlightError refuses a reset that would destroy open reviews.
type StoreResetInFlightError struct {
	Repository string
	Lineages   []StoreResetLineage
}

func (err *StoreResetInFlightError) Error() string {
	names := make([]string, 0, len(err.Lineages))
	for _, lineage := range err.Lineages {
		if lineage.Unreadable != "" {
			names = append(names, fmt.Sprintf("%s (unreadable: %s)", lineage.LineageID, lineage.Unreadable))
			continue
		}
		names = append(names, fmt.Sprintf("%s (%s)", lineage.LineageID, lineage.State))
	}
	return fmt.Sprintf(
		"review store reset refused: %d review(s) have not reached a terminal state: %s; finish or abandon them, or run `gentle-ai review store-reset --cwd %s --confirm --include-in-flight` to remove them anyway",
		len(err.Lineages), strings.Join(names, ", "), err.Repository,
	)
}

// StoreResetIncompleteError reports a run that removed some categories and not
// others. It carries no continuation of its own because the answer is always
// the same one: read the report, fix what the skip reason names, run it again.
type StoreResetIncompleteError struct {
	Skipped []StoreResetEntry
	Residue string
}

func (err *StoreResetIncompleteError) Error() string {
	if len(err.Skipped) == 0 && err.Residue != "" {
		return fmt.Sprintf("review store reset removed every category but could not delete its staging directory %s; the bytes are not reclaimed until it is removed", err.Residue)
	}
	names := make([]string, 0, len(err.Skipped))
	for _, entry := range err.Skipped {
		names = append(names, fmt.Sprintf("%s (%s)", entry.Name, entry.Skipped))
	}
	message := fmt.Sprintf("review store reset was incomplete: %s", strings.Join(names, "; "))
	if err.Residue != "" {
		message += fmt.Sprintf("; state this run moved aside is parked in %s", err.Residue)
	}
	return message
}

// storeResetRoot resolves <git-common-dir>/gentle-ai for one clone.
//
// It deliberately does not apply the receipt-driven-development gate. The kill
// switch decides whether reviews happen; it does not decide whether a user may
// delete their own accumulated state. Gating cleanup on it would recreate
// exactly the dead end this command exists to remove: a user whose store has
// degraded turns reviews off, and then discovers the only tool that could clean
// up refuses because reviews are off.
func storeResetRoot(ctx context.Context, repo string) (root, repository string, err error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return "", "", err
	}
	identity := lease.Identity()
	return filepath.Join(identity.GitCommonDir, "gentle-ai"), identity.RepositoryRoot, nil
}

// SurveyReviewStore reports what a reset would remove without touching
// anything. It creates no directories, takes no lock, and is safe to run
// against a store no other operation can open.
//
// It takes the same request the reset takes because the preview has to be the
// preview of the run the caller is about to make: an opt-in category shown as
// removable when the flag is absent would promise a removal that never comes,
// and shown as preserved when the flag is present would hide one that does.
func SurveyReviewStore(ctx context.Context, repo string, request StoreResetRequest) (StoreResetReport, error) {
	report, _, err := surveyReviewStore(ctx, repo, request)
	return report, err
}

func surveyReviewStore(ctx context.Context, repo string, request StoreResetRequest) (StoreResetReport, string, error) {
	if err := ctx.Err(); err != nil {
		return StoreResetReport{}, "", err
	}
	root, repository, err := storeResetRoot(ctx, repo)
	if err != nil {
		return StoreResetReport{}, "", err
	}
	report := StoreResetReport{
		Schema: StoreResetReportSchema, Operation: StoreResetPreview,
		Repository: repository, StoreRoot: root,
		Removable: []StoreResetEntry{}, Preserved: []StoreResetEntry{},
		Unrecognized: []StoreResetEntry{}, InFlight: []StoreResetLineage{},
		Complete: true,
	}
	withheld := []StoreResetEntry{}
	for _, target := range storeResetRemovableTargets {
		if target.optIn && !storeResetOptedIn(target, request) {
			spared := target
			spared.reason = target.withheldReason
			withheld = append(withheld, storeResetMeasure(root, spared))
			continue
		}
		report.Removable = append(report.Removable, storeResetMeasure(root, target))
	}
	for _, target := range storeResetPreservedTargets {
		report.Preserved = append(report.Preserved, storeResetMeasure(root, target))
	}
	// Withheld categories go last so the standing exclusion list keeps its
	// order and the conditional entries are visibly the conditional ones.
	report.Preserved = append(report.Preserved, withheld...)
	report.Unrecognized = storeResetUnrecognized(root)
	report.InFlight, report.Settled, err = storeResetLineages(root)
	if err != nil {
		return report, root, err
	}
	return report, root, nil
}

// storeResetOptedIn reports whether the caller asked for one opt-in category by
// name. It is a switch per category rather than a single "include everything"
// flag on purpose: each opt-in category overrides a different guarantee, and a
// blanket flag would let a caller who wanted one of them discard the others
// without ever reading why they were withheld.
func storeResetOptedIn(target storeResetTarget, request StoreResetRequest) bool {
	if target.name == "reviews" {
		return request.IncludeAdapterReviews
	}
	// An opt-in category nobody wired a switch for stays withheld. Failing
	// closed here means adding a category to the list can spare data by
	// accident, never destroy it by accident.
	return false
}

// storeResetMeasure sizes one target without following symlinks into it.
func storeResetMeasure(root string, target storeResetTarget) StoreResetEntry {
	path := filepath.Join(append([]string{root}, target.parts...)...)
	entry := StoreResetEntry{
		Name: target.name, Path: strings.Join(target.parts, "/"), Reason: target.reason,
	}
	info, err := os.Lstat(path)
	if err != nil {
		return entry
	}
	entry.Present = true
	if !info.IsDir() {
		entry.Files, entry.Bytes = 1, info.Size()
		return entry
	}
	entry.Files, entry.Bytes = storeResetUsage(path)
	return entry
}

// storeResetUsage sums one tree. Errors are absorbed rather than propagated:
// this is a report about a store that may well be damaged, and refusing to
// print a number because one subdirectory is unreadable would withhold the
// whole survey over a detail.
func storeResetUsage(path string) (files int, bytes int64) {
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is skipped, not fatal. The count that
			// results is a floor, which is the honest direction for a preview.
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files++
		if info.Mode().IsRegular() {
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes
}

// storeResetUnrecognized reports every entry under the store root that neither
// list covers. This is the allowlist made visible: a future directory shows up
// here instead of quietly disappearing.
func storeResetUnrecognized(root string) []StoreResetEntry {
	known := map[string]bool{}
	nested := map[string]bool{}
	for _, target := range append(append([]storeResetTarget{}, storeResetRemovableTargets...), storeResetPreservedTargets...) {
		if len(target.parts) == 1 {
			known[target.parts[0]] = true
			continue
		}
		// A target one level down means its parent is only partly claimed, so
		// the parent itself is not "known" -- its other children still have to
		// be reported.
		nested[target.parts[0]] = true
		known[strings.Join(target.parts, "/")] = true
	}
	unrecognized := []StoreResetEntry{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return unrecognized
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, storeResetStagingPrefix) {
			// Residue from an interrupted run of this same command. It is
			// reported, never silently reused.
			unrecognized = append(unrecognized, storeResetUnrecognizedEntry(root, name,
				"staging residue from an interrupted review store reset"))
			continue
		}
		if known[name] {
			continue
		}
		if nested[name] {
			children, err := os.ReadDir(filepath.Join(root, name))
			if err != nil {
				continue
			}
			for _, child := range children {
				relative := name + "/" + child.Name()
				if known[relative] {
					continue
				}
				unrecognized = append(unrecognized, storeResetUnrecognizedEntry(root, relative,
					"not covered by this command's inclusion or exclusion list"))
			}
			continue
		}
		unrecognized = append(unrecognized, storeResetUnrecognizedEntry(root, name,
			"not covered by this command's inclusion or exclusion list"))
	}
	sort.Slice(unrecognized, func(i, j int) bool { return unrecognized[i].Name < unrecognized[j].Name })
	return unrecognized
}

func storeResetUnrecognizedEntry(root, relative, reason string) StoreResetEntry {
	entry := StoreResetEntry{Name: relative, Path: relative, Reason: reason, Present: true}
	entry.Files, entry.Bytes = storeResetUsage(filepath.Join(root, filepath.FromSlash(relative)))
	return entry
}

// storeResetStateRecord is the smallest shape that answers "is this lineage
// still open". The full record parser is deliberately not used: it validates
// far more than this question needs, and a record it rejects is exactly the
// record this command has to be able to classify.
type storeResetStateRecord struct {
	State struct {
		LineageID string `json:"lineage_id"`
		State     State  `json:"state"`
	} `json:"state"`
}

// storeResetLineages partitions compact-v2 lineages into settled and in-flight.
//
// Terminal is the same set the rest of the package uses -- approved, escalated,
// invalidated (authorityStatusForState) -- and everything else is an open
// review. A record that cannot be read counts as in-flight, because a damaged
// store is precisely when that call gets made and the safe direction is to keep
// the work and make the user say the word.
//
// Legacy v1 lineages are not classified. Their state lives in an append-only
// event chain no current build can advance, so there is no open v1 review to
// protect; the directory is removed as settled residue.
//
// Nothing outside review-transactions/v2 is classified at all, and that is the
// whole reason the adapter-written reviews/ graph store is opt-in: a review
// living there is invisible here, so "no lineages in flight" is a statement
// about compact-v2 and must never be read as a statement about the store.
func storeResetLineages(root string) ([]StoreResetLineage, int, error) {
	inFlight := []StoreResetLineage{}
	settled := 0
	base := filepath.Join(root, "review-transactions", "v2")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return inFlight, 0, nil
		}
		return inFlight, 0, fmt.Errorf("read compact review authority: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		record := filepath.Join(base, entry.Name(), compactStateFileName)
		payload, err := os.ReadFile(record)
		if err != nil {
			if os.IsNotExist(err) {
				// No state record at all: crash residue, not a review.
				settled++
				continue
			}
			inFlight = append(inFlight, StoreResetLineage{LineageID: entry.Name(), Unreadable: err.Error()})
			continue
		}
		var decoded storeResetStateRecord
		if err := json.Unmarshal(payload, &decoded); err != nil {
			inFlight = append(inFlight, StoreResetLineage{LineageID: entry.Name(), Unreadable: "state record does not parse"})
			continue
		}
		state := decoded.State.State
		if state == "" {
			inFlight = append(inFlight, StoreResetLineage{LineageID: entry.Name(), Unreadable: "state record names no state"})
			continue
		}
		switch authorityStatusForState(state) {
		case AuthorityStatusApproved, AuthorityStatusEscalated, AuthorityStatusInvalidated:
			settled++
		default:
			inFlight = append(inFlight, StoreResetLineage{LineageID: entry.Name(), State: state})
		}
	}
	sort.Slice(inFlight, func(i, j int) bool { return inFlight[i].LineageID < inFlight[j].LineageID })
	return inFlight, settled, nil
}

// ResetReviewStore removes the clone's review lineage state.
//
// This is irreversible. Nothing is copied aside for later recovery: the
// package's existing quarantine (quarantineCompactStoreEntry) renames entries
// into review-transactions/quarantine, which is inside the very store being
// reset, so reusing it would free no disk and would refuse every lineage
// holding authoritative artifacts -- which is all of them. Reclaiming the space
// is the point, so the space is reclaimed and the report says so plainly.
func ResetReviewStore(ctx context.Context, repo string, request StoreResetRequest) (StoreResetReport, error) {
	// The lease is taken before the store is read, and everything that decides
	// what happens is decided under it.
	//
	// Surveying first and leasing afterwards would make the in-flight refusal
	// advisory against exactly the population the lease exists to exclude. A
	// review started, or transitioned out of a terminal state, in the window
	// between the read and the lease is invisible to a decision already made,
	// and what follows is not a per-lineage delete that would simply miss it:
	// it renames whole categories, so the new lineage is destroyed by a run
	// that reported zero reviews in flight. Every review writer takes the
	// shared side of this same lease before touching a per-lineage store
	// (acquireStoreLock), so holding it exclusively is what makes the
	// classification and the removal one atomic step against other processes.
	lockCtx, cancel := context.WithTimeout(ctx, storeResetLockTimeout)
	defer cancel()
	lock, err := storeResetAcquireLease(lockCtx, repo)
	if err != nil {
		// No report is returned with this. Nothing was read under the lease, so
		// there is no classification this command is entitled to state, and a
		// report rendered from an unleased read is the same stale picture the
		// ordering above exists to refuse.
		return StoreResetReport{}, fmt.Errorf("acquire review maintenance lease: %w", err)
	}
	defer func() { _ = lock.Release() }()

	report, root, err := surveyReviewStore(ctx, repo, request)
	if err != nil {
		return report, err
	}
	if len(report.InFlight) > 0 && !request.IncludeInFlight {
		// The operation stays the preview one. A refusal returns before the
		// staging directory is created, so nothing was opened, nothing was
		// moved, and nothing can be reconciled: labelling that run "reset"
		// tells every machine reader that a removal was attempted and every
		// category skipped, which is a different and much more alarming
		// outcome than the one that occurred.
		report.Complete = false
		return report, &StoreResetInFlightError{Repository: report.Repository, Lineages: report.InFlight}
	}
	report.Operation = StoreResetApplied

	return storeResetExecute(ctx, report, root)
}

func storeResetExecute(ctx context.Context, report StoreResetReport, root string) (StoreResetReport, error) {
	staging, err := os.MkdirTemp(root, storeResetStagingPrefix)
	if err != nil {
		report.Complete = false
		return report, fmt.Errorf("create review store reset staging directory: %w", err)
	}

	targets := map[string]storeResetTarget{}
	for _, target := range storeResetRemovableTargets {
		targets[target.name] = target
	}
	staged := int64(0)
	// stranded collects every administrative directory this run moved aside
	// for a category it then did not remove, and could not move back. Those
	// bytes are emphatically not this run's to delete: the category they
	// belong to is reported SKIPPED, meaning untouched, and deleting the
	// staging directory with them inside would make that report a lie that
	// nothing on disk can contradict afterwards.
	stranded := []storeResetStagedAdminDir{}
	for index, entry := range report.Removable {
		if err := ctx.Err(); err != nil {
			report.Complete = false
			return report, err
		}
		if !entry.Present {
			continue
		}
		target := targets[entry.Name]
		source := filepath.Join(append([]string{root}, target.parts...)...)
		var stagedAdmin []storeResetStagedAdminDir
		restoreMode := func() {}
		if target.candidateViews {
			// Candidate views are registered worktrees, so which administrative
			// directories belong to them has to be established before the move:
			// the gitdir linkage that proves ownership names each view at its
			// current path. They are moved aside rather than deleted, so this
			// preparation is undoable right up until the rename below commits.
			var err error
			stagedAdmin, err = storeResetStageCandidateViewAdminDirs(root, source, staging)
			if err != nil {
				unrestored, restoreErr := storeResetRestoreAdminDirs(stagedAdmin)
				stranded = append(stranded, unrestored...)
				report.Removable[index].Skipped = storeResetSkipReason(err, restoreErr)
				report.Complete = false
				continue
			}
			// Moving a directory to a new parent needs write permission on the
			// directory itself, and the adapter writes candidate views
			// read-only. Only this one directory is relaxed, and only until the
			// rename either commits or is undone.
			restoreMode = storeResetRelaxForRename(source)
		}
		destination := filepath.Join(staging, strings.ReplaceAll(entry.Path, "/", "-"))
		if err := storeResetRename(source, destination); err != nil {
			restoreMode()
			unrestored, restoreErr := storeResetRestoreAdminDirs(stagedAdmin)
			stranded = append(stranded, unrestored...)
			report.Removable[index].Skipped = storeResetSkipReason(err, restoreErr)
			report.Complete = false
			continue
		}
		report.Removable[index].Removed = true
		report.RemovedFiles += entry.Files
		// The administrative directories went into staging alongside the
		// category, so their bytes are freed with it. Leaving them out of the
		// accumulator is what let the residual subtraction below outweigh it
		// and print a negative size.
		staged += entry.Bytes + storeResetStagedAdminBytes(stagedAdmin)
	}
	_ = SyncReviewDirectory(root)

	report.UnrestoredAdminDirs = storeResetUnrestoredAdminDirs(stranded)

	// Bytes count as freed only once they are actually gone. If the staging
	// directory survives, the categories are out of the store but the disk is
	// not back, and saying otherwise would be the exact dishonest report this
	// command must not produce.
	if err := storeResetRemoveStaging(staging, stranded); err != nil {
		report.RemovedBytes = storeResetFreedBytes(staged, storeResetResidualBytes(staging, stranded))
		report.Residue = staging
		report.Complete = false
		return report, &StoreResetIncompleteError{Skipped: storeResetSkipped(report), Residue: staging}
	}
	report.RemovedBytes = storeResetFreedBytes(staged, 0)
	if len(stranded) > 0 {
		// The staging directory survived on purpose, holding registrations
		// this run could not put back. Naming it as residue is the only thing
		// that tells the user those bytes are still on disk and where the
		// registrations went; without it the report says a category was
		// skipped and shows nothing left over, which reads as "no harm done".
		report.Residue = staging
	}
	if !report.Complete {
		return report, &StoreResetIncompleteError{Skipped: storeResetSkipped(report), Residue: report.Residue}
	}
	return report, nil
}

// storeResetStagedAdminBytes sizes administrative directories already moved
// into staging.
func storeResetStagedAdminBytes(staged []storeResetStagedAdminDir) int64 {
	total := int64(0)
	for _, entry := range staged {
		_, bytes := storeResetUsage(entry.staged)
		total += bytes
	}
	return total
}

// storeResetUnrestoredAdminDirs projects the internal staging records into the
// report.
func storeResetUnrestoredAdminDirs(stranded []storeResetStagedAdminDir) []StoreResetUnrestoredAdminDir {
	if len(stranded) == 0 {
		return nil
	}
	entries := make([]StoreResetUnrestoredAdminDir, 0, len(stranded))
	for _, entry := range stranded {
		entries = append(entries, StoreResetUnrestoredAdminDir{Original: entry.original, Staged: entry.staged})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Original < entries[j].Original })
	return entries
}

// storeResetRemoveStaging deletes the staging directory, except for anything
// the run has to keep.
//
// With nothing to keep this is the whole directory, which is the ordinary case.
// With something to keep the directory itself has to survive to hold it, so the
// deletion becomes per-child: every category that really was removed still goes
// away, and the administrative directories that could not be restored stay put
// where the report can point at them. Deleting them here would destroy a
// worktree registration for a checkout this same run reported as untouched, and
// no later run could rebuild it.
func storeResetRemoveStaging(staging string, keep []storeResetStagedAdminDir) error {
	if len(keep) == 0 {
		return storeResetRemoveAll(staging)
	}
	kept := storeResetKeptPaths(keep)
	entries, err := os.ReadDir(staging)
	if err != nil {
		return err
	}
	failures := []error{}
	for _, entry := range entries {
		path := filepath.Join(staging, entry.Name())
		if kept[filepath.Clean(path)] {
			continue
		}
		if err := storeResetRemoveAll(path); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func storeResetKeptPaths(keep []storeResetStagedAdminDir) map[string]bool {
	kept := map[string]bool{}
	for _, entry := range keep {
		kept[filepath.Clean(entry.staged)] = true
	}
	return kept
}

// storeResetResidualBytes sizes what is left in staging that the run meant to
// delete. Anything deliberately kept is excluded: it was never counted as
// staged for removal, so subtracting it would charge the run for bytes it never
// claimed to free.
func storeResetResidualBytes(staging string, keep []storeResetStagedAdminDir) int64 {
	kept := storeResetKeptPaths(keep)
	entries, err := os.ReadDir(staging)
	if err != nil {
		_, bytes := storeResetUsage(staging)
		return bytes
	}
	total := int64(0)
	for _, entry := range entries {
		path := filepath.Join(staging, entry.Name())
		if kept[filepath.Clean(path)] {
			continue
		}
		_, bytes := storeResetUsage(path)
		total += bytes
	}
	return total
}

// storeResetFreedBytes reports what a run actually reclaimed, and never reports
// less than nothing. The two counts are measured at different moments against a
// tree other processes can touch, so their difference can invert; a negative
// size printed to a user is a bug report about the accounting, not information
// about their disk.
func storeResetFreedBytes(staged, residual int64) int64 {
	if freed := staged - residual; freed > 0 {
		return freed
	}
	return 0
}

func storeResetSkipped(report StoreResetReport) []StoreResetEntry {
	skipped := []StoreResetEntry{}
	for _, entry := range report.Removable {
		if entry.Skipped != "" {
			skipped = append(skipped, entry)
		}
	}
	return skipped
}

// storeResetStagedAdminDir records one Git worktree administrative directory
// moved out of the way, and where it came from.
type storeResetStagedAdminDir struct{ original, staged string }

// storeResetStageCandidateViewAdminDirs moves aside the Git worktree
// administrative directories that point into a candidate-views tree.
//
// It moves rather than deletes because the category is not actually removed
// until the rename that follows succeeds, and until then the store has to be
// exactly as it was found. Deleting first is unrepairable: a rename that then
// fails leaves the checkouts on disk with their registrations gone, so Git
// commands inside them fail, the repository's worktree list no longer names
// them, and a later review cannot register the same view again. A rename into
// the same staging directory the categories go to costs nothing, is undone by
// renaming back, and is deleted along with everything else once the category is
// really gone.
//
// An admin directory is moved only when its own gitdir file proves it belongs
// to this exact candidate view. That check is what keeps the operation from
// touching a worktree the user created: a global `git worktree prune` would
// also clear unrelated stale entries, and this command has no business
// deciding that.
func storeResetStageCandidateViewAdminDirs(root, views, staging string) ([]storeResetStagedAdminDir, error) {
	commonDir := filepath.Dir(root)
	entries, err := os.ReadDir(views)
	if err != nil {
		return nil, err
	}
	staged := []storeResetStagedAdminDir{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		view := filepath.Join(views, entry.Name())
		admin := filepath.Join(commonDir, "worktrees", entry.Name())
		if !storeResetAdminDirBelongsTo(admin, view) {
			continue
		}
		destination := filepath.Join(staging, "worktrees-"+entry.Name())
		if err := storeResetRename(admin, destination); err != nil {
			return staged, fmt.Errorf("stage worktree administrative directory for candidate view %q: %w", entry.Name(), err)
		}
		staged = append(staged, storeResetStagedAdminDir{original: admin, staged: destination})
	}
	return staged, nil
}

// storeResetRestoreAdminDirs puts back every administrative directory staged
// for a category that was then not removed, and names the ones it could not.
//
// The names matter as much as the error. A directory left in staging is still a
// worktree registration, and the run is about to delete staging; returning only
// an error would fold that fact into a sentence in a skip reason and let the
// deletion take it. Returning the entries lets the caller keep them.
//
// The second rename is not a formality that always works. It fails when
// something recreated the path in the window, on a full or failing disk, and on
// Windows whenever any process holds a handle inside the directory -- which is
// also when the first rename is most likely to have failed, so the restore path
// is at its least reliable exactly when it is reached.
func storeResetRestoreAdminDirs(staged []storeResetStagedAdminDir) ([]storeResetStagedAdminDir, error) {
	unrestored := []storeResetStagedAdminDir{}
	failures := []error{}
	for _, entry := range staged {
		if err := storeResetRename(entry.staged, entry.original); err != nil {
			unrestored = append(unrestored, entry)
			failures = append(failures, err)
		}
	}
	return unrestored, errors.Join(failures...)
}

// storeResetSkipReason names why a category was left alone, and says so louder
// when undoing the preparation for it did not fully work: a restore that failed
// is the one case where the store is not exactly as this run found it.
func storeResetSkipReason(cause, restore error) string {
	if restore == nil {
		return cause.Error()
	}
	return fmt.Sprintf(
		"%v; and the worktree administrative directories moved aside for it could not be restored: %v",
		cause, restore,
	)
}

// storeResetRelaxForRename grants write permission on one directory so it can
// be moved to a new parent, and returns the undo. It changes nothing when the
// directory is already writable, missing, or not a directory.
func storeResetRelaxForRename(path string) func() {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return func() {}
	}
	mode := info.Mode().Perm()
	if mode&0o200 != 0 {
		return func() {}
	}
	if err := os.Chmod(path, mode|0o700); err != nil {
		return func() {}
	}
	return func() { _ = os.Chmod(path, mode) }
}

// storeResetAdminDirBelongsTo reports whether a worktree admin directory points
// at exactly this candidate view.
func storeResetAdminDirBelongsTo(admin, view string) bool {
	payload, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	if err != nil {
		return false
	}
	recorded := filepath.Clean(strings.TrimSpace(string(payload)))
	return recorded == filepath.Clean(filepath.Join(view, ".git"))
}

// storeResetMakeWritable relaxes directory permissions across a tree so its
// contents can be unlinked. The adapter writes candidate views at 0o555, which
// forbids removing anything inside them.
func storeResetMakeWritable(path string) error {
	return filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o200 != 0 {
			return nil
		}
		return os.Chmod(current, info.Mode().Perm()|0o700)
	})
}

// storeResetRemoveAll deletes a tree, relaxing permissions once if the first
// attempt is refused. Retrying exactly once keeps a genuine fault (a busy file,
// a full disk, a vanished mount) from turning into a loop.
func storeResetRemoveAll(path string) error {
	err := storeResetRemoveTree(path)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return err
	}
	if writableErr := storeResetMakeWritable(path); writableErr != nil {
		return err
	}
	return storeResetRemoveTree(path)
}
