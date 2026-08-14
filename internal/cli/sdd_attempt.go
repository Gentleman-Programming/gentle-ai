package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// RunSDDAttempt exposes the artifact-store-agnostic native runtime authority.
// Legacy operations and grant emit RuntimeStatus; acquire and settle emit its
// bounded orchestration projection.
func RunSDDAttempt(args []string, stdout io.Writer) error {
	return runSDDAttempt(context.Background(), args, stdout)
}

func runSDDAttempt(ctx context.Context, args []string, stdout io.Writer) error {
	if operation, requested := sddAttemptHelpOperation(args); requested {
		return renderSDDAttemptHelp(operation, stdout)
	}
	if len(args) == 0 {
		return fmt.Errorf("sdd-attempt requires %s", joinSDDAttemptOperations())
	}
	operation := args[0]
	if !validSDDAttemptOperation(operation) {
		return fmt.Errorf("unknown sdd-attempt operation %q; want one of %s", operation, joinSDDAttemptOperations())
	}
	if err := validateSDDAttemptOperationFlags(operation, args[1:]); err != nil {
		return err
	}

	flags := flag.NewFlagSet("sdd-attempt "+operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := registerSDDAttemptStringFlag(flags, operation, "cwd")
	change := registerSDDAttemptStringFlag(flags, operation, "change")
	destinationWorktree := registerSDDAttemptStringFlag(flags, operation, "destination-worktree")
	expected := registerSDDAttemptStringFlag(flags, operation, "expected-revision")
	token := registerSDDAttemptStringFlag(flags, operation, "token")
	requestID := registerSDDAttemptStringFlag(flags, operation, "request-id")
	workUnit := registerSDDAttemptStringFlag(flags, operation, "work-unit")
	evidenceGoal := registerSDDAttemptStringFlag(flags, operation, "evidence-goal")
	maxAttempts := registerSDDAttemptIntFlag(flags, operation, "max-attempts")
	maxChangedLines := registerSDDAttemptIntFlag(flags, operation, "max-changed-lines")
	outcome := registerSDDAttemptStringFlag(flags, operation, "outcome")
	evidenceRevision := registerSDDAttemptStringFlag(flags, operation, "evidence-revision")
	diagnosis := registerSDDAttemptStringFlag(flags, operation, "diagnosis")
	harnessDisposition := registerSDDAttemptStringFlag(flags, operation, "harness-disposition")
	cleanupEvidence := registerSDDAttemptStringFlag(flags, operation, "cleanup-evidence")
	processEvidence := registerSDDAttemptStringFlag(flags, operation, "process-evidence")
	expectedBindingRevision := registerSDDAttemptStringFlag(flags, operation, "expected-binding-revision")
	successorLineage := registerSDDAttemptStringFlag(flags, operation, "successor-lineage")
	remediatesEvidenceRevision := registerSDDAttemptStringFlag(flags, operation, "remediates-evidence-revision")
	reason := registerSDDAttemptStringFlag(flags, operation, "reason")
	actor := registerSDDAttemptStringFlag(flags, operation, "actor")
	var roots sddAttemptRootList
	registerSDDAttemptRootFlag(flags, operation, &roots)
	changeInstance := registerSDDAttemptStringFlag(flags, operation, "change-instance")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-attempt argument %q", flags.Arg(0))
	}
	// Identity/revision-shaped values are trimmed at this CLI boundary so
	// incidental leading/trailing whitespace from a shell or PowerShell
	// copy-paste (e.g. `Get-FileHash`/`shasum` output) does not itself cause
	// the sha256:<64-lowercase-hex> shape rejection this exists to prevent
	// (#2294). sddstatus stays a pure validator with no normalization of its
	// own.
	*expected = strings.TrimSpace(*expected)
	*token = strings.TrimSpace(*token)
	*changeInstance = strings.TrimSpace(*changeInstance)
	*evidenceRevision = strings.TrimSpace(*evidenceRevision)
	*expectedBindingRevision = strings.TrimSpace(*expectedBindingRevision)
	*remediatesEvidenceRevision = strings.TrimSpace(*remediatesEvidenceRevision)
	if missing := missingSDDAttemptOperationFlags(args[1:], operation, *outcome); len(missing) != 0 {
		return missingSDDAttemptOperationError(operation, missing)
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("sdd-attempt requires --cwd")
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("sdd-attempt requires --change")
	}

	reviewDisabled, err := reviewDrivenDevelopmentDisabled(ctx, *cwd)
	if err != nil {
		return fmt.Errorf("read review mode: %w", err)
	}
	store, err := sddstatus.OpenRuntimeStore(ctx, *cwd, *change)
	if err != nil {
		return fmt.Errorf("open native SDD runtime authority: %w", err)
	}
	// The kill switch reaches the runtime ledger here, at the one place that
	// knows how to read both of its sources. With reviews off, closing an
	// attempt must not demand a review obligation the operator has no way to
	// satisfy.
	store.ReviewDisabled = reviewDisabled
	var result any
	switch operation {
	case "status":
		// An optional --change-instance scopes the granted-roots projection
		// to that instance (#2540 S5); without it, status projects no granted
		// roots at all, which is the conservative containment for readers
		// that have not declared which change instance they serve.
		if *changeInstance != "" {
			if store, err = store.ForInstance(*changeInstance); err != nil {
				return fmt.Errorf("sdd-attempt status: %w", err)
			}
		}
		// Given the work-unit scope, status answers with the verdict acquire
		// would reach for that exact request instead of the ledger-only view
		// that reported next_action: "begin" while acquire blocked (#2114).
		if presentSDDAttemptFlags(args[1:], "work-unit", "evidence-goal", "max-attempts", "max-changed-lines") != 0 {
			result, err = store.AdmissionStatus(ctx, sddstatus.BeginAttemptRequest{
				RequestID: "sdd-attempt-status-probe", WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
				MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines,
			})
			break
		}
		result, err = store.Status()
	case "begin":
		result, err = store.Begin(ctx, sddstatus.BeginAttemptRequest{
			ExpectedRevision: *expected, RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
			MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines,
		})
	case "finish":
		remediationFlags := presentSDDAttemptFlags(args[1:], "expected-binding-revision", "successor-lineage", "remediates-evidence-revision")
		unmanagedRemediation := *expectedBindingRevision == "" && *successorLineage == "" && *remediatesEvidenceRevision != ""
		if remediationFlags != 0 && remediationFlags != 3 && !unmanagedRemediation {
			return errors.New("remediation successor requires --expected-binding-revision, --successor-lineage, and --remediates-evidence-revision together")
		}
		result, err = store.Finish(ctx, sddstatus.FinishAttemptRequest{
			ExpectedRevision: *expected, RequestID: *requestID, Outcome: sddstatus.AttemptOutcome(*outcome),
			EvidenceRevision: *evidenceRevision, Diagnosis: *diagnosis,
			HarnessDisposition: sddstatus.HarnessDisposition(*harnessDisposition),
			CleanupEvidence:    *cleanupEvidence, ProcessEvidence: *processEvidence,
			ExpectedBindingRevision: *expectedBindingRevision, SuccessorLineageID: *successorLineage,
			RemediatesEvidenceRevision: *remediatesEvidenceRevision,
		})
	case "handoff":
		result, err = store.HandoffCompact(ctx, sddstatus.CompactHandoffRequest{
			HandoffAttemptRequest: sddstatus.HandoffAttemptRequest{
				ExpectedRevision: *expected, RequestID: *requestID, DestinationWorktree: *destinationWorktree,
			},
		})
	case "reset":
		result, err = store.Reset(ctx, sddstatus.ResetObjectiveRequest{
			ExpectedRevision: *expected, RequestID: *requestID, Reason: *reason, Actor: *actor,
		})
	case "rescope":
		result, err = store.Rescope(ctx, sddstatus.RescopeObjectiveRequest{
			ExpectedRevision: *expected, RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
			MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines, Reason: *reason, Actor: *actor,
		})
	case "repair":
		result, err = store.RepairConsecutiveRescope(ctx, sddstatus.RepairConsecutiveRescopeRequest{
			ExpectedRevision: *expected, RequestID: *requestID, Reason: *reason, Actor: *actor,
		})
	case "acquire":
		result, err = store.Acquire(ctx, sddstatus.CompactAcquireRequest{
			BeginAttemptRequest: sddstatus.BeginAttemptRequest{
				RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
				MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines,
			},
			Token:                      *token,
			RemediatesEvidenceRevision: *remediatesEvidenceRevision,
		})
	case "settle":
		result, err = store.Settle(ctx, sddstatus.CompactSettleRequest{
			Token: *token, RequestID: *requestID, Outcome: sddstatus.AttemptOutcome(*outcome),
			EvidenceRevision: *evidenceRevision, Diagnosis: *diagnosis,
			HarnessDisposition: sddstatus.HarnessDisposition(*harnessDisposition),
			CleanupEvidence:    *cleanupEvidence, ProcessEvidence: *processEvidence,
			SuccessorLineageID: *successorLineage, RemediatesEvidenceRevision: *remediatesEvidenceRevision,
		})
	case "grant":
		// --expected-revision stays optional, unlike begin/finish/reset: the
		// S2 ledger API admits an empty CAS token (normalizeGrantRootsRequest,
		// "empty or sha256"), which matches exactly the fresh pre-attempt
		// ledger the consent flow grants against; a later widening grant
		// chains the exact committed revision like every sibling mutation.
		// The grant binds the caller-owned change-instance identity (#2540
		// S5): the ledger digest-binds it into the record and replay projects
		// the grant only for the same identity, so an archived name's reuse
		// cannot resurrect it. Until S4b derives markers natively, the caller
		// mints the opaque token and must reuse it for widening grants within
		// this change's lifecycle.
		var grantStore sddstatus.RuntimeStore
		if grantStore, err = store.ForInstance(*changeInstance); err != nil {
			return fmt.Errorf("sdd-attempt grant: %w", err)
		}
		result, err = grantStore.Grant(ctx, sddstatus.GrantRootsRequest{
			ExpectedRevision: *expected, RequestID: *requestID, Roots: roots, Reason: *reason, Actor: *actor,
		})
	}
	if err != nil {
		return fmt.Errorf("sdd-attempt %s: %w", operation, err)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

type sddAttemptFlagKind uint8

const (
	sddAttemptStringFlag sddAttemptFlagKind = iota
	sddAttemptIntFlag
	sddAttemptRepeatableStringFlag
)

type sddAttemptFlagDefinition struct {
	name     string
	kind     sddAttemptFlagKind
	required bool
	usage    string
}

type sddAttemptOperationContract struct {
	name, purpose string
	flags         []sddAttemptFlagDefinition
}

var (
	sddAttemptCWDFlag    = sddAttemptFlagDefinition{name: "cwd", required: true, usage: "required; repository working directory"}
	sddAttemptChangeFlag = sddAttemptFlagDefinition{name: "change", required: true, usage: "required; SDD change identifier"}
)

// sddAttemptOperationDefinitions is the single source for operation flags.
// Help, registration, and validation consume it so a new operation cannot
// silently inherit only the common flags.
var sddAttemptOperationDefinitions = []sddAttemptOperationContract{
	{name: "status", purpose: "Read the native runtime state", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "change-instance", usage: "optional; caller-owned token, at most 128 bytes; scopes granted_roots"},
		// Naming acquire's work-unit scope turns status from "what does the
		// ledger hold" into "would this exact acquire be admitted" (#2114),
		// which is the question consumers were already asking it. All four
		// stay optional; without them status answers the request-blind ledger
		// question exactly as before.
		{name: "work-unit", usage: "optional; single-line label, at most 160 bytes; reports the verdict acquire would return"},
		{name: "evidence-goal", usage: "optional; single-line objective, at most 240 bytes; reports the verdict acquire would return"},
		{name: "max-attempts", kind: sddAttemptIntFlag, usage: "optional; default 2, limit 1..100"},
		{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: "optional; default 200, limit 1..1000000"},
	}},
	{name: "begin", purpose: "Start a bounded runtime attempt", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; empty only initially, otherwise sha256:<64 lowercase hex>"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "work-unit", required: true, usage: "required; single-line label, at most 160 bytes"},
		{name: "evidence-goal", required: true, usage: "required; single-line objective, at most 240 bytes"},
		{name: "max-attempts", kind: sddAttemptIntFlag, usage: "optional; default 2, limit 1..100"},
		{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: "optional; default 200, limit 1..1000000"},
	}},
	{name: "finish", purpose: "Complete the active runtime attempt", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; exact sha256:<64 lowercase hex> runtime revision"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "outcome", required: true, usage: "required; failed, interrupted, or passed"},
		{name: "evidence-revision", required: true, usage: "required for failed/passed; empty or canonical legacy sha256 revision for interrupted"},
		{name: "diagnosis", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "harness-disposition", required: true, usage: "required; reused or invalidated"},
		{name: "cleanup-evidence", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "process-evidence", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "expected-binding-revision", usage: "optional; with successor-lineage and remediates-evidence-revision"},
		{name: "successor-lineage", usage: "optional; lowercase approved lineage, at most 128 bytes"},
		{name: "remediates-evidence-revision", usage: "optional; repaired sha256:<64 lowercase hex> failed evidence"},
	}},
	{name: "handoff", purpose: "Atomically move the active attempt to one linked worktree", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; exact sha256:<64 lowercase hex> runtime revision"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "destination-worktree", required: true, usage: "required; canonical registered linked worktree of the same Git common directory"},
	}},
	{name: "reset", purpose: "Reset a decision-required or complete objective", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; exact sha256:<64 lowercase hex> runtime revision"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "reason", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "actor", required: true, usage: "required; trimmed single-line text, at most 128 bytes"},
	}},
	{name: "rescope", purpose: "Narrow a terminal zero-drift objective", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; exact sha256:<64 lowercase hex> runtime revision"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "work-unit", required: true, usage: "required; narrower single-line label, at most 160 bytes"},
		{name: "evidence-goal", required: true, usage: "required; narrower single-line objective, at most 240 bytes"},
		{name: "max-attempts", kind: sddAttemptIntFlag, required: true, usage: "required; explicit limit 1..100, cannot exceed current objective"},
		{name: "max-changed-lines", kind: sddAttemptIntFlag, required: true, usage: "required; explicit limit 1..1000000, cannot exceed current objective"},
		{name: "reason", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "actor", required: true, usage: "required; trimmed single-line text, at most 128 bytes"},
	}},
	{name: "repair", purpose: "Repair the historical consecutive-rescope publication defect", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", required: true, usage: "required; exact unreadable sha256:<64 lowercase hex> runtime HEAD"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "reason", required: true, usage: "required; trimmed single-line audit reason, at most 500 bytes"},
		{name: "actor", required: true, usage: "required; trimmed single-line audit actor, at most 128 bytes"},
	}},
	{name: "acquire", purpose: "Claim a bounded attempt and return its token", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "token", usage: "optional; token from an earlier acquire to continue that active attempt"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "work-unit", required: true, usage: "required; single-line label, at most 160 bytes"},
		{name: "evidence-goal", required: true, usage: "required; single-line objective, at most 240 bytes"},
		{name: "max-attempts", kind: sddAttemptIntFlag, usage: "optional; default 2, limit 1..100"},
		{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: "optional; default 200, limit 1..1000000"},
		{name: "remediates-evidence-revision", usage: "optional; sha256:<64 lowercase hex> failed evidence for unmanaged remediation"},
	}},
	{name: "settle", purpose: "Complete the attempt selected by its token", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "token", required: true, usage: "required; opaque token returned by acquire"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "outcome", required: true, usage: "required; failed, interrupted, or passed"},
		{name: "evidence-revision", required: true, usage: "required for failed/passed; omit for interrupted"},
		{name: "diagnosis", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "harness-disposition", required: true, usage: "required; reused or invalidated"},
		{name: "cleanup-evidence", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "process-evidence", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
		{name: "successor-lineage", usage: "optional; lowercase distinct approved lineage, at most 128 bytes"},
		{name: "remediates-evidence-revision", usage: "optional; repaired sha256:<64 lowercase hex> failed evidence"},
	}},
	{name: "grant", purpose: "Record per-change edit authority for roots", flags: []sddAttemptFlagDefinition{
		sddAttemptCWDFlag, sddAttemptChangeFlag,
		{name: "expected-revision", usage: "optional; empty initially, otherwise sha256:<64 lowercase hex>"},
		{name: "root", kind: sddAttemptRepeatableStringFlag, required: true, usage: "required and repeatable; 1..32 canonical roots, each at most 4096 bytes"},
		{name: "change-instance", required: true, usage: "required; caller-owned token, at most 128 bytes"},
		{name: "request-id", required: true, usage: "required; lowercase idempotency key, at most 128 bytes"},
		{name: "actor", required: true, usage: "required; trimmed single-line text, at most 128 bytes"},
		{name: "reason", required: true, usage: "required; trimmed single-line text, at most 500 bytes"},
	}},
}

