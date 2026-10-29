//go:build linux

package shellinstaller

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// userKernelFields parses /proc/self/status exactly once and refuses any
// duplicated key, so a spoofed or malformed status cannot smuggle a second
// definition past the identity checks.
func userKernelFields(status string) (map[string]string, error) {
	fields := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			if _, duplicate := fields[key]; duplicate {
				return nil, privateError("unavailable", nil)
			}
			fields[key] = strings.TrimSpace(value)
		}
	}
	return fields, nil
}

// userKernelIdentity enforces cleared capabilities, one all-own Uid row and
// NoNewPrivs. Root with the explicit acceptance binding reuses the same
// physical checks; nothing else is relaxed for it.
func userKernelIdentity(fields map[string]string, uid int) error {
	for _, key := range []string{"CapInh", "CapPrm", "CapEff", "CapAmb"} {
		if fields[key] != "0000000000000000" {
			return privateError("unavailable", fmt.Errorf("%s must be zero", key))
		}
	}
	ids := strings.Fields(fields["Uid"])
	if len(ids) != 4 || strings.Join(ids, ":") != strings.Repeat(strconv.Itoa(uid)+":", 3)+strconv.Itoa(uid) || fields["NoNewPrivs"] != "1" {
		return privateError("unavailable", errors.New("UID or NoNewPrivs differs"))
	}
	return nil
}

func userKernelStatus(status string, uid int) error {
	if uid <= 0 || runtime.GOARCH != "amd64" {
		return privateError("unavailable", errors.New("Linux amd64 non-root user required"))
	}
	fields, err := userKernelFields(status)
	if err != nil {
		return err
	}
	return userKernelIdentity(fields, uid)
}

// userKernelStatusRoot applies the identical physical checks to a root
// process that already carries the explicit acceptance binding. It is only
// reachable through the entry kernel checks after RootRiskAccepted() held.
func userKernelStatusRoot(status string) error {
	if runtime.GOARCH != "amd64" {
		return privateError("unavailable", errors.New("Linux amd64 non-root user required"))
	}
	fields, err := userKernelFields(status)
	if err != nil {
		return err
	}
	return userKernelIdentity(fields, 0)
}

// userEntryKernelStatus routes one entry kernel check: a non-root process
// keeps the strict path; root needs the binding first and then the identical
// physical checks.
func userEntryKernelStatus(status string) error {
	return userEntryKernelStatusUID(status, os.Getuid())
}

func userEntryKernelStatusUID(status string, uid int) error {
	if uid == 0 {
		if !RootRiskAccepted() {
			return &RootRiskRequiredError{}
		}
		return userKernelStatusRoot(status)
	}
	return userKernelStatus(status, uid)
}

func UserKernelCheck() error {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return err
	}
	if err := userEntryKernelStatus(string(status)); err != nil {
		return err
	}
	return privateKernel() // Real mount, membership, cgroup2 statfs and exact leaf limits.
}

func ValidateUserInstall(req UserInstallRequest) error {
	return userValidateInstall(req)
}

func InspectUserInstall(req UserInstallRequest) (string, error) {
	return userInspectInstall(req)
}

// userRuntimeDirEnv returns XDG_RUNTIME_DIR without one trailing slash, the
// form WSL2 exports ("/run/user/UID/"). Every other spelling is kept as given,
// so callers still refuse anything but the exact runtime directory.
func userRuntimeDirEnv() string {
	value := os.Getenv("XDG_RUNTIME_DIR")
	if len(value) > 1 {
		value = strings.TrimSuffix(value, "/")
	}
	return value
}

func userBusEnvironment() ([]string, error) {
	runtimeDir := "/run/user/" + strconv.Itoa(os.Getuid())
	if userRuntimeDirEnv() != runtimeDir {
		return nil, errors.New("set XDG_RUNTIME_DIR to your existing /run/user/UID; delegated user manager required")
	}
	if _, err := privateDirectory(runtimeDir); err != nil {
		return nil, err
	}
	bus := filepath.Join(runtimeDir, "bus")
	info, err := os.Lstat(bus)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return nil, errors.New("existing user-manager bus unavailable; no delegation is created automatically")
	}
	home := os.Getenv("HOME")
	if err := privateNativeHome(home); err != nil {
		return nil, err
	}
	return []string{"HOME=" + home, "XDG_RUNTIME_DIR=" + runtimeDir, "DBUS_SESSION_BUS_ADDRESS=unix:path=" + bus, "PATH=/usr/bin:/bin", "TERM=xterm-256color"}, nil
}

