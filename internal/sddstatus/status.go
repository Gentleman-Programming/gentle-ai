package sddstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const SchemaName = "gentle-ai.sdd-status"
const SchemaVersion = 2

type ArtifactStore string

const (
	ArtifactStoreOpenSpec ArtifactStore = "openspec"
	ArtifactStoreEngram   ArtifactStore = "engram"
	// ArtifactStoreHybrid used to exist only in prompt prose. #3636 reported
	// the consequence: a workspace declaring it was reported as openspec, and
	// its file writes were invisible to a status that read it as pure Engram.
	ArtifactStoreHybrid ArtifactStore = "hybrid"
	ArtifactStoreNone   ArtifactStore = "none"
)

type ArtifactState string

const (
	ArtifactMissing ArtifactState = "missing"
	ArtifactPartial ArtifactState = "partial"
	ArtifactDone    ArtifactState = "done"
)

type DependencyState string

const (
	DependencyBlocked DependencyState = "blocked"
	DependencyReady   DependencyState = "ready"
	DependencyAllDone DependencyState = "all_done"
)

type ApplyState string

const (
	ApplyBlocked ApplyState = "blocked"
	ApplyReady   ApplyState = "ready"
	ApplyAllDone ApplyState = "all_done"
)

type ActionMode string

const (
	ActionModeRepoLocal ActionMode = "repo-local"
)

type Phase string

const (
	PhasePropose   Phase = "propose"
	PhaseSpec      Phase = "spec"
	PhaseDesign    Phase = "design"
	PhaseTasks     Phase = "tasks"
	PhaseApply     Phase = "apply"
	PhaseVerify    Phase = "verify"
	PhaseRemediate Phase = "remediate"
	PhaseArchive   Phase = "archive"
)

type ArtifactPaths struct {
	Proposal      []string `json:"proposal"`
	Specs         []string `json:"specs"`
	Design        []string `json:"design"`
	Tasks         []string `json:"tasks"`
	ApplyProgress []string `json:"applyProgress"`
	VerifyReport  []string `json:"verifyReport"`
}

type PlanningHome struct {
	Mode ActionMode `json:"mode"`
	Path string     `json:"path"`
}

type TaskProgress struct {
	Total       int  `json:"total"`
	Completed   int  `json:"completed"`
	Pending     int  `json:"pending"`
	AllComplete bool `json:"allComplete"`
}

type blockerReasons struct {
	expectedPlanning []string
	genuine          []string
}

func (reasons blockerReasons) forRoute(nextRecommended string) []string {
	return reasons.finalize(nextRecommended, reasons.genuine)
}

func (reasons blockerReasons) finalize(nextRecommended string, accumulated []string) []string {
	switch Phase(nextRecommended) {
	case PhasePropose, PhaseSpec, PhaseDesign, PhaseTasks:
		return append([]string{}, accumulated...)
	default:
		return append(append([]string{}, reasons.expectedPlanning...), accumulated...)
	}
}

type Dependencies struct {
	Proposal DependencyState `json:"proposal"`
	Specs    DependencyState `json:"specs"`
	Design   DependencyState `json:"design"`
	Tasks    DependencyState `json:"tasks"`
	Apply    DependencyState `json:"apply"`
	Verify   DependencyState `json:"verify"`
	Archive  DependencyState `json:"archive"`
}

type ActionContext struct {
	Mode             ActionMode `json:"mode"`
	WorkspaceRoot    string     `json:"workspaceRoot"`
	AllowedEditRoots []string   `json:"allowedEditRoots"`
}

type Relationships struct {
	DependsOn               []string `json:"dependsOn"`
	Supersedes              []string `json:"supersedes"`
	Amends                  []string `json:"amends"`
	ConflictsWith           []string `json:"conflictsWith"`
	SameDomainActiveChanges []string `json:"sameDomainActiveChanges"`
}

type PhaseInstructions struct {
	Apply     []string `json:"apply"`
	Verify    []string `json:"verify"`
	Remediate []string `json:"remediate"`
	Archive   []string `json:"archive"`
}

// RemediationState describes only failed independent SDD verification
// evidence. It carries no review authority, lifecycle, or budget vocabulary.
type RemediationState struct {
	Required               bool   `json:"required"`
	Complete               bool   `json:"complete"`
	FailedEvidenceRevision string `json:"failedEvidenceRevision"`
	Reason                 string `json:"reason"`
}

type Status struct {
	SchemaName       string                   `json:"schemaName"`
	SchemaVersion    int                      `json:"schemaVersion"`
	ChangeName       *string                  `json:"changeName"`
	ArtifactStore    ArtifactStore            `json:"artifactStore"`
	PlanningHome     PlanningHome             `json:"planningHome"`
	ChangeRoot       *string                  `json:"changeRoot"`
	ArtifactPaths    ArtifactPaths            `json:"artifactPaths"`
	ContextFiles     ArtifactPaths            `json:"contextFiles"`
	Artifacts        map[string]ArtifactState `json:"artifacts"`
	TaskProgress     TaskProgress             `json:"taskProgress"`
	Dependencies     Dependencies             `json:"dependencies"`
	ApplyState       ApplyState               `json:"applyState"`
	ActionContext    ActionContext            `json:"actionContext"`
	Relationships    Relationships            `json:"relationships"`
	RemediationState RemediationState         `json:"remediationState"`
	// RuntimeStatus is internal execution bookkeeping. ProjectStatusV2 is the
	// only public serializer and deliberately omits it.
	RuntimeStatus *RuntimeStatus `json:"runtimeStatus,omitempty"`
	// ReviewOffer is a fresh post-verification invitation. It contains no
	// candidate identity or persisted review authority and never affects archive.
	ReviewOffer *ReviewOfferBlock `json:"reviewOffer,omitempty"`
	// Consent is #2563's (S4b of #2540) edit-authority consent question:
	// present exactly when the status reports blocked(edit_authority_missing),
	// carrying the typed gentle-ai.sdd-integration.consent/v1 envelope whose
	// granted choice names the exact runnable grant invocation. Structural
	// absence (nil, omitempty) everywhere else — the same optional-block
	// discipline ReviewOffer established.
	Consent *SDDIntegrationConsentResult `json:"consent,omitempty"`
	// Archived is #4002's positive terminal projection: present exactly when
	// the named change is already archived, so closure stops answering through
	// the blocked channel. Structural absence (nil, omitempty) everywhere else
	// — the same optional-block discipline ReviewOffer established.
	Archived          *ArchivedProjection `json:"archived,omitempty"`
	PhaseInstructions *PhaseInstructions  `json:"phaseInstructions,omitempty"`
	NextRecommended   string              `json:"nextRecommended"`
	BlockedReasons    []string            `json:"blockedReasons"`
	// runtimeAttemptTokens carries the ledger's live attempt tokens alongside
	// RuntimeStatus so status can ask the one readiness predicate the same
	// question compact acquire asks, and name the same continuation acquire
	// names. It is unexported on purpose: the tokens are an input to the
	// answer, not part of the SDD v1 wire document, and the ratified contract
	// keeps full runtime payload off that document.
	runtimeAttemptTokens map[int]string
	verifyRefreshReason  string
}

// ReviewOfferBlock contains the complete optional review boundary: current
// mode availability and the command that starts a new review. It intentionally
// has no lineage, receipt, binding, successor, transaction, or gate field.
type ReviewOfferBlock struct {
	Available  bool   `json:"available"`
	Invocation string `json:"invocation"`
}

// ArchivedProjection is the positive terminal projection for a change that is
// already archived. Path names the location fact the resolving store can
// expose: the repo-relative archive folder for OpenSpec
// (openspec/changes/archive/YYYY-MM-DD-<change>), the archive report topic key
// for Engram (sdd/<change>/archive-report). When present, NextRecommended is
// "archived", no phase remains, and nothing is blocked (#4002; gentle-pi#538
// ships the same contract).
type ArchivedProjection struct {
	Path string `json:"path"`
}

// applyReviewOfferRouting is status's one review edge. It runs only after strict
// independent verification succeeds and never reads or persists review runtime
// authority.
func applyReviewOfferRouting(ctx context.Context, status *Status, workspaceRoot string, reviewDisabled bool) {
	if reviewDisabled || status.Dependencies.Verify != DependencyAllDone {
		return
	}
	offer, err := reviewOfferForVerify(ctx, workspaceRoot)
	if err != nil {
		return
	}
	status.ReviewOffer = &ReviewOfferBlock{
		Available:  offer.Available,
		Invocation: fmt.Sprintf("gentle-ai review start --cwd %s", pathquote.Quote(workspaceRoot)),
	}
}

type ResolveOptions struct {
	CWD                 string
	WorkspaceRoot       string
	ChangeName          string
	IncludeInstructions bool
	// ReviewDisabled records that the user's receipt-driven-development kill
	// switch is off for this clone. Disabled status skips review discovery and
	// leaves review context structurally absent; it never fabricates approval.
	// When enabled, review context remains informational and cannot decide
	// archive readiness or routing. The CLI owns the switch's source of truth.
	ReviewDisabled bool
	// ReviewDisabledForWorkspace lets the composition root resolve the switch
	// against the exact workspace normalized by Resolve. When set, it is called
	// once and its result replaces ReviewDisabled for the whole status decision.
	ReviewDisabledForWorkspace func(workspaceRoot string) (bool, error)
}

type CommandArgs struct {
	ChangeName          string
	CWD                 string
	JSON                bool
	IncludeInstructions bool
	Contract            string
}

type engramObservation struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Project string `json:"project"`
	Scope   string `json:"scope"`
}

var engramExport = exportEngramObservations

func ParseCommandArgs(args []string) (CommandArgs, error) {
	parsed := CommandArgs{Contract: StatusContractV2}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.JSON = true
		case "--instructions":
			parsed.IncludeInstructions = true
		case "--cwd":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return CommandArgs{}, fmt.Errorf("--cwd requires a value")
			}
			parsed.CWD = args[i+1]
			i++
		case "--contract":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return CommandArgs{}, fmt.Errorf("--contract requires a value")
			}
			parsed.Contract = args[i+1]
			i++
			if err := validateStatusContract(parsed.Contract); err != nil {
				return CommandArgs{}, err
			}
		default:
			if strings.HasPrefix(arg, "--contract=") {
				parsed.Contract = strings.TrimPrefix(arg, "--contract=")
				if err := validateStatusContract(parsed.Contract); err != nil {
					return CommandArgs{}, err
				}
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return CommandArgs{}, fmt.Errorf("unknown sdd-status argument %q", arg)
			}
			if parsed.ChangeName == "" {
				parsed.ChangeName = arg
			} else {
				return CommandArgs{}, fmt.Errorf("unexpected sdd-status argument %q", arg)
			}
		}
	}
	return parsed, nil
}

