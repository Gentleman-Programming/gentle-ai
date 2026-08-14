package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
)

// maxSDDTaskResultPayloadBytes bounds one `sdd task-result result` stdin
// payload. The payload is the finished phase task's output plus its metadata;
// a phase result large enough to matter is never truncated silently, so the
// bound is a refusal, not a limit.
const maxSDDTaskResultPayloadBytes = 64 << 20

// RunSDDTaskResult is the CLI entry point for `gentle-ai sdd task-result
// <guard|result|clear|clear-all>`. It is the native SDD half of the OpenCode
// task-result contract (change #3138, slice 6): the reviewer-shim glue
// (slice 6.4) spawns exactly these verbs and forwards stdout as-is, so the
// handoff and latched envelopes are the ONLY bytes the session transcript
// ever sees from this boundary.
func RunSDDTaskResult(args []string, stdout io.Writer) error {
	return runSDDTaskResult(args, os.Stdin, stdout)
}

func runSDDTaskResult(args []string, stdin io.Reader, stdout io.Writer) error {
	command, flags, err := parseSDDTaskResultArgs(args)
	if err != nil {
		return err
	}

	latchPath := flags.latchPath
	if latchPath == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("sdd task-result: resolve home directory: %w", homeErr)
		}
		latchPath = sdd.SDDLatchStorePath(home)
	}
	latch := sdd.NewFileSDDLatchStore(latchPath)

	switch command {
	case "guard":
		return sddTaskResultGuard(flags, latch, stdout)
	case "result":
		return sddTaskResultResult(flags, stdin, latch, stdout)
	case "clear":
		return latch.Clear(flags.session)
	case "clear-all":
		return latch.ClearAll()
	default:
		return fmt.Errorf("sdd task-result: unknown command %q; run \"gentle-ai sync\" to reinstall the matching glue", command)
	}
}

type sddTaskResultFlags struct {
	cwd       string
	session   string
	phase     string
	latchPath string
}

func parseSDDTaskResultArgs(args []string) (string, sddTaskResultFlags, error) {
	if len(args) == 0 {
		return "", sddTaskResultFlags{}, fmt.Errorf("sdd task-result: missing command (guard, result, clear or clear-all); run \"gentle-ai sync\" to reinstall the matching glue")
	}
	command := args[0]
	flags := sddTaskResultFlags{}
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		token, inlineValue, inline := sddTaskResultSplitFlag(rest[i])
		switch token {
		case "--cwd":
			value, next, err := sddTaskResultFlagValue(rest, i, inlineValue, inline, "--cwd")
			if err != nil {
				return "", flags, err
			}
			flags.cwd = value
			i = next
		case "--session":
			value, next, err := sddTaskResultFlagValue(rest, i, inlineValue, inline, "--session")
			if err != nil {
				return "", flags, err
			}
			flags.session = value
			i = next
		case "--phase":
			value, next, err := sddTaskResultFlagValue(rest, i, inlineValue, inline, "--phase")
			if err != nil {
				return "", flags, err
			}
			flags.phase = value
			i = next
		case "--latch-path":
			value, next, err := sddTaskResultFlagValue(rest, i, inlineValue, inline, "--latch-path")
			if err != nil {
				return "", flags, err
			}
			flags.latchPath = value
			i = next
		default:
			return "", flags, fmt.Errorf("sdd task-result: unknown flag %q; run \"gentle-ai sync\" to reinstall the matching glue", rest[i])
		}
	}

	switch command {
	case "guard", "result":
		if flags.cwd == "" {
			return "", flags, fmt.Errorf("sdd task-result %s: --cwd is required; run \"gentle-ai sync\" to reinstall the matching glue", command)
		}
		if flags.session == "" {
			return "", flags, fmt.Errorf("sdd task-result %s: --session is required; run \"gentle-ai sync\" to reinstall the matching glue", command)
		}
		if flags.phase == "" {
			return "", flags, fmt.Errorf("sdd task-result %s: --phase is required; run \"gentle-ai sync\" to reinstall the matching glue", command)
		}
	case "clear":
		if flags.session == "" {
			return "", flags, fmt.Errorf("sdd task-result clear: --session is required; run \"gentle-ai sync\" to reinstall the matching glue")
		}
	}
	return command, flags, nil
}