func userManagerProbe(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "/usr/bin/systemctl", "--user", "show", "--property=Version", "--value")
}

func userManagerVersion(version []byte) error {
	refused := errors.New("existing systemd user manager version >=254 required")
	value := strings.TrimSuffix(string(version), "\n")
	if value == "" || len(version) > 4096 {
		return refused
	}
	end := strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(value)
	}
	if end == 0 {
		return refused
	}
	if end < len(value) {
		if !strings.ContainsRune(".~+-", rune(value[end])) || end+1 == len(value) {
			return refused
		}
		if strings.IndexFunc(value[end+1:], func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._~+-", r))
		}) != -1 {
			return refused
		}
	}
	number, err := strconv.Atoi(value[:end])
	if err != nil || number < 254 {
		return refused
	}
	return nil
}

type userServicePlan struct {
	arguments func(string, bool, string, string, []string, []string) ([]string, error)
	cancel    func() error
}

func userService(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return userServiceWithPlan(ctx, self, args, stdin, stdout, stderr, nil)
}

func userServiceWithPlan(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer, plan *userServicePlan) (resultErr error) {
	if ctx == nil || ctx.Err() != nil || os.Getuid() == 0 {
		return privateError("unavailable", errors.New("existing delegated non-root user manager required"))
	}
	interactive, err := userInteractive(stdin)
	if err != nil {
		return err
	}
	if _, err := userSupervisorSHA(ctx, self); err != nil {
		return err
	}
	// Drop inherited/ambient capabilities before starting Go, not on one of its threads.
	if _, err := userSupervisorSHA(ctx, userCapabilityDrop); err != nil {
		return fmt.Errorf("qualify capability-clearing helper %s: %w", userCapabilityDrop, err)
	}
	env, err := userBusEnvironment()
	if err != nil {
		return err
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
	defer probeCancel()
	probe := userManagerProbe(probeCtx)
	probe.Env = env
	version, err := probe.Output()
	if err != nil {
		return fmt.Errorf("read existing systemd user manager version: %w", err)
	}
	if err := userManagerVersion(version); err != nil {
		return err
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	unit := fmt.Sprintf("gentle-shell-%x", nonce)
	receiptDir, err := os.MkdirTemp("/tmp", ".gentle-unit-")
	if err != nil {
		return err
	}
	receiptIdentity, err := privateDirectory(receiptDir)
	if err != nil {
		return err
	}
	terminated := false
	defer func() {
		if !terminated {
			failure := privateError("uncertain", resultErr)
			failure.Workspace, failure.Destination = receiptDir, unit
			resultErr = failure
			return
		}
		if cleanupErr := privateCleanup(receiptDir, receiptIdentity); cleanupErr != nil {
			failure := privateError("uncertain", errors.Join(resultErr, cleanupErr))
			failure.Workspace, failure.Destination = receiptDir, unit
			resultErr = failure
		}
	}()
	env = append(env, "GENTLE_SHELL_UNIT="+unit, "GENTLE_SHELL_UNIT_RECEIPT="+filepath.Join(receiptDir, "kernel.json"))
	env = append(env, userProgressEnvironment(ctx)...)
	// The unit sees only this list: forward the registry, reconfigure and
	// Main fallback intents explicitly or the worker silently loses them.
	env = append(env, userServiceIntentEnvironment(os.Getenv("HOME"))...)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	commandArgs := userServiceArgs(unit, interactive, self, cwd, args, env)
	if plan != nil {
		if plan.arguments == nil {
			return privateError("refused", errors.New("service argument builder unavailable"))
		}
		commandArgs, err = plan.arguments(unit, interactive, self, cwd, args, env)
		if err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", commandArgs...)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = env, stdin, stdout, stderr
	cmd.WaitDelay = 3 * time.Second
	cmd.Cancel = func() error {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		stop := exec.CommandContext(stopCtx, "/usr/bin/systemctl", "--user", "stop", unit)
		stop.Env = env
		stop.Stdout, stop.Stderr = io.Discard, io.Discard
		if err := stop.Run(); err != nil {
			return privateError("uncertain", errors.New("unit stop failed; preserve owned files and inspect unit"))
		}
		return nil
	}
	if plan != nil && plan.cancel != nil {
		cmd.Cancel = plan.cancel
	}
	runErr := cmd.Run()
	readbackErr := userUnitReadback(filepath.Join(receiptDir, "kernel.json"))
	terminated = readbackErr == nil
	return userServiceResult(runErr, readbackErr)
}

func userServiceResult(runErr, readbackErr error) error {
	if readbackErr != nil {
		return privateError("uncertain", errors.Join(runErr, readbackErr))
	}
	if runErr == nil {
		return nil
	}
	var child *exec.ExitError
	if errors.As(runErr, &child) && child.ProcessState != nil && !errors.Is(runErr, exec.ErrWaitDelay) {
		return runErr // Known child status after physical termination readback.
	}
	return privateError("uncertain", runErr)
}

type userUnitEvidence struct {
	Path     string
	Dev, Ino uint64
}

func userRecordUnit() error {
	unit, receipt := os.Getenv("GENTLE_SHELL_UNIT"), os.Getenv("GENTLE_SHELL_UNIT_RECEIPT")
	if unit == "" && receipt == "" {
		return nil
	}
	if !strings.HasPrefix(unit, "gentle-shell-") || len(unit) != len("gentle-shell-")+16 || filepath.Base(receipt) != "kernel.json" {
		return errors.New("invalid unit receipt selection")
	}
	if _, err := privateDirectory(filepath.Dir(receipt)); err != nil {
		return err
	}
	membership, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return err
	}
	line := strings.TrimSpace(string(membership))
	if !strings.HasPrefix(line, "0::/") || !strings.HasSuffix(line, "/"+unit+".service") || strings.Contains(line, "\n") {
		return errors.New("process is not in the selected manager unit")
	}
	group := "/sys/fs/cgroup" + strings.TrimPrefix(line, "0::")
	info, err := os.Stat(group)
	var filesystem unix.Statfs_t
	if err != nil || unix.Statfs(group, &filesystem) != nil || filesystem.Type != unix.CGROUP2_SUPER_MAGIC {
		return errors.New("unit cgroup is not physical cgroup2")
	}
	st := info.Sys().(*syscall.Stat_t)
	data, err := json.Marshal(userUnitEvidence{group, st.Dev, st.Ino})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(receipt, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

// userKernelMarker is the failure marker written in the unit receipt directory
// when the inner kernel check fails: no receipt can exist in that case, so the
// waiting outer process reads this marker to replace the generic uncertain
// readback with the actual cause (#5713).
const userKernelMarker = "kernel-check-failed"

// userWriteKernelMarker records one kernel check failure for the outer
// process. It stays inert without a delegated receipt selection and reports
// write failures to the caller; the entry never upgrades them to refusals.
func userWriteKernelMarker(cause error) error {
	receipt := os.Getenv("GENTLE_SHELL_UNIT_RECEIPT")
	if receipt == "" || filepath.Base(receipt) != "kernel.json" || cause == nil {
		return nil
	}
	dir := filepath.Dir(receipt)
	if _, err := privateDirectory(dir); err != nil {
		return nil
	}
	text := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, strings.TrimSpace(cause.Error()))
	if text == "" || len(text) > 4096 {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(dir, userKernelMarker), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write([]byte(text))
	return errors.Join(writeErr, file.Close())
}

// userKernelMarkerCause reads the inner unit's kernel check failure marker;
// an absent, oversized or non-printable marker keeps the generic message.
func userKernelMarkerCause(receipt string) string {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(receipt), userKernelMarker))
	if err != nil || len(data) > 4096 {
		return ""
	}
	cause := strings.TrimSpace(string(data))
	for _, r := range cause {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return cause
}

// RunUserLaunchProbe exercises the delegated unit path exactly as a later
// launch would: one short-lived unit that must pass the kernel check and
// terminate with a physical receipt. It reuses the ordinary unit argument
// builder and, since #5713, reports the inner check failure cause.
func RunUserLaunchProbe(ctx context.Context, self string) error {
	if os.Getuid() == 0 {
		return rootSystemService(ctx, self, userLaunchProbeArgs(), nil, io.Discard, io.Discard)
	}
	return userService(ctx, self, userLaunchProbeArgs(), nil, io.Discard, io.Discard)
}

func userLaunchProbeArgs() []string {
	return []string{"internal-check"}
}

func userUnitReadback(receipt string) error {
	var evidence userUnitEvidence
	data, err := os.ReadFile(receipt)
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &evidence) != nil || !strings.HasPrefix(evidence.Path, "/sys/fs/cgroup/") || filepath.Clean(evidence.Path) != evidence.Path {
		if cause := userKernelMarkerCause(receipt); cause != "" {
			return fmt.Errorf("unit kernel check failed: %s", cause)
		}
		return errors.New("missing physical unit receipt; termination uncertain")
	}
	info, err := os.Stat(evidence.Path)
	if os.IsNotExist(err) {
		return nil // Kernel cgroup removal requires an empty owned subtree.
	}
	if err != nil {
		return err
	}
	st := info.Sys().(*syscall.Stat_t)
	if st.Dev != evidence.Dev || st.Ino != evidence.Ino {
		return errors.New("unit cgroup identity changed")
	}
	events, eventErr := os.ReadFile(filepath.Join(evidence.Path, "cgroup.events"))
	pids, pidErr := os.ReadFile(filepath.Join(evidence.Path, "pids.current"))
	if eventErr != nil || pidErr != nil || !strings.Contains(string(events), "populated 0\n") || strings.TrimSpace(string(pids)) != "0" {
		return errors.New("owned unit subtree still populated or unreadable")
	}
	return nil
}

// Inner selectors prevent recursion only. They never authorize execution:
// every internal operation repeats the kernel and physical selection checks.
func RunUserEntry(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing shell entry")
	}
	if args[0] == "internal-native-own" {
		return userNativeOwnerEntry(ctx, self, args[1:], stdin, stdout, stderr)
	}
	if args[0] == "internal-install-direct" {
		req, err := UserInstallFromEntry(args[1:])
		if err != nil {
			return err
		}
		result, err := RunUserInstallDirect(ctx, req)
		if err == nil {
			if _, err = fmt.Fprintf(stdout, "Installed %s; commands %s/bin/gentle-shell and %s/bin/pi\n", result.Destination, result.Destination, result.Destination); err != nil {
				return err
			}
			if result.LauncherNote != "" {
				if _, err = fmt.Fprintf(stdout, "%s\n", result.LauncherNote); err != nil {
					return err
				}
			}
		}
		return err
	}
	if args[0] == "install" {
		req, fresh, err := userNativeFreshRequest(args[1:])
		if err != nil {
			return err
		}
		if userNativeInstallDispatch(fresh) {
			return userNativeInstall(ctx, self, args[1:], req, stdin, stdout, stderr)
		}
	}
	inner := strings.HasPrefix(args[0], "internal-")
	if err := UserKernelCheck(); err != nil {
		if AsRootRiskRequired(err) {
			// The informed root refusal is final: never delegate a run the
			// operator has not explicitly accepted.
			if inner {
				_ = userWriteKernelMarker(err)
			}
			return err
		}
		if inner {
			// The unit cannot return a receipt without the kernel check passing;
			// record the cause for the waiting outer process before returning.
			_ = userWriteKernelMarker(err)
			return err
		}
		forward := append([]string{"internal-" + args[0]}, args[1:]...)
		if os.Getuid() == 0 {
			// The accepted root run delegates to the system manager; there is
			// no user manager to inherit and none is created.
			return rootSystemService(ctx, self, forward, stdin, stdout, stderr)
		}
		return userService(ctx, self, forward, stdin, stdout, stderr)
	}
	if err := userRecordUnit(); err != nil {
		return err
	}
	switch strings.TrimPrefix(args[0], "internal-") {
	case "check":
		return nil
	case "install":
		req, err := UserInstallFromEntry(args[1:])
		if err != nil {
			return err
		}
		result, err := RunUserInstall(ctx, req)
		if err == nil {
			if _, err = fmt.Fprintf(stdout, "Installed %s; commands %s/bin/gentle-shell and %s/bin/pi\n", result.Destination, result.Destination, result.Destination); err != nil {
				return err
			}
			if result.LauncherNote != "" {
				if _, err = fmt.Fprintf(stdout, "%s\n", result.LauncherNote); err != nil {
					return err
				}
			}
		}
		return err
	case "native-coordinate":
		return userNativeCoordinate(ctx, self, args[1:], stdin, stdout, stderr)
	case "native-produce":
		return userNativeChild(ctx, args[1:], stdout)
	case "recover":
		return userRecover(ctx, args[1:], stdout)
	case "launch-pnpm":
		if len(args) < 3 {
			return errors.New("missing owned PNPM launch root or role")
		}
		return userLaunchPNPM(ctx, args[1], args[2], args[3:], stdin, stdout, stderr)
	case "launch":
		if len(args) < 2 {
			return errors.New("missing owned launch root")
		}
		return userLaunch(ctx, args[1], args[2:], stdin, stdout, stderr)
	default:
		return errors.New("unknown shell internal entry")
	}
}

func RunUserInstall(ctx context.Context, req UserInstallRequest) (UserInstallResult, error) {
	return userRunInstall(ctx, req)
}

// RunUserInstallDirect is the portable user-level installer: no transient
// service, no cgroup claim and no process replacement. It keeps the user
// identity, no-new-privileges, verified artifacts and atomic publication.
func RunUserInstallDirect(ctx context.Context, req UserInstallRequest) (result UserInstallResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return UserInstallResult{}, privateError("refused", err)
	}
	return userRunInstall(context.WithValue(ctx, userDirectInstallKey{}, true), req)
}

