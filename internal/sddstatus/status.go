package sddstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const SchemaName = "gentle-ai.sdd-status"
const SchemaVersion = 1

type ArtifactStore string

const (
	ArtifactStoreOpenSpec ArtifactStore = "openspec"
	ArtifactStoreEngram   ArtifactStore = "engram"
	ArtifactStoreNone     ArtifactStore = "none"
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
	ReviewPolicy  []string `json:"reviewPolicy"`
	ReviewLedger  []string `json:"reviewLedger"`
	ReviewReceipt []string `json:"reviewReceipt"`
	ReviewBundle  []string `json:"reviewBundle"`
	ReviewContext []string `json:"reviewContext"`
	ReviewState   []string `json:"reviewState"`
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

// RemediationState describes bounded correction eligibility for a failed
// verification verdict. CorrectionBudgetRemaining and CorrectionBudgetTotal
// are deliberately named to be unambiguous with the review-integration
// contracts' unrelated `correction_budget` field (the frozen total assigned
// at review start, see internal/cli/review_start_contract.go and
// review_status_contract.go): neither sddstatus field is ever named plain
// "correctionBudget" on the wire, so a consumer reading both surfaces cannot
// mistake a remaining-budget value for a frozen-total value or vice versa.
type RemediationState struct {
	Required               bool   `json:"required"`
	Complete               bool   `json:"complete"`
	FailedEvidenceRevision string `json:"failedEvidenceRevision"`
	LineageID              string `json:"lineageId"`
	Generation             int    `json:"generation"`
	FixBatch               int    `json:"fixBatch"`
	// CorrectionBudgetRemaining is CorrectionBudgetTotal minus the compact
	// review authority's CumulativeCorrectionLines already charged against
	// it: the correction-line budget still available for this remediation
	// attempt. Zero when remediation is not compact-bound or not required.
	CorrectionBudgetRemaining int `json:"correctionBudgetRemaining,omitempty"`
	// CorrectionBudgetTotal is the frozen total correction-line budget
	// assigned to the compact review authority at review start
	// (reviewtransaction.CompactState.CorrectionBudget), unaffected by lines
	// already spent. Zero when remediation is not compact-bound or not
	// required.
	CorrectionBudgetTotal int    `json:"correctionBudgetTotal,omitempty"`
	Reason                string `json:"reason"`
}

type ReviewGateState struct {
	Result reviewtransaction.GateResult `json:"result"`
	Reason string                       `json:"reason"`
	// Delivery names what governs the change when the review gate itself
	// cannot, mirroring the delivery gate's own disposition field
	// (internal/cli.reviewDeliveryDisposition). It is set only while the
	// review-driven-development kill switch is off and the change has no
	// review authority of its own, where it reports
	// RDDDeliveryDisabledUnmanaged: no review governs this change and it
	// closes under ordinary repository policy rather than under a receipt.
	//
	// It is deliberately a separate field rather than a fifth
	// reviewtransaction.GateResult. Result keeps reporting only the four
	// documented gate results, so every consumer that archives on
	// `reviewGate.result: allow` keeps refusing to read this as an approval,
	// and every enabled path leaves Delivery empty, which `omitempty` keeps
	// off the wire exactly as before.
	Delivery reviewtransaction.RDDDelivery `json:"delivery,omitempty"`
}

type Status struct {
	SchemaName        string                         `json:"schemaName"`
	SchemaVersion     int                            `json:"schemaVersion"`
	ChangeName        *string                        `json:"changeName"`
	ArtifactStore     ArtifactStore                  `json:"artifactStore"`
	PlanningHome      PlanningHome                   `json:"planningHome"`
	ChangeRoot        *string                        `json:"changeRoot"`
	ArtifactPaths     ArtifactPaths                  `json:"artifactPaths"`
	ContextFiles      ArtifactPaths                  `json:"contextFiles"`
	Artifacts         map[string]ArtifactState       `json:"artifacts"`
	TaskProgress      TaskProgress                   `json:"taskProgress"`
	Dependencies      Dependencies                   `json:"dependencies"`
	ApplyState        ApplyState                     `json:"applyState"`
	ActionContext     ActionContext                  `json:"actionContext"`
	Relationships     Relationships                  `json:"relationships"`
	RemediationState  RemediationState               `json:"remediationState"`
	RuntimeStatus     *RuntimeStatus                 `json:"runtimeStatus,omitempty"`
	ReviewGate        *ReviewGateState               `json:"reviewGate,omitempty"`
	ReviewTransaction *reviewtransaction.Transaction `json:"reviewTransaction,omitempty"`
	PhaseInstructions *PhaseInstructions             `json:"phaseInstructions,omitempty"`
	NextRecommended   string                         `json:"nextRecommended"`
	BlockedReasons    []string                       `json:"blockedReasons"`
}

type ResolveOptions struct {
	CWD                 string
	WorkspaceRoot       string
	ChangeName          string
	IncludeInstructions bool
	// ReviewDisabled records that the user's review-driven-development kill
	// switch is off for this clone. While it is off review-driven development
	// does not exist, so it must have no implications: the archive gate never
	// demands a terminal review receipt the operator could not obtain anyway
	// (review/start is refused while the switch is off), which would otherwise
	// loop an orchestrator forever on `nextRecommended: "resolve-review"`.
	//
	// It removes only the IMPLICIT demand. A change that carries an explicit
	// review receipt asked for review-driven development to act, so that
	// receipt is still validated in full: an approved one still governs and a
	// scope-changed, escalated, or invalidated one still blocks. Nothing here
	// approves, advances, or invents review authority.
	//
	// The zero value enforces, so any caller that does not resolve the switch
	// keeps today's behavior. The switch itself is read in the CLI layer, which
	// owns the single source of truth for both of its sources; an unreadable
	// switch is not a disabled switch and resolves to false.
	ReviewDisabled bool
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
	var parsed CommandArgs
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
	if contract == StatusContractV1 {
		return nil
	}
	return fmt.Errorf("unsupported sdd-status contract %q; supported contract is %s", contract, StatusContractV1)
}

func ListActiveOpenSpecChanges(cwd string) ([]string, error) {
	root, err := absOrCWD(cwd)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "changes"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	changes := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "archive" {
			changes = append(changes, entry.Name())
		}
	}
	sort.Strings(changes)
	return changes, nil
}

