package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const (
	AuthorityRepairAssessmentSchema    = "gentle-ai.review-authority-repair-assessment/v1"
	AuthorityRepairAssessmentSchemaID  = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/authority-repair-assessment.schema.json"
	AuthorityRepairAuthorizationSchema = "gentle-ai.review-repair-authorization/v1"

	AuthorityRepairEligible          AuthorityRepairStatus = "eligible"
	AuthorityRepairUnsupported       AuthorityRepairStatus = "unsupported"
	AuthorityRepairAmbiguous         AuthorityRepairStatus = "ambiguous"
	AuthorityRepairConflicting       AuthorityRepairStatus = "conflicting"
	AuthorityRepairTruncated         AuthorityRepairStatus = "truncated"
	authorityRepairMaxLineages                             = 256
	authorityRepairMaxEvents                               = 1024
	authorityRepairMaxEventBytes                           = 1 << 20
	authorityRepairMaxTotalBytes                           = 8 << 20
	authorityRepairMaxOperations                           = 1024
	authorityRepairMaxProofReads                           = 4*authorityRepairMaxEvents + 4*authorityRepairMaxLineages
	authorityRepairMaxProofReadBytes                       = 3 * authorityRepairMaxTotalBytes
)

var authorityRepairSupportedOperations = []string{"review/complete-fix", "review/validate-fix"}

type AuthorityRepairStatus string
type AuthorityRepairClass string
type AuthorityRepairCause string
type AuthorityRepairDisposition string

type AuthorityRepairAssessment struct {
	Schema              string                     `json:"schema"`
	Status              AuthorityRepairStatus      `json:"status"`
	Class               AuthorityRepairClass       `json:"class,omitempty"`
	Cause               AuthorityRepairCause       `json:"cause,omitempty"`
	Disposition         AuthorityRepairDisposition `json:"disposition,omitempty"`
	RepositoryBinding   string                     `json:"repository_binding,omitempty"`
	Candidate           *AuthorityRepairCandidate  `json:"candidate,omitempty"`
	Counts              AuthorityRepairCounts      `json:"counts"`
	SupportedOperations []string                   `json:"supported_operations"`
	AuthorizationSchema string                     `json:"authorization_schema"`
	// Truncated, TruncationCap and TruncationScanned are set only when Status
	// is AuthorityRepairTruncated (#3371). The bounded scan can hit its
	// lineage cap before classifying a single entry -- e.g. a v2/ directory
	// with more entries than the cap reads none of them -- which otherwise
	// left every Counts field at zero, indistinguishable from a genuinely
	// healthy store. These fields make the truncation itself, and how far it
	// got, explicit rather than inferred from an all-zero Counts.
	Truncated         bool `json:"truncated,omitempty"`
	TruncationCap     int  `json:"truncation_cap,omitempty"`
	TruncationScanned int  `json:"truncation_scanned,omitempty"`
}

type AuthorityRepairCandidate struct {
	LineageID       string   `json:"lineage_id"`
	Revision        string   `json:"revision"`
	ChainIdentity   string   `json:"chain_identity"`
	EventCount      int      `json:"event_count"`
	AliasEventCount int      `json:"alias_event_count"`
	Operations      []string `json:"operations"`
}

type AuthorityRepairCounts struct {
	Lineages            int `json:"lineages"`
	CompactLineages     int `json:"compact_lineages"`
	LegacyLineages      int `json:"legacy_lineages"`
	Events              int `json:"events"`
	Bytes               int `json:"bytes"`
	EligibleCandidates  int `json:"eligible_candidates"`
	UnsupportedLineages int `json:"unsupported_lineages"`
	Conflicts           int `json:"conflicts"`
}

type ClassifiedAuthorityRepairRequest struct {
	Class                   AuthorityRepairClass
	LineageID               string
	ExpectedRevision        string
	Cause                   AuthorityRepairCause
	Disposition             AuthorityRepairDisposition
	RepositoryBinding       string
	Actor                   string
	Reason                  string
	MaintainerAuthorization string
}

