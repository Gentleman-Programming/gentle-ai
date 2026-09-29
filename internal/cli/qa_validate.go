package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/qastage"
)

const maxQAValidateBytes = 1024 * 1024 // 1 MiB

type qaValidateFlagDefinition struct {
	name, value, usage string
}

var qaValidateFlagDefinitions = []qaValidateFlagDefinition{
	{name: "input", value: "<path|->", usage: "Artifact path; use - to read stdin"},
	{name: "change", value: "<name>", usage: "Change name being validated"},
	{name: "stage", value: "<label>", usage: "Current stage label (explore|spec|apply|verify|docs)"},
	{name: "source-revision", value: "<rev>", usage: "Expected predecessor SHA-256 (optional)"},
}

// QAValidateResult is the JSON-encoded result of qa-validate. ArtifactRevision
// is populated only when Valid is true — it is the sha256 of the
// canonicalized artifact, and is the value qa-finish/qa-approve require as
// --evidence-revision (3A.8/3A.9). This closes the v2 gap where qa-validate
// never returned any revision for callers to bind evidence to.
type QAValidateResult struct {
	Valid            bool   `json:"valid"`
	Stage            string `json:"stage"`
	Change           string `json:"change"`
	ArtifactRevision string `json:"artifact_revision,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// RunQAValidate is the CLI entry point for `gentle-ai qa-validate`.
func RunQAValidate(args []string, stdout io.Writer) error {
	return RunQAValidateFromReader(args, os.Stdin, stdout)
}

// RunQAValidateFromReader is the testable core of qa-validate.
func RunQAValidateFromReader(args []string, stdin io.Reader, stdout io.Writer) error {
	if hasQAValidateHelp(args) {
		return renderQAValidateHelp(stdout)
	}
	flags := flag.NewFlagSet("qa-validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "")
	change := flags.String("change", "", "")
	stage := flags.String("stage", "", "")
	sourceRevision := flags.String("source-revision", "", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected qa-validate argument %q; run `gentle-ai qa-validate --help` for valid flags", flags.Arg(0))
	}
	if strings.TrimSpace(*input) == "" {
		return errors.New("qa-validate requires --input; run `gentle-ai qa-validate --help` for usage")
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("qa-validate requires --change; run `gentle-ai qa-validate --help` for usage")
	}
	if strings.TrimSpace(*stage) == "" {
		return errors.New("qa-validate requires --stage; run `gentle-ai qa-validate --help` for usage")
	}

	reader := stdin
	if *input != "-" {
		file, err := os.Open(*input)
		if err != nil {
			return fmt.Errorf("read qa artifact: %w", err)
		}
		defer file.Close()
		reader = file
	}
	payload, err := io.ReadAll(io.LimitReader(reader, maxQAValidateBytes+1))
	if err != nil {
		return fmt.Errorf("read qa artifact: %w", err)
	}
	if len(payload) > maxQAValidateBytes {
		return fmt.Errorf("qa artifact exceeds %d-byte limit", maxQAValidateBytes) // refusal:by-design world-action: the exit is shrinking the artifact content, not a command to run
	}

	admission := qastage.NewCanonicalArtifactValidator().Validate(payload, *stage, *sourceRevision)
	result := QAValidateResult{
		Valid:            admission.Valid,
		Stage:            *stage,
		Change:           *change,
		ArtifactRevision: admission.ArtifactRevision,
		Reason:           admission.Reason,
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func hasQAValidateHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func renderQAValidateHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai qa-validate --input <path|-> --change <name> --stage <label> [--source-revision <rev>]")
	_, _ = fmt.Fprintln(stdout, "\nRequired flags:")
	for _, def := range qaValidateFlagDefinitions {
		required := ""
		if def.name != "source-revision" {
			required = " (required)"
		}
		_, _ = fmt.Fprintf(stdout, "  --%-22s %s%s\n", def.name+" "+def.value, def.usage, required)
	}
	_, _ = fmt.Fprintln(stdout, "\nValid stage labels: explore, spec, apply, verify, docs")
	_, _ = fmt.Fprintln(stdout, "\nOutputs JSON: {\"valid\": bool, \"stage\": str, \"change\": str, \"artifact_revision\": str?, \"reason\": str?}")
	return nil
}
