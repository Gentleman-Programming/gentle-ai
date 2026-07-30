package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

const maxVerifyReportBytes = sddstatus.MaxVerifyReportBytes

// RunSDDVerifyValidate validates a complete report without touching an artifact store.
func RunSDDVerifyValidate(args []string, stdout io.Writer) error {
	return runSDDVerifyValidate(args, os.Stdin, stdout)
}

func runSDDVerifyValidate(args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("sdd-verify-validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() { renderSDDVerifyValidateHelp(stdout) }
	if hasSDDVerifyValidateHelp(args) {
		flags.Usage()
		return nil
	}
	input := flags.String("input", "", "report path or - for stdin")
	requirements := flags.Int("requirements", -2, "authoritative requirement count")
	scenarios := flags.Int("scenarios", -2, "authoritative scenario count")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-verify-validate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*input) == "" {
		return errors.New("sdd-verify-validate requires --input")
	}
	if *requirements == -2 {
		return errors.New("sdd-verify-validate requires --requirements")
	}
	if *scenarios == -2 {
		return errors.New("sdd-verify-validate requires --scenarios")
	}
	if *requirements < 0 || *scenarios < 0 {
		return errors.New("requirement and scenario counts must be nonnegative")
	}
	reader := stdin
	if *input != "-" {
		file, err := os.Open(*input)
		if err != nil {
			return fmt.Errorf("read verify report: %w", err)
		}
		defer file.Close()
		reader = file
	}
	payload, err := io.ReadAll(io.LimitReader(reader, maxVerifyReportBytes+1))
	if err != nil {
		return fmt.Errorf("read verify report: %w", err)
	}
	if len(payload) > maxVerifyReportBytes {
		return fmt.Errorf("verify report exceeds %d-byte limit", maxVerifyReportBytes)
	}
	admission := sddstatus.ValidateVerifyReportAdmission(string(payload), sddstatus.SpecCounts{Requirements: *requirements, Scenarios: *scenarios})
	if !admission.Valid {
		return fmt.Errorf("verify report admission denied: %s", admission.Reason)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(admission)
}

func hasSDDVerifyValidateHelp(args []string) bool {
	for _, argument := range args {
		if argument == "-h" || argument == "--help" {
			return true
		}
	}
	return false
}

func renderSDDVerifyValidateHelp(stdout io.Writer) {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-verify-validate [flags]")
	_, _ = fmt.Fprintln(stdout, "\nFlags:")
	_, _ = fmt.Fprintln(stdout, "  --input <path>         report path, or - for stdin")
	_, _ = fmt.Fprintln(stdout, "  --requirements <n>     authoritative requirement count; must be nonnegative")
	_, _ = fmt.Fprintln(stdout, "  --scenarios <n>        authoritative scenario count; must be nonnegative")
	_, _ = fmt.Fprintln(stdout, "\nReport contract:")
	_, _ = fmt.Fprintf(stdout, "  schema: %s\n", sddstatus.VerifyResultSchema)
	_, _ = fmt.Fprintf(stdout, "  base envelope fields: %s\n", strings.Join(sddstatus.VerifyReportEnvelopeFields(), ", "))
	_, _ = fmt.Fprintf(stdout, "  verdict: %s\n", strings.Join(sddstatus.VerifyVerdicts(), "|"))
	_, _ = fmt.Fprintf(stdout, "  report limit: 1 MiB (%d bytes)\n", maxVerifyReportBytes)
}