func userDirectKernelCheck() error {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return err
	}
	return userEntryKernelStatus(string(status))
}

const userBinarySize int64 = 18559160
const userBinarySHA = "9212c7b16ea9abfe93e3a16b29d8390e7762b5c07730cf05df1f3e7892ff96a4"
const userNativeManifest = `{"version":"4.2.0","asset":"gentle-ai_4.2.0_linux_amd64.tar.gz","assetSha256":"552c1642fd10f5d96cfe9bb1cc3b66b30eb47d3074cc7ca0559b598eb086defc","binarySha256":"9212c7b16ea9abfe93e3a16b29d8390e7762b5c07730cf05df1f3e7892ff96a4"}` + "\n"

// The bootstrap publishes Node itself with mv -T --no-clobber.
func userBootstrapPublish(context.Context, string, string) error {
	return nil
}

// Owned installer commands already run inside the qualified unit.
func userOwnedRun(ctx context.Context, cmd *exec.Cmd, cancel context.CancelFunc) (string, error) {
	return privateRun(ctx, cmd, cancel)
}

// The launched Pi already runs inside the qualified unit's limits.
func userLaunchLimits(*exec.Cmd) (func(error) error, error) {
	return func(err error) error { return err }, nil
}

// /proc/self/exe already names the physical executable.
func userExecutable() (string, error) {
	return os.Executable()
}