func Resolve(options ResolveOptions) (Status, error) {
	workspaceRoot, err := resolveWorkspaceRoot(options)
	if err != nil {
		return Status{}, err
	}
	planningHome := filepath.Join(workspaceRoot, "openspec")
	changesDir := filepath.Join(planningHome, "changes")
	activeChanges, err := ListActiveOpenSpecChanges(workspaceRoot)
	if err != nil {
		return Status{}, err
	}

	changeName := strings.TrimSpace(options.ChangeName)
	if changeName == "" {
		switch len(activeChanges) {
		case 0:
			if status, ok, err := resolveEngramStatus(workspaceRoot, changeName, options.IncludeInstructions, options.ReviewDisabled); ok || err != nil {
				return status, err
			}
			return blockedStatus(workspaceRoot, nil, nil, "sdd-new", []string{"No active OpenSpec changes found under openspec/changes."}, options.IncludeInstructions), nil
		case 1:
			changeName = activeChanges[0]
		default:
			return blockedStatus(workspaceRoot, nil, nil, "select-change", []string{fmt.Sprintf("Change selection is ambiguous: %s.", strings.Join(activeChanges, ", "))}, options.IncludeInstructions), nil
		}
	}

	if !contains(activeChanges, changeName) {
		if status, ok, err := resolveEngramStatus(workspaceRoot, changeName, options.IncludeInstructions, options.ReviewDisabled); ok || err != nil {
			return status, err
		}
		return blockedStatus(workspaceRoot, &changeName, nil, "sdd-new", []string{fmt.Sprintf("Active OpenSpec change not found: %s.", changeName)}, options.IncludeInstructions), nil
	}

	changeRoot := filepath.Join(changesDir, changeName)
	artifactPaths, err := resolveArtifactPaths(changeRoot)
	if err != nil {
		return Status{}, err
	}
	artifacts := map[string]ArtifactState{
		"proposal":      singleArtifactState(artifactPaths.Proposal),
		"specs":         multiArtifactState(artifactPaths.Specs, filepath.Join(changeRoot, "specs")),
		"design":        singleArtifactState(artifactPaths.Design),
		"tasks":         singleArtifactState(artifactPaths.Tasks),
		"applyProgress": singleArtifactState(artifactPaths.ApplyProgress),
		"verifyReport":  singleArtifactState(artifactPaths.VerifyReport),
		"reviewLedger":  singleArtifactState(artifactPaths.ReviewLedger),
		"reviewReceipt": singleArtifactState(artifactPaths.ReviewReceipt),
		"reviewBundle":  singleArtifactState(artifactPaths.ReviewBundle),
		"reviewContext": singleArtifactState(artifactPaths.ReviewContext),
		"reviewState":   singleArtifactState(artifactPaths.ReviewState),
	}
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
	runtimeStatus, runtimeStatusErr := loadNativeRuntimeStatus(context.Background(), workspaceRoot, changeName)
	reviewState, reviewStateReason := readReviewTransaction(firstPath(artifactPaths.ReviewState), "")
	coreReady := artifacts["proposal"] == ArtifactDone && artifacts["specs"] == ArtifactDone && artifacts["design"] == ArtifactDone && artifacts["tasks"] == ArtifactDone && taskProgress.Total > 0
	applyState := resolveApplyState(coreReady, taskProgress)
	blockedReasons := artifactBlockedReasons(artifacts, taskProgress)
	bindingPresent := false
	if runtimeStatusErr == nil {
		var bindingPathErr error
		bindingPresent, bindingPathErr = bindingExists(context.Background(), workspaceRoot, changeName)
		if bindingPathErr != nil {
			runtimeStatusErr = fmt.Errorf("read native SDD runtime binding: %w", bindingPathErr)
		}
	}
	// Stale evidence under a live allow authority needs a fresh verification,
	// not legacy remediation classification against a missing transaction.
	var staleAllowAuthority *reviewAuthorityEvaluation
	if !bindingPresent && reviewState == nil && applyState == ApplyAllDone && artifacts["verifyReport"] == ArtifactDone && verifyResult.Stale {
		evaluation := resolveReviewAuthority(context.Background(), workspaceRoot, firstPath(artifactPaths.ReviewReceipt), "", changeName)
		if evaluation.Result == reviewtransaction.GateAllow {
			staleAllowAuthority = &evaluation
		} else {
			blockedReasons.genuine = append(blockedReasons.genuine, evaluation.Reason)
		}
	}
	runtimeRemediationComplete := nativeRuntimeCompletesRemediation(runtimeStatus, verifyResult)
	remediationRequired := staleAllowAuthority == nil && !runtimeRemediationComplete && artifacts["verifyReport"] == ArtifactDone && !verifyResult.Passing && applyState == ApplyAllDone
	compactRemediation := resolveCompactRemediationAuthority(
		context.Background(), workspaceRoot, changeName, bindingPresent, remediationRequired && reviewState == nil,
		firstPath(artifactPaths.ReviewReceipt), "",
	)
	remediationState := resolveBoundedRemediation(
		remediationRequired,
		verifyResult,
		reviewState,
		compactRemediation,
		reviewStateReason,
		readText(firstPath(artifactPaths.ApplyProgress)),
	)
	dependencies := resolveDependencies(artifacts, taskProgress, applyState, coreReady, verifyResult.Passing, remediationState.Complete)
	nextRecommended := resolveNextRecommended(dependencies, applyState, artifacts["verifyReport"] == ArtifactDone, remediationState)
	if staleAllowAuthority != nil {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		nextRecommended = "verify"
	}
	var boundGate *ReviewGateState
	bridge := compactPreVerifyBridge{}
	recoverable := authorityOnlyFailedReport(readText(firstPath(artifactPaths.VerifyReport)))
	if bindingPresent {
		_, evaluation, bindingErr := validateBoundReview(context.Background(), workspaceRoot, changeName)
		if bindingErr == nil {
			staleEvidence := artifacts["verifyReport"] == ArtifactDone && verifyResult.Stale && reviewState == nil
			if applyState == ApplyAllDone && (artifacts["verifyReport"] != ArtifactDone || staleEvidence || runtimeRemediationComplete) {
				dependencies.Verify = DependencyReady
				dependencies.Archive = DependencyBlocked
				nextRecommended = "verify"
				if staleEvidence || runtimeRemediationComplete {
					remediationState = RemediationState{}
				}
			}
			boundGate = &ReviewGateState{Result: evaluation.Result, Reason: "explicit bound compact authority exactly matches the current repository"}
		} else {
			dependencies.Verify = DependencyBlocked
			dependencies.Archive = DependencyBlocked
			nextRecommended = "resolve-review"
			blockedReasons.genuine = append(blockedReasons.genuine, bindingErr.Error())
		}
	} else if applyState == ApplyAllDone && (artifacts["verifyReport"] != ArtifactDone || recoverable) && compactBridgeableReviewArtifact(artifacts["reviewState"], reviewStateReason) {
		fields, _ := authorityFailureFields(readText(firstPath(artifactPaths.VerifyReport)))
		bridge = discoverCompactPreVerifyAuthority(context.Background(), workspaceRoot, changeName, fields["observed_authority_revision"])
	}
	if !bindingPresent && recoverable && bridge.Eligible && authorityChangedSinceReport(readText(firstPath(artifactPaths.VerifyReport)), bridge.Revision) {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		nextRecommended = "verify"
		remediationState = RemediationState{}
	}
	if remediationState.Reason != "" {
		blockedReasons.genuine = append(blockedReasons.genuine, remediationState.Reason)
	}
	if !bindingPresent {
		applyPreVerifyCompactBridgeRouting(&dependencies, &nextRecommended, &blockedReasons, applyState, artifacts["verifyReport"] == ArtifactDone, reviewState, bridge)
	}
	if !bindingPresent && !bridge.Eligible && !bridge.Relevant {
		applyPreVerifyReviewRouting(&dependencies, &nextRecommended, &blockedReasons, applyState, artifacts["verifyReport"] == ArtifactDone, reviewState, reviewStateReason)
	}

	status := baseStatus(workspaceRoot, &changeName, &changeRoot, nextRecommended, append([]string{}, blockedReasons.genuine...))
	status.ArtifactPaths = artifactPaths
	status.ContextFiles = artifactPaths
	status.Artifacts = artifacts
	status.TaskProgress = taskProgress
	status.Dependencies = dependencies
	status.ApplyState = applyState
	status.RemediationState = remediationState
	status.RuntimeStatus = runtimeStatus
	status.ReviewTransaction = reviewState
	if !bindingPresent {
		if staleAllowAuthority != nil {
			status.ReviewGate = &ReviewGateState{Result: staleAllowAuthority.Result, Reason: staleAllowAuthority.Reason}
		} else {
			applyReviewGate(
				&status,
				workspaceRoot,
				firstPath(artifactPaths.ReviewReceipt),
				"",
				options.ReviewDisabled,
			)
		}
	}
	if boundGate != nil {
		status.ReviewGate = boundGate
	}
	if runtimeStatusErr != nil {
		applyNativeRuntimeErrorRouting(&status, runtimeStatusErr)
	} else {
		applyNativeRuntimeRouting(&status)
	}
	status.BlockedReasons = blockedReasons.finalize(status.NextRecommended, status.BlockedReasons)
	if options.IncludeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status, nil
}

