//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Original persistence is qualified separately from installation/publication.
func TestUserNativeInstallerPersistentRuntime(t *testing.T) {
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
	workspace, err := os.MkdirTemp("/tmp", "native-pnpm-persistence-")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := privateDirectory(workspace)
	if err != nil {
		t.Fatal(err)
	}
	// Retain any failed/inconclusive workspace, including observer failures.
	t.Log("owned persistence workspace:", workspace)
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
	stopMirror := userPersistenceMirror(t, setup, workspace, w.nativeEnvironment())
	defer stopMirror()
	plan, err := w.Prepare(setup, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.bootstrapEnv, "PNPM_CONFIG_STORE_DIR="+filepath.Join(root, "pnpm/store")) {
		t.Fatal("private store missing from sealed environment")
	}
	if !slices.Contains(plan.bootstrapEnv, "NODE_USE_SYSTEM_CA=1") {
		t.Fatal("prepared system-trust environment missing")
	}
	toolStamp, err := userTreeStamp(tools)
	if err != nil {
		t.Fatal(err)
	}
	projectStamp, err := userTreeStamp(filepath.Join(root, "project"))
	if err != nil {
		t.Fatal(err)
	}
	userPersistenceProjectObservation(t, "before", filepath.Join(root, "project"))
	watch, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		t.Error("project watch unavailable", err)
		watch = -1
	} else {
		defer unix.Close(watch)
		if _, err := unix.InotifyAddWatch(watch, filepath.Join(root, "project"), unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_FROM|unix.IN_MOVED_TO|unix.IN_ATTRIB|unix.IN_DELETE_SELF|unix.IN_MOVE_SELF); err != nil {
			t.Error("project watch unavailable", err)
			watch = -1
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
	defer func() {
		cancel()
		select {
		case <-completed:
		case <-time.After(3 * time.Second):
			t.Error("runner did not settle; retain evidence")
		}
	}()
	pid, fd, err := userObserveNativeInvocation(ctx, w, completed)
	if err != nil {
		t.Fatal("native invocation observer inconclusive:", err)
	}
	defer unix.Close(fd)
	var outcome returned
	select {
	case outcome = <-done:
	case <-ctx.Done():
		t.Fatal("persistent runtime deadline is not positive evidence")
	}
	t.Logf("original persistence result: %s; process error: %v", outcome.result.Details, outcome.err)
	var native struct {
		Outcome    string   `json:"outcome"`
		FailedStep string   `json:"failedStep"`
		Completed  []string `json:"completed"`
	}
	if outcome.err != nil || json.Unmarshal(outcome.result.Details, &native) != nil || native.Outcome != "failed" || native.FailedStep != "install-global" {
		t.Errorf("expected only the later unqualified install-global to fail: %+v %v", native, outcome.err)
	}
	for _, step := range []string{"persist-node", "persist-package-managers", "verify-persistent-runtime", "check-npm", "configure-npm-prefix"} {
		if !slices.Contains(native.Completed, step) {
			t.Errorf("original persistence step incomplete: %s", step)
		}
	}
	poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if _, err := unix.Poll(poll, 3000); err != nil || poll[0].Revents&unix.POLLIN == 0 {
		t.Fatal("owned invocation exit unproved", err)
	}
	if err := userNativeGroupAbsent(pid); err != nil {
		t.Fatal("owned group settlement unproved:", err)
	}
	stopMirror() // Normal owner pipe shutdown, wait and group check before PASS.
	readback, stopReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopReadback()
	if err := userCompareMainSource(readback, root, artifact); err != nil {
		t.Fatal(err)
	}
	if _, err := privateNativeFile(readback, w.node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		t.Fatal(err)
	}
	if after, err := userTreeStamp(tools); err != nil || after != toolStamp {
		t.Error("bootstrap tooling changed", err)
	}
	if watch >= 0 {
		userPersistenceProjectEvents(t, watch)
	}
	userPersistenceProjectObservation(t, "after", filepath.Join(root, "project"))
	if after, err := userTreeStamp(filepath.Join(root, "project")); err != nil || after != projectStamp {
		t.Error("private project changed", err)
	}
	globalBin := filepath.Join(root, "pnpm", "bin")
	node, err := filepath.EvalSymlinks(filepath.Join(globalBin, "node"))
	if err != nil || !strings.HasPrefix(node, filepath.Join(root, "pnpm")+string(os.PathSeparator)) {
		t.Fatal("persistent Node is not inside PNPM_HOME:", node, err)
	}
	// Dedicated large-file authentication; never widen userTreeStamp's limit.
	if _, err := privateNativeFile(readback, node, 0, 126595440, "7fde7b8afa198da66257f42ee2001d874c7355631e6d1579a5fb5ef1f246df4c"); err != nil {
		t.Fatal("persistent Node authentication unavailable:", err)
	}
	env := append([]string(nil), plan.bootstrapEnv...)
	for i, value := range env {
		if strings.HasPrefix(value, "PATH=") {
			env[i] = "PATH=" + globalBin + ":" + strings.TrimPrefix(value, "PATH=")
		}
	}
	for _, probe := range []struct{ command, want string }{{"node", "v24.21.0"}, {"npm", "11.19.0"}, {"pnpm", "11.1.1"}} {
		path := filepath.Join(globalBin, probe.command)
		cmd := exec.CommandContext(readback, path, "--version")
		cmd.Dir, cmd.Env = filepath.Join(root, "project"), env
		out, err := userOwnedRun(readback, cmd, stopReadback)
		if err != nil || strings.TrimSpace(out) != probe.want {
			t.Errorf("persistent %s readback differs: %q %v", probe.command, out, err)
		}
	}
	cmd := exec.CommandContext(readback, filepath.Join(globalBin, "npm"), "config", "get", "prefix")
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), env
	prefix, err := userOwnedRun(readback, cmd, stopReadback)
	if err != nil || strings.TrimSpace(prefix) != filepath.Join(root, "pnpm") {
		t.Errorf("private npm prefix differs: %q %v", prefix, err)
	}
	if after, err := userTreeStamp(filepath.Join(root, "project")); err != nil || after != projectStamp {
		t.Error("private project changed during readbacks", err)
	}
	if t.Failed() {
		return
	}
	if err := privateCleanup(workspace, identity); err != nil {
		t.Fatal(err)
	}
	t.Log("NATIVE_PERSISTENCE_READBACK original_runner=true completed_persistence=true node_npm_pnpm_versions=true node_reauthenticated=true prefix_private=true source_compared=true bootstrap_tools_unchanged=true private_project_unchanged=true group_empty=true mirror_settled=true cleanup=true installed_acceptance=false")
}