type authorityRepairBudget struct {
	ctx         context.Context
	counts      *AuthorityRepairCounts
	entries     int
	proofReads  int
	proofBytes  int
	uniqueFiles map[string]struct{}
	truncated   bool
}

type authorityRepairScan struct {
	assessment        AuthorityRepairAssessment
	base              string
	binding           string
	maintenanceOwned  bool
	unsupported       int
	conflicts         int
	truncated         bool
	truncationScanned int
	compact           map[string]CompactRecord
}

// noteTruncationScanned records how many entries the scan had actually
// looked at when a bound tripped, keeping the largest value seen across
// whichever bound fired (#3371): a directory-entry-count trip reports it
// immediately from the raw listing, before a single lineage is classified.
func (scan *authorityRepairScan) noteTruncationScanned(scanned int) {
	if scanned > scan.truncationScanned {
		scan.truncationScanned = scanned
	}
}

var errAuthorityRepairTruncated = errors.New("authority repair assessment limit exceeded")
var errAuthorityRepairConcurrent = errors.New("authority repair assessment changed during proof")

// AssessAuthorityRepair inspects the complete authority inventory without
// creating files or mutating authority bytes. It exposes only a bounded,
// path-free classification.
func AssessAuthorityRepair(ctx context.Context, repo string) (AuthorityRepairAssessment, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return AuthorityRepairAssessment{}, err
	}
	return AssessAuthorityRepairAtRepositoryRoot(ctx, root)
}

// AssessAuthorityRepairAtRepositoryRoot is the STATUS/preflight/execution
// shared classifier when the canonical repository root is already resolved.
func AssessAuthorityRepairAtRepositoryRoot(ctx context.Context, root string) (AuthorityRepairAssessment, error) {
	return assessAuthorityRepairAtRepositoryRoot(ctx, root, false)
}

func assessAuthorityRepairAtRepositoryRoot(ctx context.Context, root string, maintenanceOwned bool) (AuthorityRepairAssessment, error) {
	base, binding, err := authorityRepairRoot(root)
	if err != nil {
		return AuthorityRepairAssessment{}, err
	}
	scan := authorityRepairScan{
		assessment: AuthorityRepairAssessment{
			Schema: AuthorityRepairAssessmentSchema, Status: AuthorityRepairUnsupported,
			Counts: AuthorityRepairCounts{}, SupportedOperations: append([]string(nil), authorityRepairSupportedOperations...),
			AuthorizationSchema: AuthorityRepairAuthorizationSchema,
		},
		base: base, binding: binding, maintenanceOwned: maintenanceOwned,
		compact: map[string]CompactRecord{},
	}
	if err := scan.run(ctx); err != nil {
		return AuthorityRepairAssessment{}, err
	}
	scan.finish()
	if err := scan.assessment.Validate(); err != nil {
		return AuthorityRepairAssessment{}, err
	}
	return scan.assessment, nil
}

func UnsupportedAuthorityRepairAssessment() AuthorityRepairAssessment {
	return AuthorityRepairAssessment{
		Schema: AuthorityRepairAssessmentSchema, Status: AuthorityRepairUnsupported,
		Counts: AuthorityRepairCounts{}, SupportedOperations: append([]string(nil), authorityRepairSupportedOperations...),
		AuthorizationSchema: AuthorityRepairAuthorizationSchema,
	}
}

func (scan *authorityRepairScan) run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(scan.base); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	budget := &authorityRepairBudget{
		ctx: ctx, counts: &scan.assessment.Counts,
		uniqueFiles: map[string]struct{}{},
	}
	if !scan.maintenanceOwned {
		if status := probeAuthorityRepairLock(compactMaintenanceLockPath(scan.base)); status != authorityRepairLockReleased {
			if status != authorityRepairLockAbsent {
				scan.conflicts++
			}
		}
	}
	if _, err := os.Lstat(compactBatchReconcileMarkerPath(scan.base)); err == nil {
		scan.conflicts++
	} else if !os.IsNotExist(err) {
		scan.conflicts++
	}
	if err := scan.scanCompact(ctx, budget); errors.Is(err, errAuthorityRepairTruncated) {
		scan.truncated = true
	} else if err != nil {
		return err
	}
	// #3371: a bound tripped inside the shared per-entry loop (rather than at
	// the raw directory listing) is reflected only in budget.entries; fold it
	// in here so every truncation path reports how far the scan actually got.
	scan.noteTruncationScanned(budget.entries)
	return nil
}