func loadNativeRuntimeStatus(ctx context.Context, workspaceRoot, changeName string) (*RuntimeStatus, error) {
	// SDD status remains useful for non-Git planning fixtures and repositories.
	// A native runtime chain cannot exist without a Git common-dir, so the
	// absence of a repository means there is no runtime authority to embed.
	if _, err := (reviewtransaction.SnapshotBuilder{Repo: workspaceRoot}).ResolveRepositoryRoot(ctx); err != nil {
		if workspaceHasGitMetadata(workspaceRoot) {
			return nil, fmt.Errorf("resolve Git repository for native SDD runtime authority: %w", err)
		}
		return nil, nil
	}
	store, err := OpenRuntimeStore(ctx, workspaceRoot, changeName)
	if err != nil {
		return nil, fmt.Errorf("open native SDD runtime authority: %w", err)
	}
	status, err := store.Status()
	if err != nil {
		return nil, fmt.Errorf("read native SDD runtime authority: %w", err)
	}
	return &status, nil
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

func nativeRuntimeCompletesRemediation(runtimeStatus *RuntimeStatus, verify verifyResultEvaluation) bool {
	if runtimeStatus == nil || !runtimeStatus.Complete || runtimeStatus.DecisionRequired || runtimeStatus.ActiveAttempt != nil ||
		runtimeStatus.Binding == nil || verify.EvidenceRevision == "" || len(runtimeStatus.Attempts) == 0 {
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
		"native SDD runtime authority is unreadable and execution is blocked: %v; do not launch another actor or edit the Git-common-dir authority manually; inspect `gentle-ai sdd-attempt status --cwd %q --change %q` and require explicit maintainer recovery",
		runtimeErr, status.ActionContext.WorkspaceRoot, change,
	)
	status.Dependencies.Apply = DependencyBlocked
	status.Dependencies.Verify = DependencyBlocked
	status.Dependencies.Archive = DependencyBlocked
	status.NextRecommended = "resolve-blockers"
	status.BlockedReasons = append(status.BlockedReasons, reason)
}

func applyNativeRuntimeRouting(status *Status) {
	if status == nil || status.RuntimeStatus == nil {
		return
	}
	runtimeStatus := status.RuntimeStatus
	change := runtimeStatus.Change
	if status.ChangeName != nil {
		change = *status.ChangeName
	}
	var reason string
	switch {
	case runtimeStatus.DecisionRequired:
		reason = fmt.Sprintf(
			"native SDD runtime revision %s requires an explicit maintainer scope decision before more execution; inspect `gentle-ai sdd-attempt status --cwd %q --change %q`, then use `gentle-ai sdd-attempt reset --cwd %q --change %q --expected-revision %q --request-id \"<unique-request-id>\" --reason \"<maintainer-approved-reason>\" --actor \"<maintainer>\"` only when that reset is authorized",
			runtimeStatus.Revision, status.ActionContext.WorkspaceRoot, change,
			status.ActionContext.WorkspaceRoot, change, runtimeStatus.Revision,
		)
	case runtimeStatus.ActiveAttempt != nil:
		reason = fmt.Sprintf(
			"native SDD runtime attempt %d is active at revision %s; do not launch another continuation and finish the charged attempt with `gentle-ai sdd-attempt finish --cwd %q --change %q --expected-revision %q` plus the required outcome, evidence, diagnosis, harness, cleanup, and process fields%s",
			runtimeStatus.ActiveAttempt.Ordinal, runtimeStatus.Revision,
			status.ActionContext.WorkspaceRoot, change, runtimeStatus.Revision,
			nativeRuntimeRemediationFlagAdvice(runtimeStatus),
		)
	default:
		return
	}
	status.Dependencies.Apply = DependencyBlocked
	status.Dependencies.Verify = DependencyBlocked
	status.Dependencies.Archive = DependencyBlocked
	status.NextRecommended = "resolve-blockers"
	if !contains(status.BlockedReasons, reason) {
		status.BlockedReasons = append(status.BlockedReasons, reason)
	}
}

// nativeRuntimeRemediationFlagAdvice completes the flag set the active-attempt
// blocker advertises. The six ordinary finish fields close an UNBOUND attempt;
// a bound attempt whose candidate moved during the attempt is refused until it
// also carries the remediation trio, so advertising the short set routes the
// caller straight into that refusal. The values are the ones the ledger
// already holds, and the unobvious part — that the bound lineage is itself an
// acceptable --successor-lineage — is stated rather than left to be guessed.
//
// It stays silent for an unbound attempt: the trio does not apply there, and a
// flag set that names an inapplicable route is its own kind of dead end.
func nativeRuntimeRemediationFlagAdvice(runtimeStatus *RuntimeStatus) string {
	if runtimeStatus == nil || runtimeStatus.Binding == nil {
		return ""
	}
	// Only the caller knows which evidence a correction repairs when the
	// objective has not recorded a failed revision yet.
	remediates := runtimeStatus.EvidenceRevision
	if remediates == "" {
		remediates = "<repaired-evidence-sha256>"
	}
	return fmt.Sprintf(
		"; a bound attempt that changed the candidate cannot close as passed on those alone and must also pass --expected-binding-revision %q --successor-lineage %q --remediates-evidence-revision %s, where the bound lineage is itself the successor once the corrected candidate is approved on it",
		runtimeStatus.Binding.Revision, runtimeStatus.Binding.Lineage, runtimeRemediatesArgument(remediates),
	)
}

func authorityOnlyFailedReport(report string) bool {
	fields, ok := authorityFailureFields(report)
	return ok && fields["authority_only_failure"] == "true" && fields["missing_review_authority"] == "true" &&
		fields["substantive_failure"] == "false" && fields["command_failed"] == "false"
}

func authorityChangedSinceReport(report, revision string) bool {
	fields, ok := authorityFailureFields(report)
	return ok && fields["observed_authority_revision"] != revision
}

func authorityFailureFields(report string) (map[string]string, bool) {
	parsed, reason := parseVerifyReport(report)
	if reason != "" || !parsed.AuthorityOnly {
		return nil, false
	}
	return parsed.Fields, true
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
			return blockedEngramStatus(workspaceRoot, nil, "select-change", []string{fmt.Sprintf("Engram change selection is ambiguous: %s.", strings.Join(changes, ", "))}, includeInstructions), true, nil
		}
	}

	artifactsByType := engramArtifactsForChange(observations, project, changeName)
	if len(artifactsByType) == 0 {
		return Status{}, false, nil
	}

	artifactPaths := engramArtifactPaths(changeName, artifactsByType)
	artifacts := map[string]ArtifactState{
		"proposal":      engramArtifactState(artifactsByType["proposal"]),
		"specs":         engramArtifactState(artifactsByType["spec"]),
		"design":        engramArtifactState(artifactsByType["design"]),
		"tasks":         engramArtifactState(artifactsByType["tasks"]),
		"applyProgress": engramArtifactState(artifactsByType["apply-progress"]),
		"verifyReport":  engramArtifactState(artifactsByType["verify-report"]),
		"reviewLedger":  engramArtifactState(artifactsByType["review/ledger"]),
		"reviewPolicy":  engramArtifactState(artifactsByType["review/policy"]),
		"reviewReceipt": engramArtifactState(artifactsByType["review/receipt"]),
		"reviewBundle":  engramArtifactState(artifactsByType["review/chain-bundle"]),
		"reviewContext": engramArtifactState(artifactsByType["review/gate-context"]),
		"reviewState":   engramArtifactState(artifactsByType["review/transaction"]),
	}
	taskProgress := countTaskProgressText(artifactsByType["tasks"].Content)
	specCounts := countSpecRequirementsAndScenarios([]string{artifactsByType["spec"].Content})
	verifyResult := parseVerifyResult(artifactsByType["verify-report"].Content, specCounts)
	runtimeStatus, runtimeStatusErr := loadNativeRuntimeStatus(context.Background(), workspaceRoot, changeName)
	reviewState, reviewStateReason := readReviewTransaction("", artifactsByType["review/transaction"].Content)
	coreReady := artifacts["proposal"] == ArtifactDone && artifacts["specs"] == ArtifactDone && artifacts["design"] == ArtifactDone && artifacts["tasks"] == ArtifactDone && taskProgress.Total > 0
	applyState := resolveApplyState(coreReady, taskProgress)
	blockedReasons := artifactBlockedReasons(artifacts, taskProgress)
	bindingPresent := false
	if runtimeStatusErr == nil {
		var bindingPathErr error
		bindingPresent, bindingPathErr = bindingExists(context.Background(), workspaceRoot, changeName)
		if bindingPathErr != nil {
			runtimeStatusErr = fmt.Errorf("read native SDD runtime binding: %w", bindingPathErr)
		}
	}
	// Stale evidence under a live allow authority needs a fresh verification,
	// not legacy remediation classification against a missing transaction.
	var staleAllowAuthority *reviewAuthorityEvaluation
	if !bindingPresent && reviewState == nil && applyState == ApplyAllDone && artifacts["verifyReport"] == ArtifactDone && verifyResult.Stale {
		evaluation := resolveReviewAuthority(context.Background(), workspaceRoot, "", artifactsByType["review/receipt"].Content, changeName)
		if evaluation.Result == reviewtransaction.GateAllow {
			staleAllowAuthority = &evaluation
		} else {
			blockedReasons.genuine = append(blockedReasons.genuine, evaluation.Reason)
		}
	}
	runtimeRemediationComplete := nativeRuntimeCompletesRemediation(runtimeStatus, verifyResult)
	remediationRequired := staleAllowAuthority == nil && !runtimeRemediationComplete && artifacts["verifyReport"] == ArtifactDone && !verifyResult.Passing && applyState == ApplyAllDone
	compactRemediation := resolveCompactRemediationAuthority(
		context.Background(), workspaceRoot, changeName, bindingPresent, remediationRequired && reviewState == nil,
		"", artifactsByType["review/receipt"].Content,
	)
	remediationState := resolveBoundedRemediation(
		remediationRequired,
		verifyResult,
		reviewState,
		compactRemediation,
		reviewStateReason,
		artifactsByType["apply-progress"].Content,
	)
	if remediationState.Reason != "" {
		blockedReasons.genuine = append(blockedReasons.genuine, remediationState.Reason)
	}
	dependencies := resolveDependencies(artifacts, taskProgress, applyState, coreReady, verifyResult.Passing, remediationState.Complete)
	nextRecommended := resolveNextRecommended(dependencies, applyState, artifacts["verifyReport"] == ArtifactDone, remediationState)
	if staleAllowAuthority != nil {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		nextRecommended = "verify"
	}
	var boundGate *ReviewGateState
	bridge := compactPreVerifyBridge{}
	if bindingPresent {
		binding, bindingErr := loadEffectiveReviewBinding(context.Background(), workspaceRoot, changeName)
		if bindingErr == nil {
			bindingErr = validateRuntimeBoundCandidate(context.Background(), workspaceRoot, binding, binding.GateContext.CandidateTree)
		}
		if bindingErr == nil {
			staleEvidence := artifacts["verifyReport"] == ArtifactDone && verifyResult.Stale && reviewState == nil
			if applyState == ApplyAllDone && (artifacts["verifyReport"] != ArtifactDone || staleEvidence || runtimeRemediationComplete) {
				dependencies.Verify = DependencyReady
				dependencies.Archive = DependencyBlocked
				nextRecommended = "verify"
				if staleEvidence || runtimeRemediationComplete {
					remediationState = RemediationState{}
				}
			}
			boundGate = &ReviewGateState{Result: reviewtransaction.GateAllow, Reason: "explicit bound compact authority exactly matches the current repository"}
		} else {
			dependencies.Verify = DependencyBlocked
			dependencies.Archive = DependencyBlocked
			nextRecommended = "resolve-review"
			blockedReasons.genuine = append(blockedReasons.genuine, bindingErr.Error())
		}
	} else {
		if applyState == ApplyAllDone && artifacts["verifyReport"] != ArtifactDone && compactBridgeableReviewArtifact(artifacts["reviewState"], reviewStateReason) {
			bridge = discoverCompactPreVerifyAuthority(context.Background(), workspaceRoot, changeName, "")
		}
		applyPreVerifyCompactBridgeRouting(&dependencies, &nextRecommended, &blockedReasons, applyState, artifacts["verifyReport"] == ArtifactDone, reviewState, bridge)
		if !bridge.Eligible && !bridge.Relevant {
			applyPreVerifyReviewRouting(&dependencies, &nextRecommended, &blockedReasons, applyState, artifacts["verifyReport"] == ArtifactDone, reviewState, reviewStateReason)
		}
	}

	changeRoot := fmt.Sprintf("engram:sdd/%s", changeName)
	status := baseStatus(workspaceRoot, &changeName, &changeRoot, nextRecommended, append([]string{}, blockedReasons.genuine...))
	status.ArtifactStore = ArtifactStoreEngram
	status.PlanningHome = PlanningHome{Mode: ActionModeRepoLocal, Path: "engram:sdd"}
	status.ArtifactPaths = artifactPaths
	status.ContextFiles = artifactPaths
	status.Artifacts = artifacts
	status.TaskProgress = taskProgress
	status.Dependencies = dependencies
	status.ApplyState = applyState
	status.RemediationState = remediationState
	status.RuntimeStatus = runtimeStatus
	status.ReviewTransaction = reviewState
	if !bindingPresent {
		if staleAllowAuthority != nil {
			status.ReviewGate = &ReviewGateState{Result: staleAllowAuthority.Result, Reason: staleAllowAuthority.Reason}
		} else {
			applyReviewGate(
				&status,
				workspaceRoot,
				"",
				artifactsByType["review/receipt"].Content,
				reviewDisabled,
			)
		}
	}
	if boundGate != nil {
		status.ReviewGate = boundGate
	}
	if runtimeStatusErr != nil {
		applyNativeRuntimeErrorRouting(&status, runtimeStatusErr)
	} else {
		applyNativeRuntimeRouting(&status)
	}
	status.BlockedReasons = blockedReasons.finalize(status.NextRecommended, status.BlockedReasons)
	if includeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status, true, nil
}

