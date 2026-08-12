package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// ReviewModeSchema identifies the user-facing kill-switch projection.
const ReviewModeSchema = "gentle-ai.review-mode/v1"

const (
	reviewModeScopeGlobal = "global"
	reviewModeScopeClone  = "clone"
	reviewModeScopeBoth   = "both"
)

// ReviewModeResult reports what the command did and the resulting effective
// mode with both of its sources. It carries no review outcome.
type ReviewModeResult struct {
	Schema    string                          `json:"schema"`
	Operation string                          `json:"operation"`
	Scope     string                          `json:"scope"`
	Status    reviewtransaction.RDDModeStatus `json:"status"`
}

// RunReviewMode is the user-controlled receipt-driven-development kill switch.
// The global mode lives in uncommitted user state; the clone-local override
// lives under this clone's Git common directory and can only disable. Any off
// wins, status never mutates, and re-enabling applies to future candidates only.
func RunReviewMode(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai review mode <enable|disable|status> [--cwd <repo>] [--scope <global|clone>] [--expected-revision <revision>] [--json]")
		_, _ = fmt.Fprintln(stdout, "User-owned kill switch. Any off wins: a repository may disable receipt-driven development for this clone but can never require it, and no other clone inherits the override. status is read-only and reports both sources plus the effective mode. Re-enabling applies to future candidates only.")
		return nil
	}
	operation := args[0]
	switch operation {
	case "enable", "disable", "status":
	default:
		return fmt.Errorf("unknown review mode command %q", operation)
	}

	flags := newReviewFlagSet("review mode "+operation, stdout, "Read or set the user-controlled receipt-driven-development kill switch.")
	cwd := flags.String("cwd", ".", "repository path")
	scope := flags.String("scope", reviewModeScopeGlobal, "mode source to write: global or clone")
	expectedRevision := flags.String("expected-revision", "", "exact clone-local revision this write replaces")
	emitJSON := flags.Bool("json", false, "emit the machine-readable review mode result")
	if err := parseReviewFlags(flags, args[1:]); err != nil {
		return err
	}
	if reviewHelpRequested(args[1:]) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review mode argument %q", flags.Arg(0))
	}
	selectedScope := strings.TrimSpace(*scope)
	switch selectedScope {
	case reviewModeScopeGlobal, reviewModeScopeClone:
	default:
		return fmt.Errorf("unknown review mode scope %q", *scope)
	}
	revisionProvided := false
	flags.Visit(func(current *flag.Flag) {
		if current.Name == "expected-revision" {
			revisionProvided = true
		}
	})

	ctx := context.Background()
	result := ReviewModeResult{Schema: ReviewModeSchema, Operation: operation, Scope: selectedScope}
	var err error
	if operation == "status" {
		result.Scope = reviewModeScopeBoth
		result.Status, err = reviewModeStatus(ctx, *cwd)
	} else {
		result.Status, err = applyReviewMode(ctx, *cwd, operation, selectedScope, *expectedRevision, revisionProvided)
	}
	if emitErr := emitReviewMode(stdout, result, *emitJSON); emitErr != nil && err == nil {
		return emitErr
	}
	return err
}

// reviewModeStatus is strictly read-only: it never creates user state and never
// creates repository state.
func reviewModeStatus(ctx context.Context, repo string) (reviewtransaction.RDDModeStatus, error) {
	global, err := readGlobalRDDMode()
	if err != nil {
		return reviewtransaction.RDDModeStatus{Schema: reviewtransaction.RDDModeStatusSchema, Effective: reviewtransaction.RDDModeOff}, err
	}
	status, err := reviewtransaction.ResolveRDDMode(ctx, repo, global)
	return status, reviewModeUnreadable(ctx, repo, global, err)
}

// ReviewModeUnreadableScope names one kill-switch source whose persisted value
// cannot be read as a mode, the file that holds it, and the repository that
// file belongs to.
type ReviewModeUnreadableScope struct {
	Scope string
	Path  string
	Repo  string
}

func (scope ReviewModeUnreadableScope) label() string {
	if scope.Scope == reviewModeScopeClone {
		return "clone-local"
	}
	return "global"
}

// commands names the two invocations that overwrite this scope. Both are
// complete as printed: the clone-local scope carries the repository it belongs
// to, because a clone-local record is only reachable through its own clone.
func (scope ReviewModeUnreadableScope) commands() []string {
	suffix := " --scope=" + scope.Scope
	if scope.Scope == reviewModeScopeClone {
		suffix += " --cwd " + scope.Repo
	}
	return []string{
		"`gentle-ai review mode enable" + suffix + "`",
		"`gentle-ai review mode disable" + suffix + "`",
	}
}

