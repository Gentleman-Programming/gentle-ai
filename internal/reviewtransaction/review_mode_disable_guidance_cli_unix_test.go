//go:build !windows

// This file drives the `gentle-ai review mode disable --scope clone` CLI
// surface, but it lives beside the package it exercises rather than in
// internal/cli. The filesystem primitives that reproduce a mount ignoring the
// mode it is handed are unexported by design, and reaching them from another
// package would mean exposing a way to change how a security boundary is
// created in the production build. An external test package keeps the seam
// inside the test binary and still lets the refusal be read exactly as an
// operator reads it.
package reviewtransaction_test

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/cli"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewModeDisableCloneSucceedsWhenTheMkdirModeDoesNotStick(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	defer reviewtransaction.SetRARPrivateDirectoryPrimitivesForTest(ignoreRequestedDirectoryMode, nil)()

	var output bytes.Buffer
	if err := cli.RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output); err != nil {
		t.Fatalf("review mode disable --scope clone = %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), `"clone_local": "off"`) {
		t.Fatalf("review mode disable did not persist the clone-local override:\n%s", output.String())
	}

	var status bytes.Buffer
	if err := cli.RunReviewMode([]string{"status", "--cwd", repo, "--json"}, &status); err != nil {
		t.Fatalf("review mode status = %v\n%s", err, status.String())
	}
	if !strings.Contains(status.String(), `"effective": "off"`) {
		t.Fatalf("review mode did not stay off after disable:\n%s", status.String())
	}
}

func TestReviewModeDisableCloneRefusalPrintsARunnableRepair(t *testing.T) {
	repo := initReviewModeGuidanceRepo(t)
	defer reviewtransaction.SetRARPrivateDirectoryPrimitivesForTest(
		ignoreRequestedDirectoryMode,
		func(string, fs.FileMode) error { return nil },
	)()

	// The operator loop the product documents: run the command, run the repair
	// it prints, run the command again. It has to terminate. Before #3285 was
	// fixed it could not, because the repair named a directory the failing
	// command had just deleted.
	const attempts = 12
	for attempt := 1; ; attempt++ {
		var output bytes.Buffer
		err := cli.RunReviewMode([]string{"disable", "--cwd", repo, "--scope", "clone", "--json"}, &output)
		if err == nil {
			if !strings.Contains(output.String(), `"clone_local": "off"`) {
				t.Fatalf("review mode disable reported success without persisting the override:\n%s", output.String())
			}
			return
		}
		if attempt == attempts {
			t.Fatalf("the printed repair never let review mode disable through after %d attempts; last refusal: %v", attempts, err)
		}
		repair := repairCommandFrom(t, err.Error())
		if !strings.HasPrefix(repair, "chmod 700 ") {
			t.Fatalf("refusal printed %q, want a chmod 700 repair: %v", repair, err)
		}
		// The field symptom of #3285 is this exact command failing with
		// "chmod: cannot access '...': No such file or directory".
		if combined, runErr := exec.Command("sh", "-c", repair).CombinedOutput(); runErr != nil {
			t.Fatalf("printed repair %q is not runnable: %v\n%s\nrefusal: %v", repair, runErr, combined, err)
		}
	}
}

// ignoreRequestedDirectoryMode reproduces WSL DrvFS, exFAT, and SMB mounts
// without POSIX extensions: mkdir(2) accepts the mode and then ignores it.
func ignoreRequestedDirectoryMode(name string, _ fs.FileMode) error {
	return os.Mkdir(name, 0o755)
}

func repairCommandFrom(t *testing.T, message string) string {
	t.Helper()
	start := strings.Index(message, "`")
	if start < 0 {
		t.Fatalf("refusal %q prints no repair command", message)
	}
	rest := message[start+1:]
	end := strings.Index(rest, "`")
	if end < 0 {
		t.Fatalf("refusal %q prints an unterminated repair command", message)
	}
	return rest[:end]
}

func initReviewModeGuidanceRepo(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, gitErr := command.CombinedOutput(); gitErr != nil {
			t.Fatalf("git %v: %v: %s", args, gitErr, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "tracked.txt"}, {"commit", "-qm", "base"}} {
		command := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, gitErr := command.CombinedOutput(); gitErr != nil {
			t.Fatalf("git %v: %v: %s", args, gitErr, output)
		}
	}
	return repo
}