func (scan *authorityRepairScan) scanCompact(ctx context.Context, budget *authorityRepairBudget) error {
	root := filepath.Join(scan.base, "v2")
	entries, err := readAuthorityRepairDirectory(root, authorityRepairMaxLineages)
	if err != nil {
		if errors.Is(err, errAuthorityRepairTruncated) {
			scan.noteTruncationScanned(len(entries))
			return err
		}
		if os.IsNotExist(err) {
			return nil
		}
		scan.unsupported++
		return nil
	}
	if status := probeAuthorityRepairLock(filepath.Join(root, "LOCK")); status != authorityRepairLockReleased && status != authorityRepairLockAbsent {
		scan.conflicts++
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == "LOCK" && !entry.IsDir() {
			continue
		}
		if err := budget.addEntry(); err != nil {
			return err
		}
		if !entry.IsDir() || validateLineageID(entry.Name()) != nil {
			scan.unsupported++
			continue
		}
		budget.addLineage()
		lineage := entry.Name()
		dir := filepath.Join(root, lineage)
		residue, residueErr := authorityRepairHasAtomicResidue(dir)
		if errors.Is(residueErr, errAuthorityRepairTruncated) {
			return residueErr
		}
		if residueErr != nil || residue {
			scan.unsupported++
			continue
		}
		payload, readErr := budget.read(filepath.Join(dir, compactStateFileName), authorityRepairMaxEventBytes)
		if readErr != nil {
			if errors.Is(readErr, errAuthorityRepairTruncated) {
				return readErr
			}
			scan.unsupported++
			continue
		}
		record, parseErr := parseCompactRecord(payload, lineage)
		if parseErr != nil {
			scan.unsupported++
			continue
		}
		scan.compact[lineage] = record
	}
	// One count per lineage that actually carries a graph defect. Collapsing
	// the whole graph into a single increment reported `unsupported_lineages:
	// 1` while two independent edges were invalid, which understated the work
	// an operator still had to do and made the assessment disagree with
	// `review inspect-authority`.
	violations, _ := compactAuthorityGraphViolations(scan.compact)
	scan.unsupported += len(violations)
	return nil
}

func authorityRepairHasAtomicResidue(dir string) (bool, error) {
	entries, err := readAuthorityRepairDirectory(dir, authorityRepairMaxOperations)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".atomic-") {
			return true, nil
		}
	}
	return false, nil
}