var sddAttemptHelpAliases = []string{"--help", "-h"}

func sddAttemptOperationDefinition(operation string) (sddAttemptOperationContract, bool) {
	for _, definition := range sddAttemptOperationDefinitions {
		if operation == definition.name {
			return definition, true
		}
	}
	return sddAttemptOperationContract{}, false
}

func sddAttemptOperationNames() []string {
	names := make([]string, 0, len(sddAttemptOperationDefinitions))
	for _, definition := range sddAttemptOperationDefinitions {
		names = append(names, definition.name)
	}
	return names
}

func sddAttemptHelpOperation(args []string) (string, bool) {
	operation := ""
	requested := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if sddAttemptHelpAlias(argument) {
			requested = true
			continue
		}
		if hasInlineValue, known := sddAttemptKnownHelpFlag(argument); known {
			if hasInlineValue {
				continue
			}
			// Keep help position-independent while ensuring a following operation
			// name remains the missing flag's value rather than a help target.
			for index+1 < len(args) && sddAttemptHelpAlias(args[index+1]) {
				requested = true
				index++
			}
			if index+1 < len(args) {
				_, nextIsKnownFlag := sddAttemptKnownHelpFlag(args[index+1])
				if !nextIsKnownFlag {
					index++
				}
			}
			continue
		}
		if validSDDAttemptOperation(argument) {
			operation = argument
		}
	}
	return operation, requested
}

