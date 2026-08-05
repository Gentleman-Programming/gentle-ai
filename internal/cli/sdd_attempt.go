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
	cwd := flags.String("cwd", "", "repository path")
	change := flags.String("change", "", "SDD change")
	expected := flags.String("expected-revision", "", "exact native runtime revision")
	token := flags.String("token", "", "opaque compact attempt continuation")
	requestID := flags.String("request-id", "", "idempotency request identifier")
	workUnit := flags.String("work-unit", "", "caller-facing work-unit label")
	evidenceGoal := flags.String("evidence-goal", "", "stable runtime evidence objective")
	maxAttempts := flags.Int("max-attempts", 0, "bounded attempts for a new objective")
	maxChangedLines := flags.Int("max-changed-lines", 0, "bounded changed lines for a new objective")
	outcome := flags.String("outcome", "", "failed, interrupted, or passed")
	evidenceRevision := flags.String("evidence-revision", "", "native evidence revision")
	diagnosis := flags.String("diagnosis", "", "proven runtime diagnosis")
	harnessDisposition := flags.String("harness-disposition", "", "reused or invalidated")
	cleanupEvidence := flags.String("cleanup-evidence", "", "bounded cleanup evidence")
	processEvidence := flags.String("process-evidence", "", "bounded process evidence")
	expectedBindingRevision := flags.String("expected-binding-revision", "", "exact populated binding revision for atomic remediation")
	successorLineage := flags.String("successor-lineage", "", "approved compact recovery successor lineage")
	remediatesEvidenceRevision := flags.String("remediates-evidence-revision", "", "failed evidence revision repaired by the successor")
	reason := flags.String("reason", "", "explicit objective reset reason")
	actor := flags.String("actor", "", "explicit reset actor")
	var roots sddAttemptRootList
	flags.Var(&roots, "root", "repository root receiving per-change edit authority (repeatable)")
	changeInstance := flags.String("change-instance", "", "opaque caller-owned change-instance identity; mint one token per change instance, reuse that exact token for widening grants within the same change lifecycle, and mint a fresh one for a recreated change so an archived name's grants never resurrect (required for grant; optional projection scope for status)")
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
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("sdd-attempt requires --cwd")
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("sdd-attempt requires --change")
	}

	store, err := sddstatus.OpenRuntimeStore(ctx, *cwd, *change)
	if err != nil {
		return fmt.Errorf("open native SDD runtime authority: %w", err)
	}
	// The kill switch reaches the runtime ledger here, at the one place that
	// knows how to read both of its sources. With reviews off, closing an
	// attempt must not demand a review obligation the operator has no way to
	// satisfy.
	store.ReviewDisabled = reviewDrivenDevelopmentDisabled(ctx, *cwd)
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
		result, err = store.Status()
	case "begin":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "work-unit", "evidence-goal"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt begin requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Begin(ctx, sddstatus.BeginAttemptRequest{
			ExpectedRevision: *expected, RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
			MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines,
		})
	case "finish":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt finish requires %s", strings.Join(missing, ", "))
		}
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
	case "reset":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "reason", "actor"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt reset requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Reset(ctx, sddstatus.ResetObjectiveRequest{
			ExpectedRevision: *expected, RequestID: *requestID, Reason: *reason, Actor: *actor,
		})
	case "rescope":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "reason", "actor"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt rescope requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Rescope(ctx, sddstatus.RescopeObjectiveRequest{
			ExpectedRevision: *expected, RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
			MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines, Reason: *reason, Actor: *actor,
		})
	case "acquire":
		if missing := missingSDDAttemptFlags(args[1:], "request-id", "work-unit", "evidence-goal"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt acquire requires %s; rerun `gentle-ai sdd-attempt acquire` with those missing flags", strings.Join(missing, ", "))
		}
		result, err = store.Acquire(ctx, sddstatus.CompactAcquireRequest{
			BeginAttemptRequest: sddstatus.BeginAttemptRequest{
				RequestID: *requestID, WorkUnit: *workUnit, EvidenceGoal: *evidenceGoal,
				MaxAttempts: *maxAttempts, MaxChangedLines: *maxChangedLines,
			},
			Token:                      *token,
			RemediatesEvidenceRevision: *remediatesEvidenceRevision,
		})
	case "settle":
		if missing := missingSDDAttemptFlags(args[1:], "token", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt settle requires %s; rerun `gentle-ai sdd-attempt settle` with those missing flags", strings.Join(missing, ", "))
		}
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
		if missing := missingSDDAttemptFlags(args[1:], "root", "change-instance", "request-id", "actor", "reason"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt grant requires %s; rerun `gentle-ai sdd-attempt grant` with those missing flags", strings.Join(missing, ", "))
		}
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

// sddAttemptOperationsInOrder is the single ordered source of truth for
// every valid sdd-attempt operation. validSDDAttemptOperation and any
// refusal that must enumerate the valid set both derive from it, so the
// accepted values and the values a message names can never drift apart.
// Mirrors reviewIntegrationGatesInOrder / reviewIntegrationGateNames in
// review_operation_contract.go.
var sddAttemptOperationsInOrder = []string{"status", "begin", "finish", "reset", "rescope", "acquire", "settle", "grant"}

func validSDDAttemptOperation(operation string) bool {
	for _, valid := range sddAttemptOperationsInOrder {
		if operation == valid {
			return true
		}
	}
	return false
}

// joinSDDAttemptOperations renders sddAttemptOperationsInOrder as an
// English "a, b, c, or d" list for refusal messages that must name the
// valid operation values.
func joinSDDAttemptOperations() string {
	switch len(sddAttemptOperationsInOrder) {
	case 0:
		return ""
	case 1:
		return sddAttemptOperationsInOrder[0]
	case 2:
		return sddAttemptOperationsInOrder[0] + " or " + sddAttemptOperationsInOrder[1]
	default:
		last := len(sddAttemptOperationsInOrder) - 1
		return strings.Join(sddAttemptOperationsInOrder[:last], ", ") + ", or " + sddAttemptOperationsInOrder[last]
	}
}

func validateSDDAttemptOperationFlags(operation string, args []string) error {
	allowed := map[string]bool{"cwd": true, "change": true}
	for _, name := range map[string][]string{
		"begin":   {"expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines"},
		"finish":  {"expected-revision", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence", "expected-binding-revision", "successor-lineage", "remediates-evidence-revision"},
		"reset":   {"expected-revision", "request-id", "reason", "actor"},
		"rescope": {"expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "reason", "actor"},
		"acquire": {"request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "token", "remediates-evidence-revision"},
		"settle":  {"token", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence", "successor-lineage", "remediates-evidence-revision"},
		"grant":   {"expected-revision", "request-id", "root", "change-instance", "actor", "reason"},
		"status":  {"change-instance"},
	}[operation] {
		allowed[name] = true
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
