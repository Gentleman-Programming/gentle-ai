package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/qastage"
)

// qaLedgerRoot resolves the QAStateStore root for a repository: <cwd>/.gentle-ai.
func qaLedgerRoot(cwd string) (string, error) {
	if cwd == "" {
		abs, err := filepath.Abs(".")
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		cwd = abs
	}
	return filepath.Join(cwd, ".gentle-ai"), nil
}

func newQAMachine(cwd string) (*qastage.QAStateMachine, error) {
	root, err := qaLedgerRoot(cwd)
	if err != nil {
		return nil, err
	}
	return qastage.NewQAStateMachine(qastage.NewPersistentQAStateStore(root)), nil
}

// checkExpectedRevision honors the CONSERVAR disposition for
// --expected-revision (design doc, section 1.1): when the caller supplies
// one, it must match the change's actual current revision before any
// mutation proceeds.
func checkExpectedRevision(ctx context.Context, machine *qastage.QAStateMachine, change, expected string) error {
	if expected == "" {
		return nil
	}
	status, err := machine.Status(ctx, change)
	if err != nil {
		return err
	}
	if status.Revision != expected {
		return fmt.Errorf("expected-revision %q does not match current revision %q; rerun `gentle-ai qa-status` for the current value", expected, status.Revision)
	}
	return nil
}

// ---- qa-begin ----

// QABeginResult is the JSON-encoded result of qa-begin.
type QABeginResult struct {
	Change     string `json:"change"`
	Stage      string `json:"stage"`
	Ordinal    int    `json:"ordinal"`
	Outcome    string `json:"outcome"`
	NextAction string `json:"next_action"`
}

// RunQABegin is the CLI entry point for `gentle-ai qa-begin`. Retired flags
// (design doc 1.1): --evidence-goal, --max-attempts, --max-changed-lines.
// --max-attempts is replaced by QAPolicy (not yet wired — 3A.9 scope is the
// flag surface, not the policy engine); --evidence-goal's contract is
// derived from each stage's canonical artifact schema (3A.1) instead.
func RunQABegin(args []string, stdout io.Writer) error {
	if hasHelpFlag(args) {
		return renderQABeginHelp(stdout)
	}
	flags := flag.NewFlagSet("qa-begin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	change := flags.String("change", "", "")
	stage := flags.String("stage", "", "")
	cwd := flags.String("cwd", "", "")
	requestID := flags.String("request-id", "", "")
	expectedRevision := flags.String("expected-revision", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-begin argument %q; run `gentle-ai qa-begin --help` for valid flags", flags.Arg(0))
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("qa-begin requires --change; run `gentle-ai qa-begin --help` for usage")
	}
	if strings.TrimSpace(*stage) == "" {
		return errors.New("qa-begin requires --stage; run `gentle-ai qa-begin --help` for usage")
	}
	if strings.TrimSpace(*requestID) == "" {
		return errors.New("qa-begin requires --request-id; run `gentle-ai qa-begin --help` for usage")
	}

	ctx := context.Background()
	machine, err := newQAMachine(*cwd)
	if err != nil {
		return err
	}
	if err := checkExpectedRevision(ctx, machine, *change, *expectedRevision); err != nil {
		return fmt.Errorf("qa-begin: %w", err)
	}

	attempt, err := machine.Begin(ctx, *change, *stage, *requestID)
	if err != nil {
		return fmt.Errorf("qa-begin: %w", err)
	}
	status, err := machine.Status(ctx, *change)
	if err != nil {
		return fmt.Errorf("qa-begin: %w", err)
	}

	return encodeJSON(stdout, QABeginResult{
		Change:     *change,
		Stage:      attempt.Stage,
		Ordinal:    attempt.Ordinal,
		Outcome:    string(attempt.Outcome),
		NextAction: status.NextAction,
	})
}

func renderQABeginHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai qa-begin --change <name> --stage <label> [--cwd <repo>] --request-id <id> [--expected-revision <rev>]")
	_, _ = fmt.Fprintln(stdout, "\nFlags:")
	_, _ = fmt.Fprintln(stdout, "  --change <name>              Change name to begin/advance (required)")
	_, _ = fmt.Fprintln(stdout, "  --stage <label>              Stage label (explore|spec|apply|verify|docs) (required)")
	_, _ = fmt.Fprintln(stdout, "  --cwd <repo>                 Repository root (defaults to current directory)")
	_, _ = fmt.Fprintln(stdout, "  --request-id <id>            Idempotency key for this begin call (required)")
	_, _ = fmt.Fprintln(stdout, "  --expected-revision <rev>    Expected current ledger revision (optional CAS check)")
	_, _ = fmt.Fprintln(stdout, "\nOpens or advances the QA ledger for --change into --stage. Refuses stage")
	_, _ = fmt.Fprintln(stdout, "labels that are out of order or unapproved, from the very first begin.")
	return nil
}

// ---- qa-finish ----

