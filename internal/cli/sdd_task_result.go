package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddtaskresult"
)

// maxSDDTaskResultBytes bounds what a phase result may be before it is refused
// outright. A delegated phase returns a report, never a payload.
const maxSDDTaskResultBytes = 4 << 20

// sddTaskResultUsage is the runnable form every usage refusal names, so a
// caller is never told what is missing without being told how to supply it.
const sddTaskResultUsage = "`gentle-ai sdd-task-result --phase <phase> --cwd <repo> --input <path|->`"

// RunSDDTaskResult classifies one delegated SDD phase result and renders the
// typed terminal failure when it is not usable.
//
// #3818: this contract was enforced only by the OpenCode plugin; every other
// runtime carried it as prose with nothing evaluating it. This command is the
// enforcement surface those runtimes were missing, and it renders bytes
// identical to the OpenCode transport because both read one Go definition.
func RunSDDTaskResult(args []string, stdout io.Writer) error {
	return runSDDTaskResult(args, os.Stdin, stdout)
}

func runSDDTaskResult(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("sdd-task-result", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	phase := flags.String("phase", "", "required; SDD phase that produced the result")
	cwd := flags.String("cwd", "", "required; repository working directory for the continuation")
	change := flags.String("change", "", "optional; SDD change name the continuation selects")
	input := flags.String("input", "-", "result path; use - to read stdin")
	taskModel := flags.String("task-model", "", "optional; provider/model route that produced the result")
	latchedPhase := flags.String("latched-phase", "", "optional; phase that failed earlier in this session")
	latchedCode := flags.String("latched-code", "", "optional; code that phase failed with")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*phase) == "" {
		return errors.New("sdd-task-result requires --phase; run " + sddTaskResultUsage)
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("sdd-task-result requires --cwd; run " + sddTaskResultUsage)
	}

	// A latched session never dispatched, so the result bytes are irrelevant:
	// refuse before reading them rather than classifying output that no phase
	// actually produced.
	if *latchedPhase != "" || *latchedCode != "" {
		if *latchedPhase == "" || *latchedCode == "" {
			return errors.New("sdd-task-result requires both --latched-phase and --latched-code, or neither; run " + sddTaskResultUsage)
		}
		return errors.New(sddtaskresult.DispatchLatched(*phase, *latchedPhase, *latchedCode, *cwd, strings.TrimSpace(*change)))
	}

	output, err := readSDDTaskResult(*input, stdin)
	if err != nil {
		return err
	}
	if handoff := sddtaskresult.Handoff(sddtaskresult.Classify(output), *phase, *cwd, strings.TrimSpace(*change), *taskModel); handoff != "" {
		return errors.New(handoff)
	}
	_, err = fmt.Fprintln(stdout, `{"status":"ok"}`)
	return err
}

func readSDDTaskResult(input string, stdin io.Reader) (string, error) {
	var reader io.Reader = stdin
	if input != "-" {
		file, err := os.Open(input)
		if err != nil {
			return "", fmt.Errorf("read SDD task result: %w", err)
		}
		defer file.Close()
		reader = file
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxSDDTaskResultBytes+1))
	if err != nil {
		return "", fmt.Errorf("read SDD task result: %w", err)
	}
	if len(content) > maxSDDTaskResultBytes {
		return "", fmt.Errorf("SDD task result exceeds %d bytes; a phase returns a report, not a payload — re-run the phase and pass its report to %s", maxSDDTaskResultBytes, sddTaskResultUsage)
	}
	return string(content), nil
}
