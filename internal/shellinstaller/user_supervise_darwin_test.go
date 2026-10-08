//go:build darwin

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var userSuperviseTestResources = []int{syscall.RLIMIT_CPU, unix.RLIMIT_NPROC, syscall.RLIMIT_FSIZE, syscall.RLIMIT_NOFILE}

func userSuperviseTestHelper(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return self
}

func userSuperviseTestRlimits(t *testing.T) map[int]syscall.Rlimit {
	t.Helper()
	limits := map[int]syscall.Rlimit{}
	for _, resource := range userSuperviseTestResources {
		var limit syscall.Rlimit
		if err := syscall.Getrlimit(resource, &limit); err != nil {
			t.Fatal(err)
		}
		limits[resource] = limit
	}
	return limits
}

// userSuperviseTestFile polls for a file the supervised group writes.
func userSuperviseTestFile(t *testing.T, path string) int {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if data, err := os.ReadFile(path); err == nil && strings.HasSuffix(string(data), "\n") {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil || pid <= 1 {
				t.Fatalf("invalid pid in %s: %q", path, data)
			}
			return pid
		}
	}
	t.Fatalf("supervised group never wrote %s", path)
	return 0
}

func userSuperviseTestKind(t *testing.T, err error, kind string) {
	t.Helper()
	var private *PrivateRuntimeError
	if !errors.As(err, &private) || private.Kind != kind {
		t.Fatalf("supervise error = %v, want kind %q", err, kind)
	}
}

