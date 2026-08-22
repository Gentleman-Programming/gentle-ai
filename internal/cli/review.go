package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const (
	ReviewResumeSchema   = "gentle-ai.review-resume/v1"
	ReviewBundleSchema   = "gentle-ai.review-bundle-result/v1"
	ReviewValidateSchema = "gentle-ai.review-gate-result/v1"
)

type ReviewValidateResult struct {
	Schema  string                        `json:"schema"`
	Result  reviewtransaction.GateResult  `json:"result"`
	Allowed bool                          `json:"allowed"`
	Action  string                        `json:"action"`
	Reason  string                        `json:"reason"`
	Context reviewtransaction.GateContext `json:"context"`
	// Delivery names what governs delivery when the answer is not the receipt
	// itself. It is an additive, omitted-by-default extension of
	// gentle-ai.review-gate-result/v1: every projection that already shipped
	// keeps its exact field set, and only a candidate that is unmanaged by the
	// user's own choice carries the extra token. It never carries an approval.
	Delivery reviewtransaction.RDDDelivery `json:"delivery,omitempty"`
	// Relation and Next are Wave 5 (Gate Cutover) Slice 3's additive wire
	// projection of NativeGateEvaluation's own new fields (gate.go): omitted
	// by default, so every existing consumer's exact field set is unchanged,
	// and populated only where attachGateVerdictRelation (compact_gate.go)
	// could classify the denial this slice -- currently the "changed"
	// relation only (see that function's own doc comment for why the rest
	// is not wired yet).
	Relation reviewtransaction.CandidateRelation `json:"relation,omitempty"`
	Next     *reviewtransaction.GateNextStep     `json:"next,omitempty"`
}

// reviewDeliveryPolicyAction is the action reported when the review gate has no
// authority to apply. It is deliberately not "explicit-maintainer-action":
// nothing is wrong and nobody has to intervene, ordinary repository policy
// simply governs delivery.
const reviewDeliveryPolicyAction = "repository-policy"

func newReviewFlagSet(name string, stdout io.Writer, details string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(stdout, "Usage: gentle-ai %s [flags]\n\n%s\n\nFlags:\n", name, details)
		flags.VisitAll(func(current *flag.Flag) {
			placeholder := " <value>"
			if boolean, ok := current.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
				placeholder = ""
			}
			_, _ = fmt.Fprintf(stdout, "  --%s%s\n      %s", current.Name, placeholder, current.Usage)
			if current.DefValue != "" {
				_, _ = fmt.Fprintf(stdout, " (default %q)", current.DefValue)
			}
			_, _ = fmt.Fprintln(stdout)
		})
		_, _ = fmt.Fprintln(stdout, "  -h, --help\n      show this help")
	}
	return flags
}

func parseReviewFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return reviewPreflightError(err)
	}
	if hint := reviewBooleanFlagSpacedValueHint(flags, args); hint != "" {
		return reviewPreflightError(errors.New(hint))
	}
	return nil
}

// reviewBooleanFlagSpacedValueHint detects one exact shape: a boolean flag
// passed with a space-separated value ("--committed-only true") instead of
// "--committed-only" or "--committed-only=true". The standard library's flag
// package parses the flag alone (defaulting to true) and stops at the first
// non-flag token, leaving "true"/"false" as a stray positional argument --
// every review verb's own "unexpected review <verb> argument" refusal would
// otherwise report only that bare positional, naming no continuation.
//
// DECISION (organic-dx Phase 3f task 3f.3, recorded per its non-negotiable
// requirement): the parser stays strict. Detached boolean values are not
// accepted anywhere in the review command family -- silently accepting them
// would be a parsing behavior change with a wide blast radius across every
// command, not a wording fix. Only the refusal changes, and only for this one
// detected shape; every other "unexpected argument" refusal is untouched.
//
// This lives in the single site (parseReviewFlags) every review verb's flag
// parsing already funnels through, so every verb benefits without touching
// each verb's own unexpected-argument call site.
func reviewBooleanFlagSpacedValueHint(flags *flag.FlagSet, args []string) string {
	leftover := flags.Args()
	if len(leftover) == 0 || (leftover[0] != "true" && leftover[0] != "false") {
		return ""
	}
	stopIndex := len(args) - len(leftover)
	if stopIndex <= 0 {
		return ""
	}
	previous := args[stopIndex-1]
	if !strings.HasPrefix(previous, "-") || strings.Contains(previous, "=") {
		return ""
	}
	name := strings.TrimLeft(previous, "-")
	if name == "" {
		return ""
	}
	entry := flags.Lookup(name)
	if entry == nil {
		return ""
	}
	boolValue, ok := entry.Value.(interface{ IsBoolFlag() bool })
	if !ok || !boolValue.IsBoolFlag() {
		return ""
	}
	return fmt.Sprintf("boolean flag --%s takes --%s or --%s=%s, not a separate value; got %q", name, name, name, leftover[0], leftover[0])
}

func reviewHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

type ReviewResumeResult struct {
	Schema          string                        `json:"schema"`
	Operation       string                        `json:"operation"`
	Target          reviewtransaction.Snapshot    `json:"target"`
	Transaction     reviewtransaction.Transaction `json:"transaction"`
	StoreAuthority  string                        `json:"store_authority"`
	StoreRevision   string                        `json:"store_revision"`
	GenesisRevision string                        `json:"genesis_revision"`
	ChainIdentity   string                        `json:"chain_identity"`
}

type ReviewBundleResult struct {
	Schema          string `json:"schema"`
	Operation       string `json:"operation"`
	LineageID       string `json:"lineage_id"`
	BundleDigest    string `json:"bundle_digest"`
	StoreRevision   string `json:"store_revision"`
	GenesisRevision string `json:"genesis_revision"`
	ChainIdentity   string `json:"chain_identity"`
	BundlePath      string `json:"bundle_path,omitempty"`
}

type ReviewGateDeniedError struct {
	Result reviewtransaction.GateResult
	// Reason is the exact reason the JSON gate-result envelope publishes for
	// this denial. It is carried here so the human surface renders the same
	// specific statement the machine surface already had, instead of throwing
	// it away and substituting a placeholder.
	Reason  string
	Context reviewtransaction.GateContext
	Cause   error
}

func RunReviewStep(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-step", stdout, "Read-only legacy v1 compatibility command. Mutation is rejected; use gentle-ai review finalize for compact authority.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	operation := flags.String("operation", "", "legacy lifecycle operation rejected as read-only")
	inputPath := flags.String("input", "", "legacy JSON operation input")
	_ = flags.String("ledger", "", "legacy ledger input")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-step argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*operation) == "" || strings.TrimSpace(*inputPath) == "" {
		return errors.New("review-step requires --cwd, --lineage, --operation, and --input")
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	if _, err := store.LoadChain(); err != nil {
		return fmt.Errorf("load authoritative review transaction: %w", err)
	}
	attemptedOperation := strings.TrimSpace(*operation)
	if !strings.HasPrefix(attemptedOperation, "review/") {
		attemptedOperation = "review/" + attemptedOperation
	}
	return fmt.Errorf("%w: review-step cannot mutate shipped v1 authority; use gentle-ai review finalize", reviewtransaction.NewLegacyReadOnlyError(attemptedOperation, *lineage))
}

