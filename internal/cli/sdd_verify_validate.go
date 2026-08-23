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

type sddVerifyValidateFlagKind uint8

const (
	sddVerifyValidateStringFlag sddVerifyValidateFlagKind = iota
	sddVerifyValidateIntFlag
)

type sddVerifyValidateFlagDefinition struct {
	name, value, usage string
	kind               sddVerifyValidateFlagKind
}

var sddVerifyValidateFlagDefinitions = []sddVerifyValidateFlagDefinition{
	{name: "input", value: "<path|->", usage: "Verify report path; use - to read stdin", kind: sddVerifyValidateStringFlag},
	{name: "requirements", value: "<n>", usage: "Authoritative nonnegative requirement total", kind: sddVerifyValidateIntFlag},
	{name: "scenarios", value: "<n>", usage: "Authoritative nonnegative scenario total", kind: sddVerifyValidateIntFlag},
}

// RunSDDVerifyValidate validates a complete report without touching an artifact store.
func RunSDDVerifyValidate(args []string, stdout io.Writer) error {
	return runSDDVerifyValidate(args, os.Stdin, stdout)
}

func runSDDVerifyValidate(args []string, stdin io.Reader, stdout io.Writer) error {
	if hasSDDVerifyValidateHelp(args) {
		return renderSDDVerifyValidateHelp(stdout)
	}
	flags := flag.NewFlagSet("sdd-verify-validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := registerSDDVerifyValidateStringFlag(flags, "input", "")
	requirements := registerSDDVerifyValidateIntFlag(flags, "requirements", -2)
	scenarios := registerSDDVerifyValidateIntFlag(flags, "scenarios", -2)
	if err := flags.Parse(args); err != nil {
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
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if sddVerifyValidateHelpAlias(argument) {
			return true
		}
		if hasInlineValue, known := sddVerifyValidateKnownFlag(argument); known && !hasInlineValue && index+1 < len(args) {
			index++
		}
	}
	return false
}

func sddVerifyValidateHelpAlias(argument string) bool {
	return argument == "--help" || argument == "-h"
}

func sddVerifyValidateKnownFlag(argument string) (hasInlineValue, known bool) {
	if !strings.HasPrefix(argument, "-") || argument == "-" {
		return false, false
	}
	name := strings.TrimLeft(argument, "-")
	if separator := strings.IndexByte(name, '='); separator >= 0 {
		name = name[:separator]
		hasInlineValue = true
	}
	_, known = sddVerifyValidateFlagDefinitionFor(name)
	return hasInlineValue, known
}

func sddVerifyValidateFlagDefinitionFor(name string) (sddVerifyValidateFlagDefinition, bool) {
	for _, definition := range sddVerifyValidateFlagDefinitions {
		if definition.name == name {
			return definition, true
		}
	}
	return sddVerifyValidateFlagDefinition{}, false
}

func registerSDDVerifyValidateStringFlag(flags *flag.FlagSet, name, defaultValue string) *string {
	definition, _ := sddVerifyValidateFlagDefinitionFor(name)
	return flags.String(definition.name, defaultValue, definition.usage)
}

func registerSDDVerifyValidateIntFlag(flags *flag.FlagSet, name string, defaultValue int) *int {
	definition, _ := sddVerifyValidateFlagDefinitionFor(name)
	return flags.Int(definition.name, defaultValue, definition.usage)
}

func renderSDDVerifyValidateHelp(stdout io.Writer) error {
	contract := sddstatus.VerifyReportValidationContract()
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>")
	_, _ = fmt.Fprintln(stdout, "\nRequired flags:")
	for _, definition := range sddVerifyValidateFlagDefinitions {
		_, _ = fmt.Fprintf(stdout, "  --%-21s %s\n", definition.name+" "+definition.value, definition.usage)
	}
	_, _ = fmt.Fprintln(stdout, "\nReport contract:")
	_, _ = fmt.Fprintf(stdout, "  schema: %s\n", contract.Schema)
	_, _ = fmt.Fprintf(stdout, "  required envelope fields: %s\n", strings.Join(contract.RequiredFields, ", "))
	_, _ = fmt.Fprintf(stdout, "  accepted verdicts: %s\n", strings.Join(contract.Verdicts, ", "))
	_, _ = fmt.Fprintf(stdout, "  maximum report size: %d bytes (%s)\n", contract.MaxBytes, formatSDDVerifyValidateByteLimit(contract.MaxBytes))
	_, _ = fmt.Fprintln(stdout, "  requirements and scenarios are completed/total; each completed count must not exceed its total.")
	_, _ = fmt.Fprintln(stdout, "  --requirements and --scenarios must exactly equal their report totals.")
	_, _ = fmt.Fprintln(stdout, "  Independent test and build execution evidence is required for a passing verification result.")
	return nil
}

func formatSDDVerifyValidateByteLimit(bytes int) string {
	for _, unit := range []struct {
		bytes int
		name  string
	}{{1 << 30, "GiB"}, {1 << 20, "MiB"}, {1 << 10, "KiB"}} {
		if bytes >= unit.bytes && bytes%unit.bytes == 0 {
			return fmt.Sprintf("%d %s", bytes/unit.bytes, unit.name)
		}
	}
	return fmt.Sprintf("%d bytes", bytes)
}