func TestDarwinServiceLimitsMirrorLinuxUnit(t *testing.T) {
	before := userSuperviseTestRlimits(t)
	processes, err := userUIDProcesses()
	if err != nil || processes < 1 {
		t.Fatalf("uid process count = %d, %v", processes, err)
	}
	limits, err := userDarwinLimits(90 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if limits.CPUSeconds != 90 || limits.Deadline != 90*time.Second {
		t.Fatalf("CPU budget must equal CPUQuota=100%% over the deadline: %+v", limits)
	}
	if limits.FileBytes != 4<<30 || limits.Descriptors != 1024 || limits.DescriptorsMax == 0 || limits.DescriptorsMax > 524288 || limits.DescriptorsMax > before[syscall.RLIMIT_NOFILE].Max {
		t.Fatalf("file limits = %+v", limits)
	}
	// RLIMIT_NPROC counts every process of the real uid on darwin, so the
	// TasksMax=64 budget is headroom above the current population.
	if limits.Processes < uint64(processes)+userTasksMax-16 || limits.Processes > uint64(processes)+userTasksMax+16 || limits.Processes > before[unix.RLIMIT_NPROC].Max {
		t.Fatalf("process limit %d not uid population %d + %d", limits.Processes, processes, userTasksMax)
	}
	open, err := userDarwinLimits(0)
	if err != nil || open.CPUSeconds != 0 || open.Deadline != 0 {
		t.Fatalf("no deadline must leave CPU time inherited: %+v, %v", open, err)
	}
	if !strings.Contains(userSupervisionLimitation, "setsid") {
		t.Fatalf("limitation must name setsid: %q", userSupervisionLimitation)
	}
}

func TestDarwinSuperviseLimitsChildOnly(t *testing.T) {
	before := userSuperviseTestRlimits(t)
	limits, err := userDarwinLimits(90 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	script := `echo $$ $(ps -o pgid= -p $$); ulimit -St; ulimit -Ht; ulimit -Su; ulimit -Hu; ulimit -Sf; ulimit -Hf; ulimit -Sn; ulimit -Hn; umask`
	var stdout, stderr bytes.Buffer
	err = userSupervise(context.Background(), limits, []string{"PATH=/usr/bin:/bin"}, nil, &stdout, &stderr, "/bin/bash", "-c", script)
	if err != nil {
		t.Fatalf("supervise = %v; stderr %q", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 10 {
		t.Fatalf("child report = %q; stderr %q", stdout.String(), stderr.String())
	}
	ids := strings.Fields(lines[0])
	if len(ids) != 2 || ids[0] != ids[1] || ids[0] == strconv.Itoa(os.Getpid()) || ids[1] == strconv.Itoa(syscall.Getpgrp()) {
		t.Fatalf("child must lead a new process group: %q (test pid %d, pgrp %d)", lines[0], os.Getpid(), syscall.Getpgrp())
	}
	// bash reports -f in 1024-byte blocks outside POSIX mode.
	want := []uint64{limits.CPUSeconds, limits.CPUSeconds, limits.Processes, limits.Processes, limits.FileBytes / 1024, limits.FileBytes / 1024, limits.Descriptors, limits.DescriptorsMax}
	for index, value := range want {
		if lines[index+1] != strconv.FormatUint(value, 10) {
			t.Fatalf("child limit line %d = %q, want %d; report %q", index+1, lines[index+1], value, stdout.String())
		}
	}
	if lines[9] != "0077" {
		t.Fatalf("child umask = %q, want 0077", lines[9])
	}
	after := userSuperviseTestRlimits(t)
	for _, resource := range userSuperviseTestResources {
		if before[resource] != after[resource] {
			t.Fatalf("supervisor changed its own limit %d: %+v -> %+v", resource, before[resource], after[resource])
		}
	}
}

func TestDarwinSuperviseCancelKillsWholeGroup(t *testing.T) {
	dir := t.TempDir()
	limits, err := userDarwinLimits(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- userSupervise(ctx, limits, []string{"PATH=/usr/bin:/bin"}, nil, nil, nil,
			"/bin/bash", "-c", `(sleep 1; : > "$1/survived") & echo $$ > "$1/pid"; sleep 30`, "bash", dir)
	}()
	pid := userSuperviseTestFile(t, filepath.Join(dir, "pid"))
	cancel()
	select {
	case err := <-done:
		userSuperviseTestKind(t, err, "canceled")
	case <-time.After(10 * time.Second):
		t.Fatal("supervise ignored cancellation")
	}
	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("group after cancel = %v, want ESRCH", err)
	}
	<-time.After(1500 * time.Millisecond)
	if _, err := os.Lstat(filepath.Join(dir, "survived")); !os.IsNotExist(err) {
		t.Fatalf("backgrounded grandchild ran after cancel: %v", err)
	}
}

func TestDarwinSuperviseDeadlineReportsTimeout(t *testing.T) {
	limits, err := userDarwinLimits(300 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = userSupervise(context.Background(), limits, nil, nil, nil, nil, "/bin/sleep", "30")
	userSuperviseTestKind(t, err, "deadline")
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("deadline enforced after %v", elapsed)
	}
}

func TestDarwinSuperviseRefusesBeforeStart(t *testing.T) {
	limits, err := userDarwinLimits(0)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	marker := filepath.Join(t.TempDir(), "ran")
	touch := []string{"/usr/bin/touch", marker}
	unbounded := limits
	unbounded.Processes = 0
	inverted := limits
	inverted.Descriptors = inverted.DescriptorsMax + 1
	for name, run := range map[string]func() error{
		"canceled":        func() error { return userSupervise(canceled, limits, nil, nil, nil, nil, touch...) },
		"relative":        func() error { return userSupervise(context.Background(), limits, nil, nil, nil, nil, "touch", marker) },
		"no target":       func() error { return userSupervise(context.Background(), limits, nil, nil, nil, nil) },
		"no nproc":        func() error { return userSupervise(context.Background(), unbounded, nil, nil, nil, nil, touch...) },
		"soft above hard": func() error { return userSupervise(context.Background(), inverted, nil, nil, nil, nil, touch...) },
		// An exported bash function named ulimit would silently replace the builtin.
		"exported function": func() error {
			return userSupervise(context.Background(), limits, []string{"BASH_FUNC_ulimit%%=() { :; }"}, nil, nil, nil, touch...)
		},
		"startup file": func() error {
			return userSupervise(context.Background(), limits, []string{"BASH_ENV=/dev/null"}, nil, nil, nil, touch...)
		},
		// Older bash exports functions as __BASH_FUNC<name>()=...
		"legacy exported function": func() error {
			return userSupervise(context.Background(), limits, []string{"__BASH_FUNC<ulimit>()=() { :; }"}, nil, nil, nil, touch...)
		},
		"posix startup file": func() error {
			return userSupervise(context.Background(), limits, []string{"ENV=/dev/null"}, nil, nil, nil, touch...)
		},
		// Imported xtrace or errexit state changes how the wrapper chain runs.
		"shell options": func() error {
			return userSupervise(context.Background(), limits, []string{"SHELLOPTS=xtrace"}, nil, nil, nil, touch...)
		},
		"bash options": func() error {
			return userSupervise(context.Background(), limits, []string{"BASHOPTS=expand_aliases"}, nil, nil, nil, touch...)
		},
	} {
		if err := run(); err == nil {
			t.Fatalf("%s: supervise must refuse", name)
		}
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("refused supervision ran the command: %v", err)
	}
}

// userRefusableAbove returns a limit an unprivileged process cannot raise to
// from hard, or false when hard is already unlimited: RLIM_INFINITY plus any
// headroom is no longer a finite limit.
func userRefusableAbove(hard uint64) (uint64, bool) {
	if hard >= unix.RLIM_INFINITY-1000 {
		return 0, false
	}
	return hard + 1000, true
}

func TestDarwinRefusableLimitAvoidsInfinity(t *testing.T) {
	for _, hard := range []uint64{unix.RLIM_INFINITY, unix.RLIM_INFINITY - 1, ^uint64(0)} {
		if value, ok := userRefusableAbove(hard); ok {
			t.Fatalf("hard limit %#x has no refusable value above it, got %#x", hard, value)
		}
	}
	if value, ok := userRefusableAbove(2666); !ok || value != 3666 {
		t.Fatalf("finite hard limit 2666 = %d, %v", value, ok)
	}
}

func TestDarwinSuperviseWrapperFailsClosedOnRefusedLimit(t *testing.T) {
	limits, err := userDarwinLimits(0)
	if err != nil {
		t.Fatal(err)
	}
	var nproc, nofile unix.Rlimit
	if err := errors.Join(unix.Getrlimit(unix.RLIMIT_NPROC, &nproc), unix.Getrlimit(unix.RLIMIT_NOFILE, &nofile)); err != nil {
		t.Fatal(err)
	}
	// An unprivileged process cannot raise a hard limit; only a finite one has
	// a value above it.
	if value, ok := userRefusableAbove(nproc.Max); ok {
		limits.Processes = value
	} else if value, ok := userRefusableAbove(nofile.Max); ok {
		limits.DescriptorsMax = value
	} else {
		t.Skip("RLIMIT_NPROC and RLIMIT_NOFILE hard limits are unlimited; no limit can be refused")
	}
	marker := filepath.Join(t.TempDir(), "ran")
	var stderr bytes.Buffer
	err = userSupervise(context.Background(), limits, nil, nil, nil, &stderr, "/usr/bin/touch", marker)
	var exit *exec.ExitError
	if userSuperviseTestKind(t, err, "refused"); !errors.As(err, &exit) || exit.ExitCode() != 125 {
		t.Fatalf("refused ulimit = %v, want a refusal wrapping wrapper exit 125; stderr %q", err, stderr.String())
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran without its limits: %v", err)
	}
}

// Exit 125 is also an ordinary command status: once the wrapper attested its
// limits, the command's own 125 passes through unchanged.
func TestDarwinSuperviseCommandExit125IsNotARefusal(t *testing.T) {
	limits, err := userDarwinLimits(0)
	if err != nil {
		t.Fatal(err)
	}
	err = userSupervise(context.Background(), limits, nil, nil, nil, nil, "/bin/sh", "-c", "exit 125")
	var exit *exec.ExitError
	var failure *PrivateRuntimeError
	if !errors.As(err, &exit) || exit.ExitCode() != 125 || errors.As(err, &failure) {
		t.Fatalf("command exit 125 = %v, want the plain command status", err)
	}
	// The attestation pipe reaches the command under no descriptor number;
	// bash 3.2 would keep a copy as descriptor 10 after `exec "$@" 3>&-`.
	var stdout bytes.Buffer
	probe := `fd=3; while [ $fd -lt 256 ]; do if [ -e /dev/fd/$fd ]; then echo $fd; fi; fd=$((fd+1)); done`
	if err := userSupervise(context.Background(), limits, nil, nil, &stdout, nil, "/bin/sh", "-c", probe); err != nil || stdout.Len() != 0 {
		t.Fatalf("command inherited descriptors %q: %v", stdout.String(), err)
	}
}

// Documented limitation: the darwin kernel refuses to lower the data and
// address-space limits, so supervision has no memory cap.
func TestDarwinMemoryLimitsCannotBeLowered(t *testing.T) {
	for _, flag := range []string{"-d", "-v"} {
		output, err := exec.Command("/bin/sh", "-c", "ulimit -S "+flag+" 3145728").CombinedOutput()
		if err == nil {
			t.Fatalf("ulimit %s lowered on this kernel; update userSupervisionLimitation and enforce it: %q", flag, output)
		}
	}
	if !strings.Contains(userSupervisionLimitation, "no memory cap") {
		t.Fatalf("limitation must state the missing memory cap: %q", userSupervisionLimitation)
	}
}

// TestDarwinSetsidEscapeFixture is a grandchild that leaves the supervised
// process group with setsid; it runs only under the escape test below.
func TestDarwinSetsidEscapeFixture(t *testing.T) {
	dir := os.Getenv("GENTLE_SHELL_SETSID_DIR")
	if dir == "" {
		return
	}
	if _, err := syscall.Setsid(); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(filepath.Join(dir, "escaped"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0600); err != nil {
		os.Exit(3)
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

// The documented darwin limitation: a descendant that calls setsid leaves the
// owned process group, so group kill and reap neither see nor contain it.
func TestDarwinSuperviseSetsidDescendantIsNotContained(t *testing.T) {
	dir := t.TempDir()
	self := userSuperviseTestHelper(t)
	limits, err := userDarwinLimits(0)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- userSupervise(ctx, limits, []string{"PATH=/usr/bin:/bin", "GENTLE_SHELL_SETSID_DIR=" + dir}, nil, nil, nil,
			"/bin/bash", "-c", `"$2" -test.run='^TestDarwinSetsidEscapeFixture$' </dev/null >/dev/null 2>&1 & echo $$ > "$1/pid"; sleep 30`, "bash", dir, self)
	}()
	pid := userSuperviseTestFile(t, filepath.Join(dir, "pid"))
	escaped := userSuperviseTestFile(t, filepath.Join(dir, "escaped"))
	t.Cleanup(func() { _ = syscall.Kill(escaped, syscall.SIGKILL) })
	cancel()
	select {
	case err := <-done:
		userSuperviseTestKind(t, err, "canceled")
	case <-time.After(10 * time.Second):
		t.Fatal("supervise ignored cancellation")
	}
	if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("owned group after cancel = %v, want ESRCH", err)
	}
	if err := syscall.Kill(escaped, 0); err != nil {
		t.Fatalf("setsid descendant was contained after all (%v); update userSupervisionLimitation", err)
	}
	if group, err := syscall.Getpgid(escaped); err != nil || group == pid {
		t.Fatalf("setsid descendant group = %d, %v; want a group other than %d", group, err, pid)
	}
	if err := syscall.Kill(escaped, syscall.SIGKILL); err != nil {
		t.Fatalf("cleanup of escaped descendant: %v", err)
	}
}