func readAuthorityRepairDirectory(dir string, limit int) ([]os.DirEntry, error) {
	file, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	entries, err := file.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		// #3371: entries travels with the error (rather than nil) so a
		// truncation-counting caller can report exactly how many raw
		// directory entries it saw before giving up, instead of a bare
		// refusal that looks identical to a store with nothing to report.
		return entries, errAuthorityRepairTruncated
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (scan *authorityRepairScan) finish() {
	scan.assessment.Counts.UnsupportedLineages = scan.unsupported
	scan.assessment.Counts.Conflicts = scan.conflicts
	switch {
	case scan.truncated:
		scan.assessment.Status = AuthorityRepairTruncated
		scan.assessment.Truncated = true
		scan.assessment.TruncationCap = authorityRepairMaxLineages
		scan.assessment.TruncationScanned = scan.truncationScanned
	case scan.conflicts > 0:
		scan.assessment.Status = AuthorityRepairConflicting
	default:
		scan.assessment.Status = AuthorityRepairUnsupported
	}
}

func (budget *authorityRepairBudget) addEntry() error {
	budget.entries++
	if budget.entries > authorityRepairMaxLineages {
		budget.truncated = true
		return errAuthorityRepairTruncated
	}
	return nil
}

func (budget *authorityRepairBudget) addLineage() {
	budget.counts.Lineages++
	budget.counts.CompactLineages++
}

func (budget *authorityRepairBudget) read(path string, limit int64) ([]byte, error) {
	if err := budget.ctx.Err(); err != nil {
		return nil, err
	}
	budget.proofReads++
	if budget.proofReads > authorityRepairMaxProofReads {
		budget.truncated = true
		return nil, errAuthorityRepairTruncated
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	_, uniqueFile := budget.uniqueFiles[path]
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit ||
		budget.proofBytes+int(info.Size()) > authorityRepairMaxProofReadBytes ||
		!uniqueFile && int64(budget.counts.Bytes)+info.Size() > authorityRepairMaxTotalBytes {
		budget.truncated = true
		return nil, errAuthorityRepairTruncated
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) != info.Size() {
		return nil, errAuthorityRepairConcurrent
	}
	budget.proofBytes += len(payload)
	if !uniqueFile {
		budget.uniqueFiles[path] = struct{}{}
		budget.counts.Bytes += len(payload)
	}
	return payload, nil
}

type authorityRepairLockStatus uint8

const (
	authorityRepairLockAbsent authorityRepairLockStatus = iota
	authorityRepairLockReleased
	authorityRepairLockHeld
	authorityRepairLockUnknown
)

func probeAuthorityRepairLock(path string) authorityRepairLockStatus {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return authorityRepairLockAbsent
		}
		return authorityRepairLockUnknown
	}
	defer file.Close()
	locked, err := tryLockFile(file)
	if err != nil {
		return authorityRepairLockUnknown
	}
	if !locked {
		return authorityRepairLockHeld
	}
	if err := unlockFile(file); err != nil {
		return authorityRepairLockUnknown
	}
	return authorityRepairLockReleased
}

func authorityRepairRoot(root string) (string, string, error) {
	repository, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", "", err
	}
	dotGit := filepath.Join(repository, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", "", err
	}
	common := dotGit
	if !info.IsDir() {
		payload, err := os.ReadFile(dotGit)
		if err != nil || len(payload) > 4096 || bytes.Contains(payload, []byte{'\r'}) {
			return "", "", errors.New("repository worktree metadata is invalid")
		}
		line := strings.TrimSuffix(string(payload), "\n")
		if !strings.HasPrefix(line, "gitdir: ") || strings.Contains(line, "\n") {
			return "", "", errors.New("repository worktree metadata is invalid")
		}
		gitDir := strings.TrimPrefix(line, "gitdir: ")
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(repository, gitDir)
		}
		gitDir, err = filepath.EvalSymlinks(gitDir)
		if err != nil {
			return "", "", err
		}
		common = gitDir
		if payload, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
			if len(payload) > 4096 || bytes.Contains(payload, []byte{'\r'}) {
				return "", "", errors.New("repository common-directory metadata is invalid")
			}
			value := strings.TrimSuffix(string(payload), "\n")
			if value == "" || strings.Contains(value, "\n") {
				return "", "", errors.New("repository common-directory metadata is invalid")
			}
			common = value
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			common, err = filepath.EvalSymlinks(common)
			if err != nil {
				return "", "", err
			}
		} else if !os.IsNotExist(err) {
			return "", "", err
		}
	}
	common, err = filepath.Abs(common)
	if err != nil {
		return "", "", err
	}
	common = filepath.Clean(common)
	info, err = os.Stat(common)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("repository common directory is invalid")
	}
	sum := sha256.Sum256([]byte("gentle-ai.review-repository-binding/v1\n" + common))
	binding := "sha256:" + hex.EncodeToString(sum[:])
	return filepath.Join(common, "gentle-ai", "review-transactions"), binding, nil
}