func compactBridgeableReviewArtifact(state ArtifactState, reason string) bool {
	return state == ArtifactMissing || reason == incompatibleReviewTransactionReason
}

func blockedEngramStatus(workspaceRoot string, changeName *string, next string, reasons []string, includeInstructions bool) Status {
	status := blockedStatus(workspaceRoot, changeName, nil, next, reasons, includeInstructions)
	status.ArtifactStore = ArtifactStoreEngram
	status.PlanningHome = PlanningHome{Mode: ActionModeRepoLocal, Path: "engram:sdd"}
	return status
}

func applyPreVerifyReviewRouting(dependencies *Dependencies, next *string, blockedReasons *blockerReasons, applyState ApplyState, verifyReportDone bool, transaction *reviewtransaction.Transaction, transactionReason string) {
	if applyState != ApplyAllDone || verifyReportDone {
		return
	}
	if transaction == nil {
		dependencies.Verify = DependencyBlocked
		*next = "review"
		blockedReasons.genuine = append(blockedReasons.genuine, "explicit bounded review/start(target) is required after apply before independent final verification: "+transactionReason)
		return
	}
	switch transaction.State {
	case reviewtransaction.StateReadyFinalVerification, reviewtransaction.StateFinalVerifying:
		dependencies.Verify = DependencyReady
		*next = "verify"
	case reviewtransaction.StateEscalated, reviewtransaction.StateApproved:
		dependencies.Verify = DependencyBlocked
		*next = "resolve-review"
		blockedReasons.genuine = append(blockedReasons.genuine, fmt.Sprintf("review transaction state %q cannot start missing final verification evidence", transaction.State))
	default:
		dependencies.Verify = DependencyBlocked
		*next = "review"
		blockedReasons.genuine = append(blockedReasons.genuine, fmt.Sprintf("bounded review transaction is %q; continue it without creating a new budget before final verification", transaction.State))
	}
}

