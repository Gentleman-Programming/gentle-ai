package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// RunSDDArchiveCompose composes an OpenSpec canonical spec with a delta spec
// (#4119). It replaces the sdd-archive skill's model-driven Read/Edit merge
// for the "main spec exists" case: on success the composed bytes go to
// --output (default stdout); on an unapplied delta, nothing is written and
// the error names the offending section and requirement.
func RunSDDArchiveCompose(args []string, stdout io.Writer) error {
	return runSDDArchiveCompose(args, stdout)
}

func runSDDArchiveCompose(args []string, stdout io.Writer) error {
	if hasSDDArchiveComposeHelp(args) {
		return renderSDDArchiveComposeHelp(stdout)
	}
	flags := flag.NewFlagSet("sdd-archive-compose", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	canonicalPath := flags.String("canonical", "", "Path to the existing canonical openspec/specs/<domain>/spec.md")
	deltaPath := flags.String("delta", "", "Path to the change's delta openspec/changes/<change>/specs/<domain>/spec.md")
	outputPath := flags.String("output", "-", "Where to write the composed canonical spec; use - for stdout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-archive-compose argument %q; rerun with only --canonical <path> --delta <path> [--output <path|->]", flags.Arg(0))
	}
	if strings.TrimSpace(*canonicalPath) == "" {
		return errors.New("sdd-archive-compose requires --canonical <path to openspec/specs/<domain>/spec.md>; rerun with it")
	}
	if strings.TrimSpace(*deltaPath) == "" {
		return errors.New("sdd-archive-compose requires --delta <path to openspec/changes/<change>/specs/<domain>/spec.md>; rerun with it")
	}

	canonicalBytes, err := os.ReadFile(*canonicalPath)
	if err != nil {
		return fmt.Errorf("read canonical spec: %w", err)
	}
	deltaBytes, err := os.ReadFile(*deltaPath)
	if err != nil {
		return fmt.Errorf("read delta spec: %w", err)
	}

	composed, err := sddstatus.ComposeOpenSpecCanonicalSpec(string(canonicalBytes), string(deltaBytes))
	if err != nil {
		return err
	}

	if *outputPath == "-" || strings.TrimSpace(*outputPath) == "" {
		_, err := io.WriteString(stdout, composed)
		return err
	}
	if err := os.WriteFile(*outputPath, []byte(composed), 0o600); err != nil {
		return fmt.Errorf("write composed canonical spec: %w", err)
	}
	return nil
}

func hasSDDArchiveComposeHelp(args []string) bool {
	for _, argument := range args {
		if argument == "--help" || argument == "-h" {
			return true
		}
	}
	return false
}

func renderSDDArchiveComposeHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-archive-compose --canonical <path> --delta <path> [--output <path|->]")
	_, _ = fmt.Fprintln(stdout, "Merges an OpenSpec delta spec into a canonical spec ("+sddstatus.OpenSpecComposeSchema+").")
	_, _ = fmt.Fprintln(stdout, "On an unapplied delta, writes nothing and fails naming the section and requirement.")
	return nil
}