func (assessment AuthorityRepairAssessment) Validate() error {
	if assessment.Schema != AuthorityRepairAssessmentSchema || assessment.AuthorizationSchema != AuthorityRepairAuthorizationSchema ||
		!reflect.DeepEqual(assessment.SupportedOperations, authorityRepairSupportedOperations) {
		return errors.New("authority repair assessment identity is invalid")
	}
	counts := assessment.Counts
	for _, value := range []int{counts.Lineages, counts.CompactLineages, counts.LegacyLineages, counts.Events, counts.Bytes, counts.EligibleCandidates, counts.UnsupportedLineages, counts.Conflicts} {
		if value < 0 {
			return errors.New("authority repair assessment count is invalid")
		}
	}
	if counts.Lineages > authorityRepairMaxLineages || counts.Events > authorityRepairMaxEvents || counts.Bytes > authorityRepairMaxTotalBytes ||
		counts.UnsupportedLineages > authorityRepairMaxOperations || counts.Conflicts > authorityRepairMaxOperations ||
		counts.CompactLineages+counts.LegacyLineages != counts.Lineages || counts.EligibleCandidates > counts.LegacyLineages {
		return errors.New("authority repair assessment count exceeds its bound")
	}
	if assessment.Status != AuthorityRepairUnsupported && assessment.Status != AuthorityRepairAmbiguous && assessment.Status != AuthorityRepairConflicting && assessment.Status != AuthorityRepairTruncated {
		return errors.New("authority repair assessment status is invalid")
	}
	if assessment.Class != "" || assessment.Cause != "" || assessment.Disposition != "" || assessment.RepositoryBinding != "" || assessment.Candidate != nil {
		return errors.New("stopped authority repair assessment contains an executable candidate")
	}
	// #3371: a truncated assessment must say so and how far it got, rather
	// than leaving every Counts field at zero and indistinguishable from a
	// genuinely healthy store; every other status must carry none of this.
	if assessment.Status == AuthorityRepairTruncated {
		if !assessment.Truncated || assessment.TruncationCap != authorityRepairMaxLineages || assessment.TruncationScanned < 1 {
			// refusal:by-design world-action: this assessment is built by scan.finish() from its own truncation bookkeeping a few lines above; reaching this means a product defect in that producer, not an operator input to fix
			return errors.New("truncated authority repair assessment is missing its scan bound")
		}
	} else if assessment.Truncated || assessment.TruncationCap != 0 || assessment.TruncationScanned != 0 {
		// refusal:by-design world-action: same producer-only invariant as above, for the inverse direction -- these fields are set only inside the scan.truncated branch of finish()
		return errors.New("authority repair assessment truncation fields require a truncated status")
	}
	return nil
}

func authorityRepairAuthorizationBinding(request ClassifiedAuthorityRepairRequest) string {
	return AuthorityRepairAuthorizationSchema +
		"\nrepository=" + request.RepositoryBinding +
		"\nclass=" + string(request.Class) +
		"\nlineage=" + request.LineageID +
		"\nrevision=" + request.ExpectedRevision +
		"\ncause=" + string(request.Cause) +
		"\ndisposition=" + string(request.Disposition) +
		"\nactor=" + request.Actor +
		"\nreason=" + request.Reason
}

func validateClassifiedAuthorityRepairRequest(request ClassifiedAuthorityRepairRequest, assessment AuthorityRepairAssessment) error {
	if assessment.Status != AuthorityRepairEligible || assessment.Candidate == nil {
		return errors.New("classified authority repair is not eligible")
	}
	if request.Class != assessment.Class || request.LineageID != assessment.Candidate.LineageID || request.ExpectedRevision != assessment.Candidate.Revision ||
		request.Cause != assessment.Cause || request.Disposition != assessment.Disposition || request.RepositoryBinding != assessment.RepositoryBinding {
		return errors.New("classified authority repair inputs do not match the current assessment")
	}
	if strings.TrimSpace(request.Actor) != request.Actor || strings.TrimSpace(request.Reason) != request.Reason ||
		request.Actor == "" || request.Reason == "" || len(request.Actor) > 256 || len(request.Reason) > 512 ||
		strings.ContainsAny(request.Actor, "\r\n") || strings.ContainsAny(request.Reason, "\r\n") || strings.Contains(request.MaintainerAuthorization, "\r") ||
		request.MaintainerAuthorization != authorityRepairAuthorizationBinding(request) {
		return errors.New("classified authority repair authorization is not exact")
	}
	return nil
}