// Diagnostic only: keep the complete preservation oracle above unchanged.
func userPersistenceProjectObservation(t *testing.T, label, root string) {
	t.Helper()
	dir, err := os.Open(root)
	if err != nil {
		t.Error("project diagnostic open failed", err)
		return
	}
	names, readErr := dir.Readdirnames(16)
	closeErr := dir.Close()
	if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(names) > 15 {
		t.Errorf("bounded project diagnostic inconclusive: entries=%d read=%v close=%v", len(names), readErr, closeErr)
		return
	}
	slices.Sort(names)
	for _, name := range append([]string{"."}, names...) {
		if len(name) > 128 {
			t.Error("project diagnostic name exceeds bound")
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Errorf("project diagnostic stat failed: %q", name)
			continue
		}
		st := info.Sys().(*syscall.Stat_t)
		mtime, ctime := privateStatTimes(st)
		stamp := "-"
		if info.Mode().IsRegular() {
			stamp, err = userTreeStamp(path)
			if err != nil {
				t.Errorf("project file diagnostic inconclusive: %q", name)
				stamp = "inconclusive"
			}
		}
		t.Logf("PROJECT_%s %q dev=%d ino=%d uid=%d mode=%v size=%d mtime=%v ctime=%v stamp=%s",
			label, name, st.Dev, st.Ino, st.Uid, info.Mode(), info.Size(), mtime, ctime, stamp)
	}
}

func userPersistenceProjectEvents(t *testing.T, fd int) {
	t.Helper()
	var buffer [4096]byte
	count := 0
	for {
		n, err := unix.Read(fd, buffer[:])
		if err == unix.EAGAIN {
			t.Logf("PROJECT_EVENTS count=%d", count)
			return
		}
		if err != nil || n == 0 {
			t.Error("project event read inconclusive", err)
			return
		}
		for offset := 0; offset < n; {
			if n-offset < 16 || count == 32 {
				t.Error("project event diagnostic exceeds bounds")
				return
			}
			header := buffer[offset : offset+16]
			mask := binary.NativeEndian.Uint32(header[4:8])
			length := binary.NativeEndian.Uint32(header[12:16])
			if length > uint32(n-offset-16) || mask&(unix.IN_Q_OVERFLOW|unix.IN_IGNORED) != 0 {
				t.Error("project event stream incomplete")
				return
			}
			end := offset + 16 + int(length)
			name := strings.TrimRight(string(buffer[offset+16:end]), "\x00")
			if len(name) > 128 {
				t.Error("project event name exceeds bound")
				return
			}
			t.Logf("PROJECT_EVENT mask=%#x name=%q", mask, name)
			count++
			offset = end
		}
	}
}