func sddAttemptHelpAlias(argument string) bool {
	for _, alias := range sddAttemptHelpAliases {
		if argument == alias {
			return true
		}
	}
	return false
}

// sddAttemptKnownHelpFlag derives its vocabulary from the same operation
// contracts used for registration and validation; all current flags take values.
func sddAttemptKnownHelpFlag(argument string) (hasInlineValue, known bool) {
	if !strings.HasPrefix(argument, "--") {
		return false, false
	}
	name := strings.TrimPrefix(argument, "--")
	if separator := strings.IndexByte(name, '='); separator >= 0 {
		name = name[:separator]
		hasInlineValue = true
	}
	for _, definition := range sddAttemptOperationDefinitions {
		for _, flagDefinition := range definition.flags {
			if flagDefinition.name == name {
				return hasInlineValue, true
			}
		}
	}
	return false, false
}

func renderSDDAttemptHelp(operation string, stdout io.Writer) error {
	if operation == "" {
		_, _ = fmt.Fprintf(stdout, "Usage: gentle-ai sdd-attempt <%s> [flags]\n", strings.Join(sddAttemptOperationNames(), "|"))
		_, _ = fmt.Fprintln(stdout, "\nOperations:")
		for _, definition := range sddAttemptOperationDefinitions {
			_, _ = fmt.Fprintf(stdout, "  %-7s %s\n", definition.name, definition.purpose)
		}
		_, _ = fmt.Fprintf(stdout, "\nUse gentle-ai sdd-attempt <operation> %s for flags, required inputs, defaults, and limits.\n", strings.Join(sddAttemptHelpAliases, " or "))
		return nil
	}
	definition, _ := sddAttemptOperationDefinition(operation)
	_, _ = fmt.Fprintf(stdout, "Usage: gentle-ai sdd-attempt %s [flags]\n\n%s.\n\nFlags:\n", operation, definition.purpose)
	for _, flagDefinition := range definition.flags {
		value := "<value>"
		if flagDefinition.kind == sddAttemptIntFlag {
			value = "<n>"
		} else if flagDefinition.kind == sddAttemptRepeatableStringFlag {
			value = "<path>..."
		}
		_, _ = fmt.Fprintf(stdout, "  --%-29s %s\n", flagDefinition.name+" "+value, flagDefinition.usage)
	}
	return nil
}

