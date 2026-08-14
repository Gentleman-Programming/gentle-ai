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

func TestRunSDDAttemptSettleRefusesUnsafeDisabledRARModeWithoutMutation(t *testing.T) {
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
	runtimeBefore := snapshotRuntimeAuthorityFiles(t, store.Dir)
	rarBefore := snapshotRuntimeAuthorityFiles(t, privateRARDir)
	settleArgs := compactSettleArgs(repo, change, started.Token, "unsafe-mode-settle", "passed")
	var output bytes.Buffer
	err = RunSDDAttempt(settleArgs, &output)
	if err == nil {
		t.Fatalf("unsafe disabled mode settle error = nil, output=%s", output.String())
	}
	wantRepair := (&reviewModeUnsafePathError{Path: privateRARDir, Directory: true}).repairCommand()
	if !strings.Contains(err.Error(), wantRepair) || !strings.Contains(err.Error(), "rerun the original command") {
		t.Fatalf("unsafe disabled mode refusal = %q, want actionable %q", err, wantRepair)
	}
	if strings.Contains(err.Error(), "invalid_continuation") || output.Len() != 0 {
		t.Fatalf("unsafe disabled mode was misclassified: error=%q output=%q", err, output.String())
	}
	if runtimeAfter := snapshotRuntimeAuthorityFiles(t, store.Dir); !reflect.DeepEqual(runtimeBefore, runtimeAfter) {
		t.Fatalf("unsafe disabled mode settle mutated runtime authority\nbefore=%v\nafter=%v", runtimeBefore, runtimeAfter)
	}
	if rarAfter := snapshotRuntimeAuthorityFiles(t, privateRARDir); !reflect.DeepEqual(rarBefore, rarAfter) {
		t.Fatalf("unsafe disabled mode settle mutated RAR authority\nbefore=%v\nafter=%v", rarBefore, rarAfter)
	}
	if output, repairErr := exec.Command("sh", "-c", wantRepair).CombinedOutput(); repairErr != nil {
		t.Fatalf("execute printed repair %q: %v\n%s", wantRepair, repairErr, output)
	}
	completed, _ := runCompactSDDAttempt(t, settleArgs)
	if completed.State != "complete" {
		t.Fatalf("replayed settle after repair = %#v, want complete", completed)
	}
}

func TestUnsafeDisabledRARModeRefusesStatusAndValidationBeforeTheirReaders(t *testing.T) {
	for _, command := range []struct {
		name string
		run  func([]string, io.Writer) error
		args func(string) []string
	}{
		{name: "sdd-status", run: RunSDDStatus, args: func(repo string) []string { return []string{"--cwd", repo, "--json"} }},
		{name: "review-validate", run: RunReviewValidate, args: func(repo string) []string {
			return []string{"--cwd", repo, "--receipt", filepath.Join(repo, "missing.json")}
		}},
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
