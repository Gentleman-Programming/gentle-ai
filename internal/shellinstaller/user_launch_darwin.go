//go:build darwin

package shellinstaller

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

// SZOMB and P_WEXIT from XNU sys/proc.h; x/sys does not export them.
const (
	userDarwinZombie  = 5
	userDarwinExiting = 0x2000
)

// userReapProbe is the signal userReapGroup repeats while it waits for the
// group to empty. A darwin member forked while killpg(SIGKILL) runs can miss
// that signal and keep running, so every probe is another SIGKILL.
const userReapProbe = syscall.SIGKILL

// privateReapRun is privateRun's post-Wait cleanup. One darwin group SIGKILL
// can miss a member forked during it, so the group is reaped until the kernel
// reports it empty; any other outcome is an uncertain PrivateRuntimeError.
func privateReapRun(pid int, waitErr error, _ func() error) error {
	return userReapGroup(pid, waitErr)
}

// userKillGroup signals every member of process group pgid with the Linux
// answers. Darwin returns EPERM, not success, when every member is already
// exiting (P_WEXIT) or a zombie awaiting its parent's wait; the group is still
// occupied, as Linux reports, so that case becomes nil and the reap keeps
// polling. EPERM with any other member stays EPERM.
func userKillGroup(pgid int, sig syscall.Signal) error {
	err := syscall.Kill(-pgid, sig)
	if !errors.Is(err, syscall.EPERM) {
		return err
	}
	members, listErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if listErr != nil {
		return errors.Join(err, listErr)
	}
	if len(members) == 0 {
		return syscall.ESRCH // The last zombie was reaped after the signal.
	}
	for _, member := range members {
		if member.Proc.P_stat != userDarwinZombie && member.Proc.P_flag&userDarwinExiting == 0 {
			return err
		}
	}
	return nil
}