func validateStatusContract(contract string) error {
	if contract == StatusContractV2 {
		return nil
	}
	return fmt.Errorf("unsupported sdd-status contract %q. Start a fresh implementation state and rerun `gentle-ai sdd-status --contract gentle-ai.sdd-status/v2`.", contract)
}

func listActiveOpenSpecChanges(workspaceRoot string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(workspaceRoot, "openspec", "changes"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	changes := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "archive" && entry.Name() != "active" &&
			!openSpecChangeContainer(filepath.Join(workspaceRoot, "openspec", "changes", entry.Name())) {
			changes = append(changes, entry.Name())
		}
	}
	sort.Strings(changes)
	return changes, nil
}

// archiveDatePrefixPattern matches the 11-character YYYY-MM-DD- prefix the
// archive phase stamps on every openspec/changes/archive/ entry.
var archiveDatePrefixPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-$`)

// newestArchivedOpenSpecEntry returns the newest archive entry for a change.
// Archive folders are named YYYY-MM-DD-<change>; only an exact <change> suffix
// after a full date prefix matches (change "foo-bar" never matches
// "2026-01-01-foo-bar-baz"). Date prefixes sort lexicographically, so the
// greatest match is the newest one.
func newestArchivedOpenSpecEntry(workspaceRoot, changeName string) (string, bool) {
	entries, err := os.ReadDir(filepath.Join(workspaceRoot, "openspec", "changes", "archive"))
	if err != nil {
		return "", false
	}
	newest := ""
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || len(name) != 11+len(changeName) ||
			!archiveDatePrefixPattern.MatchString(name[:11]) || name[11:] != changeName {
			continue
		}
		if name > newest {
			newest = name
		}
	}
	return newest, newest != ""
}

// archivedOpenSpecStatus is the positive terminal projection for a change
// whose folder moved into openspec/changes/archive/ (#4002): every phase is
// all_done, nothing is blocked, and the repo-relative archive folder is the
// location fact on the wire.
func archivedOpenSpecStatus(workspaceRoot, changeName, archiveEntry string, includeInstructions bool) Status {
	status := baseStatus(ArtifactStoreOpenSpec, workspaceRoot, nil, &changeName, nil, "archived", nil)
	status.Dependencies = allDoneDependencies()
	status.ApplyState = ApplyAllDone
	status.Archived = &ArchivedProjection{Path: filepath.Join("openspec", "changes", "archive", archiveEntry)}
	if includeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status
}

// allDoneDependencies is the terminal dependency vector an archived change
// reports: no phase remains, so every phase answers all_done.
func allDoneDependencies() Dependencies {
	return Dependencies{
		Proposal: DependencyAllDone,
		Specs:    DependencyAllDone,
		Design:   DependencyAllDone,
		Tasks:    DependencyAllDone,
		Apply:    DependencyAllDone,
		Verify:   DependencyAllDone,
		Archive:  DependencyAllDone,
	}
}

// openSpecChangeContainer reports whether a directory under openspec/changes
// is a container of changes rather than a change (#2317): it holds no SDD
// artifact itself while a subdirectory does, the legacy `active/` layout. An
// empty scaffold keeps its historical standing as a candidate, and a marker
// that fails to stat for any reason other than not existing counts as present.
func openSpecChangeContainer(changeRoot string) bool {
	holdsArtifact := func(dir string) bool {
		for _, marker := range []string{"proposal.md", "design.md", "tasks.md", "specs", "spec.md", "verify-report.md", "state.yaml", "exploration.md"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); !errors.Is(err, os.ErrNotExist) {
				return true
			}
		}
		return false
	}
	entries, err := os.ReadDir(changeRoot)
	if err != nil || holdsArtifact(changeRoot) {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && holdsArtifact(filepath.Join(changeRoot, entry.Name())) {
			return true
		}
	}
	return false
}

// selectableOpenSpecChanges filters exploration-only directories out of the
// auto-selection candidates (#3278 claim C). A stale sdd-explore directory
// holding only exploration.md is not an active change: counting it made
// selection ambiguous beside the one genuinely active change, and the SDD
// task-failure continuation (which carries no selector) could never resolve
// that ambiguity. Exploration-only directories stay addressable when
// explicitly named — listActiveOpenSpecChanges still returns them — and
// directories with no SDD artifacts at all keep their historical behavior as
// candidates.
func selectableOpenSpecChanges(changesDir string, changes []string) []string {
	selectable := make([]string, 0, len(changes))
	for _, change := range changes {
		if explorationOnlyChangeDir(filepath.Join(changesDir, change)) {
			continue
		}
		selectable = append(selectable, change)
	}
	return selectable
}

// explorationOnlyChangeDir reports whether the change directory's only SDD
// artifact is an exploration artifact: exploration.md is present and none of
// proposal.md, design.md, tasks.md, or specs/ exist. Only os.ErrNotExist
// proves a marker absent; any other stat error makes the classification
// unprovable, so the directory stays an active-change candidate (failing
// toward visibility, never toward silent exclusion from selection).
func explorationOnlyChangeDir(changeRoot string) bool {
	if _, err := os.Stat(filepath.Join(changeRoot, "exploration.md")); err != nil {
		return false
	}
	for _, marker := range []string{"proposal.md", "design.md", "tasks.md", "specs"} {
		if _, err := os.Stat(filepath.Join(changeRoot, marker)); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	return true
}

// Resolve reports SDD status for a workspace. A workspace that DECLARES an
// artifact store gets that store: the declaration selects the resolver instead
// of decorating whatever the OpenSpec-first preference order happened to find.
//
// #3636 / #3814: before this, Engram was consulted only as a fallback when
// OpenSpec held nothing, so `sdd.artifact_store` could not change the outcome
// whenever OpenSpec data existed. The enum, the config key, and the documented
// "choose your store" vocabulary were decoration over a hardcoded order.
func Resolve(options ResolveOptions) (Status, error) {
	workspaceRoot, err := resolveWorkspaceRoot(options)
	if err != nil {
		return Status{}, err
	}
	declared, declaredOK := declaredArtifactStore(workspaceRoot)

	if declaredOK && declared == ArtifactStoreEngram {
		reviewDisabled, err := resolveReviewDisabled(options, workspaceRoot)
		if err != nil {
			return Status{}, err
		}
		status, resolved, err := resolveEngramStatus(workspaceRoot, strings.TrimSpace(options.ChangeName), options.IncludeInstructions, reviewDisabled)
		if err != nil {
			return Status{}, err
		}
		if resolved {
			return status, nil
		}
		// An empty declared store is an empty declared store. Serving the
		// OpenSpec artifacts that happen to sit on disk would report a store
		// the workspace did not declare.
		//
		// But an empty Engram resolution is not only the genuinely-empty case:
		// inferEngramProject falls back to the directory name, so a project
		// mismatch or an unpopulated store also returns zero changes. Saying
		// "start a new change" then, while OpenSpec work sits on disk, invites
		// an orchestrator that routes on nextRecommended to open a duplicate on
		// top of live work. That disagreement is a human decision.
		active, activeErr := listActiveOpenSpecChanges(workspaceRoot)
		if activeErr != nil {
			return Status{}, activeErr
		}
		if len(active) > 0 {
			return blockedStatus(ArtifactStoreEngram, workspaceRoot, nil, nil, "resolve-blockers", []string{
				"The workspace declares the engram artifact store, but it resolved no changes for the inferred project.",
				fmt.Sprintf("These openspec changes are on disk and were not served because they are not the declared store: %s.", strings.Join(active, ", ")),
				"Populate the declared store, correct the inferred project, or change sdd.artifact_store to match where the work lives.",
			}, options.IncludeInstructions), nil
		}
		return blockedEngramStatus(workspaceRoot, nil, "sdd-new", []string{
			"No SDD changes found in the declared Engram artifact store.",
		}, options.IncludeInstructions), nil
	}

	status, err := resolveByPreferenceOrder(options)
	if err != nil {
		return status, err
	}
	// A hybrid workspace is ONE declared store. Reporting engram for the change
	// that happened to resolve from the Engram half, and openspec for the one
	// that resolved from disk, is the same inference-over-declaration defect in
	// a different direction.
	if declaredOK && declared == ArtifactStoreHybrid && status.ArtifactStore != ArtifactStoreNone {
		status.ArtifactStore = ArtifactStoreHybrid
	}
	return status, nil
}

// resolveReviewDisabled applies the caller's per-workspace review-mode hook
// exactly once, shared by both resolution routes.
func resolveReviewDisabled(options ResolveOptions, workspaceRoot string) (bool, error) {
	if options.ReviewDisabledForWorkspace == nil {
		return options.ReviewDisabled, nil
	}
	return options.ReviewDisabledForWorkspace(workspaceRoot)
}

func resolveByPreferenceOrder(options ResolveOptions) (Status, error) {
	workspaceRoot, err := resolveWorkspaceRoot(options)
	if err != nil {
		return Status{}, err
	}
	reviewDisabled := options.ReviewDisabled
	if options.ReviewDisabledForWorkspace != nil {
		reviewDisabled, err = options.ReviewDisabledForWorkspace(workspaceRoot)
		if err != nil {
			return Status{}, err
		}
	}
	planningHome := filepath.Join(workspaceRoot, "openspec")
	changesDir := filepath.Join(planningHome, "changes")
	activeChanges, err := listActiveOpenSpecChanges(workspaceRoot)
	if err != nil {
		return Status{}, err
	}

	changeName := strings.TrimSpace(options.ChangeName)
	if changeName == "" {
		candidates := selectableOpenSpecChanges(changesDir, activeChanges)
		switch len(candidates) {
		case 0:
			if len(activeChanges) > 0 {
				// Every directory is exploration-only. This is an OpenSpec
				// workspace mid-exploration, not an empty one, so do not fall
				// through to Engram: report it honestly and keep the
				// directories addressable by explicit name.
				return blockedStatus(ArtifactStoreOpenSpec, workspaceRoot, nil, nil, "sdd-new", []string{
					"No active OpenSpec changes found under openspec/changes.",
					fmt.Sprintf(
						"Exploration-only directories are not active changes: %s. Run `gentle-ai sdd-status <change-name> --cwd %s` to inspect one explicitly.",
						strings.Join(activeChanges, ", "), workspaceRoot,
					),
				}, options.IncludeInstructions), nil
			}
			if status, ok, err := resolveEngramStatus(workspaceRoot, changeName, options.IncludeInstructions, reviewDisabled); ok || err != nil {
				return status, err
			}
			return blockedStatus(ArtifactStoreOpenSpec, workspaceRoot, nil, nil, "sdd-new", []string{"No active OpenSpec changes found under openspec/changes."}, options.IncludeInstructions), nil
		case 1:
			changeName = candidates[0]
		default:
			return blockedStatus(ArtifactStoreOpenSpec, workspaceRoot, nil, nil, "select-change", ambiguousChangeSelectionReasons("Change", workspaceRoot, candidates), options.IncludeInstructions), nil
		}
	}

	if !contains(activeChanges, changeName) {
		if status, ok, err := resolveEngramStatus(workspaceRoot, changeName, options.IncludeInstructions, reviewDisabled); ok || err != nil {
			return status, err
		}
		// #4002: an absent active folder may mean the change was archived, and
		// closure is a positive terminal fact, not a not-found block.
		if archiveEntry, ok := newestArchivedOpenSpecEntry(workspaceRoot, changeName); ok {
			return archivedOpenSpecStatus(workspaceRoot, changeName, archiveEntry, options.IncludeInstructions), nil
		}
		return blockedStatus(ArtifactStoreOpenSpec, workspaceRoot, &changeName, nil, "sdd-new", []string{fmt.Sprintf("Active OpenSpec change not found: %s.", changeName)}, options.IncludeInstructions), nil
	}

	changeRoot := filepath.Join(changesDir, changeName)
	artifactPaths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		return Status{}, err
	}
	artifacts := artifactStates{
		Proposal:      singleArtifactState(artifactPaths.Proposal),
		Specs:         multiArtifactState(artifactPaths.Specs, filepath.Join(changeRoot, "specs")),
		Design:        singleArtifactState(artifactPaths.Design),
		Tasks:         singleArtifactState(artifactPaths.Tasks),
		ApplyProgress: singleArtifactState(artifactPaths.ApplyProgress),
		VerifyReport:  singleArtifactState(artifactPaths.VerifyReport),
	}.statesFor(ArtifactStoreOpenSpec)
	taskProgress, err := countTaskProgress(firstPath(artifactPaths.Tasks))
	if err != nil {
		return Status{}, err
	}

	specCounts, err := readSpecCounts(artifactPaths.Specs)
	if err != nil {
		return Status{}, err
	}
	verifyResult, err := readVerifyResult(firstPath(artifactPaths.VerifyReport), specCounts)
	if err != nil {
		return Status{}, err
	}
	// The change-instance identity (#2563, S4b of #2540) binds the runtime
	// read so persisted grants project only for THIS instance of the change
	// name; without a marker the replay conservatively projects no granted
	// roots at all (#2557's containment).
	instance, err := readChangeInstanceMarker(changeRoot)
	if err != nil {
		return Status{}, err
	}
	runtimeStatus, runtimeAttemptTokens, _, runtimeStatusErr := loadNativeRuntimeStatus(context.Background(), workspaceRoot, changeName, instance)
	var grantedRoots []string
	if runtimeStatus != nil {
		grantedRoots = runtimeStatus.GrantedRoots
	}
	coreReady := artifacts["proposal"] == ArtifactDone && artifacts["specs"] == ArtifactDone && artifacts["design"] == ArtifactDone && artifacts["tasks"] == ArtifactDone && taskProgress.Total > 0
	applyState := resolveApplyState(coreReady, taskProgress)
	blockedReasons := artifactBlockedReasons(artifacts, taskProgress, changeName)
	if artifacts["specs"] == ArtifactPartial {
		blockedReasons.genuine = append(blockedReasons.genuine, openSpecSpecsLayoutReason(changeName))
	}
	if artifacts["verifyReport"] == ArtifactDone {
		if reason := verifyReportRefreshReason(verifyResult); reason != "" {
			blockedReasons.genuine = append(blockedReasons.genuine, reason)
		}
	}
	applyState, unauthorizedRoots := applyEditAuthorityBlock(applyState, &blockedReasons, readText(firstPath(artifactPaths.Tasks)), workspaceRoot, append([]string{workspaceRoot}, grantedRoots...))
	var consent *SDDIntegrationConsentResult
	if len(unauthorizedRoots) != 0 {
		// The envelope must name an invocation the agent executes verbatim,
		// so the instance token is minted (once) and persisted here; a
		// covering grant later projects through the same token and detection
		// finds nothing, so no envelope and no mint happen on ordinary
		// statuses.
		if instance == "" {
			if instance, err = ensureChangeInstanceMarker(changeRoot); err != nil {
				return Status{}, err
			}
		}
		expectedRevision := ""
		if runtimeStatus != nil {
			expectedRevision = runtimeStatus.Revision
		}
		envelope := newEditAuthorityConsent(changeName, workspaceRoot, unauthorizedRoots, instance, expectedRevision)
		consent = &envelope
	}
	runtimeRemediationComplete := nativeRuntimeCompletesRemediation(runtimeStatus, runtimeAttemptTokens, verifyResult)
	// Stale or incomplete evidence always re-enters independent SDD verification.
	verifyReportCurrent := artifacts["verifyReport"] == ArtifactDone && !verifyResult.Stale && !verifyResult.Incomplete
	remediationRequired := !runtimeRemediationComplete && verifyReportCurrent && !verifyResult.Passing && applyState == ApplyAllDone
	remediationState := resolveBoundedRemediation(
		remediationRequired,
		verifyResult,
		readText(firstPath(artifactPaths.ApplyProgress)),
	)
	dependencies := resolveDependencies(artifacts, taskProgress, applyState, coreReady, verifyReportCurrent, verifyResult.Passing, remediationState.Complete)
	nextRecommended := resolveNextRecommended(dependencies, applyState, verifyReportCurrent, remediationState)
	if runtimeRemediationComplete {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		nextRecommended = "verify"
		remediationState = RemediationState{}
		blockedReasons.genuine = appendMissingReason(blockedReasons.genuine, runtimeRemediationVerifyRefreshInstruction)
	}
	if len(unauthorizedRoots) == 0 && runtimeStatus != nil && runtimeStatusErr == nil {
		applyRuntimeTopologyBlock(context.Background(), &applyState, &dependencies, &nextRecommended, &blockedReasons, readText(firstPath(artifactPaths.Tasks)), workspaceRoot, changeName)
	}
	if remediationState.Reason != "" {
		blockedReasons.genuine = append(blockedReasons.genuine, remediationState.Reason)
	}
	status := baseStatus(ArtifactStoreOpenSpec, workspaceRoot, grantedRoots, &changeName, &changeRoot, nextRecommended, append([]string{}, blockedReasons.genuine...))
	status.Consent = consent
	status.ArtifactPaths = artifactPaths
	status.ContextFiles = artifactPaths
	status.Artifacts = artifacts
	status.TaskProgress = taskProgress
	status.Dependencies = dependencies
	status.ApplyState = applyState
	status.RemediationState = remediationState
	status.RuntimeStatus = runtimeStatus
	status.runtimeAttemptTokens = runtimeAttemptTokens
	if runtimeStatusErr != nil {
		applyNativeRuntimeErrorRouting(&status, runtimeStatusErr)
	} else {
		applyNativeRuntimeRouting(&status)
	}
	applyReviewOfferRouting(context.Background(), &status, workspaceRoot, reviewDisabled)
	status.BlockedReasons = blockedReasons.finalize(status.NextRecommended, status.BlockedReasons)
	if runtimeRemediationComplete && status.Dependencies.Verify == DependencyReady && status.Dependencies.Archive == DependencyBlocked && status.NextRecommended == string(PhaseVerify) {
		status.verifyRefreshReason = runtimeRemediationVerifyRefreshInstruction
	}
	if options.IncludeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status, nil
}

// loadNativeRuntimeStatus returns the runtime status together with the ledger's
// attempt tokens. Status needs the tokens because it now reports what compact
// acquire would return, and acquire's answer for a live attempt names that
// attempt's own token as the caller's continuation (#2463). A non-empty
// instance binds the read to one change-instance identity (#2563, S4b of
// #2540) so replay projects that instance's granted roots; an empty instance
// keeps #2557's conservative containment and projects none. The resolved
// repository root rides along (empty without Git) to avoid re-deriving it.
func loadNativeRuntimeStatus(ctx context.Context, workspaceRoot, changeName, instance string) (*RuntimeStatus, map[int]string, string, error) {
	// SDD status remains useful for non-Git planning fixtures and repositories.
	// A native runtime chain cannot exist without a Git common-dir, so the
	// absence of a repository means there is no runtime authority to embed.
	repositoryRoot, err := (reviewtransaction.SnapshotBuilder{Repo: workspaceRoot}).ResolveRepositoryRoot(ctx)
	if err != nil {
		if workspaceHasGitMetadata(workspaceRoot) {
			return nil, nil, "", fmt.Errorf("resolve Git repository for native SDD runtime authority: %w", err)
		}
		return nil, nil, "", nil
	}
	store, err := OpenRuntimeStore(ctx, workspaceRoot, changeName)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open native SDD runtime authority: %w", err)
	}
	if instance != "" {
		if store, err = store.ForInstance(instance); err != nil {
			return nil, nil, "", fmt.Errorf("bind native SDD runtime authority to the change instance: %w", err)
		}
	}
	replay, err := store.load()
	if err != nil {
		return nil, nil, "", fmt.Errorf("read native SDD runtime authority: %w", err)
	}
	status := replay.Status
	return &status, replay.AttemptTokens, repositoryRoot, nil
}

func workspaceHasGitMetadata(workspaceRoot string) bool {
	current := filepath.Clean(workspaceRoot)
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil || !os.IsNotExist(err) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// verifyReportRefreshReason names why a persisted verification report cannot
// stand as final evidence. A report that exists yet leaves verify at ready
// must never project a silent tuple (#3538): the stale reason carries the
// exact native totals the report has to match, so the agent re-verifies
// against the current specs instead of re-validating the same envelope.
func verifyReportRefreshReason(verify verifyResultEvaluation) string {
	switch {
	case verify.Incomplete:
		return verify.Reason
	case verify.Stale:
		return "persisted verification report is stale: " + verify.Reason + "; rerun SDD verification and persist a report whose totals match the current specs before archive"
	}
	return ""
}

// appendMissingReason appends reason unless the list already carries it, so a
// route that is explained from two sites never repeats itself.
func appendMissingReason(reasons []string, reason string) []string {
	if contains(reasons, reason) {
		return reasons
	}
	return append(reasons, reason)
}

func nativeRuntimeCompletesRemediation(runtimeStatus *RuntimeStatus, attemptTokens map[int]string, verify verifyResultEvaluation) bool {
	if runtimeStatus == nil || verify.EvidenceRevision == "" || len(runtimeStatus.Attempts) == 0 {
		return false
	}
	// "The runtime finished this objective cleanly" is the readiness question,
	// so it is asked through the one predicate rather than re-derived here.
	readiness, terminal := runtimeReadiness(runtimeReadinessInput{Status: *runtimeStatus, AttemptTokens: attemptTokens})
	if !terminal || readiness.State != CompactStateComplete {
		return false
	}
	last := runtimeStatus.Attempts[len(runtimeStatus.Attempts)-1]
	return last.Outcome == AttemptPassed && !last.ChangedLineBudgetExceeded &&
		last.RemediatesEvidenceRevision == verify.EvidenceRevision &&
		last.EvidenceRevision != "" && last.EvidenceRevision == runtimeStatus.EvidenceRevision
}

func applyNativeRuntimeErrorRouting(status *Status, runtimeErr error) {
	if status == nil || runtimeErr == nil {
		return
	}
	change := "<unresolved>"
	if status.ChangeName != nil {
		change = *status.ChangeName
	}
	reason := fmt.Sprintf(
		"native SDD runtime authority is unreadable and execution is blocked: %v; do not launch another actor or edit the Git-common-dir authority manually; the compact attempt path reports blocked(corrupt_authority), and full `gentle-ai sdd-attempt status --cwd %s --change %q` is a maintainer diagnostic only",
		runtimeErr, pathquote.Quote(status.ActionContext.WorkspaceRoot), change,
	)
	status.Dependencies.Apply = DependencyBlocked
	status.Dependencies.Verify = DependencyBlocked
	status.Dependencies.Archive = DependencyBlocked
	status.NextRecommended = "resolve-blockers"
	status.BlockedReasons = append(status.BlockedReasons, reason)
}

// applyNativeRuntimeRouting reports what compact acquire would return. It no
// longer derives that verdict, and it no longer asserts it in prose: the reason
// text is the predicate's own named exit.
//
// An active attempt is deliberately not a stop (#2463). The ratified contract
// (internal/assets/skills/_shared/sdd-status-contract.md lines 20, 21 and 142)
// makes acquire the launch authority and full runtime status a diagnostic, and
// acquire's own exit for this state is self-service: the holder of the token
// adds --token to its own call and continues that exact attempt. Status cannot
// know whether its reader holds that token, and it does not need to, because
// acquire checks. Blocking here stopped the one caller that was entitled to
// proceed, and every caller reaches acquire before launching anyway.
//
// A maintainer decision has no self-service exit, so it stays a hard stop.
func applyNativeRuntimeRouting(status *Status) {
	if status == nil || status.RuntimeStatus == nil {
		return
	}
	readiness, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: *status.RuntimeStatus, AttemptTokens: status.runtimeAttemptTokens,
	})
	if !terminal || readiness.State != CompactStateBlocked || readiness.Reason == CompactBlockActiveAttempt {
		return
	}
	change := status.RuntimeStatus.Change
	if status.ChangeName != nil {
		change = *status.ChangeName
	}
	reason := fmt.Sprintf(
		"native SDD runtime execution is blocked(%s) for %q in %s; compact acquire reports the same: %s",
		readiness.Reason, change, pathquote.Quote(status.ActionContext.WorkspaceRoot), readiness.Exit,
	)
	// The attempt ledger governs exactly one question: may a bounded
	// implementation work unit OPEN. A blocked answer is a true and permanent
	// answer to that, so Apply is blocked.
	//
	// It is not an answer about anything else. Projecting it onto Verify and
	// Archive stranded #2902's reporter: every task complete, the merged
	// candidate green on repository CI, receipt-driven review off both
	// globally and clone-locally, and one historical objective sitting at
	// maintainer_decision from accounting on work that had already landed.
	// The change could never be verified and never be archived.
	//
	// Nothing is laundered by letting the later phases answer for themselves.
	// A change whose budget was exhausted mid-flight still has incomplete
	// tasks and no passing verification, so its own Verify and Archive
	// dependencies keep it exactly where it was. Those dependencies are the
	// ones entitled to speak for those phases. The blocker stays in
	// BlockedReasons either way, so it remains auditable.
	status.Dependencies.Apply = DependencyBlocked
	if !contains(status.BlockedReasons, reason) {
		status.BlockedReasons = append(status.BlockedReasons, reason)
	}
	// resolve-blockers is the right next step only while this block is
	// actually in the change's way. Once implementation is done and verified,
	// the change's next step is its own remaining phase, not a reset of an
	// objective nobody needs to reopen.
	if status.Dependencies.Verify != DependencyAllDone || status.Dependencies.Archive == DependencyBlocked {
		status.Dependencies.Verify = DependencyBlocked
		status.Dependencies.Archive = DependencyBlocked
		status.NextRecommended = "resolve-blockers"
	}
}

func resolveEngramStatus(workspaceRoot string, requestedChange string, includeInstructions, reviewDisabled bool) (Status, bool, error) {
	if !shouldTryEngram(workspaceRoot) {
		return Status{}, false, nil
	}
	observations, err := engramExport(workspaceRoot)
	if err != nil {
		return Status{}, false, err
	}
	project := inferEngramProject(workspaceRoot)
	changes := collectEngramChanges(observations, project)
	changeName := strings.TrimSpace(requestedChange)
	if changeName == "" {
		switch len(changes) {
		case 0:
			return Status{}, false, nil
		case 1:
			changeName = changes[0]
		default:
			return blockedEngramStatus(workspaceRoot, nil, "select-change", ambiguousChangeSelectionReasons("Engram change", workspaceRoot, changes), includeInstructions), true, nil
		}
	}

	artifactsByType := engramArtifactsForChange(observations, project, changeName)
	if len(artifactsByType) == 0 {
		return Status{}, false, nil
	}

	artifactPaths := engramArtifactPaths(changeName, artifactsByType)
	artifacts := artifactStates{
		Proposal:      engramArtifactState(artifactsByType["proposal"]),
		Specs:         engramArtifactState(artifactsByType["spec"]),
		Design:        engramArtifactState(artifactsByType["design"]),
		Tasks:         engramArtifactState(artifactsByType["tasks"]),
		ApplyProgress: engramArtifactState(artifactsByType["apply-progress"]),
		VerifyReport:  engramArtifactState(artifactsByType["verify-report"]),
	}.statesFor(ArtifactStoreEngram)
	taskProgress := countTaskProgressText(artifactsByType["tasks"].Content)
	specCounts := countSpecRequirementsAndScenarios([]string{artifactsByType["spec"].Content})
	verifyResult := parseVerifyResult(artifactsByType["verify-report"].Content, specCounts)
	// The Engram store keeps the change's SDD state in Engram, not in a
	// change directory the archive flow moves, so the status layer has no
	// archive-coupled home for a change-instance marker there: persisting one
	// anywhere keyed by change name alone would recreate the #2557
	// resurrection hazard. The Engram path therefore keeps S1's honest block
	// (naming both exits) without a consent envelope, and its runtime read
	// stays instance-less, projecting no granted roots (#2563).
	runtimeStatus, runtimeAttemptTokens, _, runtimeStatusErr := loadNativeRuntimeStatus(context.Background(), workspaceRoot, changeName, "")
	coreReady := artifacts["proposal"] == ArtifactDone && artifacts["specs"] == ArtifactDone && artifacts["design"] == ArtifactDone && artifacts["tasks"] == ArtifactDone && taskProgress.Total > 0
	applyState := resolveApplyState(coreReady, taskProgress)
	blockedReasons := artifactBlockedReasons(artifacts, taskProgress, changeName)
	if artifacts["verifyReport"] == ArtifactDone {
		if reason := verifyReportRefreshReason(verifyResult); reason != "" {
			blockedReasons.genuine = append(blockedReasons.genuine, reason)
		}
	}
	applyState, unauthorizedRoots := applyEditAuthorityBlock(applyState, &blockedReasons, artifactsByType["tasks"].Content, workspaceRoot, []string{workspaceRoot})
	runtimeRemediationComplete := nativeRuntimeCompletesRemediation(runtimeStatus, runtimeAttemptTokens, verifyResult)
	// Stale or incomplete evidence always re-enters independent SDD verification.
	verifyReportCurrent := artifacts["verifyReport"] == ArtifactDone && !verifyResult.Stale && !verifyResult.Incomplete
	remediationRequired := !runtimeRemediationComplete && verifyReportCurrent && !verifyResult.Passing && applyState == ApplyAllDone
	remediationState := resolveBoundedRemediation(
		remediationRequired,
		verifyResult,
		artifactsByType["apply-progress"].Content,
	)
	if remediationState.Reason != "" {
		blockedReasons.genuine = append(blockedReasons.genuine, remediationState.Reason)
	}
	dependencies := resolveDependencies(artifacts, taskProgress, applyState, coreReady, verifyReportCurrent, verifyResult.Passing, remediationState.Complete)
	nextRecommended := resolveNextRecommended(dependencies, applyState, verifyReportCurrent, remediationState)
	if runtimeRemediationComplete {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		nextRecommended = "verify"
		remediationState = RemediationState{}
		blockedReasons.genuine = appendMissingReason(blockedReasons.genuine, runtimeRemediationVerifyRefreshInstruction)
	}
	if len(unauthorizedRoots) == 0 && runtimeStatus != nil && runtimeStatusErr == nil {
		applyRuntimeTopologyBlock(context.Background(), &applyState, &dependencies, &nextRecommended, &blockedReasons, artifactsByType["tasks"].Content, workspaceRoot, changeName)
	}
	changeRoot := fmt.Sprintf("engram:sdd/%s", changeName)
	status := baseStatus(ArtifactStoreEngram, workspaceRoot, nil, &changeName, &changeRoot, nextRecommended, append([]string{}, blockedReasons.genuine...))
	status.PlanningHome = PlanningHome{Mode: ActionModeRepoLocal, Path: "engram:sdd"}
	status.ArtifactPaths = artifactPaths
	status.ContextFiles = artifactPaths
	status.Artifacts = artifacts
	status.TaskProgress = taskProgress
	status.Dependencies = dependencies
	status.ApplyState = applyState
	status.RemediationState = remediationState
	status.RuntimeStatus = runtimeStatus
	status.runtimeAttemptTokens = runtimeAttemptTokens
	if runtimeStatusErr != nil {
		applyNativeRuntimeErrorRouting(&status, runtimeStatusErr)
	} else {
		applyNativeRuntimeRouting(&status)
	}
	applyReviewOfferRouting(context.Background(), &status, workspaceRoot, reviewDisabled)
	if _, archived := artifactsByType["archive-report"]; archived {
		// The archive phase wrote the archive report, so the change is closed.
		// Discovery already skips it (#3008); naming it must not send an
		// orchestrator back to archive (#3480). #4002: closure is a positive
		// terminal fact, not a blocker — project it the way OpenSpec projects an
		// archived folder. The location fact this store can expose is the
		// archive report topic key, and a closed change has nothing left to
		// block or remediate.
		status.Dependencies = allDoneDependencies()
		status.ApplyState = ApplyAllDone
		status.NextRecommended = "archived"
		status.Archived = &ArchivedProjection{Path: fmt.Sprintf("sdd/%s/archive-report", changeName)}
		status.BlockedReasons = []string{}
		status.RemediationState = RemediationState{}
	} else {
		status.BlockedReasons = blockedReasons.finalize(status.NextRecommended, status.BlockedReasons)
	}
	if runtimeRemediationComplete && status.Dependencies.Verify == DependencyReady && status.Dependencies.Archive == DependencyBlocked && status.NextRecommended == string(PhaseVerify) {
		status.verifyRefreshReason = runtimeRemediationVerifyRefreshInstruction
	}
	if includeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status, true, nil
}

func blockedEngramStatus(workspaceRoot string, changeName *string, next string, reasons []string, includeInstructions bool) Status {
	status := blockedStatus(ArtifactStoreEngram, workspaceRoot, changeName, nil, next, reasons, includeInstructions)
	status.PlanningHome = PlanningHome{Mode: ActionModeRepoLocal, Path: "engram:sdd"}
	return status
}

func shouldTryEngram(workspaceRoot string) bool {
	// A declaration is authoritative in both directions: it opts a workspace
	// in, and it also opts one out even when a .engram directory is present.
	if declared, ok := declaredArtifactStore(workspaceRoot); ok {
		return declared == ArtifactStoreEngram || declared == ArtifactStoreHybrid
	}
	if os.Getenv("GENTLE_AI_SDD_STATUS_ENGRAM") != "" {
		return true
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".engram")); err == nil {
		return true
	}
	return false
}

// declaredArtifactStore reads the store a workspace declares in
// openspec/config.yaml. It replaces the previous substring probe, which could
// only answer the binary question "does this mention engram or hybrid" and so
// could never distinguish the two or honour a declared openspec.
func declaredArtifactStore(workspaceRoot string) (ArtifactStore, bool) {
	for _, path := range []string{filepath.Join(workspaceRoot, "openspec", "config.yaml"), filepath.Join(workspaceRoot, "openspec", "config.yml")} {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			trimmed := strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			if !strings.HasPrefix(trimmed, "artifact_store:") && !strings.HasPrefix(trimmed, "artifactStore:") {
				continue
			}
			value := strings.ToLower(strings.Trim(strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[1]), `"'`))
			switch ArtifactStore(value) {
			case ArtifactStoreEngram, ArtifactStoreOpenSpec, ArtifactStoreHybrid, ArtifactStoreNone:
				return ArtifactStore(value), true
			}
			return "", false
		}
	}
	return "", false
}

