//go:build windows

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Windows has no O_NOFOLLOW; the Lstat/opened identity comparison remains.
const userEngramNoFollow = 0

var userEngramRefusedErrno error = windows.WSAECONNREFUSED

// userWindowsProfileFolder resolves the user's real profile from the token,
// never from the caller environment; a seam so tests never touch it.
var userWindowsProfileFolder = func() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_Profile, windows.KF_FLAG_DEFAULT)
}

// userEngramLaunchEnv is the outer entry decision for a launch of root; a
// variable only so entry tests can observe which actions run it.
var userEngramLaunchEnv = userEngramWindowsLaunch

// userEngramWindowsWorkerEnv is what the outer entry adds to the worker
// environment for shared memory: the launch decision, and nothing for any
// other action (install, overlay and verify run in the install worker).
func userEngramWindowsWorkerEnv(ctx context.Context, args []string) []string {
	if len(args) < 2 || args[0] != "launch" {
		return nil
	}
	// args[2] is the product; the rest are its arguments.
	var productArgs []string
	if len(args) > 3 {
		productArgs = args[3:]
	}
	return userEngramLaunchDecision(ctx, args[1], productArgs)
}

// userEngramWindowsLaunch is the outer entry decision for a launch of root.
// The outer process is not in the owned Job Object, so a server it starts
// survives the launch; the Windows MVP installs only Separate.
func userEngramWindowsLaunch(ctx context.Context, root string) []string {
	return userEngramDecide(ctx, userEngramHost{
		lookup: os.LookupEnv,
		probe: func(ctx context.Context, timeout time.Duration) userEngramEndpoint {
			return userEngramProbe(ctx, userEngramProbeBase, timeout)
		},
		core: func() (string, bool) { return userEngramWindowsCore(root) },
		header: func() (uint32, bool) {
			home, ok := userEngramWindowsHome()
			if !ok {
				return 0, false
			}
			return userEngramReadHeader(filepath.Join(home, ".engram", "engram.db"))
		},
		start: userEngramWindowsStart,
	})
}

// userEngramWindowsCore revalidates <agent>\bin\engram.exe: a regular
// selected file whose whole path passes the owned-identity check.
func userEngramWindowsCore(root string) (string, bool) {
	core := filepath.Join(root, "agent", "bin", "engram.exe")
	info, err := os.Lstat(core)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	if _, err := userWindowsIdentity(core, true); err != nil {
		return "", false
	}
	return core, true
}

func userEngramWindowsHome() (string, bool) {
	home, err := userWindowsProfileFolder()
	if err != nil || home == "" {
		return "", false
	}
	if _, err := userWindowsIdentity(home, false); err != nil {
		return "", false
	}
	return home, true
}

// userEngramWindowsStart runs `core serve` detached in a new process group
// with the real profile and only the system variables Windows networking
// needs: no Engram selection, token or caller PATH.
func userEngramWindowsStart(core string) (<-chan struct{}, error) {
	home, ok := userEngramWindowsHome()
	if !ok {
		return nil, errors.New("real Windows profile is not verifiable")
	}
	system, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, err
	}
	if _, err := userWindowsIdentity(system, false); err != nil {
		return nil, err
	}
	cmd := exec.Command(core, "serve")
	cmd.Dir = home
	cmd.Env = []string{"SystemRoot=" + system, "WINDIR=" + system, "SystemDrive=" + filepath.VolumeName(system),
		"PATH=" + filepath.Join(system, "System32"), "USERPROFILE=" + home, "ENGRAM_NO_UPDATE_CHECK=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	return exited, nil
}
