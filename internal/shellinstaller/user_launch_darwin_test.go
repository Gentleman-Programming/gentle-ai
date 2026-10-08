//go:build darwin

package shellinstaller

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A process group whose only member is an unreaped zombie is the window
// between a killed member's exit and its parent's wait.
func userZombieGroup(t *testing.T) *os.Process {
	t.Helper()
	process, err := os.StartProcess("/usr/bin/true", []string{"true"}, &os.ProcAttr{Sys: &syscall.SysProcAttr{Setpgid: true}})
	if err != nil {
		t.Fatal(err)
	}
	// A second Wait after the test reaped the child only returns an error.
	t.Cleanup(func() { _, _ = process.Wait() })
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", process.Pid)
		if err != nil {
			t.Fatal(err)
		}
		if len(members) == 1 && members[0].Proc.P_pid == int32(process.Pid) && members[0].Proc.P_stat == userDarwinZombie {
			return process
		}
	}
	t.Fatal("child never became a listed zombie group member")
	return nil
}

// The leader exits while its background subshell forks sleep, so the first
// group SIGKILL often races that fork; a raw signal-0 probe then watched a
// surviving sleep for its whole 2s in about one run of four on darwin. The
// repeated SIGKILL also meets members already exiting (P_WEXIT), which the
// kernel answers with EPERM.
func TestDarwinReapGroupKillsMemberForkedDuringKill(t *testing.T) {
	for iteration := 0; iteration < 25; iteration++ {
		cmd := exec.Command("/bin/sh", "-c", "(sleep 2; :) & exit 0")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		if err := userReapGroup(cmd.Process.Pid, nil); err != nil {
			t.Fatalf("iteration %d: reap = %v", iteration, err)
		}
		if err := syscall.Kill(-cmd.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("iteration %d: group after reap = %v, want ESRCH", iteration, err)
		}
	}
}

func TestDarwinKillGroupCountsZombieMembersLikeLinux(t *testing.T) {
	process := userZombieGroup(t)
	// The kernel answers EPERM for a zombie-only group; Linux answers 0.
	if err := syscall.Kill(-process.Pid, 0); !errors.Is(err, syscall.EPERM) {
		t.Fatalf("raw zombie-only group probe = %v; the darwin EPERM quirk changed, revisit userKillGroup", err)
	}
	for _, sig := range []syscall.Signal{0, syscall.SIGKILL} {
		if err := userKillGroup(process.Pid, sig); err != nil {
			t.Fatalf("zombie-only group signal %d = %v, want the Linux answer nil", sig, err)
		}
	}
	// An unreaped zombie still occupies the group: reaping must not claim it gone.
	if err := userReapGroup(process.Pid, nil); err == nil {
		t.Fatal("reap reported an empty group while a zombie member remained")
	}
	if _, err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := userReapGroup(process.Pid, nil); err != nil {
		t.Fatalf("reap after the zombie was waited = %v", err)
	}
	if err := userKillGroup(process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("emptied group = %v, want ESRCH", err)
	}
}