func exportEngramObservations(workspaceRoot string) ([]engramObservation, error) {
	tmp, err := os.CreateTemp("", "gentle-ai-sdd-engram-*.json")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	defer os.Remove(path)

	cmd := exec.Command("engram", "export", path)
	cmd.Dir = workspaceRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("engram export failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Observations []engramObservation `json:"observations"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}
	return payload.Observations, nil
}

func inferEngramProject(workspaceRoot string) string {
	if project := strings.TrimSpace(os.Getenv("ENGRAM_PROJECT")); project != "" {
		return strings.ToLower(project)
	}
	if path := gitConfigPathFor(workspaceRoot); path != "" {
		config, err := os.ReadFile(path)
		if err == nil {
			if project := projectFromGitConfig(string(config)); project != "" {
				return project
			}
		}
	}
	return strings.ToLower(filepath.Base(workspaceRoot))
}

// gitConfigPathFor resolves the `config` file that governs workspaceRoot. The
// `.git` entry is a directory for an ordinary checkout but a file holding a
// `gitdir:` pointer for a linked worktree, and a linked worktree keeps `config`
// in the shared common dir rather than under its own gitdir. Reading
// `<root>/.git/config` blindly therefore fails for every worktree. An empty
// result means workspaceRoot has no readable Git configuration.
func gitConfigPathFor(workspaceRoot string) string {
	gitEntry := filepath.Join(workspaceRoot, ".git")
	info, err := os.Stat(gitEntry)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return filepath.Join(gitEntry, "config")
	}

	pointer, err := os.ReadFile(gitEntry)
	if err != nil {
		return ""
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(pointer)), "gitdir:"))
	if gitDir == "" {
		return ""
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workspaceRoot, gitDir)
	}

	// A linked worktree records the shared directory in `commondir`, relative
	// to its own gitdir. Without it the gitdir already is the common dir.
	commonDir := gitDir
	if content, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		if trimmed := strings.TrimSpace(string(content)); trimmed != "" {
			commonDir = trimmed
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(gitDir, commonDir)
			}
		}
	}
	return filepath.Join(commonDir, "config")
}

