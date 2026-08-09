package cli

import (
	"context"
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

type sddVerifyValidateFlagDefinition struct {
	name, value, usage string
	kind               sddVerifyValidateFlagKind
}

type sddVerifyValidateFlagKind uint8

const (
	sddVerifyValidateStringFlag sddVerifyValidateFlagKind = iota
	sddVerifyValidateIntFlag
)

var sddVerifyValidateFlagDefinitions = []sddVerifyValidateFlagDefinition{
	{name: "input", value: "<path|->", usage: "Verify report path; use - to read stdin", kind: sddVerifyValidateStringFlag},
	{name: "requirements", value: "<n>", usage: "Required authoritative nonnegative total in whole mode", kind: sddVerifyValidateIntFlag},
	{name: "scenarios", value: "<n>", usage: "Required authoritative nonnegative total in whole mode", kind: sddVerifyValidateIntFlag},
	{name: "cwd", value: "<repo>", usage: "Required repository in slice mode", kind: sddVerifyValidateStringFlag},
	{name: "change", value: "<name>", usage: "Required SDD change in slice mode", kind: sddVerifyValidateStringFlag},
	{name: "scope", value: "<whole|slice>", usage: "Verification scope; defaults to whole", kind: sddVerifyValidateStringFlag},
	{name: "slice-id", value: "<id>", usage: "Required provider-owned runtime objective ID in slice mode", kind: sddVerifyValidateStringFlag},
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
	cwd := registerSDDVerifyValidateStringFlag(flags, "cwd", "")
	change := registerSDDVerifyValidateStringFlag(flags, "change", "")
	scope := registerSDDVerifyValidateStringFlag(flags, "scope", "whole")
	sliceID := registerSDDVerifyValidateStringFlag(flags, "slice-id", "")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-verify-validate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*input) == "" {
		return errors.New("sdd-verify-validate requires --input")
	}
	scopeName := strings.TrimSpace(*scope)
	var counts sddstatus.SpecCounts
	var objective *sddstatus.RuntimeObjective
	switch scopeName {
	case "whole":
		if strings.TrimSpace(*sliceID) != "" {
			// refusal:by-design operator-knowledge: only the caller can remove the slice identity that contradicts its selected whole scope
			return errors.New("whole verification does not accept a slice_id")
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
		counts = sddstatus.SpecCounts{Requirements: *requirements, Scenarios: *scenarios}
	case "slice":
		if sddVerifyValidateFlagWasProvided(flags, "requirements") || sddVerifyValidateFlagWasProvided(flags, "scenarios") {
			// refusal:by-design operator-knowledge: only the caller can omit totals from its provider-owned slice invocation
			return errors.New("slice verification does not accept --requirements or --scenarios")
		}
		if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*change) == "" {
			// refusal:by-design operator-knowledge: only the caller can select the repository and authoritative change for its slice
			return errors.New("sdd-verify-validate requires --cwd and --change with --scope slice")
		}
		if strings.TrimSpace(*sliceID) == "" {
			// refusal:by-design operator-knowledge: only the caller can select the provider-owned slice it intends to verify
			return errors.New("sdd-verify-validate requires --slice-id with --scope slice")
		}
		var err error
		counts, objective, err = sddstatus.ResolveVerifyReportAuthority(context.Background(), *cwd, *change, strings.TrimSpace(*sliceID))
		if err != nil {
			return fmt.Errorf("resolve verify report authority: %w", err)
		}
	default:
		// refusal:by-design operator-knowledge: only the caller can choose one of the validator's two supported scopes
		return errors.New("verification scope must be whole or slice")
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
	objectives := []sddstatus.RuntimeObjective(nil)
	if objective != nil {
		objectives = append(objectives, *objective)
	}
	admission := sddstatus.ValidateVerifyReportAdmission(string(payload), counts, objectives...)
	if !admission.Valid {
		return fmt.Errorf("verify report admission denied: %s", admission.Reason)
	}
	if objective != nil {
		store, err := sddstatus.OpenRuntimeStore(context.Background(), *cwd, *change)
		if err != nil {
			return fmt.Errorf("open native SDD runtime authority: %w", err)
		}
		admission, err = store.AdmitSliceProof(context.Background(), string(payload), strings.TrimSpace(*scope), strings.TrimSpace(*sliceID))
		if err != nil {
			return fmt.Errorf("verify report admission denied: %w", err)
		}
		if !admission.Valid {
			return fmt.Errorf("verify report admission denied: %s", admission.Reason)
		}
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

func sddVerifyValidateFlagWasProvided(flags *flag.FlagSet, name string) bool {
	provided := false
	flags.Visit(func(flag *flag.Flag) {
		provided = provided || flag.Name == name
	})
	return provided
}

func renderSDDVerifyValidateHelp(stdout io.Writer) error {
	contract := sddstatus.VerifyReportValidationContract()
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-verify-validate --input <path|-> --requirements <n> --scenarios <n>")
	_, _ = fmt.Fprintln(stdout, "       gentle-ai sdd-verify-validate --input <path|-> --cwd <repo> --change <name> --scope slice --slice-id <id>")
	_, _ = fmt.Fprintln(stdout, "\nFlags:")
	for _, definition := range sddVerifyValidateFlagDefinitions {
		_, _ = fmt.Fprintf(stdout, "  --%-21s %s\n", definition.name+" "+definition.value, definition.usage)
	}
	_, _ = fmt.Fprintln(stdout, "\nReport contract:")
	_, _ = fmt.Fprintf(stdout, "  schema: %s\n", contract.Schema)
	_, _ = fmt.Fprintf(stdout, "  required envelope fields: %s\n", strings.Join(contract.RequiredFields, ", "))
	_, _ = fmt.Fprintf(stdout, "  accepted verdicts: %s\n", strings.Join(contract.Verdicts, ", "))
	_, _ = fmt.Fprintf(stdout, "  maximum report size: %d bytes (%s)\n", contract.MaxBytes, formatSDDVerifyValidateByteLimit(contract.MaxBytes))
	_, _ = fmt.Fprintln(stdout, "  requirements and scenarios are completed/total; each completed count must not exceed its total.")
	_, _ = fmt.Fprintln(stdout, "  whole mode requires caller-supplied --requirements and --scenarios; they must exactly equal report totals.")
	_, _ = fmt.Fprintln(stdout, "  slice mode forbids caller totals; totals are derived from the provider-owned runtime objective.")
	_, _ = fmt.Fprintf(stdout, "  slice report fields: %s; --scope slice and --slice-id must match the provider-owned objective.\n", strings.Join(contract.ScopeFields, ", "))
	_, _ = fmt.Fprintln(stdout, "\nAuthority-only fail extension:")
	_, _ = fmt.Fprintf(stdout, "  all-or-none fields: %s\n", strings.Join(contract.AuthorityOnlyFields, ", "))
	_, _ = fmt.Fprintln(stdout, "  requires verdict fail, test_exit_code 125, build_exit_code 125, and nonzero blockers and critical_findings.")
	_, _ = fmt.Fprintln(stdout, "  values: authority_only_failure=true, missing_review_authority=true, substantive_failure=false, command_failed=false.")
	_, _ = fmt.Fprintf(stdout, "  test_output_hash and build_output_hash must equal %s.\n", contract.EmptyOutputHash)
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