func ValidateClassifiedAuthorityRepairRequest(request ClassifiedAuthorityRepairRequest, assessment AuthorityRepairAssessment) error {
	return validateClassifiedAuthorityRepairRequest(request, assessment)
}

// repairAuthorityDispositionAtRepo is the production seam that ties
// disposition plan derivation (authority_disposition_plan.go) to closure
// executor admission and mutation (authority_disposition_execute.go) behind
// the shape `review repair` CLI wiring (Slice S3, extended by Wave 6 Slice
// S3's forward-only resume) calls.
//
// Wave 6: a fresh re-derivation is not safe to use unconditionally.
// executeAuthorityDisposition's own internal resume logic
// (authorityDispositionClosureIsFresh) already knows to skip re-derivation
// entirely once ANY closure member has prior progress — but this public
// entrypoint has to decide WHICH plan to hand it in the first place, and a
// blind re-derivation from actor/reason on every call would silently
// produce a smaller, mismatched closure the moment even one member is
// already quarantined (a missing member contributes no report edge), which
// is exactly the narrowing re-derivation the resume design forbids. So this
// checks first whether planDigest already names an in-progress closure —
// reconstructing the original plan from an already-committed member's own
// AuthorityDisposition proof, never re-deriving it — and only falls back to
// a fresh derivation (compared against the caller-supplied planDigest/
// inventoryRevision) when no such record exists.
func repairAuthorityDispositionAtRepo(ctx context.Context, repo, planDigest, inventoryRevision, actor, reason, authorization string, selector *AuthorityDispositionSelector) (CompactReclaimRecord, error) {
	if reconstructed, found, err := reconstructAuthorityDispositionPlanForResume(ctx, repo, planDigest, actor, reason); err != nil {
		return CompactReclaimRecord{}, err
	} else if found {
		if selector != nil && (reconstructed.Selector == nil || *reconstructed.Selector != *selector) {
			return CompactReclaimRecord{}, fmt.Errorf("%w: submitted exact selector does not match the in-progress plan", ErrConcurrentUpdate)
		}
		reconstructed.Authorization = authorization
		return executeAuthorityDisposition(ctx, repo, reconstructed)
	}
	requested := []AuthorityDispositionSelector{}
	if selector != nil {
		requested = append(requested, *selector)
	}
	plan, err := deriveAuthorityDispositionPlanAtRepo(ctx, repo, actor, reason, requested...)
	if err != nil {
		return CompactReclaimRecord{}, err
	}
	if err := admitClosureDisposition(plan); err != nil {
		return CompactReclaimRecord{}, err
	}
	if plan.PlanDigest != planDigest || plan.AuthorityInventoryRevision != inventoryRevision {
		// This wraps ErrConcurrentUpdate (%w) — exempt plumbing propagating an
		// existing typed error, not a site that needs its own by-design
		// annotation (the underlying condition is world-action: the graph
		// changed between --preflight and this call, or the values were
		// copied from a stale preflight; the fix is running
		// `review repair --preflight` again).
		//
		// Fix cycle 2 (WARNING-4, sdd-verify cycle-2): base bb3c22a9's own
		// version of this exact refusal ended with a runnable continuation
		// ("run `gentle-ai review repair --preflight` again for the current
		// values"); Wave 6 Slice S3 replaced the CLI-level pre-check this
		// refusal now lives in without carrying that continuation text
		// forward, so cycle-1's CRITICAL-2 fix (which restored the CAUSE
		// reaching the operator) still left it silent about what to do next.
		return CompactReclaimRecord{}, fmt.Errorf("%w: submitted plan_digest/inventory_revision does not match the current provider-derived plan; run `gentle-ai review repair --preflight` again for the current values", ErrConcurrentUpdate)
	}
	plan.Authorization = authorization
	return executeAuthorityDisposition(ctx, repo, plan)
}