func projectFromGitConfig(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "url =") {
			continue
		}
		url := strings.TrimSpace(strings.TrimPrefix(line, "url ="))
		url = strings.TrimSuffix(url, ".git")
		if idx := strings.LastIndexAny(url, "/:"); idx >= 0 && idx+1 < len(url) {
			return strings.ToLower(url[idx+1:])
		}
	}
	return ""
}

var engramTitlePattern = regexp.MustCompile(`^sdd/([^/]+)/(proposal|spec|design|tasks|apply-progress|verify-report|state|archive-report)$`)

func collectEngramChanges(observations []engramObservation, project string) []string {
	// An Engram-backed change has no directory to move, so nothing about the
	// store itself says a change is finished. OpenSpec derives "active" from
	// what stays out of openspec/changes/archive/; Engram has no equivalent, so
	// before this every change that ever persisted an artifact was reported
	// active forever — thirty of them on one real project, seven of those
	// archived weeks earlier (#3008).
	//
	// The archive phase already writes sdd/{change}/archive-report and calls it
	// the audit trail. That report is the closure signal; it was simply not in
	// the title pattern, so the one artifact proving a change is done was the
	// one this never read.
	archived := map[string]bool{}
	seen := map[string]bool{}
	for _, observation := range observations {
		if !engramObservationMatchesProject(observation, project) {
			continue
		}
		matches := engramTitlePattern.FindStringSubmatch(strings.TrimSpace(observation.Title))
		if len(matches) != 3 {
			continue
		}
		switch matches[2] {
		case "archive-report":
			archived[matches[1]] = true
		case "state":
			// Progress metadata, not evidence that work exists.
		default:
			seen[matches[1]] = true
		}
	}
	changes := make([]string, 0, len(seen))
	for change := range seen {
		// Excluded from DISCOVERY only. Naming an archived change still
		// resolves it through engramArtifactsForChange, because "which changes
		// are in flight" and "show me this change" are different questions and
		// the artifacts remain the audit trail.
		if archived[change] {
			continue
		}
		changes = append(changes, change)
	}
	sort.Strings(changes)
	return changes
}