func applyPreVerifyCompactBridgeRouting(dependencies *Dependencies, next *string, blockedReasons *blockerReasons, applyState ApplyState, verifyReportDone bool, transaction *reviewtransaction.Transaction, bridge compactPreVerifyBridge) {
	if applyState != ApplyAllDone || verifyReportDone || transaction != nil {
		return
	}
	if bridge.Eligible {
		dependencies.Verify = DependencyReady
		dependencies.Archive = DependencyBlocked
		*next = "verify"
		return
	}
	if bridge.Relevant {
		dependencies.Verify = DependencyBlocked
		dependencies.Archive = DependencyBlocked
		*next = "resolve-review"
		blockedReasons.genuine = append(blockedReasons.genuine, bridge.Reason)
	}
}

func shouldTryEngram(workspaceRoot string) bool {
	if os.Getenv("GENTLE_AI_SDD_STATUS_ENGRAM") != "" {
		return true
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".engram")); err == nil {
		return true
	}
	for _, path := range []string{filepath.Join(workspaceRoot, "openspec", "config.yaml"), filepath.Join(workspaceRoot, "openspec", "config.yml")} {
		content, err := os.ReadFile(path)
		if err == nil && configMentionsEngram(string(content)) {
			return true
		}
	}
	return false
}

func configMentionsEngram(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if strings.HasPrefix(trimmed, "artifact_store:") || strings.HasPrefix(trimmed, "artifactStore:") {
			return strings.Contains(strings.ToLower(trimmed), "engram") || strings.Contains(strings.ToLower(trimmed), "hybrid")
		}
	}
	return false
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
	config, err := os.ReadFile(filepath.Join(workspaceRoot, ".git", "config"))
	if err == nil {
		if project := projectFromGitConfig(string(config)); project != "" {
			return project
		}
	}
	return strings.ToLower(filepath.Base(workspaceRoot))
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

