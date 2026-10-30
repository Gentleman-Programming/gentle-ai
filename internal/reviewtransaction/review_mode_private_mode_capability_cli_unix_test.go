//go:build !windows

// This file drives the `gentle-ai review mode` CLI surface for the #5112
// filesystem-capability class from the same external package as
// review_mode_disable_guidance_cli_unix_test.go: the primitives that reproduce
// a mount ignoring the mode it is handed are unexported by design, so a test
// binary is the only place a security boundary's creation can be changed,
// while the refusal is still read exactly as an operator reads it.
package reviewtransaction_test

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// precreateWorldAccessibleSwitchRoot reproduces the field report's trigger:
// the review-mode switch root already exists (the operator or an interrupted
// run created it), and on this mount class every path reports 0777.
func precreateWorldAccessibleSwitchRoot(t *testing.T, repo string) {
	t.Helper()
	root := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
}

func capabilityRefusalChecks(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		t.Fatal("command must refuse on a mount that cannot represent private modes")
	}
	message := err.Error()
	if strings.Contains(message, "chmod 700") || strings.Contains(message, "chmod 600") {
		t.Fatalf("refusal suggests a chmod the mount cannot ever honor: %s", message)
	}
	for _, want := range []string{"cannot be made private", "metadata"} {
		if !strings.Contains(message, want) {
			t.Fatalf("refusal %q does not name the capability or a real continuation (%q)", message, want)
		}
	}
	return message
}

func TestReviewModeStatusOnIneffectivePrivateModeMountPrintsCapabilityRefusal(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	precreateWorldAccessibleSwitchRoot(t, repo)
	defer reviewtransaction.SetRARPrivateDirectoryPrimitivesForTest(
		nil,
		func(string, fs.FileMode) error { return nil },
	)()
	defer reviewtransaction.SetRARPrivateModeProbeForTest(func(string) bool { return true })()

	var output bytes.Buffer
	err := cli.RunReviewMode([]string{"status", "--cwd", repo}, &output)
	capabilityRefusalChecks(t, err)
}

func TestReviewModeDisableCloneOnIneffectivePrivateModeMountNamesTypedContinuation(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	precreateWorldAccessibleSwitchRoot(t, repo)
	defer reviewtransaction.SetRARPrivateDirectoryPrimitivesForTest(
		nil,
		func(string, fs.FileMode) error { return nil },
	)()
	defer reviewtransaction.SetRARPrivateModeProbeForTest(func(string) bool { return true })()

	// The deadlock the issue reports: disable --scope clone must fail with
	// the typed continuation, and it must fail the same way every time, so
	// the operator is never left guessing whether a retry corrupted state.
	var first bytes.Buffer
	firstErr := cli.RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone"}, &first)
	firstMessage := capabilityRefusalChecks(t, firstErr)
	var second bytes.Buffer
	secondErr := cli.RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone"}, &second)
	if secondErr == nil || secondErr.Error() != firstMessage {
		t.Fatalf("repeated refusal is not stable: first %v, then %v", firstErr, secondErr)
	}
}

// TestReviewModeDisableCloneStillRefusesRepairablyOnCapableFilesystem pins
// the native guarantee at the CLI surface: the same pre-existing wrong-mode
// directory on a filesystem with effective POSIX metadata keeps today's
// refusal, whose printed chmod repair is exactly what the operator-loop test
// above runs to get through. The capability refusal must never replace it.
func TestReviewModeDisableCloneStillRefusesRepairablyOnCapableFilesystem(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	precreateWorldAccessibleSwitchRoot(t, repo)
	defer reviewtransaction.SetRARPrivateDirectoryPrimitivesForTest(
		nil,
		func(string, fs.FileMode) error { return nil },
	)()

	var output bytes.Buffer
	err := cli.RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone"}, &output)
	if err == nil {
		t.Fatalf("review mode disable --scope clone must refuse while the switch root is world-accessible:\n%s", output.String())
	}
	var ineffective *reviewtransaction.PrivateModeIneffectiveError
	if errors.As(err, &ineffective) {
		t.Fatalf("capable filesystem produced a capability refusal: %v", err)
	}
	repair := repairCommandFrom(t, err.Error())
	if !strings.HasPrefix(repair, "chmod 700 ") {
		t.Fatalf("refusal printed %q, want the runnable chmod 700 repair: %v", repair, err)
	}
}

// TestReviewModeStatusStillRepairsOnCapableFilesystem pins the native read
// path: status over a repairable wrong-mode leaf repairs it in the walk
// (#3416) and resolves, exactly as before this change.
func TestReviewModeStatusStillRepairsOnCapableFilesystem(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	precreateWorldAccessibleSwitchRoot(t, repo)

	var output bytes.Buffer
	if err := cli.RunReviewMode([]string{"status", "--cwd", repo}, &output); err != nil {
		t.Fatalf("review mode status = %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "clone-local: unset") {
		t.Fatalf("status did not resolve the repaired clone-local source:\n%s", output.String())
	}
	info, statErr := os.Lstat(filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority"))
	if statErr != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("switch root leaf = %v (%v), want it repaired to 0700 by the read walk", info, statErr)
	}
}
