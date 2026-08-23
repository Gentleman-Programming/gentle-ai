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

	commands := []string{"install", "uninstall", "sync", "sdd-status", "sdd-continue", "sdd-attempt", "sdd-verify-validate", "review start", "review capture-result", "review capture-correction-plan", "review capture-refuter", "review capture-validation", "review validate", "review status", "review repair", "review-start", "review-resume", "review-bundle-export", "review-bundle-import", "review-validate", "update", "upgrade", "restore", "version"}
	for _, cmd := range commands {
		if !strings.Contains(output, cmd) {
			t.Errorf("help output missing command %q", cmd)
		}
	}
}

// TestHelpAdvertisesNoRetiredWorkCommands keeps help and dispatch aligned: the
// retired remote control-plane commands no longer dispatch, so help must not
// advertise any of them.
func TestHelpAdvertisesNoRetiredWorkCommands(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()

	for _, cmd := range []string{
		"work-capabilities", "work-start", "work-route", "work-advance",
		"work-verification-decide", "work-reconcile", "work-status", "work-transition",
		"WORK CONTRACT COMPATIBILITY",
	} {
		if strings.Contains(output, cmd) {
			t.Errorf("help output still advertises retired command surface %q", cmd)
		}
	}
}

func TestHelpPresentsFlatReviewCommandsAsCompatibilityPaths(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()
	if !strings.Contains(output, "COMPATIBILITY COMMANDS\n  review-start") || !strings.Contains(output, "Read-only legacy v1 surface; rejects new v1 authority") {
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
		"the final capture closes and burns its review",
		"Read-only legacy v1 surface; rejects new v1 authority",
		"Read-only legacy v1 surface; rejects mutation",
		"Read shipped v1 authority without mutation",
		"Export a read-only legacy v1 chain transport",
		"Import a read-only legacy v1 transport",
		"--receipt <path> (--request <path> | --lineage <id> --gate <gate>)",
		"ordinary repository policy decides delivery",
		"compatibility inputs",
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
		"review finalize",
		"retry-final-verification",
		"capture-evidence",
	} {
		if strings.Contains(output, stale) {
			t.Fatalf("help output retains stale review contract %q:\n%s", stale, output)
		}
	}
}

func TestHelpDocumentsUserControlledReviewModeKillSwitch(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf, "v1.0.0-test")
	output := buf.String()
	for _, want := range []string{
		"review mode <enable|disable|status>",
		"--scope <global|clone>",
		"off wins",
		"status never mutates",
		"asks once per clone",
		"'not now' applies to that candidate only",
		"gentle-ai review mode disable",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing review mode documentation %q:\n%s", want, output)
		}
	}
	// The permanent disable stopped being a numbered answer, so the help must
	// not keep advertising it as one.
	for _, stale := range []string{"never ask again", "Never ask again"} {
		if strings.Contains(output, stale) {
			t.Fatalf("help output still offers a permanent disable as an answer %q:\n%s", stale, output)
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
