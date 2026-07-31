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
// Legacy operations emit RuntimeStatus; acquire and settle emit its bounded
// orchestration projection.
func RunSDDAttempt(args []string, stdout io.Writer) error {
	return runSDDAttempt(context.Background(), args, stdout)
}

func runSDDAttempt(ctx context.Context, args []string, stdout io.Writer) error {
	if requested, operation := sddAttemptHelpRequest(args); requested {
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
	values := registerSDDAttemptFlags(flags, operation)
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-attempt argument %q", flags.Arg(0))
	}
	// Identity/revision-shaped values are trimmed at this CLI boundary so
	// incidental leading/trailing whitespace from shell or PowerShell
	// copy-paste does not itself cause a shape rejection.
	for _, name := range []string{"expected-revision", "token", "evidence-revision", "expected-binding-revision", "remediates-evidence-revision"} {
		if value := values.strings[name]; value != nil {
			*value = strings.TrimSpace(*value)
		}
	}
	if strings.TrimSpace(values.string("cwd")) == "" {
		return errors.New("sdd-attempt requires --cwd")
	}
	if strings.TrimSpace(values.string("change")) == "" {
		return errors.New("sdd-attempt requires --change")
	}

	store, err := sddstatus.OpenRuntimeStore(ctx, values.string("cwd"), values.string("change"))
	if err != nil {
		return fmt.Errorf("open native SDD runtime authority: %w", err)
	}
	// The kill switch reaches the runtime ledger here, at the one place that
	// knows how to read both of its sources. With reviews off, closing an
	// attempt must not demand a review obligation the operator has no way to
	// satisfy.
	store.ReviewDisabled = reviewDrivenDevelopmentDisabled(ctx, values.string("cwd"))
	var result any
	switch operation {
	case "status":
		result, err = store.Status()
	case "begin":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "work-unit", "evidence-goal"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt begin requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Begin(ctx, sddstatus.BeginAttemptRequest{
			ExpectedRevision: values.string("expected-revision"), RequestID: values.string("request-id"), WorkUnit: values.string("work-unit"), EvidenceGoal: values.string("evidence-goal"),
			MaxAttempts: values.integer("max-attempts"), MaxChangedLines: values.integer("max-changed-lines"),
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
			ExpectedRevision: values.string("expected-revision"), RequestID: values.string("request-id"), Outcome: sddstatus.AttemptOutcome(values.string("outcome")),
			EvidenceRevision: values.string("evidence-revision"), Diagnosis: values.string("diagnosis"),
			HarnessDisposition: sddstatus.HarnessDisposition(values.string("harness-disposition")),
			CleanupEvidence:    values.string("cleanup-evidence"), ProcessEvidence: values.string("process-evidence"),
			ExpectedBindingRevision: values.string("expected-binding-revision"), SuccessorLineageID: values.string("successor-lineage"),
			RemediatesEvidenceRevision: values.string("remediates-evidence-revision"),
		})
	case "reset":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "reason", "actor"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt reset requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Reset(ctx, sddstatus.ResetObjectiveRequest{
			ExpectedRevision: values.string("expected-revision"), RequestID: values.string("request-id"), Reason: values.string("reason"), Actor: values.string("actor"),
		})
	case "rescope":
		if missing := missingSDDAttemptFlags(args[1:], "expected-revision", "request-id", "work-unit", "evidence-goal", "max-attempts", "max-changed-lines", "reason", "actor"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt rescope requires %s", strings.Join(missing, ", "))
		}
		result, err = store.Rescope(ctx, sddstatus.RescopeObjectiveRequest{
			ExpectedRevision: values.string("expected-revision"), RequestID: values.string("request-id"), WorkUnit: values.string("work-unit"), EvidenceGoal: values.string("evidence-goal"),
			MaxAttempts: values.integer("max-attempts"), MaxChangedLines: values.integer("max-changed-lines"), Reason: values.string("reason"), Actor: values.string("actor"),
		})
	case "acquire":
		if missing := missingSDDAttemptFlags(args[1:], "request-id", "work-unit", "evidence-goal"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt acquire requires %s; rerun `gentle-ai sdd-attempt acquire` with those missing flags", strings.Join(missing, ", "))
		}
		result, err = store.Acquire(ctx, sddstatus.CompactAcquireRequest{
			BeginAttemptRequest: sddstatus.BeginAttemptRequest{
				RequestID: values.string("request-id"), WorkUnit: values.string("work-unit"), EvidenceGoal: values.string("evidence-goal"),
				MaxAttempts: values.integer("max-attempts"), MaxChangedLines: values.integer("max-changed-lines"),
			},
			Token: values.string("token"),
		})
	case "settle":
		if missing := missingSDDAttemptFlags(args[1:], "token", "request-id", "outcome", "evidence-revision", "diagnosis", "harness-disposition", "cleanup-evidence", "process-evidence"); len(missing) != 0 {
			return fmt.Errorf("sdd-attempt settle requires %s; rerun `gentle-ai sdd-attempt settle` with those missing flags", strings.Join(missing, ", "))
		}
		result, err = store.Settle(ctx, sddstatus.CompactSettleRequest{
			Token: values.string("token"), RequestID: values.string("request-id"), Outcome: sddstatus.AttemptOutcome(values.string("outcome")),
			EvidenceRevision: values.string("evidence-revision"), Diagnosis: values.string("diagnosis"),
			HarnessDisposition: sddstatus.HarnessDisposition(values.string("harness-disposition")),
			CleanupEvidence:    values.string("cleanup-evidence"), ProcessEvidence: values.string("process-evidence"),
			SuccessorLineageID: values.string("successor-lineage"), RemediatesEvidenceRevision: values.string("remediates-evidence-revision"),
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
var sddAttemptOperationsInOrder = []string{"status", "begin", "finish", "reset", "rescope", "acquire", "settle"}

type sddAttemptFlagKind uint8

const (
	sddAttemptStringFlag sddAttemptFlagKind = iota
	sddAttemptIntFlag
)

type sddAttemptFlagDefinition struct {
	name        string
	kind        sddAttemptFlagKind
	usage       string
	defaultText string
}

type sddAttemptOperationContract struct {
	name    string
	purpose string
	flags   []sddAttemptFlagDefinition
}

var sddAttemptCommonFlags = []sddAttemptFlagDefinition{
	{name: "cwd", usage: "repository path"},
	{name: "change", usage: "SDD change name (lowercase hyphen identifier, up to 96 characters)"},
}

var sddAttemptOperationDefinitions = []sddAttemptOperationContract{
	{name: "status", purpose: "show the current native runtime status"},
	{
		name: "begin", purpose: "start a bounded runtime attempt", flags: []sddAttemptFlagDefinition{
			{name: "expected-revision", usage: "exact native runtime revision (empty only for initial begin)"},
			{name: "request-id", usage: "idempotency request identifier (lowercase canonical ID, up to 128 characters)"},
			{name: "work-unit", usage: "caller-facing work-unit label (up to 160 characters)"},
			{name: "evidence-goal", usage: "stable runtime evidence objective (up to 240 characters)"},
			{name: "max-attempts", kind: sddAttemptIntFlag, usage: fmt.Sprintf("attempt limit in the range 1..%d", sddstatus.RuntimeMaxAttemptLimit), defaultText: fmt.Sprintf("default %d", sddstatus.RuntimeDefaultAttemptLimit)},
			{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: fmt.Sprintf("changed-line limit in the range 1..%d", sddstatus.RuntimeMaxChangedLines), defaultText: fmt.Sprintf("default %d", sddstatus.RuntimeDefaultChangedLines)},
		},
	},
	{
		name: "finish", purpose: "close the active runtime attempt", flags: []sddAttemptFlagDefinition{
			{name: "expected-revision", usage: "exact current native runtime revision (sha256:<64 lowercase hex>)"},
			{name: "request-id", usage: "idempotency request identifier (lowercase canonical ID, up to 128 characters)"},
			{name: "outcome", usage: "terminal outcome: failed|interrupted|passed"},
			{name: "evidence-revision", usage: "required lowercase sha256:<64 lowercase hex>; the literal none is invalid"},
			{name: "diagnosis", usage: "proven runtime diagnosis (up to 500 characters)"},
			{name: "harness-disposition", usage: "harness disposition: reused|invalidated"},
			{name: "cleanup-evidence", usage: "bounded cleanup evidence (up to 500 characters)"},
			{name: "process-evidence", usage: "bounded process evidence (up to 500 characters)"},
			{name: "expected-binding-revision", usage: "exact populated binding revision for passed remediation"},
			{name: "successor-lineage", usage: "approved lowercase successor lineage (up to 128 characters)"},
			{name: "remediates-evidence-revision", usage: "failed evidence revision repaired by a passed successor"},
		},
	},
	{
		name: "reset", purpose: "reset a decision-required runtime objective", flags: []sddAttemptFlagDefinition{
			{name: "expected-revision", usage: "exact current native runtime revision (sha256:<64 lowercase hex>)"},
			{name: "request-id", usage: "idempotency request identifier (lowercase canonical ID, up to 128 characters)"},
			{name: "reason", usage: "explicit objective reset reason (up to 500 characters)"},
			{name: "actor", usage: "explicit reset actor (up to 128 characters)"},
		},
	},
	{
		name: "rescope", purpose: "narrow a terminal zero-drift runtime objective", flags: []sddAttemptFlagDefinition{
			{name: "expected-revision", usage: "exact current native runtime revision (sha256:<64 lowercase hex>)"},
			{name: "request-id", usage: "idempotency request identifier (lowercase canonical ID, up to 128 characters)"},
			{name: "work-unit", usage: fmt.Sprintf("narrower caller-facing work-unit label (up to %d characters)", sddstatus.RuntimeWorkUnitLimit)},
			{name: "evidence-goal", usage: fmt.Sprintf("narrower stable runtime evidence objective (up to %d characters)", sddstatus.RuntimeEvidenceGoalLimit)},
			{name: "max-attempts", kind: sddAttemptIntFlag, usage: fmt.Sprintf("narrowed attempt limit in the range 1..%d; at most the current objective", sddstatus.RuntimeMaxAttemptLimit)},
			{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: fmt.Sprintf("narrowed changed-line limit in the range 1..%d; at most the current objective", sddstatus.RuntimeMaxChangedLines)},
			{name: "reason", usage: fmt.Sprintf("explicit narrowing reason (up to %d characters)", sddstatus.RuntimeResetReasonLimit)},
			{name: "actor", usage: fmt.Sprintf("explicit rescope actor (up to %d characters)", sddstatus.RuntimeActorLimit)},
		},
	},
	{
		name: "acquire", purpose: "acquire a bounded attempt", flags: []sddAttemptFlagDefinition{
			{name: "request-id", usage: "idempotency request identifier"},
			{name: "work-unit", usage: "caller-facing work-unit label"},
			{name: "evidence-goal", usage: "stable runtime evidence objective"},
			{name: "max-attempts", kind: sddAttemptIntFlag, usage: "bounded attempts"},
			{name: "max-changed-lines", kind: sddAttemptIntFlag, usage: "bounded changed lines"},
		},
	},
	{
		name: "settle", purpose: "settle a bounded attempt", flags: []sddAttemptFlagDefinition{
			{name: "token", usage: "opaque compact attempt continuation"},
			{name: "request-id", usage: "idempotency request identifier"},
			{name: "outcome", usage: "terminal outcome"},
			{name: "evidence-revision", usage: "native evidence revision"},
			{name: "diagnosis", usage: "proven runtime diagnosis"},
			{name: "harness-disposition", usage: "reused or invalidated"},
			{name: "cleanup-evidence", usage: "bounded cleanup evidence"},
			{name: "process-evidence", usage: "bounded process evidence"},
			{name: "successor-lineage", usage: "approved compact recovery successor lineage"},
			{name: "remediates-evidence-revision", usage: "failed evidence revision repaired by the successor"},
		},
	},
}

type sddAttemptFlagValues struct {
	strings map[string]*string
	ints    map[string]*int
}

func (values sddAttemptFlagValues) string(name string) string {
	if value := values.strings[name]; value != nil {
		return *value
	}
	return ""
}

func (values sddAttemptFlagValues) integer(name string) int {
	if value := values.ints[name]; value != nil {
		return *value
	}
	return 0
}

func sddAttemptOperationDefinition(operation string) (sddAttemptOperationContract, bool) {
	for _, definition := range sddAttemptOperationDefinitions {
		if definition.name == operation {
			return definition, true
		}
	}
	return sddAttemptOperationContract{}, false
}

func sddAttemptFlagsForOperation(operation string) []sddAttemptFlagDefinition {
	definition, _ := sddAttemptOperationDefinition(operation)
	flags := make([]sddAttemptFlagDefinition, 0, len(sddAttemptCommonFlags)+len(definition.flags))
	flags = append(flags, sddAttemptCommonFlags...)
	return append(flags, definition.flags...)
}

func registerSDDAttemptFlags(flags *flag.FlagSet, operation string) sddAttemptFlagValues {
	values := sddAttemptFlagValues{strings: map[string]*string{}, ints: map[string]*int{}}
	for _, definition := range sddAttemptFlagsForOperation(operation) {
		switch definition.kind {
		case sddAttemptIntFlag:
			values.ints[definition.name] = flags.Int(definition.name, 0, definition.usage)
		default:
			values.strings[definition.name] = flags.String(definition.name, "", definition.usage)
		}
	}
	return values
}

func sddAttemptHelpRequest(args []string) (bool, string) {
	requested := false
	operation := ""
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "-h" || argument == "--help" {
			requested = true
			continue
		}
		if strings.HasPrefix(argument, "--") {
			name := strings.TrimPrefix(argument, "--")
			if separator := strings.IndexByte(name, '='); separator >= 0 {
				continue
			}
			if sddAttemptFlagTakesValue(name) {
				index++
			}
			continue
		}
		if operation == "" && validSDDAttemptOperation(argument) {
			operation = argument
		}
	}
	return requested, operation
}

func sddAttemptFlagTakesValue(name string) bool {
	for _, operation := range sddAttemptOperationDefinitions {
		for _, definition := range append(append([]sddAttemptFlagDefinition{}, sddAttemptCommonFlags...), operation.flags...) {
			if definition.name == name {
				return true
			}
		}
	}
	return false
}

func renderSDDAttemptHelp(operation string, stdout io.Writer) error {
	if operation == "" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-attempt <operation> [flags]")
		_, _ = fmt.Fprintln(stdout, "\nOperations:")
		for _, name := range sddAttemptOperationsInOrder {
			definition, _ := sddAttemptOperationDefinition(name)
			_, _ = fmt.Fprintf(stdout, "  %-7s %s\n", name, definition.purpose)
		}
		_, _ = fmt.Fprintln(stdout, "\nCommon flags:")
		for _, definition := range sddAttemptCommonFlags {
			renderSDDAttemptFlag(stdout, definition)
		}
		_, _ = fmt.Fprintf(stdout, "\nContract tokens: revision %s; request ID %s; change grammar %s; lineage grammar %s.\n", sddstatus.RuntimeRevisionPattern, sddstatus.RuntimeRequestIDPattern, sddstatus.RuntimeChangePattern, sddstatus.RuntimeLineagePattern)
		_, _ = fmt.Fprintf(stdout, "Terminal outcomes: %s; harness dispositions: %s.\n", strings.Join(sddstatus.RuntimeTerminalOutcomes(), "|"), strings.Join(sddstatus.RuntimeHarnessDispositions(), "|"))
		_, _ = fmt.Fprintln(stdout, "Request IDs are idempotency keys: replay the same request with the same ID; a different request using that ID is not accepted.")
		return nil
	}

	definition, ok := sddAttemptOperationDefinition(operation)
	if !ok || operation == "acquire" || operation == "settle" {
		return renderSDDAttemptHelp("", stdout)
	}
	_, _ = fmt.Fprintf(stdout, "Usage: gentle-ai sdd-attempt %s [flags]\n", operation)
	_, _ = fmt.Fprintf(stdout, "\nPurpose: %s.\n\nFlags:\n", definition.purpose)
	for _, flagDefinition := range sddAttemptFlagsForOperation(operation) {
		renderSDDAttemptFlag(stdout, flagDefinition)
	}
	renderSDDAttemptOperationContract(stdout, operation)
	return nil
}

func renderSDDAttemptFlag(stdout io.Writer, definition sddAttemptFlagDefinition) {
	value := "<value>"
	if definition.kind == sddAttemptIntFlag {
		value = "<n>"
	}
	defaultText := ""
	if definition.defaultText != "" {
		defaultText = "; " + definition.defaultText
	}
	_, _ = fmt.Fprintf(stdout, "  --%-30s %s%s\n", definition.name+" "+value, definition.usage, defaultText)
}

func renderSDDAttemptOperationContract(stdout io.Writer, operation string) {
	_, _ = fmt.Fprintln(stdout, "\nRuntime contract:")
	switch operation {
	case "status":
		_, _ = fmt.Fprintf(stdout, "  RuntimeStatus.revision is the native CAS value; change grammar is %s.\n", sddstatus.RuntimeChangePattern)
		_, _ = fmt.Fprintln(stdout, "  active_attempt identifies a running attempt; active_attempt.ordinal is generated by runtime, starts at 1, and callers do not supply it.")
	case "begin":
		_, _ = fmt.Fprintln(stdout, "  --expected-revision is empty only for the initial begin; later calls use the exact current revision.")
		_, _ = fmt.Fprintln(stdout, "  A non-nil RuntimeStatus.active_attempt blocks begin.")
		_, _ = fmt.Fprintf(stdout, "  Revision matches %s; request IDs match %s; change grammar %s; lineage grammar %s.\n", sddstatus.RuntimeRevisionPattern, sddstatus.RuntimeRequestIDPattern, sddstatus.RuntimeChangePattern, sddstatus.RuntimeLineagePattern)
		_, _ = fmt.Fprintln(stdout, "  Request IDs are idempotency keys: replaying the same request with the same ID returns its committed result; reusing an ID with different fields is rejected.")
		_, _ = fmt.Fprintln(stdout, `  Example: REPO_DIR="$(git rev-parse --show-toplevel)"; gentle-ai sdd-attempt begin --cwd "$REPO_DIR" --change runtime-demo --expected-revision="" --request-id begin-runtime-demo-1 --work-unit runtime-help --evidence-goal "prove runtime help" --max-attempts 2 --max-changed-lines 200`)
	case "finish":
		_, _ = fmt.Fprintln(stdout, "  RuntimeStatus.revision is the finish CAS value; finish requires a non-nil, running RuntimeStatus.active_attempt.")
		_, _ = fmt.Fprintln(stdout, "  RuntimeStatus.binding_revision/binding identify remediation authority; evidence_revision is the failed value to remediate.")
		_, _ = fmt.Fprintf(stdout, "  Revision, evidence, and binding values use %s; change grammar %s; lineage grammar %s.\n", sddstatus.RuntimeRevisionPattern, sddstatus.RuntimeChangePattern, sddstatus.RuntimeLineagePattern)
		_, _ = fmt.Fprintf(stdout, "  Outcomes are %s; dispositions are %s.\n", strings.Join(sddstatus.RuntimeTerminalOutcomes(), "|"), strings.Join(sddstatus.RuntimeHarnessDispositions(), "|"))
		_, _ = fmt.Fprintln(stdout, "  The remediation flags are all-or-none and valid only for passed attempts; a nonempty evidence revision is required.")
		_, _ = fmt.Fprintln(stdout, "  Request IDs are idempotency keys: replaying the same request with the same ID returns its committed result; reusing an ID with different fields is rejected.")
		_, _ = fmt.Fprintln(stdout, `  Example: REPO_DIR="$(git rev-parse --show-toplevel)"; gentle-ai sdd-attempt finish --cwd "$REPO_DIR" --change runtime-demo --expected-revision "${CURRENT_REVISION:?set from status.revision}" --request-id finish-runtime-demo-1 --outcome failed --evidence-revision "${EVIDENCE_REVISION:?set to a lowercase sha256 evidence revision}" --diagnosis "runtime evidence shows the bounded attempt failed" --harness-disposition reused --cleanup-evidence "cleanup completed" --process-evidence "process scan completed"`)
	case "reset":
		_, _ = fmt.Fprintln(stdout, "  --expected-revision must equal the exact current RuntimeStatus.revision; reset is for decision-required or complete objectives.")
		_, _ = fmt.Fprintln(stdout, "  A non-nil RuntimeStatus.active_attempt blocks reset.")
		_, _ = fmt.Fprintf(stdout, "  Revision matches %s; request IDs match %s; change grammar %s; lineage grammar %s.\n", sddstatus.RuntimeRevisionPattern, sddstatus.RuntimeRequestIDPattern, sddstatus.RuntimeChangePattern, sddstatus.RuntimeLineagePattern)
		_, _ = fmt.Fprintln(stdout, "  Request IDs are idempotency keys: replaying the same request with the same ID returns its committed result; reusing an ID with different fields is rejected.")
	case "rescope":
		_, _ = fmt.Fprintln(stdout, "  Rescope requires no active attempt, an existing objective that is not decision-required or complete, and a terminal failed or interrupted attempt whose candidate has not drifted.")
		_, _ = fmt.Fprintf(stdout, "  Revision matches %s; request IDs match %s; change and lineage identifiers match %s.\n", sddstatus.RuntimeRevisionPattern, sddstatus.RuntimeRequestIDPattern, sddstatus.RuntimeChangePattern)
		_, _ = fmt.Fprintln(stdout, "  --max-attempts and --max-changed-lines may only narrow or hold the current objective ceilings; cumulative attempts and changed lines carry forward unchanged.")
		_, _ = fmt.Fprintln(stdout, "  Rescope is a maintainer-authorized successor scope; reason and actor are persisted in the runtime audit record.")
	}
}

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
	allowed := map[string]bool{}
	for _, definition := range sddAttemptFlagsForOperation(operation) {
		allowed[definition.name] = true
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