// sddTaskResultSplitFlag splits a "--flag=value" token into ("--flag",
// "value", true). A bare "--flag" returns ("--flag", "", false), leaving the
// value to the next argument.
func sddTaskResultSplitFlag(arg string) (string, string, bool) {
	if name, value, ok := strings.Cut(arg, "="); ok {
		return name, value, true
	}
	return arg, "", false
}

func sddTaskResultFlagValue(args []string, index int, inlineValue string, inline bool, flag string) (string, int, error) {
	if inline {
		if inlineValue == "" {
			return "", index, fmt.Errorf("sdd task-result: %s requires a value; run \"gentle-ai sync\" to reinstall the matching glue", flag)
		}
		return inlineValue, index, nil
	}
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("sdd task-result: %s requires a value; run \"gentle-ai sync\" to reinstall the matching glue", flag)
	}
	return args[index+1], index + 1, nil
}

// sddTaskResultGuard answers "may this SDD phase launch dispatch in this
// session?" Empty stdout means yes. A latched session prints the latched
// envelope refusing THIS launch -- naming the requested phase, the original
// failure, and the exit (#2948) -- which the glue throws as-is.
// Non-SDD phases are never answered by this guard, so a latched session can
// never block an unrelated launch.
func sddTaskResultGuard(flags sddTaskResultFlags, latch sdd.SDDLatchStore, stdout io.Writer) error {
	if !sdd.IsSDDPhase(flags.phase) {
		return nil
	}
	failure, latched, err := latch.Recall(flags.session)
	if err != nil {
		return fmt.Errorf("sdd task-result guard: recall latch: %w", err)
	}
	if !latched {
		return nil
	}
	_, err = io.WriteString(stdout, sdd.SDDDispatchLatchedEnvelope(flags.phase, failure, flags.cwd))
	return err
}

// sddTaskResultResult classifies one finished SDD phase task. A passing task
// prints nothing (the glue leaves the tool result untouched). A failing task
// prints the terminal handoff and records the latch for this session, so the
// next SDD phase launch in this session is refused before it reaches the
// provider (SEN-SOA-2).
func sddTaskResultResult(flags sddTaskResultFlags, stdin io.Reader, latch sdd.SDDLatchStore, stdout io.Writer) error {
	payload, err := io.ReadAll(io.LimitReader(stdin, maxSDDTaskResultPayloadBytes+1))
	if err != nil {
		return fmt.Errorf("sdd task-result result: read payload: %w", err)
	}
	if len(payload) > maxSDDTaskResultPayloadBytes {
		return fmt.Errorf("sdd task-result result: payload exceeds %d bytes; inspect the phase state with \"gentle-ai sdd-status --cwd\"", maxSDDTaskResultPayloadBytes)
	}
	var body struct {
		Output   any `json:"output"`
		Metadata any `json:"metadata"`
	}
	// An undecodable payload fails closed exactly like an unclassifiable
	// result: the boundary cannot establish that the phase succeeded, so the
	// failure latch is recorded and the terminal handoff is written, never a
	// silent pass for the next SDD phase launch (SEN-SOA-2). The raw Go
	// decode-error text stays out of the transcript -- the handoff envelope
	// is the only permanent record.
	class := "malformed_result"
	if err := json.Unmarshal(payload, &body); err == nil {
		var unwrapErr error
		if _, class, unwrapErr = sdd.UnwrapSDDTaskResult(body.Output); unwrapErr == nil {
			return nil
		}
	}

	handoff := sdd.SDDTaskFailureEnvelope(flags.phase, flags.cwd, class, body.Metadata)
	if err := latch.Record(flags.session, sdd.SDDTaskFailure{
		Phase:   flags.phase,
		Code:    sdd.SDDTaskResultCode(class),
		Handoff: handoff,
	}); err != nil {
		return fmt.Errorf("sdd task-result result: record latch: %w", err)
	}
	_, err = io.WriteString(stdout, handoff)
	return err
}