// ReviewModeUnreadableError reports a kill-switch record that exists but cannot
// be read as a mode. Failing closed is deliberate and stays exactly as it was:
// an unintelligible switch is not a disabled switch, so it resolves to managed
// and every start and mutation still refuses. What the refusal has to carry is
// the file holding the value and the command that overwrites it, both of which
// the product already knows and neither of which it used to say.
type ReviewModeUnreadableError struct {
	Scopes []ReviewModeUnreadableScope
	Cause  error
}

func (err *ReviewModeUnreadableError) Unwrap() error { return err.Cause }

func (err *ReviewModeUnreadableError) Error() string {
	places := make([]string, 0, len(err.Scopes))
	commands := make([]string, 0, 2*len(err.Scopes))
	for _, scope := range err.Scopes {
		places = append(places, fmt.Sprintf("the %s value in %s", scope.label(), scope.Path))
		commands = append(commands, scope.commands()...)
	}
	subject := strings.Join(places, " and ")
	if len(places) > 1 {
		subject = "both review mode sources hold a value this product cannot read as a mode: " + subject
	} else {
		subject += " is not a mode this product can read"
	}
	return fmt.Sprintf(
		"%s; an unreadable switch is not a disabled switch, so receipt-driven development stays managed until the value is overwritten: run %s to turn reviews on, or %s to turn them off",
		subject,
		strings.Join(reviewModeCommandsByVerb(commands, "enable"), " and "),
		strings.Join(reviewModeCommandsByVerb(commands, "disable"), " and "),
	)
}

type reviewModeUnsafePathError struct {
	Path      string
	Directory bool
	Cause     error
}

func (err *reviewModeUnsafePathError) Unwrap() error { return err.Cause }

func (err *reviewModeUnsafePathError) Error() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(
			"the clone-local review mode path %s is unsafe; run `%s` in a NON-ELEVATED Windows PowerShell 5.1 shell because an elevated TokenOwner can differ from the current user, then rerun the original command",
			pathquote.Quote(err.Path), err.repairCommand(),
		)
	}
	return fmt.Sprintf(
		"the clone-local review mode path %s is unsafe; run `%s`, then rerun the original command",
		pathquote.Quote(err.Path), err.repairCommand(),
	)
}

func (err *reviewModeUnsafePathError) repairCommand() string {
	if runtime.GOOS != "windows" {
		mode := "600"
		if err.Directory {
			mode = "700"
		}
		return "chmod " + mode + " " + quotePOSIXShellToken(err.Path)
	}
	securityType := "FileSecurity"
	inheritance := "'None'"
	setAccessControl := "[System.IO.File]::SetAccessControl($p, $acl)"
	if err.Directory {
		securityType = "DirectorySecurity"
		inheritance = "'ContainerInherit, ObjectInherit'"
		setAccessControl = "[System.IO.Directory]::SetAccessControl($p, $acl)"
	}
	return strings.Join([]string{
		"$p = " + quotePowerShellLiteral(err.Path),
		"$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User",
		"$acl = New-Object System.Security.AccessControl." + securityType,
		"$acl.SetOwner($sid)",
		"$acl.SetAccessRuleProtection($true, $false)",
		"$rule = New-Object -TypeName System.Security.AccessControl.FileSystemAccessRule -ArgumentList @($sid, 'FullControl', " + inheritance + ", 'None', 'Allow')",
		"$acl.SetAccessRule($rule)",
		setAccessControl,
	}, "; ")
}

