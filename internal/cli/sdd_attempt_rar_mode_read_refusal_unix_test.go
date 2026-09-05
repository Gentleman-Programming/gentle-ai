//go:build !windows

package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestRunSDDAttemptSettleIgnoresUnsafeRDDModeAuthority(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	const change = "unsafe-rar-mode"
	disableReviewForClone(t, repo)
	started, _ := runCompactSDDAttempt(t, compactAcquireArgs(repo, change, "unsafe-mode-acquire", 2))
	store, err := sddstatus.OpenRuntimeStore(context.Background(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	privateRARDir := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1")
	if err := os.Chmod(privateRARDir, 0o755); err != nil {
		t.Fatalf("make private RAR directory unsafe: %v", err)
	}
	defer os.Chmod(privateRARDir, 0o700)
	rarBefore := snapshotRuntimeAuthorityFiles(t, privateRARDir)
	completed, _ := runCompactSDDAttempt(t, compactSettleArgs(repo, change, started.Token, "unsafe-mode-settle", "passed"))
	if completed.State != "complete" {
		t.Fatalf("unsafe RDD metadata changed SDD settlement = %#v", completed)
	}
	if rarAfter := snapshotRuntimeAuthorityFiles(t, privateRARDir); !reflect.DeepEqual(rarBefore, rarAfter) {
		t.Fatalf("SDD settlement touched unsafe RDD authority\nbefore=%v\nafter=%v", rarBefore, rarAfter)
	}
	if status, err := store.Status(); err != nil || !status.Complete {
		t.Fatalf("settled runtime status = %#v err=%v", status, err)
	}
}

func TestUnsafeDisabledRARModeRefusesStatusAndValidationBeforeTheirReaders(t *testing.T) {
	for _, command := range []struct {
		name string
		run  func([]string, io.Writer) error
		args func(string) []string
	}{
		{name: "sdd-status", run: RunSDDStatus, args: func(repo string) []string { return []string{"--cwd", repo, "--json"} }},
		{name: "review-facade gate", run: RunReviewFacadeValidate, args: func(repo string) []string { return []string{"--cwd", repo, "--gate", "post-apply"} }},
	} {
		t.Run(command.name, func(t *testing.T) {
			reviewModeHome(t)
			repo := initReviewCLIRepo(t)
			disableReviewForClone(t, repo)
			privateRARDir := filepath.Join(repo, ".git", "gentle-ai", "review-mode", "rar-authority", "v1")
			if err := os.Chmod(privateRARDir, 0o755); err != nil {
				t.Fatalf("make private RAR directory unsafe: %v", err)
			}
			defer os.Chmod(privateRARDir, 0o700)
			before := snapshotRuntimeAuthorityFiles(t, privateRARDir)

			var output bytes.Buffer
			err := command.run(command.args(repo), &output)
			wantRepair := (&reviewModeUnsafePathError{Path: privateRARDir, Directory: true}).repairCommand()
			if err == nil || !strings.Contains(err.Error(), wantRepair) || output.Len() != 0 {
				t.Fatalf("unsafe mode %s result: error=%v output=%q, want actionable refusal", command.name, err, output.String())
			}
			if after := snapshotRuntimeAuthorityFiles(t, privateRARDir); !reflect.DeepEqual(before, after) {
				t.Fatalf("unsafe mode %s mutated RAR authority\nbefore=%v\nafter=%v", command.name, before, after)
			}
		})
	}
}

func TestReviewModeUnsafeFileRefusalRendersRunnableChmod600(t *testing.T) {
	root := t.TempDir()
	decoy := filepath.Join(root, "decoy")
	target := filepath.Join(root, "unsafe $(chmod 000 decoy) `chmod 000 decoy` $HOME 'quote'")
	for _, path := range []string{target, decoy} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	command := (&reviewModeUnsafePathError{Path: target}).repairCommand()
	for _, args := range [][]string{{"-n", "-c", command}, {"-c", command}} {
		shell := exec.Command("sh", args...)
		shell.Dir = root
		if output, err := shell.CombinedOutput(); err != nil {
			t.Fatalf("run printed repair %q: %v\n%s", command, err, output)
		}
	}
	for path, want := range map[string]os.FileMode{target: 0o600, decoy: 0o644} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("mode after printed repair for %q = %v, %v; want %04o", path, info.Mode(), err, want)
		}
	}
}
