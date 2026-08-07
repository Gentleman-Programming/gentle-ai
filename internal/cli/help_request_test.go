package cli

import (
	"bytes"
	"strings"
	"testing"
)

// An explicit --help is a request, not a failure. Every command must answer it
// with the usage the flag package derives from its own registered flags, and
// must report success so a shell that checks the exit code is not told the
// request failed.
func TestHelpRequestPrintsDerivedUsageAndSucceeds(t *testing.T) {
	for _, test := range []struct {
		name    string
		run     func(args []string, stdout *bytes.Buffer) error
		expects []string
	}{
		{
			name: "uninstall",
			run: func(args []string, stdout *bytes.Buffer) error {
				_, err := RunUninstall(args, stdout)
				return err
			},
			expects: []string{"agent", "component", "all", "yes"},
		},
		{
			name: "restore",
			run: func(args []string, stdout *bytes.Buffer) error {
				return RunRestore(args, stdout)
			},
			expects: []string{"list", "yes"},
		},
	} {
		for _, flagName := range []string{"--help", "-h"} {
			t.Run(test.name+" "+flagName, func(t *testing.T) {
				var stdout bytes.Buffer
				if err := test.run([]string{flagName}, &stdout); err != nil {
					t.Fatalf("%s %s returned %v, want success", test.name, flagName, err)
				}
				output := stdout.String()
				if strings.TrimSpace(output) == "" {
					t.Fatalf("%s %s printed nothing", test.name, flagName)
				}
				for _, want := range test.expects {
					if !strings.Contains(output, want) {
						t.Fatalf("%s %s usage omits %q:\n%s", test.name, flagName, want, output)
					}
				}
			})
		}
	}
}

// restore accepts a positional backup name before its flags, and flag.Parse
// stops at the first non-flag token. A help request that follows the positional
// must still be answered, and an unknown flag in that position must still fail
// instead of being silently swallowed into a no-op success.
func TestRestoreAnswersHelpAndRejectsUnknownFlagAfterPositional(t *testing.T) {
	for _, flagName := range []string{"--help", "-h"} {
		t.Run("help "+flagName, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := RunRestore([]string{"backup-001", flagName}, &stdout); err != nil {
				t.Fatalf("restore backup-001 %s returned %v, want success", flagName, err)
			}
			if !strings.Contains(stdout.String(), "list") || !strings.Contains(stdout.String(), "yes") {
				t.Fatalf("restore backup-001 %s printed no usage: %q", flagName, stdout.String())
			}
		})
	}

	var stdout bytes.Buffer
	err := RunRestore([]string{"backup-001", "--unknown"}, &stdout)
	if err == nil {
		t.Fatalf("restore backup-001 --unknown succeeded silently, output %q", stdout.String())
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("restore unknown-flag refusal does not name the flag: %v", err)
	}
}

// A genuine parse failure must stay a failure, and it must carry the same
// derived usage so the refusal names a continuation instead of only rejecting.
// This is parseCommandFlags' other half: flag.ErrHelp becomes a success with
// usage, everything else stays an error that now carries the same usage.
func TestUnknownFlagStillFailsAndCarriesDerivedUsage(t *testing.T) {
	var stdout bytes.Buffer
	_, err := RunUninstall([]string{"--nonexistent-flag"}, &stdout)
	if err == nil {
		t.Fatal("unknown flag was accepted")
	}
	message := err.Error()
	if !strings.Contains(message, "agent") || !strings.Contains(message, "--help") {
		t.Fatalf("unknown-flag refusal names no usage or continuation: %v", err)
	}
}