var engramTitlePattern = regexp.MustCompile(`^sdd/([^/]+)/(proposal|spec|design|tasks|apply-progress|verify-report|review/(?:transaction|policy|ledger|receipt|chain-bundle|gate-context)|state)$`)

func collectEngramChanges(observations []engramObservation, project string) []string {
	seen := map[string]bool{}
	for _, observation := range observations {
		if !engramObservationMatchesProject(observation, project) {
			continue
		}
		matches := engramTitlePattern.FindStringSubmatch(strings.TrimSpace(observation.Title))
		if len(matches) != 3 || matches[2] == "state" {
			continue
		}
		seen[matches[1]] = true
	}
	changes := make([]string, 0, len(seen))
	for change := range seen {
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
	if _, ok := artifacts["review/ledger"]; ok {
		paths.ReviewLedger = []string{fmt.Sprintf("sdd/%s/review/ledger", changeName)}
	}
	if _, ok := artifacts["review/policy"]; ok {
		paths.ReviewPolicy = []string{fmt.Sprintf("sdd/%s/review/policy", changeName)}
	}
	if _, ok := artifacts["review/receipt"]; ok {
		paths.ReviewReceipt = []string{fmt.Sprintf("sdd/%s/review/receipt", changeName)}
	}
	if _, ok := artifacts["review/chain-bundle"]; ok {
		paths.ReviewBundle = []string{fmt.Sprintf("sdd/%s/review/chain-bundle", changeName)}
	}
	if _, ok := artifacts["review/gate-context"]; ok {
		paths.ReviewContext = []string{fmt.Sprintf("sdd/%s/review/gate-context", changeName)}
	}
	if _, ok := artifacts["review/transaction"]; ok {
		paths.ReviewState = []string{fmt.Sprintf("sdd/%s/review/transaction", changeName)}
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

	jsonBytes, err := marshalStatusV1Indent(status)
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

	jsonBytes, err := marshalStatusV1Indent(status)
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

	jsonBytes, err := marshalStatusV1Indent(status)
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
	return root, nil
}

func absOrCWD(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return os.Getwd()
	}
	return filepath.Abs(path)
}

func blockedStatus(workspaceRoot string, changeName *string, changeRoot *string, next string, reasons []string, includeInstructions bool) Status {
	status := baseStatus(workspaceRoot, changeName, changeRoot, next, reasons)
	if includeInstructions {
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
	}
	return status
}

func baseStatus(workspaceRoot string, changeName *string, changeRoot *string, next string, reasons []string) Status {
	emptyPaths := emptyArtifactPaths()
	if reasons == nil {
		reasons = []string{}
	}
	return Status{
		SchemaName:    SchemaName,
		SchemaVersion: SchemaVersion,
		ChangeName:    changeName,
		ArtifactStore: ArtifactStoreOpenSpec,
		PlanningHome: PlanningHome{
			Mode: ActionModeRepoLocal,
			Path: filepath.Join(workspaceRoot, "openspec"),
		},
		ChangeRoot:    changeRoot,
		ArtifactPaths: emptyPaths,
		ContextFiles:  emptyPaths,
		Artifacts: map[string]ArtifactState{
			"proposal":      ArtifactMissing,
			"specs":         ArtifactMissing,
			"design":        ArtifactMissing,
			"tasks":         ArtifactMissing,
			"applyProgress": ArtifactMissing,
			"verifyReport":  ArtifactMissing,
			"reviewLedger":  ArtifactMissing,
			"reviewReceipt": ArtifactMissing,
			"reviewBundle":  ArtifactMissing,
			"reviewContext": ArtifactMissing,
			"reviewState":   ArtifactMissing,
		},
		TaskProgress: TaskProgress{},
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
			AllowedEditRoots: []string{workspaceRoot},
		},
		Relationships: Relationships{
			DependsOn:               []string{},
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
	paths.ReviewLedger = existingPath(filepath.Join(changeRoot, "reviews", "ledger.json"))
	paths.ReviewPolicy = existingPath(filepath.Join(changeRoot, "reviews", "policy.md"))
	paths.ReviewReceipt = existingPath(filepath.Join(changeRoot, "reviews", "receipt.json"))
	paths.ReviewBundle = existingPath(filepath.Join(changeRoot, "reviews", "chain-bundle.json"))
	paths.ReviewContext = existingPath(filepath.Join(changeRoot, "reviews", "gate-context.json"))
	paths.ReviewState = existingPath(filepath.Join(changeRoot, "reviews", "transaction.json"))

	specFiles, err := findSpecFiles(filepath.Join(changeRoot, "specs"))
	if err != nil {
		return ArtifactPaths{}, err
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
		ReviewLedger:  []string{},
		ReviewPolicy:  []string{},
		ReviewReceipt: []string{},
		ReviewBundle:  []string{},
		ReviewContext: []string{},
		ReviewState:   []string{},
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
	for _, line := range strings.Split(content, "\n") {
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

func artifactBlockedReasons(artifacts map[string]ArtifactState, taskProgress TaskProgress) blockerReasons {
	var reasons blockerReasons
	if artifacts["proposal"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, "proposal.md is missing or partial.")
	}
	if artifacts["specs"] != ArtifactDone {
		reasons.expectedPlanning = append(reasons.expectedPlanning, "specs/**/spec.md is missing or partial.")
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

func resolveApplyState(coreReady bool, taskProgress TaskProgress) ApplyState {
	if !coreReady {
		return ApplyBlocked
	}
	if taskProgress.AllComplete {
		return ApplyAllDone
	}
	return ApplyReady
}

func resolveDependencies(artifacts map[string]ArtifactState, taskProgress TaskProgress, applyState ApplyState, coreReady bool, verifyReportPassing bool, remediationComplete bool) Dependencies {
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

	verifyReportDone := artifacts["verifyReport"] == ArtifactDone
	if verifyReportDone && coreReady && taskProgress.AllComplete && verifyReportPassing {
		dependencies.Verify = DependencyAllDone
	} else if coreReady && applyState == ApplyAllDone && (!verifyReportDone || remediationComplete) {
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
	if dependencies.Verify == DependencyReady {
		return string(PhaseVerify)
	}
	if applyState == ApplyAllDone && verifyReportDone && dependencies.Verify != DependencyAllDone {
		if remediation.Required {
			return "remediate"
		}
		return "resolve-review"
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
		"Implement only unchecked tasks and update tasks.md checkboxes as work completes.",
	}
	verifyInstructions := []string{
		fmt.Sprintf("Change: %s", change),
		fmt.Sprintf("State: %s", status.Dependencies.Verify),
		"Verify implementation against proposal, specs, design, and task completion.",
		"Run final verification only after every task is complete; apply-progress never makes final verification ready.",
	}
	remediateInstructions := []string{
		fmt.Sprintf("Change: %s", change),
		"Remediation is allowed only when the persisted review transaction has remaining mode-specific budget.",
		"Bind focused tests, runtime harness evidence, and rollback evidence to the exact failed evidence revision.",
		"A bare remediation envelope or stale failed revision never completes remediation.",
		"A passing bound remediation MUST finish atomically with --expected-binding-revision, --successor-lineage, and --remediates-evidence-revision so the charged evidence and approved compact successor share one native HEAD CAS.",
	}
	return PhaseInstructions{
		Apply:     append(applyInstructions, runtimeInstructions...),
		Verify:    append(verifyInstructions, runtimeInstructions...),
		Remediate: append(remediateInstructions, runtimeInstructions...),
		Archive: []string{
			fmt.Sprintf("Change: %s", change),
			fmt.Sprintf("State: %s", status.Dependencies.Archive),
			"Archive only when verify-report.md exists and every task checkbox is complete.",
		},
	}
}

func nativeRuntimeInstructions(status Status, change string) []string {
	workspace := status.ActionContext.WorkspaceRoot
	return []string{
		fmt.Sprintf("Before any runtime-bearing apply, verify, or remediation launch, read `gentle-ai sdd-attempt status --cwd %q --change %q`; the Git-common-dir native ledger is authoritative for both OpenSpec and Engram.", workspace, change),
		fmt.Sprintf("When next_action is begin, consume the ordinal before launch with `gentle-ai sdd-attempt begin --cwd %q --change %q --expected-revision \"<runtime-revision>\" --request-id \"<unique-request-id>\" --work-unit \"<label>\" --evidence-goal \"<stable-goal>\" --max-attempts <count> --max-changed-lines <count>`.", workspace, change),
		fmt.Sprintf("After every passed, failed, or interrupted run, persist its evidence with `gentle-ai sdd-attempt finish --cwd %q --change %q --expected-revision \"<runtime-revision>\" --request-id \"<unique-request-id>\" --outcome <passed|failed|interrupted> --evidence-revision <sha256> --diagnosis \"<proven-diagnosis>\" --harness-disposition <reused|invalidated> --cleanup-evidence \"<evidence>\" --process-evidence \"<evidence>\"`.", workspace, change),
		"Never launch while active_attempt is populated or decision_required is true. `gentle-ai sdd-attempt reset` is an explicit maintainer scope decision, never an automatic counter reset.",
	}
}

// nonPhaseRoutingInstructions renders actionable continuations for
// next_recommended values that are routing states rather than SDD phases.
// nextRecommendedPhase() only recognizes real phases, so without this every
// routing-only next value (e.g. "resolve-review", "select-change") would
// render its blocked reason with no way out — the blocked reason IS the
// entire guidance. Where a continuation already exists elsewhere in this
// file (the "review" operation block), it is reused rather than duplicated.
func nonPhaseRoutingInstructions(status Status) ([]string, bool) {
	switch status.NextRecommended {
	case "review", "resolve-review":
		return []string{
			"",
			"### Next Review Operation",
			fmt.Sprintf("- Run `gentle-ai review start --cwd %q`; the facade derives intended untracked scope, lineage, tier, lenses, and correction budget from live Git.", status.ActionContext.WorkspaceRoot),
			"- Pass reviewer result and verification evidence to `gentle-ai review finalize`; do not hand-author lifecycle operation JSON.",
			"- Continue discovered authority instead of starting another budget, and reconcile existing terminal mirrors only after `gentle-ai review validate --gate post-apply` allows.",
		}, true
	case "select-change":
		return []string{
			"",
			"### Next Selection Operation",
			fmt.Sprintf("- Rerun with an explicit change name from Blocked Reasons above: `gentle-ai sdd-status --cwd %q <change-name>` or `gentle-ai sdd-continue --cwd %q <change-name>`.", status.ActionContext.WorkspaceRoot, status.ActionContext.WorkspaceRoot),
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
			"Create specs/<domain>/spec.md with requirements and scenarios.",
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