var userToolSources = []userToolSource{
	{"fd", "sharkdp/fd", "v10.5.0", "fd-v10.5.0-x86_64-unknown-linux-musl", "761c72dc8e120d85b22292063be8a796e2eeb20eb3e4f38b8fa2343ccf3514a7", 1573549},
	{"rg", "BurntSushi/ripgrep", "15.2.0", "ripgrep-15.2.0-x86_64-unknown-linux-musl", "33e15bcf1624b25cdd2a55813a47a2f95dbe126268203e76aa6a585d1e7b149c", 2265718},
}

// userReapProbe only checks for members: Linux delivers a group SIGKILL to a
// member forked concurrently with it.
const userReapProbe syscall.Signal = 0

// userKillGroup signals every member of process group pgid. Linux counts
// zombie members, so a zombie-only group answers 0 until it is reaped.
func userKillGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(-pgid, sig)
}

// privateReapRun is privateRun's post-Wait cleanup: one group SIGKILL, whose
// ESRCH answer means the group already emptied.
func privateReapRun(_ int, _ error, kill func() error) error {
	return kill()
}

// userNativeInstallEnabled gates the native coordinator for fresh Separate
// installs. Its guards require an empty capability bounding set and no
// supplementary groups, which an ordinary user under a systemd user manager
// cannot meet: the coordinator unit exits 218 on CapabilityBoundingSet=, and
// the namespace guard refuses the user's groups. Fresh installs therefore keep
// the legacy worker, which installs end to end, until the native path supports
// ordinary users. A variable rather than a constant keeps the native path
// reachable for its own tests and the dead-code ratchet.
var userNativeInstallEnabled = false

// userNativeInstallDispatch reports whether an install takes the native
// coordinator instead of the legacy worker.
func userNativeInstallDispatch(fresh bool) bool {
	return fresh && userNativeInstallEnabled
}