// Error renders the human-surface denial. It always names a continuation:
// every branch below derives it from the SAME source the negotiated
// gate_scope_changed/gate_escalated/gate_invalidated envelope already uses
// (reviewGateAction, and for scope-changed the frozen
// GateScopeChangeDiagnostics.RecoveryOperation/RecoveryRequiredInputs) so the
// wording can never drift from the routing knowledge the JSON envelope
// carries. When no derivable continuation exists, it states the exact Reason
// the same envelope publishes instead of inventing a command -- and only when
// there is no reason either does it fall back to the terminal precondition.
//
// Nothing rendered here may be a dotted operation identifier. Those are wire
// tokens for the negotiated contract; an operator who types one gets nothing,
// so every command named here goes through reviewRunnableCommand.
func (err ReviewGateDeniedError) Error() string {
	message := fmt.Sprintf("review lifecycle gate denied: %s", err.Result)
	if err.Result == reviewtransaction.GateEscalated {
		return fmt.Sprintf("%s: %s", message, reviewEscalatedContinuation(err))
	}
	if err.Result == reviewtransaction.GateScopeChanged && err.Context.ScopeChange != nil && err.Context.ScopeChange.RecoveryOperation != "" {
		scope := err.Context.ScopeChange
		operation := reviewRunnableCommand(scope.RecoveryOperation)
		// Name the situation before the mechanism. "scope-changed" plus a
		// recovery command is accurate and says nothing about what happened,
		// which is that this candidate carries work no receipt covers. That is
		// the shape an operator meets after switching reviews off, doing work,
		// and switching them back on: the re-validation is correct, but reading
		// it as a state mismatch rather than as accumulated unreviewed work
		// leaves them guessing. The count is already derived and frozen here,
		// so this costs no extra derivation; when it is absent nothing is said,
		// because a fabricated "0 paths" would be worse than silence.
		if scope.DifferingPathCount > 0 {
			noun := "paths"
			if scope.DifferingPathCount == 1 {
				noun = "path"
			}
			message = fmt.Sprintf("%s: %d %s in this candidate that no terminal receipt covers", message, scope.DifferingPathCount, noun)
		}
		// The gate-conditional half. A bare recovery freezes a current-changes
		// successor over the live workspace; at pre-push over an already
		// committed delivery that successor is empty and re-trips the same
		// rule. When the frozen diagnostics carry the committed base-diff
		// shape, the named continuation carries the exact selectors that make
		// it the recovery that actually clears this gate.
		if scope.RecoveryScope == reviewtransaction.RecoveryScopeCommittedBaseDiff && scope.RecoveryBaseRef != "" {
			operation = fmt.Sprintf("%s --base-ref %s --committed-only", operation, scope.RecoveryBaseRef)
		}
		// List only what the operator must actually supply. The frozen
		// diagnostics carry the operation's complete input set (the negotiated
		// envelope keeps naming all of it, pinned by the published v1 schema)
		// plus the subset the recovery mints for itself from this predecessor
		// shape. Naming a derived input here would be a false instruction: it
		// sends an operator hunting for a value the command already knows.
		if demanded := reviewRecoveryDemandedInputs(scope); len(demanded) > 0 {
			return fmt.Sprintf("%s: recover via %s (requires: %s)", message, operation, strings.Join(demanded, ", "))
		}
		return fmt.Sprintf("%s: recover via %s", message, operation)
	}
	if continuation := reviewDiscoveryDenialContinuation(err.Context.Denial); continuation != "" {
		return fmt.Sprintf("%s: %s", message, continuation)
	}
	if reviewGateAction(err.Result) == "continue" {
		return message
	}
	// A base mismatch is the one denial whose published reason is a complete
	// sentence and still leaves the operator with nothing to act on: it says
	// the base no longer matches and the envelope beside it repeats the base
	// the operator supplied. When the gate froze both sides of the comparison,
	// the terminal states both. No command is named because none is derivable
	// here -- what unblocks this is rebuilding the publication on the reviewed
	// base, and that is a fact about the repository, not a gentle-ai
	// invocation.
	if mismatch := err.Context.BaseMismatch; mismatch != nil && strings.TrimSpace(err.Reason) != "" {
		return fmt.Sprintf("%s: %s: the reviewed base is %s, but this target was derived from %s",
			message, strings.TrimSpace(err.Reason), mismatch.Expected, mismatch.Actual)
	}
	// No recovery is derivable here, so no command is named. What IS available
	// is the exact reason the JSON gate-result envelope publishes beside this
	// error, and throwing it away was the whole defect: an operator whose
	// already-published delivery no longer binds to its receipt read
	// "requires explicit maintainer action" while the envelope one line above
	// said "reviewed delivery base commit is missing or ambiguous in
	// publication range". Some of those reasons carry their own runnable
	// continuation (the empty-base first publication, the plural-receipt
	// --lineage selection); the ones that do not state a situation, which is
	// still incomparably more than a maintainer hand-off.
	if reason := strings.TrimSpace(err.Reason); reason != "" {
		return fmt.Sprintf("%s: %s", message, reason)
	}
	// "explicit-maintainer-action" is a routing token, not a runnable command.
	// A denial that carries no reason at all lands here, honestly, instead of
	// fabricating a fix.
	return fmt.Sprintf("%s: requires explicit maintainer action", message)
}

// reviewEscalatedSuccessorPlaceholder marks the one recovery input an operator
// genuinely has to choose. Every other input is derived from the denial itself,
// and naming a derived one would send them hunting for a value the command
// already knows.
const reviewEscalatedSuccessorPlaceholder = "<new-lineage-name>"

// reviewEscalatedContinuation renders what an operator does about a terminally
// escalated lineage.
//
// It deliberately does NOT name `review status`. Status re-derives escalated
// recovery eligibility, which is genuine routing knowledge for an integrator
// polling the negotiated contract -- reviewGateAction still publishes
// "review.status" as the wire next_action for exactly that reason. But for a
// human it resolves nothing: with the candidate unchanged (which is the only
// shape that reaches this gate, since any change re-routes the gate to
// scope-changed) status reports action "stop", so printing it named a dead end
// twice over -- an identifier that is not a command, pointing at a read that
// does not unblock.
//
// The one continuation that exists is the one the corpus proves: change the
// candidate, then create a successor authority over the fix. Its actor, reason
// and maintainer authorization are self-derived for this predecessor shape, so
// the four selectors below are the complete input set.
func reviewEscalatedContinuation(err ReviewGateDeniedError) string {
	statement := strings.TrimSpace(err.Reason)
	if statement == "" {
		statement = "this authority is terminally escalated"
	}
	command := reviewRunnableCommand("review.recover")
	lineage, revision := strings.TrimSpace(err.Context.LineageID), strings.TrimSpace(err.Context.StoreRevision)
	if lineage == "" || revision == "" {
		return fmt.Sprintf("%s; this lineage is terminal, so change the candidate and review the fix as a successor, reading the predecessor lineage and revision from %s: %s --predecessor-lineage <lineage-id> --expected-predecessor-revision <store-revision> --successor-lineage %s --disposition escalated",
			statement, reviewRunnableCommand("review.status"), command, reviewEscalatedSuccessorPlaceholder)
	}
	return fmt.Sprintf("%s; this lineage is terminal, so change the candidate and review the fix as a successor: %s --predecessor-lineage %s --expected-predecessor-revision %s --successor-lineage %s --disposition escalated",
		statement, command, lineage, revision, reviewEscalatedSuccessorPlaceholder)
}

