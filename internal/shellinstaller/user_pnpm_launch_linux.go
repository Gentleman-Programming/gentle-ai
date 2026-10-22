//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func userLaunchPNPM(ctx context.Context, root, role string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if (role != "pi" && role != "gentle-shell") || strings.ContainsRune(root, os.PathListSeparator) {
		return privateError("refused", errors.New("invalid PNPM launcher role or PATH root"))
	}
	manifest, err := userReadManifest(ctx, root)
	if err != nil || manifest.PNPM == nil {
		return privateError("refused", err)
	}
	p := manifest.PNPM
	location := NativeInstallationLocation{PackageRoot: filepath.Join(root, p.PackageRoot), PNPMHome: manifest.Prefix, GlobalBin: filepath.Join(manifest.Prefix, "bin"), ShellVersion: p.ShellVersion, PiVersion: p.PeerVersion}
	installed, err := ReadNativePNPMInstallation(ctx, root, location)
	if err != nil {
		return err
	}
	for _, name := range []string{"home", "config", "state", "data", "tmp", "data/cache"} {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		if _, err := privateDirectory(path); err != nil {
			return err
		}
	}
	cli := installed.PiCLI
	if role == "gentle-shell" {
		cli = installed.ShellCLI
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, installed.Node), append([]string{filepath.Join(root, cli)}, args...)...)
	cmd.Env = userEnvironment(root, manifest.Prefix, manifest.Agent)
	for i, entry := range cmd.Env {
		if strings.HasPrefix(entry, "PATH=") {
			cmd.Env[i] = "PATH=" + manifest.Prefix + "/bin:/usr/bin:/bin"
		} else if strings.HasPrefix(entry, "NPM_CONFIG_CACHE=") {
			cmd.Env[i] = "NPM_CONFIG_CACHE=" + root + "/data/cache"
		}
	}
	userCodeGraphLaunchEnv(root, manifest, cmd)
	// Keep supplier home selection: direct Pi uses Agent; Shell's isolated
	// default stays under private HOME. Never inherit caller runtime injection.
	cmd.Env = append(cmd.Env, "PNPM_HOME="+manifest.Prefix, "XDG_DATA_HOME="+root+"/data", "XDG_CACHE_HOME="+root+"/data/cache", "GIT_OPTIONAL_LOCKS=0", "GOMAXPROCS=1", "UV_THREADPOOL_SIZE=1", "NODE_OPTIONS=--v8-pool-size=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return userLaunchGroup(cmd, stdin, func(exitErr error) error {
		if _, err := userReadManifest(context.Background(), root); err != nil {
			failure := privateError("uncertain", errors.Join(exitErr, err))
			failure.Workspace, failure.Destination = root, root
			return failure
		}
		return exitErr
	})
}