func validSDDAttemptOperation(operation string) bool {
	_, ok := sddAttemptOperationDefinition(operation)
	return ok
}

func joinSDDAttemptOperations() string {
	operations := sddAttemptOperationNames()
	switch len(operations) {
	case 0:
		return ""
	case 1:
		return operations[0]
	case 2:
		return operations[0] + " or " + operations[1]
	default:
		last := len(operations) - 1
		return strings.Join(operations[:last], ", ") + ", or " + operations[last]
	}
}

func validateSDDAttemptOperationFlags(operation string, args []string) error {
	definition, _ := sddAttemptOperationDefinition(operation)
	allowed := make(map[string]bool, len(definition.flags))
	for _, flagDefinition := range definition.flags {
		allowed[flagDefinition.name] = true
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			continue
		}
		if !strings.HasPrefix(argument, "--") {
			return fmt.Errorf("flag provided but not defined: %s", argument)
		}
		name := strings.TrimPrefix(argument, "--")
		hasInlineValue := false
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
			hasInlineValue = true
		}
		if !allowed[name] {
			return fmt.Errorf("flag provided but not defined: -%s", name)
		}
		if !hasInlineValue && index+1 < len(args) {
			index++
		}
	}
	return nil
}

func sddAttemptOperationFlag(operation, name string) (sddAttemptFlagDefinition, bool) {
	definition, ok := sddAttemptOperationDefinition(operation)
	if !ok {
		return sddAttemptFlagDefinition{}, false
	}
	for _, flagDefinition := range definition.flags {
		if flagDefinition.name == name {
			return flagDefinition, true
		}
	}
	return sddAttemptFlagDefinition{}, false
}

