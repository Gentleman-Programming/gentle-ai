package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// RunSDDStatus is the CLI entry point for `gentle-ai sdd-status [change]`.
func RunSDDStatus(args []string, stdout io.Writer) error {
	parsed, err := sddstatus.ParseCommandArgs(args)
	if err != nil {
		return err
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD:                 parsed.CWD,
		ChangeName:          parsed.ChangeName,
		IncludeInstructions: parsed.IncludeInstructions,
	})
	if err != nil {
		return fmt.Errorf("resolve sdd status: %w", err)
	}

	if parsed.JSON {
		projected, projectionErr := sddstatus.ProjectStatusV2(status)
		if projectionErr != nil {
			return fmt.Errorf("project SDD status v2: %w", projectionErr)
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(projected)
	}

	_, err = fmt.Fprintln(stdout, sddstatus.RenderMarkdown(status))
	return err
}

// RunSDDContinue is the CLI entry point for `gentle-ai sdd-continue [change]`.
func RunSDDContinue(args []string, stdout io.Writer) error {
	parsed, err := sddstatus.ParseCommandArgs(args)
	if err != nil {
		return err
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD:                 parsed.CWD,
		ChangeName:          parsed.ChangeName,
		IncludeInstructions: true,
	})
	if err != nil {
		return fmt.Errorf("resolve sdd status: %w", err)
	}
	if err := sddstatus.PrepareChangeInstanceConsent(status); err != nil {
		return fmt.Errorf("prepare sdd continuation consent: %w", err)
	}
	status, err = sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD:                 parsed.CWD,
		ChangeName:          parsed.ChangeName,
		IncludeInstructions: true,
	})
	if err != nil {
		return fmt.Errorf("resolve prepared sdd continuation status: %w", err)
	}

	if parsed.JSON {
		projected, projectionErr := sddstatus.ProjectStatusV2(status)
		if projectionErr != nil {
			return fmt.Errorf("project SDD status v2: %w", projectionErr)
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(projected)
	}

	_, err = fmt.Fprintln(stdout, sddstatus.RenderDispatcherMarkdown(status))
	return err
}