func engramArtifactsForChange(observations []engramObservation, project string, changeName string) map[string]engramObservation {
	artifacts := map[string]engramObservation{}
	for _, observation := range observations {
		if !engramObservationMatchesProject(observation, project) {
			continue
		}
		matches := engramTitlePattern.FindStringSubmatch(strings.TrimSpace(observation.Title))
		if len(matches) != 3 || matches[1] != changeName {
			continue
		}
		artifacts[matches[2]] = observation
	}
	return artifacts
}

func engramObservationMatchesProject(observation engramObservation, project string) bool {
	return strings.EqualFold(strings.TrimSpace(observation.Project), project) && strings.TrimSpace(observation.Scope) != "personal"
}

func engramArtifactPaths(changeName string, artifacts map[string]engramObservation) ArtifactPaths {
	paths := emptyArtifactPaths()
	if _, ok := artifacts["proposal"]; ok {
		paths.Proposal = []string{fmt.Sprintf("sdd/%s/proposal", changeName)}
	}
	if _, ok := artifacts["spec"]; ok {
		paths.Specs = []string{fmt.Sprintf("sdd/%s/spec", changeName)}
	}
	if _, ok := artifacts["design"]; ok {
		paths.Design = []string{fmt.Sprintf("sdd/%s/design", changeName)}
	}
	if _, ok := artifacts["tasks"]; ok {
		paths.Tasks = []string{fmt.Sprintf("sdd/%s/tasks", changeName)}
	}
	if _, ok := artifacts["apply-progress"]; ok {
		paths.ApplyProgress = []string{fmt.Sprintf("sdd/%s/apply-progress", changeName)}
	}
	if _, ok := artifacts["verify-report"]; ok {
		paths.VerifyReport = []string{fmt.Sprintf("sdd/%s/verify-report", changeName)}
	}
	return paths
}

func engramArtifactState(observation engramObservation) ArtifactState {
	if observation.Title == "" {
		return ArtifactMissing
	}
	if strings.TrimSpace(observation.Content) == "" {
		return ArtifactPartial
	}
	return ArtifactDone
}

func reportTextIsClearlyPassing(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	hasPassSignal := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if reportLineHasBlocker(line) {
			return false
		}
		if reportLineHasPassSignal(line) {
			hasPassSignal = true
		}
	}
	return hasPassSignal
}

func RenderMarkdown(status Status) string {
	changeName := "unresolved"
	if status.ChangeName != nil {
		changeName = *status.ChangeName
	}

	jsonBytes, err := marshalStatusV2Indent(status)
	if err != nil {
		jsonBytes = []byte("{}")
	}

	lines := []string{
		fmt.Sprintf("## SDD Status: %s", changeName),
		"",
		fmt.Sprintf("schema: %s@%d", status.SchemaName, status.SchemaVersion),
		fmt.Sprintf("store: %s", status.ArtifactStore),
		fmt.Sprintf("planning_home: %s", status.PlanningHome.Path),
		fmt.Sprintf("next: %s", status.NextRecommended),
		"",
		"### Summary",
		fmt.Sprintf("- apply: %s", status.Dependencies.Apply),
		fmt.Sprintf("- verify: %s", status.Dependencies.Verify),
		fmt.Sprintf("- archive: %s", status.Dependencies.Archive),
		fmt.Sprintf("- tasks: %d/%d complete", status.TaskProgress.Completed, status.TaskProgress.Total),
	}
	if len(status.BlockedReasons) > 0 {
		lines = append(lines, "", "### Blocked Reasons")
		for _, reason := range status.BlockedReasons {
			lines = append(lines, fmt.Sprintf("- %s", reason))
		}
	}
	lines = append(lines, "", "### JSON", "```json", string(jsonBytes), "```")
	return strings.Join(lines, "\n")
}