// reviewRunnableCommand renders a dotted operation identifier ("review.recover")
// as the command an operator can actually type.
//
// The dotted form is the wire token the negotiated review-integration contract
// publishes, and it stays exactly as it is on that surface. It is not a shell
// command though, so a human-facing refusal that prints it is naming a dead
// end: this is the single boundary where the two representations are
// translated, so the mapping cannot drift into per-message copies.
func reviewRunnableCommand(operation string) string {
	trimmed := strings.TrimSpace(operation)
	verb, dotted := strings.CutPrefix(trimmed, "review.")
	if !dotted {
		return trimmed
	}
	return "gentle-ai review " + strings.ReplaceAll(verb, "_", "-")
}

func (err ReviewGateDeniedError) Unwrap() error { return err.Cause }

// reviewDiscoveryDenialContinuation names the operator-runnable continuation
// for a receipt-discovery denial whose kind is a COMPLETED proof that no
// terminal receipt governs this candidate.
//
// The routing knowledge is not restated here: the denial's Stage/Code pair is
// the same value the negotiated envelope publishes, written at the single site
// that classifies a ReviewReceiptDiscoveryError into a gate evaluation, so this
// cannot drift from what the JSON says. Only the two kinds below are answered,
// and both mean the same thing -- discovery ran to completion and found nothing
// governing -- which is a block the operator clears by reviewing the candidate,
// exactly as the all-stale ambiguous discovery message already words it. That
// was the whole defect: "invalidated: requires explicit maintainer action" sent
// an operator with no receipt at all to a maintainer, while the envelope beside
// it already said "no terminal review receipt exists for gate validation".
//
// Everything else keeps the terminal fallback, deliberately:
//
//   - authority_corrupted is damage to the authority inventory, not an absent
//     receipt. Reviewing again neither repairs nor supersedes it, so naming
//     review start there would be a dead end.
//   - receipt_ambiguous carries two different situations under ONE wire code
//     (proven-stale-only, which wants review start, and genuinely competing
//     exact receipts, which want an operator-chosen --lineage). The code alone
//     cannot tell them apart, and guessing would name the wrong command half
//     the time.
//   - Every non-discovery stage -- receipt-binding, delivery-derivation,
//     target-resolution -- is untouched. Their recovery, when one exists, is
//     already derived from frozen GateScopeChangeDiagnostics above.
func reviewDiscoveryDenialContinuation(denial *reviewtransaction.GateDenial) string {
	if denial == nil || denial.Stage != "receipt-discovery" {
		return ""
	}
	switch ReviewReceiptDiscoveryKind(denial.Code) {
	case ReviewReceiptMissing, ReviewReceiptUnrelated:
		return "no terminal review receipt governs this candidate; review it with gentle-ai review start"
	}
	return ""
}

// reviewRecoveryDemandedInputs narrows the frozen recovery input set to the
// inputs an operator must supply, by removing the ones the recovery command
// self-mints for the predecessor shape the diagnostics named. Order is
// preserved. When nothing is marked derived -- a legacy transaction that
// records no predecessor state, or a state outside the self-deriving set --
// the complete set is demanded, exactly as before.
func reviewRecoveryDemandedInputs(scope *reviewtransaction.GateScopeChangeDiagnostics) []string {
	if len(scope.RecoveryDerivedInputs) == 0 {
		return scope.RecoveryRequiredInputs
	}
	derived := make(map[string]struct{}, len(scope.RecoveryDerivedInputs))
	for _, input := range scope.RecoveryDerivedInputs {
		derived[input] = struct{}{}
	}
	demanded := make([]string, 0, len(scope.RecoveryRequiredInputs))
	for _, input := range scope.RecoveryRequiredInputs {
		if _, self := derived[input]; !self {
			demanded = append(demanded, input)
		}
	}
	return demanded
}

type repeatedString []string