// reconstructAuthorityDispositionPlanForResume scans the quarantine root for
// any existing record whose AuthorityDisposition.PlanDigest matches
// planDigest — evidence this exact plan already began — and, if found,
// reconstructs the full AuthorityDispositionPlan from that record's own
// proof (Schema, AnomalyClass, SeedSet, Closure, ExpectedRevisions,
// AuthorityInventoryRevision, PlanDigest all come from the proof; only
// RepositoryBinding is freshly resolved, and Actor/Reason are the CURRENT
// call's, not the original attempt's — plan_digest's pre-image excludes
// both, so this changes nothing about plan identity). It is the resume half
// of repairAuthorityDispositionAtRepo's fresh-vs-resume decision, mirroring
// authorityDispositionClosureIsFresh's identical principle one layer up:
// never attempt a narrowing re-derivation once real progress exists.
func reconstructAuthorityDispositionPlanForResume(ctx context.Context, repo, planDigest, actor, reason string) (AuthorityDispositionPlan, bool, error) {
	root, err := (SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return AuthorityDispositionPlan{}, false, err
	}
	base, binding, err := authorityRepairRoot(root)
	if err != nil {
		return AuthorityDispositionPlan{}, false, err
	}
	quarantineRoot := filepath.Join(base, "quarantine")
	entries, err := os.ReadDir(quarantineRoot)
	if os.IsNotExist(err) {
		return AuthorityDispositionPlan{}, false, nil
	}
	if err != nil {
		return AuthorityDispositionPlan{}, false, fmt.Errorf("inspect authority disposition quarantine for resume: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return AuthorityDispositionPlan{}, false, err
		}
		if !entry.IsDir() {
			continue
		}
		payload, readErr := os.ReadFile(filepath.Join(quarantineRoot, entry.Name(), "reclaim-record.json"))
		if readErr != nil {
			continue
		}
		var record CompactReclaimRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			continue
		}
		proof := record.AuthorityDisposition
		if proof == nil || proof.PlanDigest != planDigest {
			continue
		}
		return AuthorityDispositionPlan{
			Schema: AuthorityDispositionPlanSchema, RepositoryBinding: binding,
			AuthorityInventoryRevision: proof.AuthorityInventoryRevision, AnomalyClass: proof.AnomalyClass,
			Selector: proof.Selector,
			SeedSet:  append([]string(nil), proof.SeedSet...), Closure: append([]string(nil), proof.Closure...),
			ExpectedRevisions: cloneAuthorityDispositionRevisions(proof.ExpectedRevisions),
			Actor:             strings.TrimSpace(actor), Reason: strings.TrimSpace(reason),
			PlanDigest: proof.PlanDigest,
		}, true, nil
	}
	return AuthorityDispositionPlan{}, false, nil
}

// RepairAuthorityDisposition is the exported form of
// repairAuthorityDispositionAtRepo — the one public entrypoint Slice S3's
// `review repair` CLI wiring calls to execute a closure authority
// disposition plan (rdd-authority-disposition-plan / "No New Public Repair
// Verb": an exported Go seam behind the existing verb, not a new CLI
// command). Wave 6: planDigest and inventoryRevision are now required
// parameters — repairAuthorityDispositionAtRepo needs them to decide
// fresh-vs-resume; it no longer blindly re-derives from actor/reason alone.
func RepairAuthorityDisposition(ctx context.Context, repo, planDigest, inventoryRevision, actor, reason, authorization string, selectors ...AuthorityDispositionSelector) (CompactReclaimRecord, error) {
	if len(selectors) > 1 {
		return CompactReclaimRecord{}, fmt.Errorf("%w: multiple exact selectors supplied", errAuthorityDispositionPlanNotDerivable)
	}
	if len(selectors) == 1 {
		return repairAuthorityDispositionAtRepo(ctx, repo, planDigest, inventoryRevision, actor, reason, authorization, &selectors[0])
	}
	return repairAuthorityDispositionAtRepo(ctx, repo, planDigest, inventoryRevision, actor, reason, authorization, nil)
}