func quotePowerShellLiteral(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

func quotePOSIXShellToken(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'"'"'`) + "'"
}

func reviewModeUnsafePathRefusal(err error) error {
	var unsafePath *reviewtransaction.UnsafeRARPathError
	if errors.As(err, &unsafePath) {
		return &reviewModeUnsafePathError{Path: unsafePath.Path, Directory: unsafePath.Directory, Cause: err}
	}
	return nil
}

func reviewModeCommandsByVerb(commands []string, verb string) []string {
	selected := make([]string, 0, len(commands))
	for _, command := range commands {
		if strings.HasPrefix(command, "`gentle-ai review mode "+verb+" ") {
			selected = append(selected, command)
		}
	}
	return selected
}

// reviewModeUnreadable turns a kill-switch resolution failure into a refusal
// that names the file holding the value and the command that overwrites it.
//
// Both sources are probed rather than only the one that failed, because
// ResolveRDDMode stops at the first source it cannot read. Naming that one
// alone would send the operator round the loop: they overwrite what the message
// named, rerun, and hit the same wall from the other source.
func reviewModeUnreadable(
	ctx context.Context,
	repo string,
	global reviewtransaction.RDDGlobalMode,
	err error,
) error {
	if err == nil {
		return nil
	}
	if unsafePath := reviewModeUnsafePathRefusal(err); unsafePath != nil {
		return unsafePath
	}
	scopes := make([]ReviewModeUnreadableScope, 0, 2)
	if reviewtransaction.RDDModeValueUnintelligible(global.Value) {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			scopes = append(scopes, ReviewModeUnreadableScope{Scope: reviewModeScopeGlobal, Path: state.Path(home), Repo: repo})
		}
	}
	// An unset global can never fail, so this isolates the clone-local source
	// even when the global one already failed ahead of it.
	if _, cloneErr := reviewtransaction.ResolveRDDMode(ctx, repo, reviewtransaction.RDDGlobalMode{}); cloneErr != nil {
		if path, pathErr := reviewtransaction.CloneLocalRDDModeRecordPath(ctx, repo); pathErr == nil && path != "" {
			scopes = append(scopes, ReviewModeUnreadableScope{Scope: reviewModeScopeClone, Path: path, Repo: repo})
		}
	}
	if len(scopes) == 0 {
		return err
	}
	return &ReviewModeUnreadableError{Scopes: scopes, Cause: err}
}

// reviewDeliveryDisposition reports what governs delivery for the candidate
// currently under a lifecycle gate. The kill switch alone never decides: an
// existing receipt keeps governing because disabling freezes authority
// read-only rather than unmaking an approval that is content-bound to exactly
// these bytes.
//
// An unreadable switch is not a disabled switch. When the mode cannot be
// resolved this fails closed to the managed disposition, so a broken or
// tampered mode file can never relax a gate into reporting work as unmanaged by
// choice.
func reviewDeliveryDisposition(ctx context.Context, repo string, receiptPresent bool) (reviewtransaction.RDDDelivery, error) {
	status, err := reviewModeStatus(ctx, repo)
	if refusal := reviewModeUnsafePathRefusal(err); refusal != nil {
		return reviewtransaction.RDDDeliveryUnmanaged, refusal
	}
	if err != nil {
		if receiptPresent {
			return reviewtransaction.RDDDeliveryReceiptGoverned, nil
		}
		return reviewtransaction.RDDDeliveryUnmanaged, nil
	}
	return reviewtransaction.RDDDeliveryDisposition(status, receiptPresent), nil
}

// reviewDrivenDevelopmentDisabled reports whether the user's kill switch is off
// for this clone. It is the reach the switch needs outside the review gate:
// every enforcement point that can refuse work on review grounds must be able
// to ask this cheaply, because a switched-off system has no implications.
//
// An unreadable switch is not a disabled switch. It fails closed to "enabled"
// for the same reason reviewDeliveryDisposition does: a broken or tampered mode
// record must never be able to relax an enforcement point.
func reviewDrivenDevelopmentDisabled(ctx context.Context, repo string) (bool, error) {
	status, err := reviewModeStatus(ctx, repo)
	if err != nil {
		return false, reviewModeUnsafePathRefusal(err)
	}
	return !status.Enabled(), nil
}

func applyReviewMode(
	ctx context.Context,
	repo string,
	operation string,
	scope string,
	expectedRevision string,
	revisionProvided bool,
) (reviewtransaction.RDDModeStatus, error) {
	disabled := reviewtransaction.RDDModeStatus{Schema: reviewtransaction.RDDModeStatusSchema, Effective: reviewtransaction.RDDModeOff}
	if scope == reviewModeScopeGlobal {
		if err := writeGlobalRDDMode(operation); err != nil {
			return disabled, err
		}
		return reviewModeStatus(ctx, repo)
	}
	global, err := readGlobalRDDMode()
	if err != nil {
		return disabled, err
	}
	mode := reviewtransaction.RDDModeOff
	if operation == "enable" {
		// The override is off-only, so enabling clears this clone's opinion
		// instead of asserting on: a repository may never force review on.
		mode = reviewtransaction.RDDModeUnset
	}
	if !revisionProvided {
		// The compare-and-set token belongs to the clone-local source alone, so
		// it is read with an unset global. Repairing this clone must not depend
		// on the other source being readable, or a switch whose two records are
		// both unreadable would have no repair command at all.
		current, resolveErr := reviewtransaction.ResolveRDDMode(ctx, repo, reviewtransaction.RDDGlobalMode{})
		switch {
		case resolveErr == nil:
			expectedRevision = current.Revision
		case errors.Is(resolveErr, reviewtransaction.ErrRDDModeCorrupt):
			// An unreadable head carries no revision to compare against, and
			// replacing it is the whole point of this command.
			expectedRevision = ""
		default:
			return disabled, reviewModeUnreadable(ctx, repo, global, resolveErr)
		}
	}
	status, err := reviewtransaction.SetCloneLocalRDDMode(ctx, repo, mode, expectedRevision, global)
	return status, reviewModeUnreadable(ctx, repo, global, err)
}