func (values *repeatedString) String() string { return strings.Join(*values, ",") }
func (values *repeatedString) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func RunReviewStart(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-start", stdout, "Read-only legacy v1 compatibility command. New authority is created with gentle-ai review start.")
	cwd := flags.String("cwd", "", "repository root")
	_ = flags.String("kind", string(reviewtransaction.TargetCurrentChanges), "legacy target kind")
	_ = flags.String("base-ref", "", "legacy base revision")
	_ = flags.String("revision", "", "legacy exact commit or A..B range")
	_ = flags.String("intended-untracked-manifest", "", "legacy intended-untracked manifest")
	lineage := flags.String("lineage", "", "review lineage identifier")
	_ = flags.String("mode", string(reviewtransaction.ModeOrdinary4R), "legacy review mode")
	_ = flags.Int("generation", 1, "legacy lineage generation")
	policyFile := flags.String("policy-file", "", "legacy review policy argument (read-only compatibility)")
	_ = flags.String("machine-transaction-out", "", "legacy non-authoritative transaction output")
	var ignored repeatedString
	flags.Var(&ignored, "intended-untracked", "legacy intended-untracked path")
	flags.Var(&ignored, "ledger-id", "legacy ledger finding ID")
	flags.Var(&ignored, "lens", "legacy selected review lens")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-start argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*policyFile) == "" {
		return errors.New("review-start requires --cwd, --lineage, and --policy-file")
	}
	return fmt.Errorf("%w: review-start cannot create v1 authority; use gentle-ai review start", reviewtransaction.NewLegacyReadOnlyError("review/start", *lineage))
}

func RunReviewResume(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-resume", stdout, "Re-emit the current authoritative review transaction without consuming budget.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	machineTransactionOut := flags.String("machine-transaction-out", "", "optional non-authoritative transaction JSON output path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-resume argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" {
		return errors.New("review-resume requires --cwd and --lineage")
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		return fmt.Errorf("load authoritative review transaction: %w", err)
	}
	transaction := chain.Records[len(chain.Records)-1].Transaction
	if strings.TrimSpace(*machineTransactionOut) != "" {
		if err := reviewtransaction.WriteTransactionAtomic(*machineTransactionOut, transaction); err != nil {
			return fmt.Errorf("write non-authoritative machine transaction output: %w", err)
		}
	}
	return encodeReviewJSON(stdout, ReviewResumeResult{
		Schema: ReviewResumeSchema, Operation: "review/resume", Target: transaction.Snapshot,
		Transaction: transaction, StoreAuthority: "repository-git-common-dir",
		StoreRevision: chain.HeadRevision, GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity,
	})
}

func RunReviewBundleExport(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-bundle-export", stdout, "Export compact current authority or a read-only legacy v1 chain.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	out := flags.String("out", "", "portable review chain bundle output path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-export argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("review-bundle-export requires --cwd, --lineage, and --out")
	}
	compact, compactErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), *cwd, *lineage)
	if compactErr == nil {
		transport, loadErr := compact.ExportTransport()
		if errors.Is(loadErr, reviewtransaction.ErrHistoricalCompatReadOnly) {
			// A v2-only historical lineage has no legacy chain; the fallback
			// would surface an unrelated missing-chain error.
			return fmt.Errorf("export compact review transport: %w", loadErr)
		}
		if loadErr == nil {
			legacy, legacyErr := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
			if legacyErr == nil {
				if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
					return errors.New("lineage exists in both compact and legacy stores; explicit cleanup is required")
				}
			}
			if err := reviewtransaction.WriteCompactTransportAtomic(*out, transport); err != nil {
				return fmt.Errorf("write compact review transport: %w", err)
			}
			return encodeReviewJSON(stdout, ReviewBundleResult{
				Schema: ReviewBundleSchema, Operation: "review/bundle-export", LineageID: transport.Record.State.LineageID,
				BundleDigest: transport.BundleDigest, StoreRevision: transport.Record.Revision,
				GenesisRevision: transport.Record.Revision, ChainIdentity: transport.Record.Revision, BundlePath: *out,
			})
		}
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		return fmt.Errorf("export authoritative review chain: %w", err)
	}
	if err := reviewtransaction.WriteChainBundleAtomic(*out, bundle); err != nil {
		return fmt.Errorf("write portable review chain bundle: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewBundleResult{
		Schema: ReviewBundleSchema, Operation: "review/bundle-export", LineageID: bundle.LineageID,
		BundleDigest: bundle.BundleDigest, StoreRevision: bundle.HeadRevision,
		GenesisRevision: bundle.GenesisRevision, ChainIdentity: bundle.ChainIdentity, BundlePath: *out,
	})
}

