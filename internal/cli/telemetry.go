package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// TelemetryStatusSchema identifies the `gentle-ai telemetry status|enable|disable`
// projection (gentle-ai.telemetry-status/v1).
const TelemetryStatusSchema = telemetry.StatusSchema

// TelemetryStdinReadLimit caps how many bytes `telemetry send` will read from
// its own stdin: one more than the contract's payload ceiling, so an
// oversized or runaway stream is refused rather than read without bound.
const TelemetryStdinReadLimit = telemetry.MaxPayloadBytes + 1

// TelemetryStatusResult is the typed answer for status, enable, and disable:
// each reports the operation it ran and the resulting effective state.
type TelemetryStatusResult struct {
	Schema            string             `json:"schema"`
	Operation         string             `json:"operation"`
	Enabled           bool               `json:"enabled"`
	Source            string             `json:"source"`
	InstallID         string             `json:"install_id"`
	NoticeShown       bool               `json:"notice_shown"`
	Endpoint          string             `json:"endpoint"`
	LastInstallSentAt string             `json:"last_install_sent_at,omitempty"`
	LastHeartbeatAt   string             `json:"last_heartbeat_at,omitempty"`
	LastFailureAt     string             `json:"last_failure_at,omitempty"`
	BackoffUntil      string             `json:"backoff_until,omitempty"`
	Counters          telemetry.Counters `json:"counters"`
}

// TelemetryTriggerSchema identifies `gentle-ai telemetry trigger`'s answer
// (gentle-ai.telemetry-trigger/v1).
const TelemetryTriggerSchema = "gentle-ai.telemetry-trigger/v1"

// TelemetryTriggerResult is what `telemetry trigger` reports: the exhaustive
// classification of what Opportunistic just did, never an error.
type TelemetryTriggerResult struct {
	Schema   string `json:"schema"`
	Decision string `json:"decision"`
	Source   string `json:"source"`
}

// RunTelemetry implements `gentle-ai telemetry status|enable|disable|preview`
// plus the hidden `telemetry send` subcommand that SpawnDetachedSend
// launches in the background (payload arrives on its stdin, never a file
// path, so there is never a path for it to remove). status, enable, disable,
// and preview never send anything themselves.
func RunTelemetry(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai telemetry <status|enable|disable|preview|trigger> [--json]")
		_, _ = fmt.Fprintln(stdout, "Anonymous, opt-out usage telemetry. status reports whether sending is enabled and why; enable/disable set the local opt-out; preview prints the exact payload that would be sent next, without sending it; trigger runs the same opportunistic check install/update/sync run internally (enrollment, install-once, the 24h heartbeat limit, and the failure backoff all apply) — hosts such as Gentle Pi that call gentle-ai only through review/sdd-attempt use this to still get a heartbeat. The first run only shows the notice and sends nothing; sending starts on the following trigger. Opt out permanently with `gentle-ai telemetry disable`, or for one run with DO_NOT_TRACK=1.")
		return nil
	}

	switch args[0] {
	case "status", "enable", "disable":
		return runTelemetryStateCommand(args[0], args[1:], stdout)
	case "preview":
		return runTelemetryPreview(args[1:], stdout)
	case "trigger":
		return runTelemetryTriggerCommand(args[1:], stdout)
	case "send":
		return runTelemetrySend(args[1:], stdout)
	default:
		// refusal:by-design operator-knowledge: telemetry has exactly five subcommands and the usage line above already names every one of them
		return fmt.Errorf("unknown telemetry command %q", args[0])
	}
}

