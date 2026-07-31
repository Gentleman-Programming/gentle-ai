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
	_, _ = fmt.Fprintf(stdout, "  report limit: %s\n", verifyReportLimitText())
	_, _ = fmt.Fprintln(stdout, "\nCount semantics:")
	_, _ = fmt.Fprintln(stdout, "  requirements and scenarios use completed/total; completed must not exceed total.")
	_, _ = fmt.Fprintln(stdout, "  --requirements and --scenarios must equal the report totals exactly; a mismatch is rejected.")
	_, _ = fmt.Fprintln(stdout, "\nScenario and evidence dispositions:")
	_, _ = fmt.Fprintln(stdout, "  A non-fail verdict requires test_exit_code 0, build_exit_code 0, blockers 0, critical_findings 0, and completed == total for requirements and scenarios.")
	_, _ = fmt.Fprintln(stdout, "  A fail verdict without the authority-only extension is rejected when all evidence is green; exit code 125 requires the authority-only extension.")
	_, _ = fmt.Fprintln(stdout, "  The authority-only extension adds authority_only_failure, missing_review_authority, substantive_failure, command_failed, and observed_authority_revision; it requires verdict fail, exit code 125, nonzero blockers and critical_findings, and the empty output hashes.")
}

func verifyReportLimitText() string {
	const bytesPerMiB = 1 << 20
	if maxVerifyReportBytes%bytesPerMiB != 0 {
		return fmt.Sprintf("%d bytes", maxVerifyReportBytes)
	}
	return fmt.Sprintf("%d MiB (%d bytes)", maxVerifyReportBytes/bytesPerMiB, maxVerifyReportBytes)
}