func RunReviewBundleImport(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-bundle-import", stdout, "Recover compact current authority or install a read-only legacy v1 chain.")
	cwd := flags.String("cwd", "", "repository root")
	bundlePath := flags.String("bundle", "", "portable review chain bundle")
	receiptPath := flags.String("receipt", "", "terminal review receipt")
	requestPath := flags.String("request", "", "gate request binding current artifacts and expected chain identity")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-import argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*bundlePath) == "" {
		return errors.New("review-bundle-import requires --cwd and --bundle")
	}
	if err := authorizeManagedReviewerAssets(); err != nil {
		return err
	}
	bundlePayload, err := os.ReadFile(*bundlePath)
	if err != nil {
		return fmt.Errorf("read review chain bundle: %w", err)
	}
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(bundlePayload, &envelope); err != nil {
		return fmt.Errorf("parse review transport header: %w", err)
	}
	if envelope.Schema == reviewtransaction.CompactTransportSchema {
		if strings.TrimSpace(*receiptPath) != "" || strings.TrimSpace(*requestPath) != "" {
			return errors.New("compact transport contains its receipt and does not accept legacy --receipt or --request ceremony")
		}
		transport, err := reviewtransaction.ParseCompactTransport(bundlePayload)
		if err != nil {
			return fmt.Errorf("parse compact review transport: %w", err)
		}
		record, err := reviewtransaction.ImportCompactTransport(context.Background(), *cwd, transport)
		if err != nil {
			return fmt.Errorf("recover compact review authority: %w", err)
		}
		return encodeReviewJSON(stdout, ReviewBundleResult{
			Schema: ReviewBundleSchema, Operation: "review/bundle-import", LineageID: record.State.LineageID,
			BundleDigest: transport.BundleDigest, StoreRevision: record.Revision,
			GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundlePath: *bundlePath,
		})
	}
	if strings.TrimSpace(*requestPath) == "" {
		return errors.New("legacy v1 review bundle import requires --request")
	}
	bundle, err := reviewtransaction.ParseChainBundle(bundlePayload)
	if err != nil {
		return fmt.Errorf("parse review chain bundle: %w", err)
	}
	var receipt reviewtransaction.Receipt
	if strings.TrimSpace(*receiptPath) != "" {
		receiptPayload, err := os.ReadFile(*receiptPath)
		if err != nil {
			return fmt.Errorf("read review receipt: %w", err)
		}
		receipt, err = reviewtransaction.ParseReceipt(receiptPayload)
		if err != nil {
			return fmt.Errorf("parse review receipt: %w", err)
		}
		if bundle.TerminalReceipt == nil {
			return errors.New("nonterminal review bundle cannot be imported with a terminal receipt")
		}
	} else if bundle.TerminalReceipt != nil {
		return errors.New("terminal review bundle import requires --receipt")
	}
	requestPayload, err := os.ReadFile(*requestPath)
	if err != nil {
		return fmt.Errorf("read review gate request: %w", err)
	}
	request, err := reviewtransaction.ParseGateRequest(requestPayload)
	if err != nil {
		return fmt.Errorf("parse review gate request: %w", err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).Build(context.Background(), request.Target)
	if err != nil {
		return fmt.Errorf("derive current repository target: %w", err)
	}
	policyHash, ledgerHash, evidenceHash := bundle.PolicyHash, bundle.LedgerHash, bundle.EvidenceHash
	if bundle.TerminalReceipt != nil {
		policyHash, err = reviewtransaction.HashArtifact(request.PolicyArtifact)
		if err != nil {
			return fmt.Errorf("hash policy artifact: %w", err)
		}
		ledgerHash, err = reviewtransaction.HashLedgerArtifact(request.LedgerArtifact)
		if err != nil {
			return fmt.Errorf("hash ledger artifact: %w", err)
		}
		evidenceHash, err = reviewtransaction.HashArtifact(request.EvidenceArtifact)
		if err != nil {
			return fmt.Errorf("hash evidence artifact: %w", err)
		}
	}
	fixDeltaHash := ""
	chain, err := reviewtransaction.ImportBundle(context.Background(), *cwd, bundle, reviewtransaction.BundleImportExpectation{
		LineageID: bundle.LineageID, Snapshot: snapshot,
		PolicyHash: policyHash, LedgerHash: ledgerHash, EvidenceHash: evidenceHash, FixDeltaHash: fixDeltaHash, Receipt: receipt,
		GenesisRevision: request.GenesisRevision, HeadRevision: request.StoreRevision,
		ChainIdentity: request.ChainIdentity, BundleDigest: request.BundleDigest,
	})
	if err != nil {
		return fmt.Errorf("install validated review chain bundle: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewBundleResult{
		Schema: ReviewBundleSchema, Operation: "review/bundle-import", LineageID: bundle.LineageID,
		BundleDigest: bundle.BundleDigest, StoreRevision: chain.HeadRevision,
		GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity, BundlePath: *bundlePath,
	})
}

