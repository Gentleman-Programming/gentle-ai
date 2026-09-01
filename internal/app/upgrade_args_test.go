package app

import (
	"errors"
	"strings"
	"testing"
)

// TestParseUpgradeArgs is the RED table-driven matrix for the single-pass,
// delimiter-aware upgrade argument parser introduced by issue #535.
//
// PR 1 covers supported flags, filters, delimiter handling, and early rejection.
// The follow-up help-behavior change owns help selection and printUpgradeHelp.
func TestParseUpgradeArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantArgs  upgradeArgs
		wantErr   bool
		wantToken string // substring the error must contain when wantErr
	}{
		{
			name:     "empty args parses to zero state",
			args:     nil,
			wantArgs: upgradeArgs{},
		},
		{
			name:     "dry-run long flag sets dry-run only",
			args:     []string{"--dry-run"},
			wantArgs: upgradeArgs{dryRun: true},
		},
		{
			name:     "dry-run short alias -n sets dry-run only",
			args:     []string{"-n"},
			wantArgs: upgradeArgs{dryRun: true},
		},
		{
			name:     "no-backup flag sets noBackup only",
			args:     []string{"--no-backup"},
			wantArgs: upgradeArgs{noBackup: true},
		},
		{
			name:     "all supported flags set together",
			args:     []string{"--dry-run", "-n", "--no-backup"},
			wantArgs: upgradeArgs{dryRun: true, noBackup: true},
		},
		{
			name:     "positional filters are preserved in order",
			args:     []string{"tool-a", "tool-b"},
			wantArgs: upgradeArgs{toolFilter: []string{"tool-a", "tool-b"}},
		},
		{
			name:     "supported flags and filters preserved together (spec scenario)",
			args:     []string{"--dry-run", "-n", "--no-backup", "tool-a", "tool-b"},
			wantArgs: upgradeArgs{dryRun: true, noBackup: true, toolFilter: []string{"tool-a", "tool-b"}},
		},
		{
			name: "delimiter preserves trailing dash-prefixed filters", args: []string{"--dry-run", "--", "--not-a-flag"},
			wantArgs: upgradeArgs{dryRun: true, toolFilter: []string{"--not-a-flag"}},
		},
		{
			name:      "unsupported long option rejected with exact token",
			args:      []string{"--verbose"},
			wantErr:   true,
			wantToken: `unsupported upgrade argument: "--verbose"`,
		},
		{
			name:      "unsupported short option rejected with exact token",
			args:      []string{"-x"},
			wantErr:   true,
			wantToken: `unsupported upgrade argument: "-x"`,
		},
		{
			name:      "unsupported option rejected before any effect (first unsupported wins)",
			args:      []string{"--dry-run", "--bogus", "tool-a"},
			wantErr:   true,
			wantToken: `unsupported upgrade argument: "--bogus"`,
		},
		{
			name:      "unsupported option after positional filter still rejected",
			args:      []string{"tool-a", "--no-such-flag"},
			wantErr:   true,
			wantToken: `unsupported upgrade argument: "--no-such-flag"`,
		},
		{
			// #535 remediation: --help/-h rejected as unsupported until PR2.
			name: "help long flag rejected as unsupported with exact token", args: []string{"--help"},
			wantErr: true, wantToken: `unsupported upgrade argument: "--help"`,
		},
		{
			name: "help short flag rejected as unsupported with exact token", args: []string{"-h"},
			wantErr: true, wantToken: `unsupported upgrade argument: "-h"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUpgradeArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseUpgradeArgs(%v) error = nil, want error containing %q", tt.args, tt.wantToken)
				}
				if !strings.Contains(err.Error(), tt.wantToken) {
					t.Fatalf("parseUpgradeArgs(%v) error = %v, want error containing %q", tt.args, err, tt.wantToken)
				}
				// Error must be identifiable: it must wrap or be the sentinel so
				// callers can match the offending token deterministically.
				if !errors.Is(err, errUnsupportedUpgradeArgument) {
					t.Fatalf("parseUpgradeArgs(%v) error is not identifiable: %v (want errors.Is errUnsupportedUpgradeArgument)", tt.args, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUpgradeArgs(%v) error = %v, want nil", tt.args, err)
			}
			if !equalUpgradeArgs(got, tt.wantArgs) {
				t.Fatalf("parseUpgradeArgs(%v) = %+v, want %+v", tt.args, got, tt.wantArgs)
			}
		})
	}
}

// equalUpgradeArgs compares two upgradeArgs field-by-field, treating nil and
// empty toolFilter slices as equivalent so the table stays readable.
func equalUpgradeArgs(a, b upgradeArgs) bool {
	if a.dryRun != b.dryRun || a.noBackup != b.noBackup {
		return false
	}
	if len(a.toolFilter) == 0 && len(b.toolFilter) == 0 {
		return true
	}
	if len(a.toolFilter) != len(b.toolFilter) {
		return false
	}
	for i := range a.toolFilter {
		if a.toolFilter[i] != b.toolFilter[i] {
			return false
		}
	}
	return true
}
