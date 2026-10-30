//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const userEngramNoFollow = syscall.O_NOFOLLOW

var userEngramRefusedErrno error = syscall.ECONNREFUSED

// userEngramLaunchEnv is the outer entry decision for a launch of root; a
// variable only so entry tests can observe which actions run it.
var userEngramLaunchEnv = userEngramUnixLaunch

// userEngramUnixLaunch decides shared memory for a Separate installation.
// Any other mode, or a manifest that does not read back, forwards nothing:
// their launch environment stays exactly what it was.
func userEngramUnixLaunch(ctx context.Context, root string) []string {
	agent, ok := userEngramSeparateAgent(ctx, root)
	if !ok {
		return nil
	}
	home := os.Getenv("HOME")
	return userEngramDecide(ctx, userEngramHost{
		lookup: os.LookupEnv,
		probe: func(ctx context.Context, timeout time.Duration) userEngramEndpoint {
			return userEngramProbe(ctx, userEngramProbeBase, timeout)
		},
		core: func() (string, bool) { return userEngramUnixCore(ctx, agent) },
		header: func() (uint32, bool) {
			if privateNativeHome(home) != nil {
				return 0, false
			}
			return userEngramReadHeader(filepath.Join(home, ".engram", "engram.db"))
		},
		start: func(core string) (<-chan struct{}, error) { return userEngramUnixStart(core, home) },
	})
}

// userEngramSeparateAgent reads only the recorded mode and agent of root's
// manifest, through the same owned-file checks every launch applies.
func userEngramSeparateAgent(ctx context.Context, root string) (string, bool) {
	if !privateHierarchyPath(root) {
		return "", false
	}
	if _, err := privateDirectory(root); err != nil {
		return "", false
	}
	path := filepath.Join(root, "installation.json")
	if _, err := privateNativeFile(ctx, path, 0600, -1, ""); err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	var manifest userManifest
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &manifest) != nil || manifest.Destination != root || manifest.Mode != "separate" || !privateHierarchyPath(manifest.Agent) {
		return "", false
	}
	return manifest.Agent, true
}

// userEngramUnixCore revalidates <agent>/bin/engram as an owned physical
// 0700 file in an owned physical bin directory, the checks agent tools get.
func userEngramUnixCore(ctx context.Context, agent string) (string, bool) {
	bin := filepath.Join(agent, "bin")
	if privateNativeDirectory(bin) != nil {
		return "", false
	}
	core := filepath.Join(bin, "engram")
	if _, err := privateNativeFile(ctx, core, 0700, -1, ""); err != nil {
		return "", false
	}
	return core, true
}

// userEngramUnixStart runs `core serve` in its own session with the real
// HOME, so it outlives the launch's process group, unit and terminal. Its
// environment carries no caller setting: no Engram selection, token or PATH.
func userEngramUnixStart(core, home string) (<-chan struct{}, error) {
	if privateNativeHome(home) != nil {
		return nil, errors.New("real HOME is not an owned directory")
	}
	cmd := exec.Command(core, "serve")
	cmd.Dir = home
	cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "ENGRAM_NO_UPDATE_CHECK=1"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
