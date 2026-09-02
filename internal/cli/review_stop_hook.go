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

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// reviewStopHookSchema identifies the reminder envelope this hook prints to
// stdout. Claude Code reads "decision" and "reason" as top-level keys.
const reviewStopHookSchema = "gentle-ai.review-stop-hook/v1"

// reviewStopHookReminderSchema identifies the small per-session dedupe record
// kept under ~/.gentle-ai/review-stop-hook/v1.
const reviewStopHookReminderSchema = "gentle-ai.review-stop-hook-reminder/v1"

// maxReviewStopHookPayloadBytes bounds the Stop hook stdin payload, mirroring
// the sdd-task-result precedent (internal/cli/sdd_task_result.go): a hook
// reports a small event, never a large or unbounded stream.
const maxReviewStopHookPayloadBytes = 1 << 20

// reviewStopHookSessionIDPattern is the safe filename alphabet a session id
// must match before it is used to name a state file.
var reviewStopHookSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// reviewStopHookPayload is Claude Code's Stop hook stdin payload. Unknown
// fields are ignored by json.Unmarshal, never rejected.
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

// reviewStopHookReminderRecord is the per-session dedupe record: one file
// keeps the last target_identity a session was reminded about, so a second
// Stop for the same unreviewed candidate stays silent.
type reviewStopHookReminderRecord struct {
	Schema         string `json:"schema"`
	TargetIdentity string `json:"target_identity"`
}

// RunReviewStopHook is the `gentle-ai review stop-hook` entry point. Claude
// Code runs it as a Stop hook: it reads one Stop hook payload from stdin and,
// only when receipt-driven development is enabled and the repository holds an
// unreviewed candidate, prints one reminder that routes the agent through the
// selectorless STATUS preflight before it reports completion. Every other
// case is silent (exit 0, no output), and this command never starts a review
// itself.
func RunReviewStopHook(args []string, stdout io.Writer) error {
	return runReviewStopHook(args, os.Stdin, stdout, os.Stderr)
}

// runReviewStopHook is RunReviewStopHook's testable form: stdin and stderr are
// both explicit so a test can supply a payload and assert on diagnostics
// without touching the process streams. stderr is also where flag usage/help
// text is written (never stdout), so stdout stays reserved for the exact one
// JSON reminder object this command may print.
func runReviewStopHook(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := newReviewFlagSet("review stop-hook", stderr,
		"Read one Claude Code Stop hook payload from stdin. When receipt-driven development is enabled and the repository holds an unreviewed candidate, print one reminder that routes the agent through the selectorless STATUS preflight before it reports completion. Every other case is silent, and this command never runs review start itself.")
	agent := flags.String("agent", "", "required; generated active runtime identity (only claude-code is accepted today)")
	cwdOverride := flags.String("cwd", "", "optional; overrides the Stop hook payload's cwd")
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
		return fmt.Errorf("review stop-hook: read Stop hook payload: %w", err)
	}
	var payload reviewStopHookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("review stop-hook: decode Stop hook payload: %w", err)
	}

	if payload.StopHookActive {
		return nil
	}

	repo := strings.TrimSpace(*cwdOverride)
	if repo == "" {
		repo = strings.TrimSpace(payload.Cwd)
	}
	if repo == "" {
		return nil
	}

	ctx := context.Background()

	// Confirm the cwd is inside a Git repository without ever bootstrapping
	// one: ResolveRepositoryRoot alone (never PrepareReviewRepositoryRoot)
	// never runs `git init`. This has to happen before resolving the review
	// mode below: the clone-local override lookup shells out to git, and on a
	// genuinely non-Git cwd it returns a hard error rather than "no override"
	// -- so silently exiting here, before that call, is what keeps a
	// non-repository cwd silent instead of surfacing as an internal error.
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return nil
	}

	// Resolve the effective receipt-driven-development mode exactly like
	// reviewtransaction.OfferReviewAfterVerify does: global mode plus this
	// clone's off-only override, with zero side effects. Off means silent.
	global, err := readGlobalRDDMode()
	if err != nil {
		return fmt.Errorf("review stop-hook: read global review mode: %w", err)
	}
	rddStatus, err := reviewtransaction.ResolveRDDMode(ctx, root, global)
	if err != nil {
		return fmt.Errorf("review stop-hook: resolve review mode: %w", err)
	}
	if !rddStatus.Enabled() {
		return nil
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
		return fmt.Errorf("review stop-hook: review status: %w", statusErr)
	}
	var status ReviewTargetStatusResult
	if err := json.Unmarshal(statusOutput.Bytes(), &status); err != nil {
		return fmt.Errorf("review stop-hook: decode review status: %w", err)
	}
	if status.NextTransition == nil || status.NextTransition.Kind != reviewNextTransitionExecute ||
		status.NextTransition.Execute == nil || status.NextTransition.Execute.Operation != "review.start" {
		return nil
	}

	alreadyReminded, persistErr := recordReviewStopHookReminder(payload.SessionID, status.TargetIdentity)
	if persistErr != nil {
		return fmt.Errorf("review stop-hook: record reminder: %w", persistErr)
	}
	if alreadyReminded {
		return nil
	}

	output := reviewStopHookOutput{
		Schema:   reviewStopHookSchema,
		Decision: "block",
		Reason:   reviewStopHookReasonText(status.TargetIdentity, root, runtimeAgent, status.NextTransition.Execute.Command),
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("review stop-hook: encode reminder: %w", err)
	}
	_, err = fmt.Fprintln(stdout, string(encoded))
	return err
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

// recordReviewStopHookReminder persists that sessionID was reminded about
// targetIdentity, and reports whether it already had been. An invalid or
// missing session id still reminds (the caller proceeds) but is never
// persisted, since there is no safe file name to record it under.
func recordReviewStopHookReminder(sessionID, targetIdentity string) (alreadyReminded bool, err error) {
	if !reviewStopHookSessionIDPattern.MatchString(sessionID) {
		return false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("resolve user home directory: %w", err)
	}
	dir := filepath.Join(home, ".gentle-ai", "review-stop-hook", "v1")
	path := filepath.Join(dir, sessionID+".json")
	existing, readErr := os.ReadFile(path)
	if readErr == nil {
		var record reviewStopHookReminderRecord
		if json.Unmarshal(existing, &record) == nil && record.TargetIdentity == targetIdentity {
			return true, nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return false, readErr
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	payload, err := json.Marshal(reviewStopHookReminderRecord{
		Schema:         reviewStopHookReminderSchema,
		TargetIdentity: targetIdentity,
	})
	if err != nil {
		return false, err
	}
	_, err = filemerge.WriteFileAtomic(path, payload, 0o644)
	return false, err
}
