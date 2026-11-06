//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestUserNativeRuntimeAfterStartCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires authentic source/Node in the bounded offline Guest")
	}
	if os.Getuid() != 1000 {
		t.Fatal("after-start qualification requires Guest UID1000")
	}
	if err := UserKernelCheck(); err != nil {
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
	for _, name := range []string{"canceled", "deadline"} {
		t.Run(name, func(t *testing.T) {
			setup, stopSetup := context.WithTimeout(context.Background(), 60*time.Second)
			defer stopSetup()
			workspace, err := os.MkdirTemp("/tmp", "native-cancel-")
			if err != nil {
				t.Fatal(err)
			}
			identity, err := privateDirectory(workspace)
			if err != nil {
				t.Fatal(err)
			}
			// Failed observers preserve their owned evidence rather than delete it.
			t.Log("owned cancellation workspace:", workspace)
			root := filepath.Join(workspace, "worker")
			if err := userPrepareInstallStage(setup, root, "separate", nil); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
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
			marker := filepath.Join(root, "project", "preserved")
			if err := os.WriteFile(marker, []byte("caller project"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			completed := make(chan struct{})
			go func() {
				_, err := w.invokeWithEnvironment(ctx, "collect", nil, false, nil) // Genuine original factory, never an installer spy.
				done <- err
				close(completed)
			}()
			settled := false
			defer func() {
				cancel()
				if !settled {
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("native invocation did not settle; preserve workspace")
					}
				}
			}()
			pid, fd, err := userObserveNativeInvocation(ctx, w, completed)
			if err != nil {
				t.Fatal("after-start observer unavailable; not behavioral evidence:", err)
			}
			defer unix.Close(fd)
			// Hold only this exact owned invocation after exec, not before Start.
			// pidfd prevents sending the signal to a subsequently reused PID.
			if err := userHoldNativeInvocation(ctx, completed, pid, fd); err != nil {
				t.Fatal("after-start hold unavailable; observer/infrastructure evidence only:", err)
			}
			if name == "canceled" {
				cancel()
			} else {
				<-ctx.Done()
			}
			select {
			case err = <-done:
				settled = true
			case <-time.After(3 * time.Second):
				t.Fatal("after-start cancellation did not return; preserve workspace")
			}
			var failure *PrivateRuntimeError
			if !errors.As(err, &failure) || failure.Kind != name {
				t.Errorf("after-start verdict: %v", err)
			}
			want := context.Canceled
			if name == "deadline" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Errorf("after-start context cause lost: %v", err)
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
				t.Fatal("independent source readback failed:", err)
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "caller project" {
				t.Fatal("private project marker changed", err)
			}
			if t.Failed() {
				return
			}
			if err := privateCleanup(workspace, identity); err != nil {
				t.Fatal("owned cleanup not settled:", err)
			}
			t.Log("NATIVE_AFTER_START_READBACK node_exec=true stopped_before_cancel=true pidfd_exit=true group_empty=true source_compared=true private_marker_unchanged=true cleanup=true installed_acceptance=false")
		})
	}
}

func userObserveNativeInvocation(ctx context.Context, w *NativeInstallerRuntime, completed <-chan struct{}) (int, int, error) {
	wanted := strings.Join([]string{w.node, "--liftoff-only", "--input-type=module", "-e", nativeRuntimeClient, w.source, ""}, "\x00")
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if err := userNativeInvocationActive(ctx, completed); err != nil {
			return 0, -1, err
		}
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return 0, -1, err
		}
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil || pid <= 0 {
				continue
			}
			path := "/proc/" + entry.Name()
			command, err := os.ReadFile(path + "/cmdline")
			if err != nil || !bytes.Equal(command, []byte(wanted)) {
				continue
			}
			fd, err := unix.PidfdOpen(pid, 0)
			if err != nil {
				return 0, -1, err
			}
			fresh, err := os.ReadFile(path + "/cmdline")
			cwd, cwdErr := os.Readlink(path + "/cwd")
			group, groupErr := unix.Getpgid(pid)
			if err == nil && cwdErr == nil && groupErr == nil && bytes.Equal(command, fresh) && cwd == filepath.Join(w.root, "project") && group == pid {
				if err := userNativeInvocationActive(ctx, completed); err == nil {
					return pid, fd, nil
				}
			}
			unix.Close(fd)
		}
		select {
		case <-ctx.Done():
			return 0, -1, ctx.Err()
		case <-tick.C:
		}
	}
}

func userNativeInvocationActive(ctx context.Context, completed <-chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-completed:
		return errors.New("invocation completed before observation/hold")
	default:
		return nil
	}
}

func userHoldNativeInvocation(ctx context.Context, completed <-chan struct{}, pid, fd int) error {
	if err := userNativeInvocationActive(ctx, completed); err != nil {
		return err
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGSTOP, nil, 0); err != nil {
		return err
	}
	limit := time.NewTimer(200 * time.Millisecond)
	defer limit.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if err := userNativeInvocationActive(ctx, completed); err != nil {
			return err
		}
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if n, err := unix.Poll(poll, 0); err != nil || n != 0 {
			return errors.Join(err, errors.New("owned Node exited before hold"))
		}
		status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return err
		}
		if bytes.Contains(status, []byte("State:\tT")) {
			if n, err := unix.Poll(poll, 0); err != nil || n != 0 {
				return errors.Join(err, errors.New("owned Node exited during stop readback"))
			}
			return userNativeInvocationActive(ctx, completed)
		}
		select {
		case <-limit.C:
			return errors.New("owned Node stop not confirmed")
		case <-tick.C:
		}
	}
}

func userNativeGroupAbsent(group int) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		actual, err := unix.Getpgid(pid)
		if err == nil && actual == group {
			return fmt.Errorf("owned process group still contains PID %d", pid)
		}
		if err != nil && !errors.Is(err, unix.ESRCH) {
			return err
		}
	}
	return nil
}