func RenderDispatcherMarkdown(status Status) string {
	changeName := "unresolved"
	if status.ChangeName != nil {
		changeName = *status.ChangeName
	}

	jsonBytes, err := marshalStatusV2Indent(status)
	if err != nil {
		jsonBytes = []byte("{}")
	}

	lines := []string{
		fmt.Sprintf("## Native SDD Dispatcher: %s", changeName),
		"",
		"Native status is authoritative. Route by next_recommended and dependency state, not by prompt inference.",
		"",
		fmt.Sprintf("next_recommended: %s", status.NextRecommended),
		"",
		"### Dependency States",
		fmt.Sprintf("- proposal: %s", status.Dependencies.Proposal),
		fmt.Sprintf("- specs: %s", status.Dependencies.Specs),
		fmt.Sprintf("- design: %s", status.Dependencies.Design),
		fmt.Sprintf("- tasks: %s", status.Dependencies.Tasks),
		fmt.Sprintf("- apply: %s", status.Dependencies.Apply),
		fmt.Sprintf("- verify: %s", status.Dependencies.Verify),
		fmt.Sprintf("- archive: %s", status.Dependencies.Archive),
		fmt.Sprintf("- task_progress: %d/%d complete", status.TaskProgress.Completed, status.TaskProgress.Total),
	}

	if len(status.BlockedReasons) > 0 {
		lines = append(lines, "", "### Blocked Reasons")
		for _, reason := range status.BlockedReasons {
			lines = append(lines, fmt.Sprintf("- %s", reason))
		}
	}
	if extra, ok := nonPhaseRoutingInstructions(status); ok {
		lines = append(lines, extra...)
	} else if phase, ok := nextRecommendedPhase(status.NextRecommended); ok {
		lines = append(lines, "", fmt.Sprintf("### Next Phase Instructions: %s", phase))
		for _, instruction := range instructionsForPhase(status, phase) {
			lines = append(lines, fmt.Sprintf("- %s", instruction))
		}
	}

	lines = append(lines, "", "### JSON", "```json", string(jsonBytes), "```")
	return strings.Join(lines, "\n")
}

func RenderNativePhasePrompt(status Status, phase Phase) string {
	changeName := "unresolved"
	if status.ChangeName != nil {
		changeName = *status.ChangeName
	}

	jsonBytes, err := marshalStatusV2Indent(status)
	if err != nil {
		jsonBytes = []byte("{}")
	}

	lines := []string{
		fmt.Sprintf("## Native SDD Phase Prompt: %s", phase),
		"",
		fmt.Sprintf("Change: %s", changeName),
		"Native status is authoritative over prompt inference. Do not infer phase readiness from instructions alone.",
		"If this phase is blocked, return the blockers instead of acting.",
		"",
		"### Phase State",
		fmt.Sprintf("- requested_phase: %s", phase),
		fmt.Sprintf("- dependency_state: %s", dependencyForPhase(status, phase)),
		fmt.Sprintf("- next_recommended: %s", status.NextRecommended),
	}

	if len(status.BlockedReasons) > 0 {
		lines = append(lines, "", "### Blocked Reasons")
		for _, reason := range status.BlockedReasons {
			lines = append(lines, fmt.Sprintf("- %s", reason))
		}
	}

	lines = append(lines, "", "### Phase Instructions")
	for _, instruction := range instructionsForPhase(status, phase) {
		lines = append(lines, fmt.Sprintf("- %s", instruction))
	}

	lines = append(lines, "", "### JSON", "```json", string(jsonBytes), "```")
	return strings.Join(lines, "\n")
}

func resolveWorkspaceRoot(options ResolveOptions) (string, error) {
	var root string
	var err error
	if strings.TrimSpace(options.WorkspaceRoot) != "" {
		root, err = filepath.Abs(options.WorkspaceRoot)
	} else {
		root, err = absOrCWD(options.CWD)
	}
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root is not a directory: %s", root)
	}
	// A filesystem root is never a workspace, and answering as if it might be
	// is worse than refusing (#2790). A failure continuation that resolved its
	// working directory to the drive root used to get a successful, empty,
	// entirely plausible status back, so the operator read "SDD lost my
	// project" instead of "that command was pointed at the wrong directory".
	//
	// Nothing legitimate is rejected: no project lives at `/` or at `C:\`, so
	// there is no false positive to weigh against the confusion this prevents.
	if filepath.Dir(root) == root {
		return "", fmt.Errorf("workspace root %q is a filesystem root, which never holds an SDD project: whatever produced this call passed the wrong --cwd. Rerun it against the project: `gentle-ai sdd-status --cwd \"<project-directory>\" --json`. If the change is Engram-backed, this dispatcher is blind to it and should not be called at all", root)
	}
	return root, nil
}

func absOrCWD(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return os.Getwd()
	}
	return filepath.Abs(path)
}

// ambiguousChangeSelectionReasons names one runnable command per candidate
// change instead of listing names and stopping there.
//
// The markdown dispatcher already spelled these commands out, but --json never
// reaches it, and --json is what machine consumers read. #2117 step 5 is the
// cost: the SDD task-failure envelope hands back
// `gentle-ai sdd-status --cwd <cwd> --json` as its continuation, which lands
// here whenever more than one change is active, and the caller found a reason
// that listed options and named no command. The list stays first because it is
// what a human scanning the refusal wants; the commands follow because that is
// what makes the refusal runnable.
//
// The selector is positional: ParseCommandArgs has no --change flag, so the
// emitted spelling must be `gentle-ai sdd-status <change> --cwd <root>`. The
// first shipped spelling used `--change` and every emitted command was
// rejected by the very parser it targets (#3278, #2790), burning the
// operator's single sanctioned observation on a syntax error. The guard test
// in selection_continuation_test.go feeds every emitted continuation back
// through ParseCommandArgs so the spelling cannot drift from the parser again.
func ambiguousChangeSelectionReasons(subject, workspaceRoot string, changes []string) []string {
	reasons := make([]string, 0, len(changes)+1)
	reasons = append(reasons, fmt.Sprintf("%s selection is ambiguous: %s.", subject, strings.Join(changes, ", ")))
	for _, change := range changes {
		reasons = append(reasons, fmt.Sprintf(
			"Run `gentle-ai sdd-status %s --cwd %s` to continue with %s.",
			change, workspaceRoot, change,
		))
	}
	return reasons
}

func blockedStatus(store ArtifactStore, workspaceRoot string, changeName *string, changeRoot *string, next string, reasons []string, includeInstructions bool) Status {
	status := baseStatus(store, workspaceRoot, nil, changeName, changeRoot, next, reasons)
	if includeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status
}

// baseStatus takes the artifact store as an input so the artifact map is built
// for the store the status actually reports. Issue #2346: it used to hardcode
// ArtifactStoreOpenSpec and leave callers to relabel the store afterwards,
// which produced an Engram status carrying an OpenSpec-shaped map.
// grantedRoots extends AllowedEditRoots with the per-change grants the caller
// projected for the current change instance (#2563, S4b of #2540); this is
// the single assignment site for edit authority, and it stays outside the
// #2515 readiness triple.
func baseStatus(store ArtifactStore, workspaceRoot string, grantedRoots []string, changeName *string, changeRoot *string, next string, reasons []string) Status {
	emptyPaths := emptyArtifactPaths()
	if reasons == nil {
		reasons = []string{}
	}
	return Status{
		SchemaName:    SchemaName,
		SchemaVersion: SchemaVersion,
		ChangeName:    changeName,
		ArtifactStore: store,
		PlanningHome: PlanningHome{
			Mode: ActionModeRepoLocal,
			Path: filepath.Join(workspaceRoot, "openspec"),
		},
		ChangeRoot:    changeRoot,
		ArtifactPaths: emptyPaths,
		ContextFiles:  emptyPaths,
		Artifacts:     artifactStates{}.statesFor(store),
		TaskProgress:  TaskProgress{},
		Dependencies: Dependencies{
			Proposal: DependencyBlocked,
			Specs:    DependencyBlocked,
			Design:   DependencyBlocked,
			Tasks:    DependencyBlocked,
			Apply:    DependencyBlocked,
			Verify:   DependencyBlocked,
			Archive:  DependencyBlocked,
		},
		ApplyState: ApplyBlocked,
		ActionContext: ActionContext{
			Mode:             ActionModeRepoLocal,
			WorkspaceRoot:    workspaceRoot,
			AllowedEditRoots: append([]string{workspaceRoot}, grantedRoots...),
		},
		Relationships: Relationships{
			DependsOn:               openSpecStateDependsOn(store, changeRoot),
			Supersedes:              []string{},
			Amends:                  []string{},
			ConflictsWith:           []string{},
			SameDomainActiveChanges: []string{},
		},
		NextRecommended: next,
		BlockedReasons:  reasons,
	}
}

func resolveArtifactPaths(changeRoot string) (ArtifactPaths, error) {
	paths := emptyArtifactPaths()
	paths.Proposal = existingPath(filepath.Join(changeRoot, "proposal.md"))
	paths.Design = existingPath(filepath.Join(changeRoot, "design.md"))
	paths.Tasks = existingPath(filepath.Join(changeRoot, "tasks.md"))
	paths.ApplyProgress = existingPath(filepath.Join(changeRoot, "apply-progress.md"))
	paths.VerifyReport = existingPath(filepath.Join(changeRoot, "verify-report.md"))

	specFiles, err := findSpecFiles(filepath.Join(changeRoot, "specs"))
	if err != nil {
		return ArtifactPaths{}, err
	}
	if len(specFiles) == 0 {
		flatSpec := filepath.Join(changeRoot, "spec.md")
		if hasContent(flatSpec) {
			specFiles = []string{flatSpec}
		}
	}
	paths.Specs = specFiles
	return paths, nil
}

func emptyArtifactPaths() ArtifactPaths {
	return ArtifactPaths{
		Proposal:      []string{},
		Specs:         []string{},
		Design:        []string{},
		Tasks:         []string{},
		ApplyProgress: []string{},
		VerifyReport:  []string{},
	}
}

func existingPath(path string) []string {
	if _, err := os.Stat(path); err == nil {
		return []string{path}
	}
	return []string{}
}

func findSpecFiles(specsRoot string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(specsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "spec.md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func singleArtifactState(paths []string) ArtifactState {
	if len(paths) == 0 {
		return ArtifactMissing
	}
	if hasContent(paths[0]) {
		return ArtifactDone
	}
	return ArtifactPartial
}

func multiArtifactState(paths []string, root string) ArtifactState {
	if len(paths) == 0 {
		if entries, err := os.ReadDir(root); err == nil && len(entries) > 0 {
			return ArtifactPartial
		}
		return ArtifactMissing
	}
	for _, path := range paths {
		if !hasContent(path) {
			return ArtifactPartial
		}
	}
	return ArtifactDone
}

func hasContent(path string) bool {
	content, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(content)) != ""
}

