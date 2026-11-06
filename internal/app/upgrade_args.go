package app

import (
	"errors"
	"fmt"
	"strings"
)

// upgradeArgs is the structured, once-parsed representation of the `upgrade`
// command's argument list. RunArgs parses the raw CLI args into this value
// exactly once before any platform, system, self-update, update-check, backup,
// or execution effect, and runUpgrade consumes only these fields without
// reparsing. See issue #535.
type upgradeArgs struct {
	dryRun     bool
	noBackup   bool
	toolFilter []string
}

// errUnsupportedUpgradeArgument is the identifiable sentinel returned when an
// unsupported dash-prefixed argument appears before the standalone "--"
// delimiter. Callers can match it with errors.Is to surface the offending
// token deterministically.
var errUnsupportedUpgradeArgument = errors.New("unsupported upgrade argument")

// parseUpgradeArgs parses the upgrade command's argument list in a single
// pass, honouring the standalone "--" delimiter. Before the delimiter it
// recognises --dry-run/-n and --no-backup; --help and -h are intentionally
// NOT recognised here so the unsupported-option path rejects them (help
// output is deferred to the follow-up help-behavior change). Every other
// dash-prefixed token is rejected with an error wrapping
// errUnsupportedUpgradeArgument and naming the exact offending token. After
// the delimiter, every token is a positional tool filter (not validated).
func parseUpgradeArgs(args []string) (upgradeArgs, error) {
	var parsed upgradeArgs
	pastDelimiter := false

	for _, arg := range args {
		if pastDelimiter {
			parsed.toolFilter = append(parsed.toolFilter, arg)
			continue
		}

		// The standalone "--" ends option recognition. Everything after it
		// (including dash-prefixed values) becomes a positional filter.
		if arg == "--" {
			pastDelimiter = true
			continue
		}

		// Before the delimiter, only recognised options are allowed.
		switch arg {
		case "--dry-run", "-n":
			parsed.dryRun = true
		case "--no-backup":
			parsed.noBackup = true
		default:
			// Positional tokens (no leading dash) are tool filters; unknown
			// dash-prefixed options are rejected with the exact token so the
			// caller can report it without ambiguity.
			if strings.HasPrefix(arg, "-") {
				return upgradeArgs{}, fmt.Errorf("%w: %q", errUnsupportedUpgradeArgument, arg)
			}
			parsed.toolFilter = append(parsed.toolFilter, arg)
		}
	}

	return parsed, nil
}