func readGlobalRDDMode() (reviewtransaction.RDDGlobalMode, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return reviewtransaction.RDDGlobalMode{}, fmt.Errorf("resolve user home directory: %w", err)
	}
	persisted, err := state.Read(home)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return reviewtransaction.RDDGlobalMode{}, nil
		}
		return reviewtransaction.RDDGlobalMode{}, fmt.Errorf("read global review mode: %w", err)
	}
	global := reviewtransaction.RDDGlobalMode{Value: persisted.RDDMode}
	if persisted.RDDModeRecordedAt != nil {
		global.RecordedAt = persisted.RDDModeRecordedAt.UTC()
	}
	return global, nil
}

func writeGlobalRDDMode(operation string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	persisted, err := state.Read(home)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read global review mode: %w", err)
	}
	mode := reviewtransaction.RDDModeOff
	if operation == "enable" {
		mode = reviewtransaction.RDDModeOn
	}
	recorded := time.Now().UTC()
	persisted.RDDMode = string(mode)
	persisted.RDDModeRecordedAt = &recorded
	if err := state.Write(home, persisted); err != nil {
		return fmt.Errorf("persist global review mode: %w", err)
	}
	return nil
}

func emitReviewMode(stdout io.Writer, result ReviewModeResult, emitJSON bool) error {
	if emitJSON {
		payload, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s\n", payload)
		return err
	}
	_, err := fmt.Fprintf(
		stdout,
		"receipt-driven development: %s (decided by %s)\n  global:      %s\n  clone-local: %s\n",
		reviewModeLabel(result.Status.Effective),
		result.Status.Source,
		reviewModeLabel(result.Status.Global),
		reviewModeLabel(result.Status.CloneLocal),
	)
	return err
}

func reviewModeLabel(mode reviewtransaction.RDDMode) string {
	if mode == reviewtransaction.RDDModeUnset {
		return "unset"
	}
	return string(mode)
}

// errReviewDeclinedForCandidate signals internally that the user declined the
// review for this candidate only. Nothing is persisted, so the next candidate
// asks again. It never surfaces as a command error: START reports the decline
// as a typed consent outcome, because a deliberate user choice is not a
// failure — the same principle that keeps disabled/unmanaged delivery a
// reported disposition rather than a veto.
var errReviewDeclinedForCandidate = errors.New("review declined for this candidate")

// reviewStartConsentMode is the caller's --consent declaration on a negotiated
// START. Empty means undeclared: today's behavior, byte for byte.
type reviewStartConsentMode string

const (
	reviewConsentModeNone reviewStartConsentMode = ""
	// reviewConsentModeRelay declares that the caller can relay a blocking
	// question to a human. Only under this declaration does START answer with
	// the typed consent question instead of the console prompt or the silent
	// skip-and-notice.
	reviewConsentModeRelay reviewStartConsentMode = "relay"
	// reviewConsentModeGranted and reviewConsentModeDeclined carry the relayed
	// human answer back for the exact frozen candidate. They are the two
	// runnable follow-up invocations the consent question names.
	reviewConsentModeGranted  reviewStartConsentMode = "granted"
	reviewConsentModeDeclined reviewStartConsentMode = "declined"
)

// errReviewConsentQuestionRequired signals internally that a relay-declared
// START stopped at the consent moment: the typed question is the response, and
// nothing has been persisted.
var errReviewConsentQuestionRequired = errors.New("the review consent question awaits a relayed answer; rerun gentle-ai review start with --consent granted or --consent declined for the exact frozen candidate")