func reportIsClearlyPassing(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(content)
	if strings.TrimSpace(text) == "" {
		return false, nil
	}
	hasPassSignal := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if reportLineHasBlocker(line) {
			return false, nil
		}
		if reportLineHasPassSignal(line) {
			hasPassSignal = true
		}
	}
	return hasPassSignal, nil
}

var taskCheckbox = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+\[([ xX])\]`)

var reportFieldPattern = regexp.MustCompile(`^\s*(?:[-*]\s+)?(?:\*\*)?([A-Za-z][A-Za-z\s-]*?)(?:\*\*)?\s*:\s*(.*)$`)

var reportFailedCountPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bfailed\s*:\s*(\d+)\b`),
	regexp.MustCompile(`(?i)\b(\d+)\s+failed\b`),
}

var reportPassValuePattern = regexp.MustCompile(`(?i)^(?:PASS|PASSED|PASS\s+WITH\s+WARNINGS|SUCCESS|SUCCESSFUL)$`)
var reportFailValuePattern = regexp.MustCompile(`(?i)^(?:FAIL|FAILED|FAILING|FAILURE|BLOCKED|UNTESTED)$`)
var reportCriticalGlyphStatusPattern = regexp.MustCompile(`(?i)❌\s*(?:FAIL|FAILED|FAILING|FAILURE|BLOCKED|UNTESTED)\b`)
var reportPassNegationPattern = regexp.MustCompile(`(?i)\bnot\s+(?:pass|passed|passing|successful|complete|completed)\b|\b(?:pass|passed|success|successful|complete|completed)\s*:\s*no\b`)
var reportPendingPattern = regexp.MustCompile(`(?i)\b(?:TODO|PENDING)\b`)
var reportBenignValuePattern = regexp.MustCompile(`(?i)^(?:none|no|n/a|not\s+applicable|0\s+(?:failed|blockers?|critical|issues?))\.?$`)

func reportLineHasBlocker(line string) bool {
	if line == "" {
		return false
	}
	if reportPassNegationPattern.MatchString(line) || reportPendingPattern.MatchString(line) {
		return true
	}
	if reportCriticalGlyphStatusPattern.MatchString(line) {
		return true
	}
	for _, pattern := range reportFailedCountPatterns {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) == 2 && matches[1] != "0" {
			return true
		}
	}
	label, value, hasField := reportField(line)
	if hasField {
		normalizedLabel := normalizeReportToken(label)
		trimmedValue := strings.TrimSpace(value)
		switch normalizedLabel {
		case "critical", "blocker", "blockers", "verificationblocker", "verificationblockers", "failure", "fail", "failed":
			return !reportValueIsBenign(trimmedValue)
		case "verdict", "status", "result", "verification", "finalverdict", "build", "tests":
			if reportFailValuePattern.MatchString(stripMarkdownSignal(trimmedValue)) {
				return true
			}
		}
	}
	trimmed := stripMarkdownSignal(line)
	return reportFailValuePattern.MatchString(trimmed)
}

func reportLineHasPassSignal(line string) bool {
	if line == "" {
		return false
	}
	_, value, hasField := reportField(line)
	if hasField && reportPassValuePattern.MatchString(stripMarkdownSignal(value)) {
		return true
	}
	trimmed := stripMarkdownSignal(line)
	return reportPassValuePattern.MatchString(trimmed) || strings.EqualFold(trimmed, "all checks passed") || strings.EqualFold(trimmed, "all checks passed.") || strings.EqualFold(trimmed, "ready for archive") || strings.EqualFold(trimmed, "ready for archive.")
}

func reportField(line string) (string, string, bool) {
	matches := reportFieldPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return "", "", false
	}
	return matches[1], matches[2], true
}

func reportValueIsBenign(value string) bool {
	value = strings.TrimSpace(stripMarkdownSignal(value))
	if value == "" || value == "0" {
		return true
	}
	return reportBenignValuePattern.MatchString(value) || strings.EqualFold(value, "no blockers")
}

func stripMarkdownSignal(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "*`_")
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"✅", "❌", "⚠️", "⚠"} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	return strings.TrimSpace(value)
}

func normalizeReportToken(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func countTaskProgress(tasksPath string) (TaskProgress, error) {
	if tasksPath == "" {
		return TaskProgress{}, nil
	}
	content, err := os.ReadFile(tasksPath)
	if err != nil {
		return TaskProgress{}, err
	}
	return countTaskProgressText(string(content)), nil
}

func countTaskProgressText(content string) TaskProgress {
	var progress TaskProgress
	fence := ""
	for _, line := range strings.Split(content, "\n") {
		// #2480: a checkbox row inside a ``` or ~~~ fence is an example. A fence
		// closes on a same-character run at least as long as the opener, alone.
		if run := fencedCodeRun(line); run != "" {
			switch {
			case fence == "":
				fence = run
			case run[0] == fence[0] && len(run) >= len(fence) && strings.TrimSpace(line) == run:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		matches := taskCheckbox.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		progress.Total++
		if matches[1] == "x" || matches[1] == "X" {
			progress.Completed++
		} else {
			progress.Pending++
		}
	}
	progress.AllComplete = progress.Total > 0 && progress.Pending == 0
	return progress
}

// fencedCodeRun returns the leading run of three or more backticks or tildes
// that opens or closes a fenced code block, or "" when the line is not a fence.
func fencedCodeRun(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	run := strings.TrimLeft(trimmed, string(trimmed[0]))
	if len(trimmed)-len(run) < 3 {
		return ""
	}
	return trimmed[:len(trimmed)-len(run)]
}

func artifactBlockedReasons(artifacts map[string]ArtifactState, taskProgress TaskProgress, changeName string) blockerReasons {
	var reasons blockerReasons
	if changeName == "" {
		changeName = "<change>"
	}
	specsDir := "openspec/changes/" + changeName + "/specs/"
	if artifacts["proposal"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, "proposal.md is missing or partial.")
	}
	if artifacts["specs"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, specsDir+"<domain>/spec.md is missing or partial.")
	}
	if artifacts["design"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, "design.md is missing or partial.")
	}
	if artifacts["tasks"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, "tasks.md is missing or partial.")
	}
	if artifacts["tasks"] == ArtifactDone && taskProgress.Total == 0 {
		reasons.genuine = append(reasons.genuine, "tasks.md has no markdown task checkboxes.")
	}
	return reasons
}

// openSpecSpecsLayoutReason names the change-local layout when the OpenSpec
// specs/ directory has entries but no <domain>/spec.md (#2212). That flat
// layout is what an actor produces after the shipped skill sent a new
// capability elsewhere, and the dispatcher can never read it. The spec route
// drops expected planning blockers, so this guidance is a genuine reason;
// otherwise the reporter sees nextRecommended: spec with no reason forever.
// The Engram store reports partial for an empty artifact, not a layout, so
// only the OpenSpec resolver appends it.
func openSpecSpecsLayoutReason(changeName string) string {
	specsDir := "openspec/changes/" + changeName + "/specs/"
	return specsDir + " has files but no non-empty <domain>/spec.md; the spec phase writes every capability (new ones as full specs) at " + specsDir + "<domain>/spec.md, and sdd-archive promotes new ones to openspec/specs/<domain>/spec.md"
}

func resolveApplyState(coreReady bool, taskProgress TaskProgress) ApplyState {
	if !coreReady {
		return ApplyBlocked
	}
	if taskProgress.AllComplete {
		return ApplyAllDone
	}
	return ApplyReady
}

func resolveDependencies(artifacts map[string]ArtifactState, taskProgress TaskProgress, applyState ApplyState, coreReady, verifyReportCurrent, verifyReportPassing, remediationComplete bool) Dependencies {
	dependencies := Dependencies{
		Proposal: artifactDependency(artifacts["proposal"]),
		Specs:    artifactDependency(artifacts["specs"]),
		Design:   artifactDependency(artifacts["design"]),
		Tasks:    artifactDependency(artifacts["tasks"]),
		Apply:    DependencyBlocked,
		Verify:   DependencyBlocked,
		Archive:  DependencyBlocked,
	}
	if applyState == ApplyReady {
		dependencies.Apply = DependencyReady
	} else if applyState == ApplyAllDone {
		dependencies.Apply = DependencyAllDone
	}

	if verifyReportCurrent && coreReady && taskProgress.AllComplete && verifyReportPassing {
		dependencies.Verify = DependencyAllDone
	} else if coreReady && applyState == ApplyAllDone && (!verifyReportCurrent || remediationComplete) {
		dependencies.Verify = DependencyReady
	}
	if dependencies.Verify == DependencyAllDone && taskProgress.AllComplete {
		dependencies.Archive = DependencyReady
	}
	return dependencies
}

func artifactDependency(state ArtifactState) DependencyState {
	if state == ArtifactDone {
		return DependencyAllDone
	}
	return DependencyBlocked
}

func resolveNextRecommended(dependencies Dependencies, applyState ApplyState, verifyReportDone bool, remediation RemediationState) string {
	// Prefer apply over verify when there is still remaining implementation work.
	if dependencies.Apply == DependencyReady {
		return string(PhaseApply)
	}
	if remediation.Required {
		return "remediate"
	}
	if dependencies.Verify == DependencyReady {
		return string(PhaseVerify)
	}
	if applyState == ApplyAllDone && verifyReportDone && dependencies.Verify != DependencyAllDone {
		return string(PhaseVerify)
	}
	if dependencies.Verify == DependencyAllDone && applyState == ApplyAllDone {
		return string(PhaseArchive)
	}

	// Route toward the next missing planning artifact in dependency order.
	// Missing planning artifacts are the expected output of planning phases,
	// not genuine blockers. Reserve resolve-blockers for genuine anomalies.
	if dependencies.Proposal != DependencyAllDone {
		return string(PhasePropose)
	}
	if dependencies.Specs != DependencyAllDone {
		return string(PhaseSpec)
	}
	if dependencies.Design != DependencyAllDone {
		return string(PhaseDesign)
	}
	if dependencies.Tasks != DependencyAllDone {
		return string(PhaseTasks)
	}

	// Genuine anomaly: all planning artifacts are done but apply is still blocked.
	// This indicates a corrupted or ambiguous state that needs human intervention.
	return "resolve-blockers"
}

const runtimeRemediationVerifyRefreshInstruction = "A passing native remediation settlement completed after the persisted verification report; run fresh verification and persist a report bound after that settlement before archive."

// artifactLocator renders the locators the native surface already resolved
// for one artifact. #3814: phase instructions must name what Resolve produced
// for the ACTIVE artifact store -- an OpenSpec path or an Engram topic key --
// so a delegated actor never has to detect the store or guess a filename.
// Unresolved is explicit rather than silently omitted: a phase actor that
// cannot be told where its input lives must fail loudly, not read the wrong
// store.
func artifactLocator(locators []string) string {
	if len(locators) == 0 {
		return "<unresolved>"
	}
	return strings.Join(locators, ", ")
}

