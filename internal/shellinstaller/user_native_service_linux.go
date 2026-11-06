//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
)

// Prepared custody is not re-entry authority or evidence of active mounts.
// The caller finishes source/Node preparation first, supplies the runtime
// selected by userBusEnvironment, and retains exclusive custody until settled.
type userNativeServiceCustody struct {
	root, mask, managerRuntime string
	artifact                   userSnapshotArtifact
	identities                 map[string]string
}

func prepareUserNativeService(ctx context.Context, root string, artifact userSnapshotArtifact, mask, managerRuntime string) (*userNativeServiceCustody, error) {
	custody := &userNativeServiceCustody{root: root, artifact: artifact, mask: mask, managerRuntime: managerRuntime}
	identities, err := custody.capture(ctx)
	if err != nil {
		return nil, err
	}
	custody.identities = identities
	return custody, nil
}

// capture is read-only. Replacements with identical bytes are still refused.
func (c *userNativeServiceCustody) capture(ctx context.Context) (map[string]string, error) {
	if c == nil || ctx == nil {
		return nil, privateError("refused", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := UserKernelCheck(); err != nil {
		return nil, err
	}
	return c.captureFiles(ctx)
}

// captureFiles authenticates file custody only; it is not process admission.
// Production preparation and argument building reach it through capture.
func (c *userNativeServiceCustody) captureFiles(ctx context.Context) (map[string]string, error) {
	if c == nil || ctx == nil {
		return nil, privateError("refused", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths := []string{c.root, c.mask, c.managerRuntime}
	for i, path := range paths {
		if !userSelectionPath(path) {
			return nil, privateError("refused", nil)
		}
		for _, other := range paths[:i] {
			if userPathsOverlap(path, other) {
				return nil, privateError("refused", nil)
			}
		}
	}
	if err := userCompareMainSource(ctx, c.root, c.artifact); err != nil {
		return nil, err
	}
	entries, err := privateNativeEntries(c.mask)
	if err != nil || len(entries) != 0 {
		return nil, privateError("refused", err)
	}
	identities := map[string]string{}
	record := func(path string, immutable bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		st := info.Sys().(*syscall.Stat_t)
		if st.Uid != uint32(os.Getuid()) || st.Gid != uint32(os.Getgid()) {
			return privateError("refused", nil)
		}
		if info.IsDir() {
			if _, err := privateDirectory(path); err != nil {
				return err
			}
		} else if _, err := privatePhysical(path); err != nil {
			return err
		}
		identities[path] = fmt.Sprintf("%d:%d:%d:%d:%v", st.Dev, st.Ino, st.Uid, st.Gid, info.Mode())
		if immutable {
			identities[path] = privateStamp(info)
		}
		return nil
	}
	for _, path := range []string{c.root, filepath.Join(c.root, "runtime"), c.managerRuntime} {
		if err := record(path, false); err != nil {
			return nil, err
		}
	}
	if err := record(c.mask, true); err != nil {
		return nil, err
	}
	for _, base := range []string{filepath.Join(c.root, "runtime/main"), filepath.Join(c.root, "runtime/main-source")} {
		err := filepath.WalkDir(base, func(path string, _ os.DirEntry, cause error) error {
			if cause != nil {
				return cause
			}
			return record(path, true)
		})
		if err != nil {
			return nil, err
		}
	}
	return identities, ctx.Err()
}

// args preserves generic supervision and adds only the native custody boundary.
// Launching and checking actual mounts remain the ordinary bridge's job.
func (c *userNativeServiceCustody) args(ctx context.Context, unit string, interactive bool, self, cwd string, args, env []string) ([]string, error) {
	identities, err := c.capture(ctx)
	if err != nil {
		return nil, err
	}
	return c.argsForIdentities(identities, unit, interactive, self, cwd, args, env)
}

// argsForIdentities constructs data, not launch authority. The production args
// wrapper always obtains the fresh identities through kernel-checked capture.
func (c *userNativeServiceCustody) argsForIdentities(identities map[string]string, unit string, interactive bool, self, cwd string, args, env []string) ([]string, error) {
	if !reflect.DeepEqual(identities, c.identities) {
		return nil, privateError("preimage", nil)
	}
	expected := map[string]string{"XDG_RUNTIME_DIR": c.managerRuntime, "DBUS_SESSION_BUS_ADDRESS": "unix:path=" + c.managerRuntime + "/bus"}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if want, selected := expected[key]; selected {
			if !ok || value != want {
				return nil, privateError("refused", nil)
			}
			delete(expected, key)
		} else if key == "XDG_RUNTIME_DIR" || key == "DBUS_SESSION_BUS_ADDRESS" {
			return nil, privateError("refused", nil)
		}
	}
	if len(expected) != 0 {
		return nil, privateError("refused", nil)
	}
	base := userServiceArgs(unit, interactive, self, cwd, args, env)
	command := -1
	for i, arg := range base {
		if arg == "/usr/bin/env" {
			command = i
			break
		}
	}
	if command < 0 {
		return nil, errors.New("generic service command unavailable")
	}
	native := []string{
		"--property=PrivateUsers=yes",
		"--property=CapabilityBoundingSet=",
		"--property=RuntimeMaxSec=300s",
		"--property=ReadOnlyPaths=" + filepath.Join(c.root, "runtime/main") + " " + filepath.Join(c.root, "runtime/main-source") + " /sys/fs/cgroup",
		"--property=BindReadOnlyPaths=" + c.mask + ":" + c.managerRuntime,
	}
	result := append([]string(nil), base[:command]...)
	result = append(result, native...)
	return append(result, base[command:]...), nil
}