// RunReviewValidateNonDeciding is the shipped legacy review-validate route.
// It retains legacy syntax only far enough to identify the repository and gate;
// it never reads a receipt, authority, candidate, or gate evidence. The retained
// RunReviewValidate evaluator below remains available for internal historical
// coverage that explicitly exercises the old gate algebra.
func RunReviewValidateNonDeciding(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-validate", stdout, "Report the repository policy that governs delivery without deciding review authority.")
	cwd := flags.String("cwd", "", "repository root")
	_ = flags.String("receipt", "", "accepted historical review receipt JSON")
	requestPath := flags.String("request", "", "optional historical gate request JSON used only to identify the gate")
	_ = flags.String("lineage", "", "accepted historical authoritative review lineage identifier")
	gate := flags.String("gate", "", "lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release")
	_ = flags.String("bundle", "", "accepted historical authoritative chain bundle artifact")
	_ = flags.String("policy", "", "accepted historical receipt-bound policy artifact")
	_ = flags.String("ledger", "", "accepted historical frozen ledger artifact")
	_ = flags.String("fix-delta", "", "accepted historical correction delta artifact")
	_ = flags.String("evidence", "", "accepted historical final verification evidence artifact")
	_ = flags.String("base-ref", "", "accepted historical expected remote publication base")
	_ = flags.String("pre-pr-ci-attestation", "", "accepted historical pre-pr CI attestation")
	_ = flags.String("request-out", "", "accepted historical canonical native gate request output path")
	_ = flags.String("release-configuration", "", "accepted historical release configuration artifact")
	_ = flags.String("release-generated", "", "accepted historical generated artifact manifest")
	_ = flags.String("release-provenance", "", "accepted historical release provenance artifact")
	_ = flags.String("release-publication-boundary", "", "accepted historical sealed publication boundary artifact")
	_ = flags.String("release-evidence-freshness", "", "accepted historical current release evidence artifact")
	_ = flags.String("intended-untracked-manifest", "", "accepted historical intended untracked manifest")
	var intended repeatedString
	flags.Var(&intended, "intended-untracked", "accepted historical repository-relative intended untracked path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-validate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("review-validate requires --cwd") // refusal:by-design operator-knowledge: only the caller knows the repository; retry with --cwd <repo>
	}
	selectedGate := reviewtransaction.GateKind(strings.TrimSpace(*gate))
	if selectedGate == "" && strings.TrimSpace(*requestPath) != "" {
		var requestHeader struct {
			Gate reviewtransaction.GateKind `json:"gate"`
		}
		payload, err := os.ReadFile(*requestPath)
		if err != nil {
			return fmt.Errorf("read review gate request: %w", err)
		}
		if err := json.Unmarshal(payload, &requestHeader); err != nil {
			return fmt.Errorf("parse review gate request gate: %w", err)
		}
		selectedGate = requestHeader.Gate
	}
	if !validReviewIntegrationGate(selectedGate) {
		return fmt.Errorf("review-validate requires --gate, or --request containing one of %s", strings.Join(reviewIntegrationGateNames(), ", ")) // refusal:by-design operator-knowledge: only the caller knows the intended delivery gate
	}
	return runNonDecidingReviewGate(context.Background(), *cwd, selectedGate, false, "", stdout)
}

func reviewGateAction(result reviewtransaction.GateResult) string {
	switch result {
	case reviewtransaction.GateAllow:
		return "continue"
	case reviewtransaction.GateScopeChanged:
		return "explicit-maintainer-action"
	case reviewtransaction.GateEscalated:
		// STATUS already re-derives escalated-authority recovery eligibility
		// (accounting-only, changed-target, or final-verification-retry); a
		// bare stop here told the caller nothing it did not already know.
		return "review.status"
	default:
		return "explicit-maintainer-action"
	}
}

// reviewManifestArrayKey is the one emitted object key whose array elements are
// written one per line instead of one field per line.
//
// The changed-path manifest is the only field this product emits whose size is
// proportional to the candidate's changed-file count and whose shape it cannot
// reduce: the published contract fixes its eight required per-entry fields and
// forbids additional ones (contracts/review-integration/v2/schemas/
// start.schema.json#/$defs/changed_path), and every negotiated route that
// carries reviewer context is required to carry it. What this product does own
// is how many lines those fixed fields are spread across. Indented per field
// they cost ten lines per entry, which is how a 149-path candidate reached a
// 1,505-line payload in a consumer's conversation (issue #3281). One line per
// entry is the floor that keeps the field intact.
//
// This is insignificant whitespace only. The emitted bytes decode to exactly
// the same value, so every published-schema validation, every artifact-subject
// digest taken over the manifest, and every consumer that parses rather than
// pattern-matches the payload are unaffected.
const reviewManifestArrayKey = `"changed_path_manifest": [`