// errReviewConsentDeclineWithoutQuestion refuses a decline for a candidate
// that asks no question: tier 0 is silent structural readback, so there is no
// consent moment to answer.
var errReviewConsentDeclineWithoutQuestion = errors.New("this low-risk candidate asks no consent question, so there is nothing to decline; rerun gentle-ai review start without --consent")

const (
	reviewConsentAnswerRun    = "1"
	reviewConsentAnswerNotNow = "2"
)

type reviewConsentLocale string

const (
	reviewConsentLocaleEnglish reviewConsentLocale = "en"
	reviewConsentLocaleSpanish reviewConsentLocale = "es"
)

// normalizeReviewConsentLocale keeps the absent locale on the published English
// projection while accepting the two native consent-envelope localizations.
func normalizeReviewConsentLocale(value string) (reviewConsentLocale, error) {
	switch strings.TrimSpace(value) {
	case "", string(reviewConsentLocaleEnglish):
		return reviewConsentLocaleEnglish, nil
	case string(reviewConsentLocaleSpanish):
		return reviewConsentLocaleSpanish, nil
	default:
		// refusal:by-design operator-knowledge: only the caller knows which supported locale matches the active conversation
		return "", errors.New("review consent locale must be en or es")
	}
}

const (
	reviewConsentHeadline = "Gentle AI can review this change before you call it done."
	reviewConsentValue    = "Reviewing takes a bit longer, and it makes the result substantially safer."

	// reviewConsentAnswerRunLabel and reviewConsentAnswerNotNowLabel are the
	// single wording source for the two offered answers: the interactive
	// prompt and the relayed consent envelope both speak them, so the two
	// surfaces cannot drift.
	reviewConsentAnswerRunLabel    = "Run the review now"
	reviewConsentAnswerNotNowLabel = "Not now, just this once"
	reviewConsentAnswers           = "  1) " + reviewConsentAnswerRunLabel + "\n  2) " + reviewConsentAnswerNotNowLabel + "\n"

	// reviewConsentOffPath keeps the permanent disable reachable but deliberate.
	// It is a trailing line rather than a numbered answer because turning a
	// safety net off for good must cost more than pressing a number in a hurry.
	// The relayed consent envelope carries the same note as a documented off
	// path outside the choice set, for exactly the same reason.
	reviewConsentOffPathCommand = "gentle-ai review mode disable"
	reviewConsentOffPathNote    = "To turn reviews off for good, run '" + reviewConsentOffPathCommand + "'."
	reviewConsentOffPath        = reviewConsentOffPathNote + "\n"
	reviewConsentQuestion       = "Choose 1 or 2 [1]: "

	// reviewConsentSkippedNotice keeps the fail-safe default discoverable: an
	// unanswerable question must never look like a silent yes.
	reviewConsentSkippedNotice = "Gentle AI reviewed this change without asking, because this session has no terminal to answer on. " +
		"Run 'gentle-ai review mode disable' to turn reviews off, or 'gentle-ai review mode status' to see the current setting."

	// reviewConsentSkippedDefaultProvenance rides with the skip notice only
	// when the resolved mode source is `default`: reviews are on because
	// nobody chose anything, and the operator deserves to know the switch was
	// never explicitly set, with both commands that make it a real choice.
	reviewConsentSkippedDefaultProvenance = "Reviews are on by default; this was never explicitly chosen. " +
		"Run 'gentle-ai review mode enable' to make reviews an explicit choice, or 'gentle-ai review mode disable' to turn them off."
	reviewConsentUnreadableNotice = "Gentle AI could not read an answer, so it reviewed this change and will ask again next time."
	reviewConsentUnknownNotice    = "Gentle AI did not recognize that answer, so it reviewed this change and will ask again next time."

	// reviewConsentDeclinedNotice confirms a decline in the user's own terms.
	// It goes to the console stream, never stdout: stdout stays pure JSON.
	reviewConsentDeclinedNotice = "Review skipped for this candidate at your request. It will be offered again on the next change."
)

// reviewConsentNoticeDirName and reviewConsentNoticeMarkerFile locate the
// once-per-clone marker for reviewConsentSkippedNotice, mirroring the
// existing <GitCommonDir>/gentle-ai/<subdir> convention this package already
// uses for clone-local, uncommitted state (see writeReviewDefectReport).
const (
	reviewConsentNoticeDirName    = "review-mode"
	reviewConsentNoticeMarkerFile = "non-interactive-notice-shown"
)

