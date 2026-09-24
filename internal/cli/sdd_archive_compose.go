package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/sddstatus"
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
	supersede := flags.Bool("supersede", false, "Marca los requisitos en REMOVED con [SUPERSEDED / DEPRECADO] en lugar de borrarlos físicamente")
	supersededReqs := flags.String("superseded-requirements", "", "Lista de requisitos adicionales separados por comas para marcar como [SUPERSEDED / DEPRECADO]")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-archive-compose argument %q; rerun `gentle-ai sdd-archive-compose --canonical <path> --delta <path> [--output <path|->]` with no positional arguments", flags.Arg(0))
	}
	if strings.TrimSpace(*canonicalPath) == "" {
		return errors.New("sdd-archive-compose requires --canonical; rerun `gentle-ai sdd-archive-compose --canonical openspec/specs/<domain>/spec.md --delta openspec/changes/<change>/specs/<domain>/spec.md`")
	}
	if strings.TrimSpace(*deltaPath) == "" {
		return errors.New("sdd-archive-compose requires --delta; rerun `gentle-ai sdd-archive-compose --canonical openspec/specs/<domain>/spec.md --delta openspec/changes/<change>/specs/<domain>/spec.md`")
	}

	canonicalBytes, err := os.ReadFile(*canonicalPath)
	if err != nil {
		return fmt.Errorf("read canonical spec: %w", err)
	}
	deltaBytes, err := os.ReadFile(*deltaPath)
	if err != nil {
		return fmt.Errorf("read delta spec: %w", err)
	}

	var explicit []string
	if strings.TrimSpace(*supersededReqs) != "" {
		for _, s := range strings.Split(*supersededReqs, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				explicit = append(explicit, trimmed)
			}
		}
	}

	composed, err := sddstatus.ComposeOpenSpecCanonicalSpecWithOptions(string(canonicalBytes), string(deltaBytes), sddstatus.ComposeOptions{
		SupersedeRemoved:   *supersede,
		ExplicitSuperseded: explicit,
	})
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
	_, _ = fmt.Fprintln(stdout, "Usage: gentle-ai sdd-archive-compose --canonical <path> --delta <path> [--output <path|->] [--supersede] [--superseded-requirements <req1,req2>]")
	_, _ = fmt.Fprintln(stdout, "Merges an OpenSpec delta spec into a canonical spec ("+sddstatus.OpenSpecComposeSchema+").")
	_, _ = fmt.Fprintln(stdout, "Opciones de Poda Progresiva (ODD-4.1):")
	_, _ = fmt.Fprintln(stdout, "  --supersede                 Marca los requisitos de REMOVED con [SUPERSEDED / DEPRECADO] en vez de eliminarlos.")
	_, _ = fmt.Fprintln(stdout, "  --superseded-requirements   Lista de requisitos adicionales separados por coma a marcar como superados.")
	_, _ = fmt.Fprintln(stdout, "On an unapplied delta, writes nothing and fails naming the section and requirement.")
	return nil
}