func registerSDDAttemptStringFlag(flags *flag.FlagSet, operation, name string) *string {
	definition, ok := sddAttemptOperationFlag(operation, name)
	if !ok {
		return new(string)
	}
	return flags.String(name, "", definition.usage)
}

func registerSDDAttemptIntFlag(flags *flag.FlagSet, operation, name string) *int {
	definition, ok := sddAttemptOperationFlag(operation, name)
	if !ok {
		return new(int)
	}
	return flags.Int(name, 0, definition.usage)
}

func registerSDDAttemptRootFlag(flags *flag.FlagSet, operation string, roots *sddAttemptRootList) {
	if definition, ok := sddAttemptOperationFlag(operation, "root"); ok {
		flags.Var(roots, definition.name, definition.usage)
	}
}

func missingSDDAttemptOperationFlags(args []string, operation, parsedOutcome string) []string {
	definition, _ := sddAttemptOperationDefinition(operation)
	names := make([]string, 0, len(definition.flags))
	for _, flagDefinition := range definition.flags {
		if flagDefinition.required {
			names = append(names, flagDefinition.name)
		}
	}
	if (operation == "finish" || operation == "settle") && parsedOutcome == "interrupted" {
		filtered := names[:0]
		for _, name := range names {
			if name != "evidence-revision" {
				filtered = append(filtered, name)
			}
		}
		names = filtered
	}
	return missingSDDAttemptFlags(args, names...)
}