func runTelemetryStateCommand(operation string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("telemetry "+operation, flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	emitJSON := flags.Bool("json", false, "emit the machine-readable telemetry status")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected telemetry %s argument %q", operation, flags.Arg(0)) // refusal:by-design operator-knowledge: rerun `gentle-ai telemetry status|enable|disable` with no positional arguments
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	// EnsureState (never Load) so install_id is stable across consecutive
	// status/preview calls instead of a fresh random one on every call where
	// no state file exists yet. status still never mutates anything beyond
	// this one, unavoidable, create-if-missing side effect: it does not
	// touch enabled, notice_shown, or counters.
	persisted, err := telemetry.EnsureState(homeDir)
	if err != nil {
		return fmt.Errorf("load telemetry state: %w", err)
	}
	switch operation {
	case "enable":
		persisted.Enabled = true
		if err := telemetry.Save(homeDir, persisted); err != nil {
			return fmt.Errorf("persist telemetry state: %w", err)
		}
	case "disable":
		persisted.Enabled = false
		if err := telemetry.Save(homeDir, persisted); err != nil {
			return fmt.Errorf("persist telemetry state: %w", err)
		}
	}

	decision := telemetry.Decide(os.Getenv, persisted)
	result := TelemetryStatusResult{
		Schema: TelemetryStatusSchema, Operation: operation, Enabled: decision.Enabled, Source: string(decision.Source),
		InstallID: persisted.InstallID, NoticeShown: persisted.NoticeShown, Endpoint: telemetry.Endpoint(os.Getenv),
		Counters: persisted.Counters,
	}
	if persisted.LastInstallSentAt != nil {
		result.LastInstallSentAt = persisted.LastInstallSentAt.UTC().Format(time.RFC3339)
	}
	if persisted.LastHeartbeatAt != nil {
		result.LastHeartbeatAt = persisted.LastHeartbeatAt.UTC().Format(time.RFC3339)
	}
	if persisted.LastFailureAt != nil {
		result.LastFailureAt = persisted.LastFailureAt.UTC().Format(time.RFC3339)
		if until := persisted.LastFailureAt.UTC().Add(telemetry.FailureBackoff); until.After(time.Now().UTC()) {
			result.BackoffUntil = until.Format(time.RFC3339)
		}
	}

	if *emitJSON {
		return encodeReviewJSON(stdout, result)
	}
	_, _ = fmt.Fprintf(stdout, "telemetry: %s (source: %s)\n", enabledWord(result.Enabled), result.Source)
	_, _ = fmt.Fprintf(stdout, "install_id: %s\n", result.InstallID)
	_, _ = fmt.Fprintf(stdout, "endpoint: %s\n", result.Endpoint)
	if !result.Enabled {
		_, _ = fmt.Fprintln(stdout, "run `gentle-ai telemetry enable` to opt back in")
	} else {
		_, _ = fmt.Fprintln(stdout, "run `gentle-ai telemetry disable` to opt out")
	}
	return nil
}

func enabledWord(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func runTelemetryPreview(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("telemetry preview", flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	cwd := flags.String("cwd", ".", "repository path, used to resolve rdd_enabled")
	emitJSON := flags.Bool("json", false, "emit the raw event JSON (this is always the format actually sent)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected telemetry preview argument %q", flags.Arg(0)) // refusal:by-design operator-knowledge: rerun `gentle-ai telemetry preview` with no positional arguments
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	ev, err := previewEvent(homeDir, *cwd)
	if err != nil {
		return err
	}
	payload, err := telemetry.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal preview event: %w", err)
	}
	if *emitJSON {
		_, err = stdout.Write(append(payload, '\n'))
		return err
	}
	_, _ = fmt.Fprintf(stdout, "next event: %s\n", ev.Event)
	_, err = stdout.Write(append(payload, '\n'))
	return err
}

// previewEvent builds the exact event Opportunistic would send once enrolled
// and once a send is actually due, ignoring the heartbeat rate limit
// (preview is inspection, never a send, so there is nothing to throttle) —
// see EnsureState for why this, not Load, is what keeps the install_id
// stable across repeated preview/status calls.
func previewEvent(homeDir, cwd string) (telemetry.Event, error) {
	persisted, err := telemetry.EnsureState(homeDir)
	if err != nil {
		return telemetry.Event{}, fmt.Errorf("load telemetry state: %w", err)
	}
	kind := telemetry.EventHeartbeat
	if persisted.LastInstallSentAt == nil {
		kind = telemetry.EventInstall
	}
	agents, components, err := telemetryInstalledSelection(homeDir)
	if err != nil {
		return telemetry.Event{}, err
	}
	rddEnabled := telemetryRDDEnabled(cwd)
	return telemetry.Build(telemetry.BuildInput{
		Kind: kind, InstallID: persisted.InstallID, Now: time.Now(), Version: strings.TrimSpace(AppVersion),
		Agents: agents, Components: components, RDDEnabled: rddEnabled, Counters: persisted.Counters,
	}), nil
}

// runTelemetrySend is the hidden subcommand SpawnDetachedSend launches in
// the background. It reads the event payload from its own stdin (capped at
// one byte over the contract's 4 KiB ceiling) rather than a file path: there
// is deliberately no path here for this process to ever remove.
func runTelemetrySend(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("telemetry send", flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("telemetry send takes no arguments; it reads its payload from stdin") // refusal:by-design operator-knowledge: this hidden subcommand is only ever invoked by gentle-ai's own detached sender, which always pipes the payload on stdin
	}
	limited := io.LimitReader(os.Stdin, TelemetryStdinReadLimit)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read telemetry payload from stdin: %w", err)
	}
	if len(payload) >= TelemetryStdinReadLimit {
		return fmt.Errorf("telemetry payload on stdin exceeds %d bytes", telemetry.MaxPayloadBytes) // refusal:by-design operator-knowledge: this hidden subcommand only ever receives a payload gentle-ai's own event builder produced, which never exceeds the contract's ceiling; an oversized stream indicates a malformed caller, not a state any command can fix
	}
	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	return telemetry.PerformSend(homeDir, payload, os.Getenv, time.Now, telemetry.NewHTTPClient())
}

// telemetryInstalledSelection reads the persisted install selection for the
// event builder's agents/components fields. A missing state file (telemetry
// running before any install ever completed) reports empty selections
// rather than an error.
func telemetryInstalledSelection(homeDir string) ([]string, []string, error) {
	persisted, err := state.Read(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read install state: %w", err)
	}
	components := make([]string, 0, len(persisted.Components))
	for _, c := range persisted.Components {
		components = append(components, string(c))
	}
	return append([]string(nil), persisted.InstalledAgents...), components, nil
}

// telemetryRDDEnabled reads the effective receipt-driven-development mode
// through the same read-only reader `gentle-ai review mode status` uses. A
// resolution failure (e.g. cwd is not a repository) is reported as disabled
// rather than failing the caller: telemetry must never block on this.
func telemetryRDDEnabled(cwd string) bool {
	status, err := ReviewModeStatus(context.Background(), cwd)
	if err != nil {
		return false
	}
	return status.Enabled()
}

// telemetryEnabledHomeDir resolves the home directory only if telemetry is
// currently allowed to touch disk at all, checking cheapest-first:
//
// It calls telemetry.Decide before touching disk: when any kill switch is
// already off, given a state of {Enabled: true} (i.e. "nothing yet known
// about local state"), Decide can only have been tripped by an environment
// switch, so this returns false without ever resolving a home directory or
// reading/writing anything. Only when the environment allows it does this
// resolve home and re-run Decide against the real persisted state (which a
// prior `gentle-ai telemetry disable` may have turned off).
func telemetryEnabledHomeDir() (string, bool) {
	if envDecision := telemetry.Decide(os.Getenv, telemetry.State{Enabled: true}); !envDecision.Enabled {
		return "", false
	}
	homeDir, err := osUserHomeDir()
	if err != nil {
		return "", false
	}
	persisted, err := telemetry.Load(homeDir)
	switch {
	case err == nil:
		if !telemetry.Decide(os.Getenv, persisted).Enabled {
			return "", false
		}
	case os.IsNotExist(err):
		// No state file yet: nothing has ever opted out locally, so the
		// environment-only decision above already settled it as enabled.
	default:
		// Unreadable state: fail safe rather than guess.
		return "", false
	}
	return homeDir, true
}

// telemetryRecordReviewOutcome increments the one matching counter when a
// review reaches a terminal outcome (approved, correction-required, or
// escalated), then opportunistically triggers a send: hosts such as Gentle
// Pi that drive gentle-ai only through `review ...` (never install/sync/
// update) would otherwise accumulate counters forever without ever sending a
// heartbeat. TelemetryTrigger already enforces enrollment-first, the 24h
// heartbeat limit, the failure backoff, and every kill switch, so this adds
// at most one detached send per day. Best-effort throughout: recording
// telemetry must never fail or slow down the review command that triggered
// it.
func telemetryRecordReviewOutcome(kind string) {
	defer func() { _ = recover() }()
	homeDir, ok := telemetryEnabledHomeDir()
	if !ok {
		return
	}
	switch kind {
	case "approved":
		_ = telemetry.IncrementReviewsApproved(homeDir)
	case "correction":
		_ = telemetry.IncrementReviewsCorrection(homeDir)
	case "escalated":
		_ = telemetry.IncrementReviewsEscalated(homeDir)
	}
	telemetryTriggerQuiet(homeDir)
}

// telemetryRecordSDDPhaseRun increments sdd_phase_runs when a `sdd-attempt
// finish|settle` completes successfully, then opportunistically triggers a
// send for the same reason telemetryRecordReviewOutcome does.
func telemetryRecordSDDPhaseRun() {
	defer func() { _ = recover() }()
	homeDir, ok := telemetryEnabledHomeDir()
	if !ok {
		return
	}
	_ = telemetry.IncrementSDDPhaseRuns(homeDir)
	telemetryTriggerQuiet(homeDir)
}

// runTelemetryTriggerCommand runs exactly the opportunistic path a
// successful install/update/sync runs internally, for hosts (e.g. Gentle Pi)
// that otherwise never call gentle-ai through any of those three. It always
// exits 0: TelemetryTrigger is best-effort by construction, and the detached
// child (if one was spawned) does the actual network call, so this never
// blocks.
func runTelemetryTriggerCommand(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("telemetry trigger", flag.ContinueOnError)
	flags.SetOutput(ioDiscard{})
	cwd := flags.String("cwd", ".", "repository path, used to resolve rdd_enabled")
	emitJSON := flags.Bool("json", false, "emit the machine-readable trigger result")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected telemetry trigger argument %q", flags.Arg(0)) // refusal:by-design operator-knowledge: rerun `gentle-ai telemetry trigger` with no positional arguments
	}

	homeDir, err := osUserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory: %w", err)
	}
	agents, components, err := telemetryInstalledSelection(homeDir)
	if err != nil {
		agents, components = nil, nil
	}
	outcome := telemetry.Opportunistic(telemetry.Deps{
		HomeDir: homeDir, Getenv: os.Getenv, Stderr: os.Stderr, Version: strings.TrimSpace(AppVersion),
		Agents: agents, Components: components, RDDEnabled: telemetryRDDEnabled(*cwd),
	})
	source := outcome.Source
	if source == "" {
		source = telemetry.SourceDefault
	}
	result := TelemetryTriggerResult{Schema: TelemetryTriggerSchema, Decision: string(outcome.Decision), Source: string(source)}
	if *emitJSON {
		return encodeReviewJSON(stdout, result)
	}
	_, _ = fmt.Fprintf(stdout, "telemetry trigger: %s (source: %s)\n", result.Decision, result.Source)
	return nil
}