// artifactReadVerb names how the active store's locators are read. It is the
// second half of the locator contract: the brief carries WHERE the artifact
// lives and HOW to read it, which is exactly what the phase agent contracts
// used to hardcode per store.
func artifactReadVerb(store ArtifactStore) string {
	switch store {
	case ArtifactStoreEngram:
		return "read the Engram observation named by that locator"
	case ArtifactStoreHybrid:
		return "read the file at that path when the locator is a path, or the Engram observation when it is a topic key"
	default:
		return "read the file at that path"
	}
}

func renderPhaseInstructions(status Status) PhaseInstructions {
	change := "<unresolved>"
	if status.ChangeName != nil {
		change = *status.ChangeName
	}
	runtimeInstructions := nativeRuntimeInstructions(status, change)
	applyInstructions := []string{
		fmt.Sprintf("Change: %s", change),
		fmt.Sprintf("State: %s", status.Dependencies.Apply),
		"Read proposal, specs, design, and tasks before editing.",
		fmt.Sprintf("Artifact store: %s; %s.", status.ArtifactStore, artifactReadVerb(status.ArtifactStore)),
		fmt.Sprintf("Tasks locator: %s", artifactLocator(status.ArtifactPaths.Tasks)),
		fmt.Sprintf("Apply-progress locator: %s", artifactLocator(status.ArtifactPaths.ApplyProgress)),
		"Resume from the apply-progress locator when it resolves; implement only unchecked tasks and mark each complete at the tasks locator as work completes.",
	}
	verifyInstructions := []string{
		fmt.Sprintf("Change: %s", change),
		fmt.Sprintf("State: %s", status.Dependencies.Verify),
		"Verify implementation against proposal, specs, design, and task completion.",
		"Run final verification only after every task is complete; apply-progress never makes final verification ready.",
	}
	if status.verifyRefreshReason != "" {
		verifyInstructions = append(verifyInstructions, status.verifyRefreshReason)
	}
	remediateInstructions := []string{
		fmt.Sprintf("Change: %s", change),
		"Remediation follows ordinary SDD failed-evidence accounting.",
		"Bind focused tests, runtime harness evidence, and rollback evidence to the exact failed evidence revision.",
		"A bare remediation envelope or stale failed revision never completes remediation.",
		"A passing remediation requires fresh independent verification before archive.",
	}
	return PhaseInstructions{
		Apply:     append(applyInstructions, runtimeInstructions...),
		Verify:    append(verifyInstructions, runtimeInstructions...),
		Remediate: append(remediateInstructions, runtimeInstructions...),
		Archive: []string{
			fmt.Sprintf("Change: %s", change),
			fmt.Sprintf("State: %s", status.Dependencies.Archive),
			fmt.Sprintf("Verify-report locator: %s", artifactLocator(status.ArtifactPaths.VerifyReport)),
			fmt.Sprintf("Archive only when a verify report resolves at that locator (%s) and every task is complete.", artifactReadVerb(status.ArtifactStore)),
		},
	}
}

func nativeRuntimeInstructions(status Status, change string) []string {
	workspace := status.ActionContext.WorkspaceRoot
	instructions := []string{
		fmt.Sprintf("Before any runtime-bearing apply, verify, or remediation launch, run `gentle-ai sdd-attempt acquire --cwd %s --change %q --request-id \"<unique-request-id>\" --work-unit \"<label>\" --evidence-goal \"<stable-goal>\" --max-attempts <count> --max-changed-lines <count>`.", pathquote.Quote(workspace), change),
		"Launch only for state proceed and retain its opaque token. State blocked or complete stops the launch; full runtime status is a diagnostic escape hatch, not normal model context.",
		fmt.Sprintf("After a failed or passed run, call `gentle-ai sdd-attempt settle --cwd %s --change %q --token \"<acquire-token>\" --request-id \"<unique-request-id>\" --outcome <passed|failed> --evidence-revision <sha256> --diagnosis \"<proven-diagnosis>\" --harness-disposition <reused|invalidated> --cleanup-evidence \"<evidence>\" --process-evidence \"<evidence>\"`.", pathquote.Quote(workspace), change),
		fmt.Sprintf("After an interrupted run, call `gentle-ai sdd-attempt settle --cwd %s --change %q --token \"<acquire-token>\" --request-id \"<unique-request-id>\" --outcome interrupted --diagnosis \"<proven-diagnosis>\" --harness-disposition <reused|invalidated> --cleanup-evidence \"<evidence>\" --process-evidence \"<evidence>\"` and omit --evidence-revision.", pathquote.Quote(workspace), change),
		"Treat settle state proceed as permission for another bounded acquire, blocked as a hard stop, and complete as terminal. Reset is exceptional, requires an explicit maintainer scope decision, and is never automatic.",
	}
	if status.RemediationState.Required && status.RuntimeStatus != nil && status.RuntimeStatus.Objective != nil {
		evidence, found := runtimeChainFailedEvidence(status.RuntimeStatus.Attempts)
		if found {
			objective := status.RuntimeStatus.Objective
			instructions = append(instructions,
				fmt.Sprintf("For failed SDD evidence %s, run `gentle-ai sdd-attempt acquire --cwd %s --change %q --request-id \"<unique-request-id>\" --work-unit %q --evidence-goal %q --max-attempts %d --max-changed-lines %d --remediates-evidence-revision %s`.", evidence, pathquote.Quote(workspace), change, objective.WorkUnit, objective.EvidenceGoal, objective.MaxAttempts, objective.MaxChangedLines, evidence),
				fmt.Sprintf("After the candidate changes, settle that token with `--remediates-evidence-revision %s`; fresh independent verification is required before archive.", evidence),
			)
		}
	}
	return append(instructions, liveRuntimeAttemptInstructions(status)...)
}

// liveRuntimeAttemptInstructions names the continuation for an attempt that is
// already active. This is the informational half of #2463: status stops
// blocking a live attempt, and instead hands the caller the exact exit compact
// acquire itself names, including that attempt's own token. A caller that owns
// the token continues it; a caller that does not learns it must settle first.
// The text is the predicate's, never a paraphrase.
func liveRuntimeAttemptInstructions(status Status) []string {
	if status.RuntimeStatus == nil {
		return nil
	}
	readiness, terminal := runtimeReadiness(runtimeReadinessInput{
		Status: *status.RuntimeStatus, AttemptTokens: status.runtimeAttemptTokens,
	})
	if !terminal || readiness.Reason != CompactBlockActiveAttempt {
		return nil
	}
	return []string{"An attempt is already active for this change: " + readiness.Exit}
}

// nonPhaseRoutingInstructions renders actionable continuations for
// next_recommended values that are routing states rather than SDD phases.
// nextRecommendedPhase() only recognizes real phases, so without this every
// routing-only next value (for example, "select-change") would render its
// blocked reason with no way out — the blocked reason is the entire guidance.
func nonPhaseRoutingInstructions(status Status) ([]string, bool) {
	switch status.NextRecommended {
	case "select-change":
		return []string{
			"",
			"### Next Selection Operation",
			fmt.Sprintf("- Rerun with an explicit change name from Blocked Reasons above: `gentle-ai sdd-status --cwd %s <change-name>` or `gentle-ai sdd-continue --cwd %s <change-name>`.", pathquote.Quote(status.ActionContext.WorkspaceRoot), pathquote.Quote(status.ActionContext.WorkspaceRoot)),
		}, true
	case "archived":
		location := ""
		if status.Archived != nil {
			location = fmt.Sprintf(" at %s", status.Archived.Path)
		}
		return []string{
			"",
			"### Archived Change",
			fmt.Sprintf("- This change is already archived%s; no phase remains and nothing is blocked.", location),
			fmt.Sprintf("- Start new work with a fresh change: `gentle-ai sdd-status --cwd %s` lists what is active.", pathquote.Quote(status.ActionContext.WorkspaceRoot)),
		}, true
	default:
		return nil, false
	}
}

func nextRecommendedPhase(next string) (Phase, bool) {
	switch Phase(next) {
	case PhasePropose, PhaseSpec, PhaseDesign, PhaseTasks, PhaseApply, PhaseVerify, PhaseRemediate, PhaseArchive:
		return Phase(next), true
	default:
		return "", false
	}
}

func dependencyForPhase(status Status, phase Phase) DependencyState {
	switch phase {
	case PhasePropose:
		return status.Dependencies.Proposal
	case PhaseSpec:
		return status.Dependencies.Specs
	case PhaseDesign:
		return status.Dependencies.Design
	case PhaseTasks:
		return status.Dependencies.Tasks
	case PhaseApply:
		return status.Dependencies.Apply
	case PhaseVerify:
		return status.Dependencies.Verify
	case PhaseRemediate:
		return status.Dependencies.Verify
	case PhaseArchive:
		return status.Dependencies.Archive
	default:
		return DependencyBlocked
	}
}

func instructionsForPhase(status Status, phase Phase) []string {
	instructions := status.PhaseInstructions
	if instructions == nil {
		rendered := renderPhaseInstructions(status)
		instructions = &rendered
	}

	switch phase {
	case PhasePropose, PhaseSpec, PhaseDesign, PhaseTasks:
		return planningInstructionsForPhase(status, phase)
	case PhaseApply:
		return instructions.Apply
	case PhaseVerify:
		return instructions.Verify
	case PhaseRemediate:
		return instructions.Remediate
	case PhaseArchive:
		return instructions.Archive
	default:
		return []string{"Unknown native SDD phase; return blockers and request a valid phase."}
	}
}

func planningInstructionsForPhase(status Status, phase Phase) []string {
	change := "<unresolved>"
	if status.ChangeName != nil {
		change = *status.ChangeName
	}
	switch phase {
	case PhasePropose:
		return []string{
			fmt.Sprintf("Change: %s", change),
			"Write proposal.md in the change directory.",
			"Capture intent, scope, and approach before writing specs.",
		}
	case PhaseSpec:
		return []string{
			fmt.Sprintf("Change: %s", change),
			"Read proposal.md before writing specs.",
			"Create spec.md or specs/<domain>/spec.md with requirements and scenarios.",
		}
	case PhaseDesign:
		return []string{
			fmt.Sprintf("Change: %s", change),
			"Read proposal.md before writing design.",
			"Write design.md with architecture decisions and implementation approach.",
		}
	case PhaseTasks:
		return []string{
			fmt.Sprintf("Change: %s", change),
			"Read spec and design before writing tasks.",
			"Write tasks.md with an ordered checklist of implementation tasks.",
		}
	default:
		return []string{"Unknown planning phase."}
	}
}

func firstPath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[0]
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
