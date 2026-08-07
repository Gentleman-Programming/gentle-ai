package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// helpRequestError carries the usage block the flag package derived from a
// command's own registered flags. It exists because several parse functions
// (ParseInstallFlags, ParseUninstallFlags) own no writer, so they cannot print
// usage themselves; they return this instead and the caller that does own a
// writer answers it through writeHelpRequest.
type helpRequestError struct {
	usage string
}

func (e *helpRequestError) Error() string {
	return e.usage
}

// parseCommandFlags parses args while preserving the usage the flag package
// generates from fs's registered flags, so no command has to restate its own
// flag documentation and none can drift from it.
//
// It draws one distinction the standard library leaves to the caller:
// flag.ErrHelp is a sentinel meaning "the operator asked for help", not a
// failure. Returning it verbatim renders an explicit request as an opaque
// error with a nonzero exit and no flag names, which is the defect this
// replaces. A genuine parse error stays an error and is wrapped together with
// the same derived usage, so the refusal names a continuation.
func parseCommandFlags(fs *flag.FlagSet, args []string) error {
	var usage bytes.Buffer
	fs.SetOutput(&usage)
	err := fs.Parse(args)
	if err == nil {
		return nil
	}
	text := derivedUsageText(&usage)
	if errors.Is(err, flag.ErrHelp) {
		return &helpRequestError{usage: text}
	}
	if text == "" {
		return err
	}
	return fmt.Errorf("%w — run `gentle-ai %s --help` for the supported flags:\n%s", err, fs.Name(), text)
}

// writeHelpRequest answers a help request by printing the derived usage and
// reporting success. Any other error passes through untouched, so callers can
// wrap this around an existing parse result without changing failure handling.
func writeHelpRequest(err error, stdout io.Writer) error {
	var request *helpRequestError
	if errors.As(err, &request) {
		_, _ = fmt.Fprintln(stdout, request.usage)
		return nil
	}
	return err
}

// derivedUsageText trims the flag package's duplicated error line so the usage
// block is not preceded by the same message its caller is about to report.
func derivedUsageText(usage *bytes.Buffer) string {
	text := usage.String()
	if index := strings.Index(text, "Usage of "); index >= 0 {
		text = text[index:]
	}
	return strings.TrimRight(text, "\n")
}