// TelemetryTrigger is what install, update, and sync call at the end of a
// successful run. It reads the just-persisted install selection back from
// state (so the reported agents/components always match what is actually on
// disk, regardless of which in-memory selection the caller built along the
// way) and hands it to telemetry.Opportunistic. It never returns an error
// and never panics: telemetry is best-effort and must never affect the
// caller's own exit code or output.
func TelemetryTrigger(homeDir string) { telemetryTrigger(homeDir, os.Stderr) }

// telemetryTriggerQuiet is the closure-hook variant: review and SDD phase
// closures are machine-driven JSON verbs whose stderr belongs to the host
// agent, so the one-time notice is never printed there. Until an
// interactive command has shown it, these triggers record nothing and send
// nothing.
func telemetryTriggerQuiet(homeDir string) { telemetryTrigger(homeDir, nil) }

func telemetryTrigger(homeDir string, stderr io.Writer) {
	defer func() { _ = recover() }()
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return
	}
	agents, components, err := telemetryInstalledSelection(homeDir)
	if err != nil {
		return
	}
	telemetry.Opportunistic(telemetry.Deps{
		HomeDir: homeDir, Getenv: os.Getenv, Stderr: stderr, Version: strings.TrimSpace(AppVersion),
		Agents: agents, Components: components, RDDEnabled: telemetryRDDEnabled("."),
	})
}