// reviewConsentNoticeAlreadyShown reports whether this clone already saw
// reviewConsentSkippedNotice. An error resolving the marker is treated as
// "not shown" so a broken marker can never silently suppress the notice.
func reviewConsentNoticeAlreadyShown(ctx context.Context, repo string) (bool, error) {
	path, err := reviewConsentNoticeMarkerPath(ctx, repo)
	if err != nil {
		return false, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, statErr
	}
	return true, nil
}

// recordReviewConsentNoticeShown persists that this clone saw the notice.
// It is best-effort: a write failure here must never fail review start, the
// same fail-open posture the rest of this consent flow already uses for the
// consent-asked latch and the other console notices.
func recordReviewConsentNoticeShown(ctx context.Context, repo string) error {
	path, err := reviewConsentNoticeMarkerPath(ctx, repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339Nano)+"\n"), 0o644)
}

func reviewConsentNoticeMarkerPath(ctx context.Context, repo string) (string, error) {
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(lease.Identity().GitCommonDir, "gentle-ai", reviewConsentNoticeDirName, reviewConsentNoticeMarkerFile), nil
}

// reviewConsentSession is the console the one-time question uses.
type reviewConsentSession struct {
	Interactive bool
	Input       io.Reader
	Output      io.Writer
}

// reviewConsole is a variable so the question can be driven in tests without a
// real terminal; production always resolves the process console.
var reviewConsole = defaultReviewConsole

// defaultReviewConsole never writes the question to the command's stdout: that
// stream carries the machine-readable review result, and a prompt mixed into it
// would corrupt every caller that parses it.
func defaultReviewConsole() reviewConsentSession {
	return reviewConsentSession{Interactive: reviewConsoleInteractive(), Input: os.Stdin, Output: os.Stderr}
}

// reviewConsoleInteractive requires a real terminal on both ends and no CI
// marker. A question nobody can answer must never become a blocking read.
func reviewConsoleInteractive() bool {
	if strings.TrimSpace(os.Getenv("CI")) != "" {
		return false
	}
	return reviewConsoleTerminal(os.Stdin) && reviewConsoleTerminal(os.Stderr)
}

func reviewConsoleTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// authorizeReviewStart is the single call site where a new review start meets
// the user. The kill switch decides whether a start may run at all, and the
// one-time question is asked here — after the candidate is frozen and the tier
// is classified — so it can state the real reason instead of a generic warning.
//
// A consent declaration selects candidate-scoped negotiated semantics: relay
// always returns the typed question, while granted and declined apply only to
// the exact frozen candidate and never touch the legacy clone-wide latch. An
// undeclared START keeps the one-time console behavior unchanged.
func authorizeReviewStart(ctx context.Context, repo string, assessment reviewtransaction.RiskAssessment, consent reviewStartConsentMode) error {
	global, err := readGlobalRDDMode()
	if err != nil {
		return err
	}
	status, err := reviewtransaction.AuthorizeRDDOperation(
		ctx, repo, global, reviewtransaction.RDDOperationStart)
	if err != nil {
		return reviewModeUnreadable(ctx, repo, global, err)
	}
	if err := authorizeManagedReviewerAssets(); err != nil {
		return reviewPreflightRefusal(reviewPreflightManagedAssetsReason, err)
	}
	if assessment.Level == reviewtransaction.RiskLow {
		// Tier 0 is silent structural readback. Asking here would reintroduce
		// exactly the ceremony the silent readback exists to remove, and a
		// relayed decline has no question to answer.
		if consent == reviewConsentModeDeclined {
			return errReviewConsentDeclineWithoutQuestion
		}
		return nil
	}
	switch consent {
	case reviewConsentModeRelay:
		// The caller declared it can relay a blocking question, so the typed
		// envelope is the question. Candidate-scoped consent must not be
		// suppressed by the legacy clone-wide console latch.
		return errReviewConsentQuestionRequired
	case reviewConsentModeDeclined:
		// An explicit relayed "no" is honored exactly like the interactive
		// answer 2: this candidate only, nothing persisted, never latched.
		return fmt.Errorf("%w: the next candidate is asked again", errReviewDeclinedForCandidate)
	case reviewConsentModeGranted:
		// The exact target binding was revalidated before this call. Authorize
		// only that candidate; later candidates must receive their own question.
		return nil
	}
	console := reviewConsole()
	asked, err := reviewtransaction.RDDConsentAsked(ctx, repo)
	if err != nil {
		// A damaged latch must neither block the review nor silently disable it:
		// review the candidate, and say why the question was skipped.
		_, _ = fmt.Fprintf(console.Output, "Gentle AI reviewed this change without asking: %v.\n", err)
		return nil
	}
	if asked {
		return nil
	}
	if !console.Interactive {
		// The notice carries the same information every time (issue #1848), so
		// a clone that already saw it once does not need it again. This is
		// deliberately a separate marker from the RDDConsentAsked latch above:
		// that latch means the one-time consent question is resolved, but a
		// non-interactive run never actually asked anyone, so a later
		// interactive session in this same clone must still be free to ask.
		// An unreadable marker fails open to showing the notice — the first
		// occurrence must never be silently suppressed.
		if shown, shownErr := reviewConsentNoticeAlreadyShown(ctx, repo); shownErr != nil || !shown {
			_, _ = fmt.Fprintln(console.Output, reviewConsentSkippedNotice)
			if status.Source == reviewtransaction.RDDModeSourceDefault {
				// The status already knows this provenance: reviews are on
				// because no source expressed an opinion, and the operator
				// working headless deserves to learn the switch was never
				// explicitly chosen.
				_, _ = fmt.Fprintln(console.Output, reviewConsentSkippedDefaultProvenance)
			}
			_ = recordReviewConsentNoticeShown(ctx, repo)
		}
		return nil
	}
	_, _ = fmt.Fprint(console.Output, reviewConsentPrompt(assessment))
	answer, err := readReviewConsentAnswer(console.Input)
	if err != nil {
		// Leaving the latch unset is deliberate: the human never got to answer,
		// so the one-time question must survive for a session that can ask it.
		_, _ = fmt.Fprintln(console.Output, reviewConsentUnreadableNotice)
		return nil
	}
	// The two answers persist asymmetrically. Accepting the review is the safe
	// direction, so it is latched and never asked again. Declining is not: each
	// work unit carries different risk, and today's README says nothing about
	// tomorrow's migration, so the next candidate gets its own question.
	switch answer {
	case reviewConsentAnswerRun:
		return recordReviewConsentAsked(ctx, repo)
	case reviewConsentAnswerNotNow:
		_, _ = fmt.Fprintln(console.Output, reviewConsentDeclinedNotice)
		return fmt.Errorf("%w: the next candidate is asked again", errReviewDeclinedForCandidate)
	default:
		_, _ = fmt.Fprintln(console.Output, reviewConsentUnknownNotice)
		return nil
	}
}

func recordReviewConsentAsked(ctx context.Context, repo string) error {
	if err := reviewtransaction.RecordRDDConsentAsked(ctx, repo); err != nil {
		return fmt.Errorf("record the review question as asked: %w", err)
	}
	return nil
}

// readReviewConsentAnswer reads exactly one line. An empty line accepts the
// offered default, which is the safe direction: review the change.
func readReviewConsentAnswer(input io.Reader) (string, error) {
	if input == nil {
		return "", errors.New("review consent console has no input")
	}
	line, err := bufio.NewReader(input).ReadString('\n')
	answer := strings.TrimSpace(line)
	if err != nil && answer == "" {
		return "", err
	}
	if answer == "" {
		return reviewConsentAnswerRun, nil
	}
	return answer, nil
}

func reviewConsentPrompt(assessment reviewtransaction.RiskAssessment) string {
	return fmt.Sprintf("%s\nWhy: %s\n%s\n%s%s%s",
		reviewConsentHeadline, reviewConsentReason(assessment), reviewConsentValue,
		reviewConsentAnswers, reviewConsentOffPath, reviewConsentQuestion)
}

// reviewConsentMediumReason is the single wording source for the tier-1
// reason: the interactive consent prompt speaks it and the machine-readable
// START result relays it verbatim, so the two surfaces cannot drift.
const reviewConsentMediumReason = "this change is not purely passive documentation, so it gets one consolidated review."

// reviewConsentReason states why this candidate is reviewed, in the user's own
// terms. Both tiers name the evidence that triggered them through the same
// phrasing helper, so neither review's cost is ever unexplained: tier 1 keeps
// the consolidated-review sentence and appends what made the candidate
// non-passive (issue #1827); tier 2 names what triggered the deeper review.
func reviewConsentReason(assessment reviewtransaction.RiskAssessment) string {
	evidence := reviewConsentEvidence(assessment.Reasons)
	if assessment.Level != reviewtransaction.RiskHigh {
		if evidence == "" {
			return reviewConsentMediumReason
		}
		return reviewConsentMediumReason + " The review starts from " + evidence + "."
	}
	if evidence == "" {
		return "this change touches something sensitive, so it gets a deeper review."
	}
	return "this change gets a deeper review because it touches " + evidence + "."
}

