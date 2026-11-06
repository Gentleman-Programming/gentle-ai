//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Cancel only with a genuine original persistence child alive and observed
// PNPM directory effects. This does not qualify package installation or rollback.
func TestUserNativeInstallerPNPMMutationCancellation(t *testing.T) {
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
	workspace, err := os.MkdirTemp("/tmp", "native-pnpm-cancel-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	// Retain any failed/inconclusive workspace, including observer failures.
	t.Log("owned PNPM cancellation workspace:", workspace)
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

	before, err := userNativeEffectStamp(root, "pnpm")
	if err != nil {
		t.Fatal(err)
	}
	toolStamp, err := userTreeStamp(tools)
	if err != nil {
		t.Fatal(err)
	}
	projectStamp, err := userTreeStamp(filepath.Join(root, "project"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	done := make(chan error, 1)
	completed := make(chan struct{})
	go func() {
		_, _, err := w.RunWithLocation(ctx, plan, true)
		done <- err
		close(completed)
	}()
	defer func() {
		cancel()
		select {
		case <-completed:
		case <-time.After(3 * time.Second):
			t.Error("PNPM cancellation did not return; retain workspace")
		}
	}()
	parent, parentFD, err := userObserveNativeInvocation(ctx, w, completed)
	if err != nil {
		t.Fatalf("native parent observer inconclusive: %v", err)
	}
	defer unix.Close(parentFD)
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", parent))
	if err != nil || exe != w.node {
		t.Fatalf("native parent executable association differs: %v", err)
	}
	if err := userPNPMInvocationLive(ctx, completed, parentFD); err != nil {
		t.Fatal(err)
	}
	// Require the actual original persist-node argv, parent and process group;
	// unrelated PNPM version/probe children cannot satisfy this observation.
	child, childFD, err := userObservePNPMPersistence(ctx, completed, w, parent, parentFD, tools)
	if err != nil {
		t.Fatalf("PNPM persistence observer inconclusive: %v", err)
	}
	defer unix.Close(childFD)
	// Wait for actual directory effects, not a timer or presumed mutation.
	for !userPNPMDirectoryWitness(root) {
		for _, fd := range []int{parentFD, childFD} {
			if err := userPNPMInvocationLive(ctx, completed, fd); err != nil {
				t.Fatalf("PNPM effect observation inconclusive: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal("PNPM directory mutation not observed before context expiry")
		case <-completed:
			t.Fatal("PNPM directory mutation not observed before runner completion")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := userHoldNativeInvocation(ctx, completed, child, childFD); err != nil {
		t.Fatalf("PNPM stopped-state observer inconclusive: %v", err)
	}
	if err := userPNPMInvocationLive(ctx, completed, parentFD); err != nil {
		t.Fatalf("native parent association lost: %v", err)
	}
	// PNPM pre-gates may also create these directories. Do not attribute every
	// write to persist-node, or infer a successfully installed artifact.
	mutated, err := userNativeEffectStamp(root, "pnpm")
	if err != nil || mutated == before {
		t.Fatalf("real PNPM directory mutation unproved: changed=%t err=%v", mutated != before, err)
	}
	for _, relative := range []string{"pnpm/global/v11", "pnpm/store/v11/files/00"} {
		if _, err := privateDirectory(filepath.Join(root, relative)); err != nil {
			t.Fatalf("PNPM physical directory witness missing: %s err=%v", relative, err)
		}
	}
	if err := userPNPMInvocationLive(ctx, completed, childFD); err != nil {
		t.Fatalf("PNPM child association lost before cancellation: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		var private *PrivateRuntimeError
		if !errors.As(err, &private) || private.Kind != "canceled" || !errors.Is(err, context.Canceled) {
			t.Errorf("PNPM after-mutation cancellation differs: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("PNPM cancellation settlement inconclusive; retain workspace")
	}
	// Parent return need not coincide with the child's kernel exit notification.
	// Wait only on the two already-owned pidfds; do not add kill/reap behavior.
	for _, fd := range []int{parentFD, childFD} {
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(poll, 3000); err != nil || poll[0].Revents&unix.POLLIN == 0 {
			t.Fatalf("owned pidfd exit unproved: events=%v err=%v", poll, err)
		}
	}
	// Exit notification does not prove reaping. Still require the group empty,
	// with bounded read-only observation; never exclude zombies as a success.
	groupLimit := time.NewTimer(3 * time.Second)
	defer groupLimit.Stop()
	for {
		err := userNativeGroupAbsent(parent)
		if err == nil {
			break
		}
		select {
		case <-groupLimit.C:
			// Numeric /proc status is diagnostic only, not identity authority.
			if status, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/status", child)); readErr == nil {
				for _, line := range strings.Split(string(status), "\n") {
					if strings.HasPrefix(line, "State:") || strings.HasPrefix(line, "PPid:") {
						t.Log("unsettled child diagnostic:", line)
					}
				}
			}
			t.Fatal(err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	readback, stopReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopReadback()
	if err := userCompareMainSource(readback, root, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(readback, w.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal(err)
	}
	if after, err := userTreeStamp(tools); err != nil || after != toolStamp {
		t.Fatalf("bootstrap tooling changed: %v", err)
	}
	if after, err := userTreeStamp(filepath.Join(root, "project")); err != nil || after != projectStamp {
		t.Fatalf("private project changed: %v", err)
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "private project marker" {
		t.Fatalf("private marker changed: %v", err)
	}
	if _, err := userNativeEffectStamp(root, "pnpm"); err != nil {
		t.Fatalf("post-cancel PNPM effect readback failed: %v", err)
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(workspace, identity); err != nil {
		t.Fatal(err)
	}
	t.Log("NATIVE_PNPM_MUTATION_CANCEL_READBACK original_persistence_child=true directory_mutation_before_cancel=true stopped_child=true both_pidfds_exit=true group_empty=true source_compared=true node_reauthenticated=true bootstrap_tools_unchanged=true private_marker_unchanged=true cleanup=true installed_acceptance=false")
}

// pidfd plus a second exact association read prevents numeric PID reuse from
// retargeting the owned stop. No new kill/reap or detached-child policy is added.
func userObservePNPMPersistence(ctx context.Context, completed <-chan struct{}, w *NativeInstallerRuntime, parent, parentFD int, tools string) (int, int, error) {
	want := []byte(strings.Join([]string{w.node, filepath.Join(tools, "pnpm", "package", "bin", "pnpm.mjs"), "runtime", "set", "node", "24.21.0", "-g", ""}, "\x00"))
	match := func(pid int) bool {
		proc := fmt.Sprintf("/proc/%d", pid)
		cmd, err := os.ReadFile(proc + "/cmdline")
		if err != nil || !bytes.Equal(cmd, want) {
			return false
		}
		exe, err := os.Readlink(proc + "/exe")
		if err != nil || exe != w.node {
			return false
		}
		cwd, err := os.Readlink(proc + "/cwd")
		if err != nil || cwd != filepath.Join(w.root, "project") {
			return false
		}
		status, err := os.ReadFile(proc + "/status")
		if err != nil || !strings.Contains(string(status), fmt.Sprintf("\nPPid:\t%d\n", parent)) {
			return false
		}
		group, err := unix.Getpgid(pid)
		return err == nil && group == parent
	}
	for {
		if err := userPNPMInvocationLive(ctx, completed, parentFD); err != nil {
			return 0, -1, err
		}
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return 0, -1, err
		}
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil || !match(pid) {
				continue
			}
			fd, err := unix.PidfdOpen(pid, 0)
			if err != nil {
				return 0, -1, err
			}
			if !match(pid) {
				unix.Close(fd)
				return 0, -1, errors.New("PNPM invocation association changed")
			}
			if err := userPNPMInvocationLive(ctx, completed, fd); err != nil {
				unix.Close(fd)
				return 0, -1, err
			}
			return pid, fd, nil
		}
		select {
		case <-ctx.Done():
			return 0, -1, ctx.Err()
		case <-completed:
			return 0, -1, errors.New("native runner exited before genuine PNPM child observation")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func userPNPMInvocationLive(ctx context.Context, completed <-chan struct{}, fd int) error {
	if err := userNativeInvocationActive(ctx, completed); err != nil {
		return err
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if n, err := unix.Poll(poll, 0); err != nil || n != 0 {
		return errors.Join(err, errors.New("owned invocation pidfd no longer live"))
	}
	return userNativeInvocationActive(ctx, completed)
}

func userPNPMDirectoryWitness(root string) bool {
	for _, relative := range []string{"pnpm/global/v11", "pnpm/store/v11/files/00"} {
		if _, err := privateDirectory(filepath.Join(root, relative)); err != nil {
			return false
		}
	}
	return true
}
