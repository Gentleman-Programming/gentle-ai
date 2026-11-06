//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// This is an offline failure qualification, not an installation or rollback.
// Original probes, actions, pins and process adapters remain unchanged.
func TestUserNativeInstallerOfflineEffects(t *testing.T) {
	if testing.Short() {
		t.Skip("requires authentic source/Node/held PNPM in the bounded offline Guest")
	}
	if os.Getuid() != 1000 {
		t.Fatal("native PNPM qualification requires Guest UID1000")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	setup, stopSetup := context.WithTimeout(context.Background(), 60*time.Second)
	defer stopSetup()
	workspace, err := os.MkdirTemp("/tmp", "native-pnpm-effects-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	// Retain any failed/inconclusive workspace, including observer failures.
	t.Log("owned offline effects workspace:", workspace)
	root := filepath.Join(workspace, "worker")
	if err := userPrepareInstallStage(setup, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("/native-source.zip")
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := userMainDescriptor("73960fcf003b37e612a7b18960d6acf75f33a517", "feed2eb9961c6ab3b3a3d55ea314bfca881a5d901c44f97493231d72e0326453")
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageMainArchive(setup, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(setup, root, artifact); err != nil {
		t.Fatal(err)
	}
	w, err := userNativeRuntimeFromSnapshot(setup, root, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(w.node), 0700); err != nil {
		t.Fatal(err)
	}
	cp := exec.CommandContext(setup, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", w.node)
	if _, err := userOwnedRun(setup, cp, stopSetup); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(w.node, 0700); err != nil {
		t.Fatal(err)
	}
	tools := filepath.Join(root, "runtime", "native-bootstrap")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	seed := exec.CommandContext(setup, w.node, "--liftoff-only", "--input-type=module", "-e", nativeBootstrapFixture, w.source, tools)
	seed.Dir = filepath.Join(root, "project")
	seed.Env = w.nativeEnvironment()
	// The existing fixture supplies held archive bytes to original bootstrap;
	// it never replaces hashing, the shim, probes, or the installation runner.
	if _, err := userOwnedRun(setup, seed, stopSetup); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "project", "preserved")
	if err := os.WriteFile(marker, []byte("private project marker"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := w.Prepare(setup, true)
	if err != nil {
		t.Fatal(err)
	}
	var prepared struct {
		Actions []struct {
			ID string `json:"id"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(plan.JSON(), &prepared); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(prepared.Actions))
	for _, action := range prepared.Actions {
		ids = append(ids, action.ID)
	}
	// The original multi-action plan need not set ready=true. Use its actual
	// runner verdict, never a rewritten plan or a synthetic readiness flag.
	if !slices.Contains(ids, "persist-node") || !slices.Contains(ids, "persist-package-managers") {
		t.Fatalf("original persistence plan unavailable: %s", plan.JSON())
	}
	scopes := []string{"pnpm", "home", "config", "state", "agent", "project", "bin", "runtime/native-bootstrap"}
	before := make(map[string]string, len(scopes))
	for _, scope := range scopes {
		before[scope], err = userNativeEffectStamp(root, scope)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()
	type returned struct {
		result NativeInstallerResult
		err    error
	}
	done := make(chan returned, 1)
	completed := make(chan struct{})
	go func() {
		result, _, err := w.RunWithLocation(ctx, plan, true)
		done <- returned{result, err}
		close(completed)
	}()
	settled := false
	defer func() {
		cancel()
		if !settled {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("native run did not settle; preserve workspace")
			}
		}
	}()
	pid, fd, err := userObserveNativeInvocation(ctx, w, completed)
	if err != nil {
		t.Fatal("native observer unavailable; not behavioral evidence:", err)
	}
	defer unix.Close(fd)
	var outcome returned
	select {
	case outcome = <-done:
		settled = true
	case <-ctx.Done():
		select {
		case outcome = <-done:
			settled = true
		case <-time.After(3 * time.Second):
			t.Fatal("native run settlement unavailable; preserve workspace")
		}
	}
	// Even a failed verdict/classification must leave settlement and independent
	// source/input readbacks visible. Never turn a timeout into a failure PASS.
	t.Logf("original runner result: %s; process error: %v", outcome.result.Details, outcome.err)
	var native struct {
		Outcome    string   `json:"outcome"`
		FailedStep string   `json:"failedStep"`
		Completed  []string `json:"completed"`
	}
	if outcome.err != nil || json.Unmarshal(outcome.result.Details, &native) != nil || native.Outcome != "failed" || native.FailedStep != "persist-node" {
		t.Errorf("offline original runner did not return failed/persist-node: %+v %v", native, outcome.err)
	}
	if !slices.Contains(native.Completed, "check-global-bin") || !slices.Contains(native.Completed, "check-existing-stack") || slices.Contains(native.Completed, "persist-node") || slices.Contains(native.Completed, "install-global") {
		t.Errorf("original pre-mutation gates or incomplete action differ: %v", native.Completed)
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if n, err := unix.Poll(poll, 0); err != nil || n != 1 || poll[0].Revents&unix.POLLIN == 0 {
		t.Fatal("kernel has not confirmed owned Node exit; preserve workspace", err)
	}
	if err := userNativeGroupAbsent(pid); err != nil {
		t.Fatal("owned process group settlement differs; preserve workspace", err)
	}
	readback, stopReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopReadback()
	if err := userCompareMainSource(readback, root, artifact); err != nil {
		t.Fatal("source changed after original runner:", err)
	}
	if _, err := privateNativeFile(readback, w.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal("authenticated Node changed after original runner:", err)
	}
	changed := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		after, err := userNativeEffectStamp(root, scope)
		if err != nil {
			t.Fatal("private effect readback unavailable:", scope, err)
		}
		if before[scope] != after {
			changed = append(changed, scope)
			// The full stamp covers each scope; this bounded path listing is only
			// diagnostic recognition, not another integrity or ownership gate.
			shown := 0
			err := filepath.WalkDir(filepath.Join(root, scope), func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if shown == 32 {
					return filepath.SkipAll
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				t.Logf("private effect entry: %s mode=%s size=%d", rel, info.Mode(), info.Size())
				shown++
				return nil
			})
			if err != nil {
				t.Fatal("effect listing unavailable:", err)
			}
		}
	}
	t.Logf("private scopes changed after original runner: %v", changed)
	if slices.Contains(changed, "project") || slices.Contains(changed, "runtime/native-bootstrap") {
		t.Error("private project or bootstrap tooling changed")
	}
	if got, err := os.ReadFile(marker); err != nil || !bytes.Equal(got, []byte("private project marker")) {
		t.Error("private project marker changed", err)
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(workspace, identity); err != nil {
		t.Fatal("owned evidence cleanup unavailable:", err)
	}
	t.Logf("NATIVE_OFFLINE_EFFECTS_READBACK original_runner=true failed_step=persist-node changed_scopes=%v source_compared=true node_reauthenticated=true bootstrap_tools_unchanged=true private_marker_unchanged=true group_empty=true cleanup=true installed_acceptance=false", changed)
}

func userNativeEffectStamp(root, scope string) (string, error) {
	path := filepath.Join(root, scope)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return "absent", nil
	} else if err != nil {
		return "", err
	}
	return userTreeStamp(path)
}