func encodeReviewJSON(stdout io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	// json.Encoder.SetIndent is exactly Marshal followed by Indent, so keeping
	// both steps explicit leaves every non-manifest envelope byte-identical to
	// the encoder this product has always used.
	var indented bytes.Buffer
	if err := json.Indent(&indented, payload, "", "  "); err != nil {
		return err
	}
	emitted := append(compactReviewChangedPathManifests(indented.Bytes()), '\n')
	_, err = stdout.Write(emitted)
	return err
}

// compactReviewChangedPathManifests rewrites every changed-path manifest array
// in already-indented JSON so each entry occupies one line. Any array it cannot
// fully account for is copied through untouched, because emitting a payload
// this pass only partly understood would be worse than emitting a verbose one.
func compactReviewChangedPathManifests(indented []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(indented))
	cursor := 0
	for cursor < len(indented) {
		key := indexReviewManifestArrayKey(indented, cursor)
		if key < 0 {
			break
		}
		open := key + len(reviewManifestArrayKey) - 1
		end := reviewJSONArrayEnd(indented, open)
		var entries []json.RawMessage
		if end < 0 || json.Unmarshal(indented[open:end+1], &entries) != nil {
			out.Write(indented[cursor : open+1])
			cursor = open + 1
			continue
		}
		compacted, ok := compactReviewManifestEntries(entries, reviewJSONLineIndent(indented, key))
		if !ok {
			out.Write(indented[cursor : end+1])
			cursor = end + 1
			continue
		}
		out.Write(indented[cursor:open])
		out.Write(compacted)
		cursor = end + 1
	}
	out.Write(indented[cursor:])
	return out.Bytes()
}

// compactReviewManifestEntries renders one manifest array in the exact layout
// json.Indent would use for the array itself, with each entry on a single line.
func compactReviewManifestEntries(entries []json.RawMessage, indent string) ([]byte, bool) {
	var out bytes.Buffer
	out.WriteByte('[')
	for index, entry := range entries {
		if index > 0 {
			out.WriteByte(',')
		}
		out.WriteByte('\n')
		out.WriteString(indent)
		out.WriteString("  ")
		if err := json.Compact(&out, entry); err != nil {
			return nil, false
		}
	}
	if len(entries) > 0 {
		out.WriteByte('\n')
		out.WriteString(indent)
	}
	out.WriteByte(']')
	return out.Bytes(), true
}

// indexReviewManifestArrayKey finds the next manifest array key that is a real
// object key rather than text inside a string value. Indented JSON always
// precedes an object key with a line break and its indent, and a raw line break
// can never appear inside a JSON string, so that prefix is proof of position.
func indexReviewManifestArrayKey(indented []byte, from int) int {
	for cursor := from; cursor < len(indented); {
		offset := bytes.Index(indented[cursor:], []byte(reviewManifestArrayKey))
		if offset < 0 {
			return -1
		}
		match := cursor + offset
		if reviewJSONLineStart(indented, match) >= 0 {
			return match
		}
		cursor = match + 1
	}
	return -1
}

// reviewJSONLineStart reports the offset just past the line break preceding
// position, provided only indent spaces separate them, and -1 otherwise.
func reviewJSONLineStart(indented []byte, position int) int {
	for cursor := position - 1; cursor >= 0; cursor-- {
		switch indented[cursor] {
		case ' ':
		case '\n':
			return cursor + 1
		default:
			return -1
		}
	}
	return -1
}

// reviewJSONLineIndent returns the indent of the line holding position.
func reviewJSONLineIndent(indented []byte, position int) string {
	start := reviewJSONLineStart(indented, position)
	if start < 0 {
		return ""
	}
	return string(indented[start:position])
}

// reviewJSONArrayEnd returns the offset of the bracket closing the array that
// opens at open, or -1 when the input is not a balanced array from there.
func reviewJSONArrayEnd(indented []byte, open int) int {
	if open >= len(indented) || indented[open] != '[' {
		return -1
	}
	depth := 0
	inString := false
	escaped := false
	for cursor := open; cursor < len(indented); cursor++ {
		character := indented[cursor]
		switch {
		case escaped:
			escaped = false
		case inString && character == '\\':
			escaped = true
		case character == '"':
			inString = !inString
		case inString:
		case character == '[' || character == '{':
			depth++
		case character == ']' || character == '}':
			depth--
			if depth == 0 {
				if character != ']' {
					return -1
				}
				return cursor
			}
		}
	}
	return -1
}
