package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// reviewStopHookSchema identifies the reminder envelope this hook prints to
// stdout. Claude Code reads "decision" and "reason" as top-level keys.
const reviewStopHookSchema = "gentle-ai.review-stop-hook/v1"

// reviewStopHookReminderSchema identifies the small per-session record kept
// under ~/.gentle-ai/review-stop-hook/v1. The schema name is unchanged from
// its first shape; it now optionally carries baseline_target_identity
// alongside target_identity (see reviewStopHookReminderRecord).
const reviewStopHookReminderSchema = "gentle-ai.review-stop-hook-reminder/v1"

// maxReviewStopHookPayloadBytes bounds the hook stdin payload, mirroring the
// sdd-task-result precedent (internal/cli/sdd_task_result.go): a hook reports
// a small event, never a large or unbounded stream.
const maxReviewStopHookPayloadBytes = 1 << 20

// reviewStopHookSessionIDPattern is the safe filename alphabet a session id
// must match before it is used to name a state file.
var reviewStopHookSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// reviewStopHookPayload is Claude Code's hook stdin payload, shared by the
// SessionStart and Stop shapes this command handles. Unknown fields
// (including SessionStart's "source") are ignored by json.Unmarshal, never
// rejected.
type reviewStopHookPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// reviewStopHookOutput is the sole reminder envelope printed to stdout.
type reviewStopHookOutput struct {
	Schema   string `json:"schema"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// reviewStopHookReminderRecord is the per-session state: BaselineTargetIdentity
// is the unreviewed-candidate identity (if any) that SessionStart observed
// when the session began, so a Stop later in the same session never reminds
// about a candidate the session did not produce. TargetIdentity is the
// identity of the last candidate a Stop actually reminded about, so a repeat
// Stop for the same candidate stays silent. The two fields are independent
// and each written without disturbing the other.
type reviewStopHookReminderRecord struct {
	Schema                 string `json:"schema"`
	TargetIdentity         string `json:"target_identity,omitempty"`
	BaselineTargetIdentity string `json:"baseline_target_identity,omitempty"`
	// LastSeenTargetIdentity is the last candidate observed during a Stop that
	// stayed quiet under a fresh consent-stale marker (#4494). The quiet window
	// ends as soon as the candidate is stable across two Stop events -- the
	// concurrent writer settled -- so the ordinary renegotiation reminder can
	// resume; while the identity keeps changing, reminding would only push the
	// caller back into the same unexecutable negotiation.
	LastSeenTargetIdentity string `json:"last_seen_target_identity,omitempty"`
}

// RunReviewStopHook is the `gentle-ai review stop-hook` entry point. Claude
// Code runs it as both a SessionStart and a Stop hook. On SessionStart it
// records the repository's current unreviewed-candidate identity (if any) as
// this session's baseline, with zero output. On Stop it prints one reminder,
// only when receipt-driven development is enabled and the repository holds a
// candidate the session did not start with, that routes the agent through the
// selectorless STATUS preflight before it reports completion. Every other
// case -- including an unrecognized hook_event_name -- is silent (exit 0, no
// output), and this command never starts a review itself.
func RunReviewStopHook(args []string, stdout io.Writer) error {
	return runReviewStopHook(args, os.Stdin, stdout, os.Stderr)
}

// runReviewStopHook is RunReviewStopHook's testable form: stdin and stderr are
// both explicit so a test can supply a payload and assert on diagnostics
// without touching the process streams. stderr is also where flag usage/help
// text is written (never stdout), so stdout stays reserved for the exact one
// JSON reminder object a Stop event may print.
func runReviewStopHook(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := newReviewFlagSet("review stop-hook", stderr,
		"Read one Claude Code hook payload from stdin (SessionStart or Stop). On SessionStart, record the repository's current unreviewed-candidate identity as this session's baseline, with no output. On Stop, when receipt-driven development is enabled and the repository holds a candidate the session did not start with, print one reminder that routes the agent through the selectorless STATUS preflight before it reports completion. Every other case -- including an unrecognized hook_event_name -- is silent, and this command never runs review start itself.")
	agent := flags.String("agent", "", "required; generated active runtime identity (only claude-code is accepted today)")
	cwdOverride := flags.String("cwd", "", "optional; overrides the hook payload's cwd")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		// refusal:by-design operator-knowledge: only the caller knows which positional token it meant to pass; no runnable command can guess it.
		return reviewPreflightError(fmt.Errorf("unexpected review stop-hook argument %q", flags.Arg(0)))
	}
	runtimeAgent := strings.TrimSpace(*agent)
	if runtimeAgent != "claude-code" {
		// refusal:by-design operator-knowledge: only the caller can declare which runtime is executing, and only claude-code is wired to this hook today.
		return reviewPreflightError(fmt.Errorf("review stop-hook requires --agent to be claude-code; got %q", runtimeAgent))
	}

	raw, err := io.ReadAll(io.LimitReader(stdin, maxReviewStopHookPayloadBytes))
	if err != nil {
		return fmt.Errorf("review stop-hook: read hook payload: %w", err)
	}
	var payload reviewStopHookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("review stop-hook: decode hook payload: %w", err)
	}

	repo := strings.TrimSpace(*cwdOverride)
	if repo == "" {
		repo = strings.TrimSpace(payload.Cwd)
	}

	ctx := context.Background()

	switch payload.HookEventName {
	case "SessionStart":
		return runReviewStopHookSessionStart(ctx, payload.SessionID, repo, runtimeAgent)
	case "Stop":
		return runReviewStopHookStop(ctx, payload, repo, runtimeAgent, stdout)
	default:
		return nil
	}
}

// runReviewStopHookSessionStart records the repository's current
// unreviewed-candidate identity (if any) as sessionID's baseline. It is
// always silent: SessionStart never prints a reminder, and every
// precondition that keeps Stop silent (no usable cwd, RDD off, non-repository
// cwd) also means there is nothing to baseline here.
func runReviewStopHookSessionStart(ctx context.Context, sessionID, repo, runtimeAgent string) error {
	_, targetIdentity, _, ok, err := reviewStopHookResolveTargetIdentity(ctx, repo, runtimeAgent)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := recordReviewStopHookBaseline(sessionID, targetIdentity); err != nil {
		return fmt.Errorf("review stop-hook: record baseline: %w", err)
	}
	return nil
}

// runReviewStopHookStop is the original Stop behavior, now also silent when
// the current candidate matches the session's recorded SessionStart baseline
// -- that candidate predates the session, so this session did not produce it.
func runReviewStopHookStop(ctx context.Context, payload reviewStopHookPayload, repo, runtimeAgent string, stdout io.Writer) error {
	if payload.StopHookActive {
		return nil
	}

	root, targetIdentity, status, ok, err := reviewStopHookResolveTargetIdentity(ctx, repo, runtimeAgent)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		return nil
	}
	// Only a review_due candidate earns a reminder
	// (verify-always-rdd-high S2): high risk, by the same assessment
	// `review assess` reports. The gate mirrors the selectors the offered
	// START carries, so a committed range is assessed over the same range
	// STATUS offered (#4815). The gate reports due for a passive mirrored
	// range on purpose: a passive delta normally never reaches this gate
	// (STATUS suppresses it with an authority-free STOP), so a START over a
	// passive range means the suppression was withheld -- a live review is
	// in flight and the reminder must point the agent back at it.
	if !reviewStopHookReviewDue(ctx, root, runtimeAgent, &status) {
		return nil
	}

	// Check first, persist after delivery below: recording before the write
	// risks losing the reminder if it fails or the process dies in between.
	if reviewStopHookSessionIDPattern.MatchString(payload.SessionID) {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return fmt.Errorf("resolve user home directory: %w", herr)
		}
		record, hadRecord, rerr := readReviewStopHookRecord(reviewStopHookRecordPath(home, payload.SessionID))
		if rerr != nil {
			return rerr
		}
		if hadRecord && ((record.BaselineTargetIdentity != "" && record.BaselineTargetIdentity == targetIdentity) ||
			(record.TargetIdentity != "" && record.TargetIdentity == targetIdentity)) {
			return nil
		}
		if marker, ok, merr := readReviewConsentStaleMarker(root); merr == nil && ok &&
			time.Since(marker.LastRefusedAt) < reviewConsentStaleMarkerWindow {
			// Issue #4494: a consent answer was just spent on a candidate that
			// moved between negotiation and answer. While the candidate keeps
			// changing between Stop events, every reminder would push the caller
			// back into the same unexecutable negotiation, so the hook stays
			// quiet and remembers the last candidate it saw. Once the candidate
			// is stable across two Stops (the writer settled), the ordinary
			// reminder resumes; the window bounds the silence either way.
			if record.LastSeenTargetIdentity != targetIdentity {
				if serr := recordReviewStopHookLastSeen(payload.SessionID, targetIdentity); serr != nil {
					return serr
				}
				return nil
			}
		}
	}

	output := reviewStopHookOutput{
		Schema:   reviewStopHookSchema,
		Decision: "block",
		Reason:   reviewStopHookReasonText(targetIdentity, root, runtimeAgent, status.NextTransition.Execute.Command),
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("review stop-hook: encode reminder: %w", err)
	}
	if _, err := fmt.Fprintln(stdout, string(encoded)); err != nil {
		return err
	}
	_, err = recordReviewStopHookReminder(payload.SessionID, targetIdentity)
	return err
}

// reviewStopHookResolveTargetIdentity resolves the repository root, the
// effective receipt-driven-development mode, and the current STATUS target
// identity together -- the read-only preflight both SessionStart and Stop
// need. ok is false whenever there is nothing to check: no usable cwd, a cwd
// outside a Git repository (never bootstrapped: ResolveRepositoryRoot alone,
// never PrepareReviewRepositoryRoot, so `git init` never runs), RDD off, or a
// Git common directory whose filesystem cannot hold review authority (#4670).
// err is non-nil only for a genuine unexpected internal failure.
//
// The Git-repository check runs before resolving RDD mode: the clone-local
// override lookup shells out to git, and on a genuinely non-Git cwd it
// returns a hard error rather than "no override" -- so resolving the root
// first is what keeps a non-repository cwd silent instead of surfacing as an
// internal error.
func reviewStopHookResolveTargetIdentity(ctx context.Context, repo, runtimeAgent string) (root, targetIdentity string, status ReviewTargetStatusResult, ok bool, err error) {
	if repo == "" {
		return "", "", ReviewTargetStatusResult{}, false, nil
	}
	root, rootErr := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if rootErr != nil {
		return "", "", ReviewTargetStatusResult{}, false, nil
	}

	global, globalErr := readGlobalRDDMode()
	if globalErr != nil {
		return "", "", ReviewTargetStatusResult{}, false, fmt.Errorf("review stop-hook: read global review mode: %w", globalErr)
	}
	rddStatus, rddErr := reviewtransaction.ResolveRDDMode(ctx, root, global)
	if rddErr != nil {
		if reviewStopHookAuthorityUnsupported(rddErr) {
			return "", "", ReviewTargetStatusResult{}, false, nil
		}
		return "", "", ReviewTargetStatusResult{}, false, fmt.Errorf("review stop-hook: resolve review mode: %w", rddErr)
	}
	if !rddStatus.Enabled() {
		return "", "", ReviewTargetStatusResult{}, false, nil
	}

	// Calling runReviewStatus directly (rather than RunReview) avoids a
	// package-level initialization cycle: RunReview's dispatch is reached
	// through reviewFacadeCommandRunner, a package var whose initializer is
	// runReviewCommandContext -- the same function that routes to this verb.
	var statusOutput bytes.Buffer
	statusErr := runReviewStatus(ctx, []string{
		"--cwd", root, "--contract", ReviewIntegrationContractV2,
		"--agent", runtimeAgent, "--next-transition",
	}, &statusOutput)
	if statusErr != nil {
		if reviewStopHookAuthorityUnsupported(statusErr) {
			return "", "", ReviewTargetStatusResult{}, false, nil
		}
		return "", "", ReviewTargetStatusResult{}, false, fmt.Errorf("review stop-hook: review status: %w", statusErr)
	}
	var result ReviewTargetStatusResult
	if err := json.Unmarshal(statusOutput.Bytes(), &result); err != nil {
		return "", "", ReviewTargetStatusResult{}, false, fmt.Errorf("review stop-hook: decode review status: %w", err)
	}
	return root, result.TargetIdentity, result, true, nil
}

// reviewStopHookAuthorityUnsupported reports the #4670 typed refusal for a
// Git common directory whose filesystem cannot keep review authority private.
// No review can start on such a volume, so the reminder would point at a
// START that refuses with the same message; the refusal is surfaced by the
// review commands themselves rather than by failing every turn here.
func reviewStopHookAuthorityUnsupported(err error) bool {
	var unsupported *reviewtransaction.UnsupportedAuthorityFilesystemError
	return errors.As(err, &unsupported)
}

// reviewStopHookReasonText composes the one-paragraph reminder Claude Code
// reads back. It names the unreviewed candidate, the entry-rule obligation,
// the canonical selectorless STATUS command, the exact START STATUS returned
// (carrying --consent=relay), the lossless-relay instruction for any consent
// envelope that START returns, and the once-per-candidate scope of this hook.
func reviewStopHookReasonText(targetIdentity, root, runtimeAgent, startCommand string) string {
	statusCommand := fmt.Sprintf("gentle-ai review status --cwd %s --contract %s --agent %s --next-transition", root, ReviewIntegrationContractV2, runtimeAgent)
	return strings.Join([]string{
		"Receipt-driven development is enabled for this repository, and it holds an unreviewed candidate (target_identity " + targetIdentity + ").",
		"By the review contract entry rule, you must run the selectorless STATUS preflight below and route only from its returned next_transition before reporting completion; never infer a command from prose or a stale reply.",
		"Run: " + statusCommand,
		"STATUS returned this exact provider-issued START (carrying --consent=relay); run it unchanged: " + startCommand,
		"If that START returns a consent envelope, relay it to the human losslessly and never answer it on the human's behalf.",
		"This hook never runs review start itself, and it reminds once per session and candidate.",
	}, "\n")
}

// reviewStopHookRecordPath is the per-session state file path for sessionID.
func reviewStopHookRecordPath(home, sessionID string) string {
	return filepath.Join(home, ".gentle-ai", "review-stop-hook", "v1", sessionID+".json")
}

// readReviewStopHookRecord reads sessionID's existing record, if any. A
// missing file reports a zero record with ok false and no error; a corrupt
// file is treated the same as missing, since replacing it is safe (this
// state is a dedupe hint, not authority).
func readReviewStopHookRecord(path string) (record reviewStopHookReminderRecord, ok bool, err error) {
	existing, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return reviewStopHookReminderRecord{}, false, nil
		}
		return reviewStopHookReminderRecord{}, false, readErr
	}
	if json.Unmarshal(existing, &record) != nil {
		return reviewStopHookReminderRecord{}, false, nil
	}
	return record, true, nil
}

// recordReviewStopHookBaseline writes targetIdentity as sessionID's
// SessionStart baseline, leaving any already-recorded TargetIdentity (the
// last candidate a Stop reminded about) untouched. An invalid or missing
// session id is a silent no-op: there is no safe file name to record it
// under.
func recordReviewStopHookBaseline(sessionID, targetIdentity string) error {
	if !reviewStopHookSessionIDPattern.MatchString(sessionID) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	path := reviewStopHookRecordPath(home, sessionID)

	record, _, err := readReviewStopHookRecord(path)
	if err != nil {
		return err
	}
	record.Schema = reviewStopHookReminderSchema
	record.BaselineTargetIdentity = targetIdentity

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = filemerge.WriteFileAtomic(path, payload, 0o644)
	return err
}

// recordReviewStopHookLastSeen persists the last candidate identity observed
// during a marker-quiet Stop, preserving the recorded baseline and reminder
// identity untouched. An invalid session id is a silent no-op.
func recordReviewStopHookLastSeen(sessionID, targetIdentity string) error {
	if !reviewStopHookSessionIDPattern.MatchString(sessionID) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	path := reviewStopHookRecordPath(home, sessionID)
	record, _, err := readReviewStopHookRecord(path)
	if err != nil {
		return err
	}
	if record.LastSeenTargetIdentity == targetIdentity {
		return nil
	}
	record.Schema = reviewStopHookReminderSchema
	record.LastSeenTargetIdentity = targetIdentity
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = filemerge.WriteFileAtomic(path, payload, 0o644)
	return err
}

// recordReviewStopHookReminder decides, and records, whether a Stop for
// targetIdentity should stay silent. It reports silent=true when: the session
// id is invalid (there is nothing safe to record, so the caller still
// reminds -- see below); the recorded SessionStart baseline equals
// targetIdentity (the candidate predates this session); or the session was
// already reminded about this exact targetIdentity. Otherwise it records
// targetIdentity as the session's last reminder (preserving any recorded
// baseline untouched) and reports silent=false so the caller reminds.
//
// An invalid or missing session id is the one case that reminds (silent
// false) without persisting: there is no safe file name to record it under.
func recordReviewStopHookReminder(sessionID, targetIdentity string) (silent bool, err error) {
	if !reviewStopHookSessionIDPattern.MatchString(sessionID) {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve user home directory: %w", err)
	}
	path := reviewStopHookRecordPath(home, sessionID)

	record, hadRecord, err := readReviewStopHookRecord(path)
	if err != nil {
		return false, err
	}
	if hadRecord {
		if record.BaselineTargetIdentity != "" && record.BaselineTargetIdentity == targetIdentity {
			return true, nil
		}
		if record.TargetIdentity != "" && record.TargetIdentity == targetIdentity {
			return true, nil
		}
	}

	record.Schema = reviewStopHookReminderSchema
	record.TargetIdentity = targetIdentity
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	_, err = filemerge.WriteFileAtomic(path, payload, 0o644)
	return false, err
}

// reviewStopHookReviewDue reports review_due for the current candidate by
// running the same in-process assessment `review assess` publishes, over the
// same selector set the offered START carries. A selectorless STATUS that
// derived a committed range offers START with --base-ref --committed-only
// (#4412); assessing that range is the only way the hook and `review assess`
// answer the same question. Mirroring the default selectorless assessment
// here classified an empty workspace candidate as a high-risk due review
// while assess over the offered range reported under_budget -- the exact
// disagreement issue #4815 documents. Only the committed-range selectors are
// mirrored: `review assess` has no --workspace-overlay form, so overlay and
// current-changes candidates keep the selectorless evaluation. An assessment
// that cannot answer still fails closed to due, so a reminder is never lost
// to an assessment error; the one exception is the empty committed range
// (the boundary already reaches HEAD), where there is nothing to review and
// assess refuses rather than answering not-due.
func reviewStopHookReviewDue(ctx context.Context, root, runtimeAgent string, status *ReviewTargetStatusResult) bool {
	args := []string{"--cwd", root, "--json"}
	if strings.TrimSpace(runtimeAgent) != "" {
		args = append(args, "--agent", runtimeAgent)
	}
	args = append(args, reviewStopHookAssessSelectors(status)...)
	var output bytes.Buffer
	if err := RunReviewAssess(args, &output); err != nil {
		if reviewStopHookEmptyCommittedRange(err) {
			return false
		}
		return true
	}
	var result ReviewAssessmentResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return true
	}
	if !result.ReviewDue && result.ReviewDueReason == "already_reviewed" {
		// already_reviewed means an authority covers the offered range. A
		// live (non-terminal) authority is an in-flight obligation the
		// reminder must point back at; only burned authority settles the
		// candidate, per #4815's "stays quiet when every commit is covered
		// by burned authority" expectation.
		return reviewStopHookLiveReviewInFlight(ctx, root)
	}
	if !result.ReviewDue && result.ReviewDueReason == "passive" {
		// A passive delta reaches this gate only because STATUS withheld
		// its authority-free suppression, which it does exactly when a
		// live review is underway; silence would hide that review.
		return true
	}
	return result.ReviewDue
}

// reviewStopHookLiveReviewInFlight reports whether any review authority in
// root is still in flight (a state short of approved, escalated, or
// invalidated). Read-only: unreadable stores contribute nothing, matching
// the recovery scan's treatment of absent history.
func reviewStopHookLiveReviewInFlight(ctx context.Context, root string) bool {
	stores, err := reviewtransaction.DiscoverCompactStores(ctx, root)
	if err != nil {
		return false
	}
	for _, store := range stores {
		record, loadErr := store.LoadContext(ctx)
		if loadErr != nil {
			continue
		}
		switch record.State.State {
		case reviewtransaction.StateApproved, reviewtransaction.StateEscalated, reviewtransaction.StateInvalidated:
			continue
		default:
			return true
		}
	}
	return false
}

// reviewStopHookAssessSelectors extracts the assess selectors the offered
// START carries. Only the committed-range pair is mirrored: an overlay or
// current-changes START (or an Execute without arguments) keeps the
// selectorless assessment, which is what those scopes assessed before this
// gate existed.
func reviewStopHookAssessSelectors(status *ReviewTargetStatusResult) []string {
	if status == nil || status.NextTransition == nil || status.NextTransition.Execute == nil {
		return nil
	}
	var baseRef, committedOnly string
	for _, argument := range status.NextTransition.Execute.Arguments {
		switch argument.Name {
		case "base-ref":
			baseRef = argument.Value
		case "committed-only":
			committedOnly = argument.Value
		}
	}
	if strings.TrimSpace(baseRef) == "" || committedOnly != "true" {
		return nil
	}
	return []string{"--base-ref", baseRef, "--committed-only"}
}

// reviewStopHookEmptyCommittedRange reports whether err is assess's refusal
// to classify an empty candidate ("no pending changes"): with the offered
// --base-ref the committed range is empty, so there is nothing to review and
// the hook stays silent instead of failing open into a reminder.
func reviewStopHookEmptyCommittedRange(err error) bool {
	return err != nil && strings.Contains(err.Error(), "has no pending changes")
}