// QAFinishResult is the JSON-encoded result of qa-finish.
type QAFinishResult struct {
	Change     string `json:"change"`
	Stage      string `json:"stage"`
	Outcome    string `json:"outcome"`
	NextAction string `json:"next_action"`
}

// RunQAFinish is the CLI entry point for `gentle-ai qa-finish`. Retired
// flags: --diagnosis, --harness-disposition, --cleanup-evidence,
// --process-evidence — that metadata now belongs in the stage artifact body
// qa-validate admits, not in ledger-tracked CLI flags. --evidence-revision
// is kept, but must be the artifact_revision qa-validate actually produced
// (3A.1/3A.2), not an arbitrary well-formed hash.
func RunQAFinish(args []string, stdout io.Writer) error {
	if hasHelpFlag(args) {
		return renderQAFinishHelp(stdout)
	}
	flags := flag.NewFlagSet("qa-finish", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	change := flags.String("change", "", "")
	cwd := flags.String("cwd", "", "")
	requestID := flags.String("request-id", "", "")
	outcome := flags.String("outcome", "", "")
	evidenceRevision := flags.String("evidence-revision", "", "")
	expectedRevision := flags.String("expected-revision", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-finish argument %q; run `gentle-ai qa-finish --help` for valid flags", flags.Arg(0))
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("qa-finish requires --change; run `gentle-ai qa-finish --help` for usage")
	}
	if strings.TrimSpace(*requestID) == "" {
		return errors.New("qa-finish requires --request-id; run `gentle-ai qa-finish --help` for usage")
	}
	if strings.TrimSpace(*outcome) == "" {
		return errors.New("qa-finish requires --outcome; run `gentle-ai qa-finish --help` for usage")
	}
	if strings.TrimSpace(*evidenceRevision) == "" {
		return errors.New("qa-finish requires --evidence-revision; run `gentle-ai qa-finish --help` for usage")
	}

	ctx := context.Background()
	machine, err := newQAMachine(*cwd)
	if err != nil {
		return err
	}
	if err := checkExpectedRevision(ctx, machine, *change, *expectedRevision); err != nil {
		return fmt.Errorf("qa-finish: %w", err)
	}

	attempt, err := machine.Finish(ctx, *change, qastage.AttemptOutcome(*outcome), *evidenceRevision, *requestID)
	if err != nil {
		return fmt.Errorf("qa-finish: %w", err)
	}
	status, err := machine.Status(ctx, *change)
	if err != nil {
		return fmt.Errorf("qa-finish: %w", err)
	}

	return encodeJSON(stdout, QAFinishResult{
		Change:     *change,
		Stage:      attempt.Stage,
		Outcome:    string(attempt.Outcome),
		NextAction: status.NextAction,
	})
}

func renderQAFinishHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai qa-finish --change <name> [--cwd <repo>] --request-id <id> --outcome <passed|failed|interrupted> --evidence-revision <rev> [--expected-revision <rev>]")
	_, _ = fmt.Fprintln(stdout, "\nFlags:")
	_, _ = fmt.Fprintln(stdout, "  --change <name>              Change name whose active attempt is finishing (required)")
	_, _ = fmt.Fprintln(stdout, "  --cwd <repo>                 Repository root (defaults to current directory)")
	_, _ = fmt.Fprintln(stdout, "  --request-id <id>            Idempotency key for this finish call (required)")
	_, _ = fmt.Fprintln(stdout, "  --outcome <value>            passed, failed, or interrupted (required)")
	_, _ = fmt.Fprintln(stdout, "  --evidence-revision <rev>    The artifact_revision qa-validate produced for this stage (required)")
	_, _ = fmt.Fprintln(stdout, "  --expected-revision <rev>    Expected current ledger revision (optional CAS check)")
	return nil
}

// ---- qa-approve ----

// QAApproveResult is the JSON-encoded result of qa-approve.
type QAApproveResult struct {
	Change           string `json:"change"`
	Stage            string `json:"stage"`
	ArtifactRevision string `json:"artifact_revision"`
	Actor            string `json:"actor"`
}

// RunQAApprove is the CLI entry point for `gentle-ai qa-approve`.
// --evidence-revision here is the artifact_revision of the completed stage
// being approved; a new revision of that stage invalidates this approval
// (3A.8).
func RunQAApprove(args []string, stdout io.Writer) error {
	if hasHelpFlag(args) {
		return renderQAApproveHelp(stdout)
	}
	flags := flag.NewFlagSet("qa-approve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	change := flags.String("change", "", "")
	cwd := flags.String("cwd", "", "")
	stage := flags.String("stage", "", "")
	evidenceRevision := flags.String("evidence-revision", "", "")
	actor := flags.String("actor", "", "")
	reason := flags.String("reason", "", "")
	requestID := flags.String("request-id", "", "")
	expectedRevision := flags.String("expected-revision", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-approve argument %q; run `gentle-ai qa-approve --help` for valid flags", flags.Arg(0))
	}
	for name, value := range map[string]string{"change": *change, "stage": *stage, "evidence-revision": *evidenceRevision, "actor": *actor, "reason": *reason, "request-id": *requestID} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("qa-approve requires --%s; run `gentle-ai qa-approve --help` for usage", name)
		}
	}

	ctx := context.Background()
	machine, err := newQAMachine(*cwd)
	if err != nil {
		return err
	}
	if err := checkExpectedRevision(ctx, machine, *change, *expectedRevision); err != nil {
		return fmt.Errorf("qa-approve: %w", err)
	}

	approval, err := machine.Approve(ctx, *change, *stage, *evidenceRevision, *actor, *reason, *requestID)
	if err != nil {
		return fmt.Errorf("qa-approve: %w", err)
	}

	return encodeJSON(stdout, QAApproveResult{
		Change:           *change,
		Stage:            approval.Stage,
		ArtifactRevision: approval.ArtifactRevision,
		Actor:            approval.Actor,
	})
}

func renderQAApproveHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai qa-approve --change <name> [--cwd <repo>] --stage <label> --evidence-revision <rev> --actor <name> --reason <text> --request-id <id> [--expected-revision <rev>]")
	_, _ = fmt.Fprintln(stdout, "\nRecords an audited approval for --stage bound to --evidence-revision. A")
	_, _ = fmt.Fprintln(stdout, "later revision of that stage invalidates this approval.")
	return nil
}

// ---- qa-reset ----

// QAResetResult is the JSON-encoded result of qa-reset.
type QAResetResult struct {
	Change  string `json:"change"`
	Stage   string `json:"stage"`
	Outcome string `json:"outcome"`
	Actor   string `json:"actor"`
	Reason  string `json:"reason"`
}

// RunQAReset is the CLI entry point for `gentle-ai qa-reset`: an audited
// manual recovery for a stuck active attempt (3A.6). It never deletes
// history — the closed attempt remains in Attempts, marked interrupted with
// its actor/reason, and the stage becomes retryable again.
func RunQAReset(args []string, stdout io.Writer) error {
	if hasHelpFlag(args) {
		_, err := fmt.Fprintln(stdout, "Usage: gentle-ai qa-reset --change <name> [--cwd <repo>] --actor <name> --reason <text>\n\nCloses a stuck active attempt as interrupted, audited with --actor/--reason. Never deletes history.")
		return err
	}
	flags := flag.NewFlagSet("qa-reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	change := flags.String("change", "", "")
	cwd := flags.String("cwd", "", "")
	actor := flags.String("actor", "", "")
	reason := flags.String("reason", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-reset argument %q; run `gentle-ai qa-reset --help` for valid flags", flags.Arg(0))
	}
	for name, value := range map[string]string{"change": *change, "actor": *actor, "reason": *reason} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("qa-reset requires --%s; run `gentle-ai qa-reset --help` for usage", name)
		}
	}

	ctx := context.Background()
	machine, err := newQAMachine(*cwd)
	if err != nil {
		return err
	}

	attempt, err := machine.Reset(ctx, *change, *actor, *reason)
	if err != nil {
		return fmt.Errorf("qa-reset: %w", err)
	}

	return encodeJSON(stdout, QAResetResult{
		Change:  *change,
		Stage:   attempt.Stage,
		Outcome: string(attempt.Outcome),
		Actor:   attempt.ResetBy,
		Reason:  attempt.ResetReason,
	})
}

// ---- qa-status ----

// QAStatusResult is the JSON-encoded result of qa-status. Stage is the stage
// next_action applies to (what to begin, or what's running to finish) —
// callers like qa-supervisor use it to know what to delegate without ever
// deciding order themselves.
type QAStatusResult struct {
	Change     string `json:"change"`
	Revision   string `json:"revision"`
	NextAction string `json:"next_action"`
	Stage      string `json:"stage,omitempty"`
	Complete   bool   `json:"complete"`
}

// RunQAStatus is the CLI entry point for `gentle-ai qa-status`: read-only,
// always recalculated from history — never cached (preserved from v2).
func RunQAStatus(args []string, stdout io.Writer) error {
	if hasHelpFlag(args) {
		_, err := fmt.Fprintln(stdout, "Usage: gentle-ai qa-status --change <name> [--cwd <repo>]\n\nOutputs JSON with the current QA stage and next action.")
		return err
	}
	flags := flag.NewFlagSet("qa-status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	change := flags.String("change", "", "")
	cwd := flags.String("cwd", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-status argument %q; run `gentle-ai qa-status --help` for valid flags", flags.Arg(0))
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("qa-status requires --change; run `gentle-ai qa-status --help` for usage")
	}

	ctx := context.Background()
	machine, err := newQAMachine(*cwd)
	if err != nil {
		return err
	}
	status, err := machine.Status(ctx, *change)
	if err != nil {
		return fmt.Errorf("qa-status: %w", err)
	}

	return encodeJSON(stdout, QAStatusResult{
		Change:     status.Change,
		Revision:   status.Revision,
		NextAction: status.NextAction,
		Stage:      status.Stage,
		Complete:   status.Complete,
	})
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func encodeJSON(stdout io.Writer, v interface{}) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}
