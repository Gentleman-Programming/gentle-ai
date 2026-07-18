package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpContainsAllCommands(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()

	commands := []string{"install", "uninstall", "sync", "sdd-status", "sdd-continue", "review start", "review finalize", "review validate", "review status", "review-start", "review-resume", "review-bundle-export", "review-bundle-import", "review-validate", "update", "upgrade", "restore", "version"}
	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("help output missing command %q", cmd)
		}
	}
	if strings.Count(output, "[--lineage <id>]") < 2 {
		t.Fatalf("help does not expose explicit lineage for both SDD commands:\n%s", output)
	}
}

func TestHelpPresentsFlatReviewCommandsAsCompatibilityPaths(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()
	if !strings.Contains(output, "COMPATIBILITY COMMANDS\n  review-start") || !strings.Contains(output, "Normal review path; ordinary authority is compact state plus receipt") {
		t.Fatalf("help does not separate facade from compatibility commands:\n%s", output)
	}
}

func TestHelpContainsVersion(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.2.3")
	if !strings.Contains(buf.String(), "v1.2.3") {
		t.Error("help output should contain the version string")
	}
}

func TestHelpDescribesCurrentReviewAuthorityAndCompatibilitySyntax(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()
	for _, want := range []string{
		"ordinary authority is compact state plus receipt",
		"Read-only legacy v1 surface; rejects new v1 authority",
		"Read-only legacy v1 surface; rejects mutation",
		"Read shipped v1 authority without mutation",
		"Export compact current-state transport or a legacy v1 chain transport",
		"receipt/request extras apply only to legacy v1 transport",
		"--receipt <path> (--request <path> | --lineage <id> --gate <gate>)",
		"native mode needs lineage/gate and derives authority",
		"optional compatibility or exceptional inputs",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q", want)
		}
	}
}

func TestHelpRejectsStaleMutableAndMandatoryReviewWording(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()
	for _, stale := range []string{
		"build a target and append to the review store",
		"Append a lifecycle step",
		"Export the validated full chain as a portable content-addressed bundle",
		"--bundle <path> --policy <path> --ledger <path> --evidence <path>",
		"Canonical empty ledger bytes",
	} {
		if strings.Contains(output, stale) {
			t.Fatalf("help output retains stale review contract %q:\n%s", stale, output)
		}
	}
}

func TestHelpCommandsHeadingIsAligned(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.2.3")
	if !strings.Contains(buf.String(), "\nCOMMANDS\n  install") {
		t.Fatalf("help output has inconsistent command indentation:\n%s", buf.String())
	}
}
