//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"
)

// An in-process owner, not a caller-supplied slice name or delivery authority.
// Unknown creation outcomes retain this handle; they are never retried.
type userNativeSlice struct {
	name                      string
	group                     userUnitEvidence
	env                       []string
	runtimeIdentity, busStamp string
	tools                     map[string]string
}

func userNativeSliceName(name string) bool {
	if len(name) != len("gentleshell")+16+len(".slice") || !strings.HasPrefix(name, "gentleshell") || !strings.HasSuffix(name, ".slice") {
		return false
	}
	token := strings.TrimSuffix(strings.TrimPrefix(name, "gentleshell"), ".slice")
	_, err := hex.DecodeString(token)
	return err == nil && strings.ToLower(token) == token
}

func userNativeSliceCreateArgs(name string) []string {
	return []string{"--user", "call", "org.freedesktop.systemd1", "/org/freedesktop/systemd1", "org.freedesktop.systemd1.Manager", "StartTransientUnit", "ssa(sv)a(sa(sv))", name, "fail", "5", "MemoryMax", "t", "3221225472", "MemorySwapMax", "t", "0", "CPUQuotaPerSecUSec", "t", "1000000", "CPUQuotaPeriodUSec", "t", "100000", "TasksMax", "t", "64", "0"}
}

func newUserNativeSlice(ctx context.Context) (*userNativeSlice, error) {
	if ctx == nil {
		return nil, privateError("refused", nil)
	}
	if os.Getuid() == 0 {
		if !RootRiskAccepted() {
			return nil, &RootRiskRequiredError{}
		}
		return nil, privateError("refused", errors.New("root installation has no delegated user manager for the fresh native slice; create a regular user or track the system-slice root path"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	env, err := userBusEnvironment()
	if err != nil {
		return nil, err
	}
	s := &userNativeSlice{env: env, tools: map[string]string{}}
	s.runtimeIdentity, err = userIdentity(os.Getenv("XDG_RUNTIME_DIR"))
	if err != nil {
		return nil, err
	}
	socket, err := os.Lstat(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "bus"))
	if err != nil {
		return nil, err
	}
	s.busStamp = privateStamp(socket)
	for _, tool := range []string{"/usr/bin/busctl", "/usr/bin/systemctl"} {
		s.tools[tool], err = userSupervisorSHA(ctx, tool)
		if err != nil {
			return nil, err
		}
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	s.name = "gentleshell" + hex.EncodeToString(nonce) + ".slice"
	if _, err = s.run(ctx, "/usr/bin/busctl", userNativeSliceCreateArgs(s.name)...); err != nil {
		return s, s.uncertain(err)
	}
	output, err := s.run(ctx, "/usr/bin/systemctl", "--user", "show", s.name, "--property=ControlGroup", "--value")
	member := strings.TrimSpace(output)
	if err != nil || !privateHierarchyPath(member) || filepath.Base(member) != s.name {
		return s, s.uncertain(err)
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return s, s.uncertain(err)
	}
	s.group.Path, err = privateCgroupPath("0::"+member+"\n", string(mounts))
	if err != nil {
		return s, s.uncertain(err)
	}
	if err = privateCgroupPhysical(s.group.Path, true); err != nil {
		return s, s.uncertain(err)
	}
	info, err := os.Stat(s.group.Path)
	if err != nil {
		return s, s.uncertain(err)
	}
	st := info.Sys().(*syscall.Stat_t)
	s.group.Dev, s.group.Ino = st.Dev, st.Ino
	if err = s.read(); err != nil {
		return s, s.uncertain(err)
	}
	if err = s.empty(); err != nil {
		return s, s.uncertain(err)
	}
	return s, nil
}

func (s *userNativeSlice) uncertain(cause error) error {
	err := privateError("uncertain", cause)
	err.Workspace, err.Destination = s.group.Path, s.name
	return err
}

// Every manager call repeats canonical runtime/socket and executable checks.
func (s *userNativeSlice) run(ctx context.Context, tool string, args ...string) (string, error) {
	if ctx == nil || s == nil || !userNativeSliceName(s.name) {
		return "", privateError("refused", nil)
	}
	env, err := userBusEnvironment()
	if err != nil || !reflect.DeepEqual(env, s.env) {
		return "", privateError("preimage", err)
	}
	identity, err := userIdentity(os.Getenv("XDG_RUNTIME_DIR"))
	if err != nil || identity != s.runtimeIdentity {
		return "", privateError("preimage", err)
	}
	socket, err := os.Lstat(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "bus"))
	if err != nil || privateStamp(socket) != s.busStamp {
		return "", privateError("preimage", err)
	}
	digest, err := userSupervisorSHA(ctx, tool)
	if err != nil || s.tools[tool] == "" || digest != s.tools[tool] {
		return "", privateError("preimage", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output := &privateOutput{cancel: cancel}
	command := exec.CommandContext(bounded, tool, args...)
	command.Env, command.Stdout, command.Stderr = s.env, output, output
	command.WaitDelay = 2 * time.Second
	err = command.Run()
	if output.overflow || bounded.Err() != nil {
		return "", errors.Join(err, bounded.Err(), privateError("output", nil))
	}
	return string(output.data), err
}

func (s *userNativeSlice) read() error {
	if s == nil || !userNativeSliceName(s.name) || filepath.Base(s.group.Path) != s.name || !strings.HasPrefix(s.group.Path, "/sys/fs/cgroup/") || s.group.Ino == 0 {
		return privateError("refused", nil)
	}
	if err := privateCgroupPhysical(s.group.Path, true); err != nil {
		return err
	}
	info, err := os.Stat(s.group.Path)
	if err != nil {
		return err
	}
	st := info.Sys().(*syscall.Stat_t)
	if st.Dev != s.group.Dev || st.Ino != s.group.Ino {
		return privateError("preimage", nil)
	}
	for i, name := range []string{"memory.max", "memory.swap.max", "cpu.max", "pids.max"} {
		path := filepath.Join(s.group.Path, name)
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != []string{"3221225472", "0", "100000 100000", "64"}[i] {
			return privateError("unavailable", err)
		}
	}
	fresh, err := os.Stat(s.group.Path)
	if err != nil || !os.SameFile(info, fresh) {
		return privateError("preimage", err)
	}
	return nil
}

func (s *userNativeSlice) serviceArg(ctx context.Context) (string, error) {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return "", err
	}
	if err := s.read(); err != nil {
		return "", err
	}
	return "--slice=" + s.name, nil
}

func (s *userNativeSlice) member(ctx context.Context) error {
	if err := nativeRuntimeError(ctx, nil); err != nil {
		return err
	}
	if err := s.read(); err != nil {
		return err
	}
	if err := UserKernelCheck(); err != nil {
		return err
	}
	membership, e1 := os.ReadFile("/proc/self/cgroup")
	mounts, e2 := os.ReadFile("/proc/self/mountinfo")
	path, err := privateCgroupPath(string(membership), string(mounts))
	if e1 != nil || e2 != nil || err != nil || !strings.HasPrefix(path, s.group.Path+"/") {
		return privateError("refused", errors.Join(e1, e2, err))
	}
	return s.read()
}

func (s *userNativeSlice) empty() error {
	if err := s.read(); err != nil {
		return err
	}
	for name, want := range map[string]string{"pids.current": "0", "cgroup.events": "populated 0\nfrozen 0"} {
		path := filepath.Join(s.group.Path, name)
		if err := privateCgroupPhysical(path, false); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != want {
			return privateError("uncertain", err)
		}
	}
	return s.read()
}

// Leaves must settle first. Never stop an unbound, replaced or populated slice.
func (s *userNativeSlice) settle(ctx context.Context) error {
	if s == nil {
		return privateError("refused", nil)
	}
	if err := s.empty(); err != nil {
		return s.uncertain(err)
	}
	if _, err := s.run(ctx, "/usr/bin/systemctl", "--user", "stop", s.name); err != nil {
		return s.uncertain(err)
	}
	if _, err := os.Lstat(s.group.Path); !os.IsNotExist(err) {
		return s.uncertain(err)
	}
	return nil
}