func missingSDDAttemptOperationError(operation string, missing []string) error {
	message := fmt.Sprintf("sdd-attempt %s requires %s", operation, strings.Join(missing, ", "))
	switch operation {
	case "acquire", "settle", "handoff", "grant":
		message += fmt.Sprintf("; rerun `gentle-ai sdd-attempt %s` with those missing flags", operation)
	}
	return errors.New(message)
}

func missingSDDAttemptFlags(args []string, names ...string) []string {
	present := make(map[string]bool, len(names))
	for _, argument := range args {
		for _, name := range names {
			if argument == "--"+name || strings.HasPrefix(argument, "--"+name+"=") {
				present[name] = true
			}
		}
	}
	missing := make([]string, 0, len(names))
	for _, name := range names {
		if !present[name] {
			missing = append(missing, "--"+name)
		}
	}
	return missing
}

func presentSDDAttemptFlags(args []string, names ...string) int {
	present := len(names) - len(missingSDDAttemptFlags(args, names...))
	return present
}

// sddAttemptRootList is grant's repeatable --root flag (#2540 S3). stdlib
// flag has no repeatable string flag, so the bounded multi-root grant shape
// needs this minimal flag.Value: each occurrence appends in caller order, and
// the ledger owns canonicalization, deduplication, and the bounded count
// (normalizeGrantRootsRequest).
type sddAttemptRootList []string

func (roots *sddAttemptRootList) String() string { return strings.Join(*roots, ", ") }

func (roots *sddAttemptRootList) Set(value string) error {
	*roots = append(*roots, value)
	return nil
}