func reviewConsentEvidence(reasons []reviewtransaction.RiskReason) string {
	phrases := reviewConsentEvidencePhrases(reasons)
	switch len(phrases) {
	case 0:
		return ""
	case 1:
		return phrases[0]
	case 2:
		return phrases[0] + " and " + phrases[1]
	default:
		// Naming every path would bury the decision the user has to make.
		return fmt.Sprintf("%s, %s, and %d more", phrases[0], phrases[1], len(phrases)-2)
	}
}

// reviewConsentRiskEvidence projects an already-classified assessment into
// the risk_evidence phrases a non-interactive START result carries. It is an
// output-only projection of facts the start already computed: tier 2 names
// the triggering evidence, tier 1 leads with the exact consolidated-review
// reason the consent prompt speaks and appends the same evidence phrases so
// the path that made the candidate non-passive is named (issue #1827), and
// tier 0 stays nil so the omitempty field is absent rather than empty-string
// noise.
func reviewConsentRiskEvidence(assessment reviewtransaction.RiskAssessment) []string {
	switch assessment.Level {
	case reviewtransaction.RiskHigh:
		return reviewConsentEvidencePhrases(assessment.Reasons)
	case reviewtransaction.RiskMedium:
		return append([]string{reviewConsentMediumReason}, reviewConsentEvidencePhrases(assessment.Reasons)...)
	default:
		return nil
	}
}

// reviewConsentEvidencePhrases is the single phrasing source for risk
// evidence: the interactive consent prompt and the machine-readable START
// result both project reasons through it, so the two surfaces cannot drift.
// It returns nil when no reason carries speakable evidence, which keeps the
// omitempty START field absent instead of empty-string noise.
func reviewConsentEvidencePhrases(reasons []reviewtransaction.RiskReason) []string {
	var phrases []string
	for _, reason := range reasons {
		if phrase := reviewConsentEvidencePhrase(reason); phrase != "" {
			phrases = append(phrases, phrase)
		}
	}
	return phrases
}

func reviewConsentEvidencePhrase(reason reviewtransaction.RiskReason) string {
	// An empty file is named first and described second. Every other subject
	// reads "<what changed> in <path>", which for a file with no bytes would
	// assert something about content that is not there.
	if reason.Code == reviewtransaction.RiskReasonEmptyContent {
		if strings.TrimSpace(reason.Path) == "" {
			return ""
		}
		return fmt.Sprintf("%s, an empty file whose type cannot be determined from its content", reason.Path)
	}
	subject := reviewConsentEvidenceSubject(reason)
	if subject == "" || strings.TrimSpace(reason.Path) == "" {
		return subject
	}
	return fmt.Sprintf("%s in %s", subject, reason.Path)
}

func reviewConsentEvidenceSubject(reason reviewtransaction.RiskReason) string {
	switch reason.Code {
	case reviewtransaction.RiskReasonServiceToken:
		return "service credentials"
	case reviewtransaction.RiskReasonShellSource:
		return "shell scripting"
	case reviewtransaction.RiskReasonProcessBoundary, reviewtransaction.RiskReasonProcessScanLimit:
		return "code that starts other processes"
	case reviewtransaction.RiskReasonExecutableMode:
		return "an executable permission change"
	case reviewtransaction.RiskReasonExecutableChange:
		return "an executable change"
	case reviewtransaction.RiskReasonConfigurationChange:
		return "a configuration change"
	case reviewtransaction.RiskReasonHotPath:
		return reviewConsentSignalSubject(reason.Signal)
	default:
		return ""
	}
}

func reviewConsentSignalSubject(signal reviewtransaction.RiskSignal) string {
	switch signal {
	case reviewtransaction.SignalAuth:
		return "authentication"
	case reviewtransaction.SignalSecurity:
		return "security"
	case reviewtransaction.SignalPayments:
		return "payments"
	case reviewtransaction.SignalDataExposure:
		return "data exposure"
	case reviewtransaction.SignalDataLoss:
		return "data loss"
	case reviewtransaction.SignalPermissions:
		return "permissions"
	case reviewtransaction.SignalUpdate:
		return "the update path"
	case reviewtransaction.SignalShellProcess:
		return "shell or process execution"
	default:
		return "a sensitive area"
	}
}
