//go:build linux

package shellinstaller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
	"golang.org/x/sys/unix"
)

func userKernelStatus(status string, uid int) error {
	if uid <= 0 || runtime.GOARCH != "amd64" {
		return privateError("unavailable", errors.New("Linux amd64 non-root user required"))
	}
	fields := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			if _, duplicate := fields[key]; duplicate {
				return privateError("unavailable", nil)
			}
			fields[key] = strings.TrimSpace(value)
		}
	}
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

func UserKernelCheck() error {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return err
	}
	if err := userKernelStatus(string(status), os.Getuid()); err != nil {
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

func userBusEnvironment() ([]string, error) {
	runtimeDir := "/run/user/" + strconv.Itoa(os.Getuid())
	if os.Getenv("XDG_RUNTIME_DIR") != runtimeDir {
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
	if err := privateNativeDirectory(home); err != nil {
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

func userService(ctx context.Context, self string, args []string, stdin io.Reader, stdout, stderr io.Writer) (resultErr error) {
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
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", userServiceArgs(unit, interactive, self, cwd, args, env)...)
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

func userUnitReadback(receipt string) error {
	var evidence userUnitEvidence
	data, err := os.ReadFile(receipt)
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &evidence) != nil || !strings.HasPrefix(evidence.Path, "/sys/fs/cgroup/") || filepath.Clean(evidence.Path) != evidence.Path {
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
	inner := strings.HasPrefix(args[0], "internal-")
	if err := UserKernelCheck(); err != nil {
		if inner {
			return err
		}
		forward := append([]string{"internal-" + args[0]}, args[1:]...)
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
			_, err = fmt.Fprintf(stdout, "Installed %s; commands %s/bin/gentle-shell and %s/bin/pi\n", result.Destination, result.Destination, result.Destination)
		}
		return err
	case "recover":
		return userRecover(ctx, args[1:], stdout)
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

func userRecover(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) != 2 || !privateHierarchyPath(args[0]) {
		return errors.New("recovery: supply ROOT and inspect or printed confirmation")
	}
	root := args[0]
	identity, err := userIdentity(root)
	if err != nil {
		return err
	}
	selectionPath := filepath.Join(root, "state/selection.json")
	upgrade := filepath.Join(root, "state/upgrade/selection.json")
	if _, statErr := os.Lstat(upgrade); statErr == nil {
		selectionPath = upgrade
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	selectionInfo, err := privatePhysical(selectionPath)
	if err != nil || selectionInfo.Size() > 4096 || selectionInfo.Mode().Perm()&0022 != 0 || selectionInfo.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return errors.New("recovery selection is absent or malformed; preserve evidence")
	}
	data, err := os.ReadFile(selectionPath)
	var selection struct{ Mode, Prefix, Agent, FinalPrefix, FinalRoot, PrefixSHA, AgentSHA string }
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &selection) != nil || selection.Mode != "shared" || !privateHierarchyPath(selection.Prefix) || !privateHierarchyPath(selection.Agent) || !privateHierarchyPath(selection.FinalRoot) || !privateHierarchyPath(selection.FinalPrefix) || selection.FinalPrefix != selection.Prefix {
		return errors.New("recovery selection is absent or malformed; preserve evidence")
	}
	for _, digest := range []string{selection.PrefixSHA, selection.AgentSHA} {
		if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
			return errors.New("recovery preimage hashes are absent or malformed; preserve evidence")
		}
	}
	binding := map[string]string{"selectionSHA": fmt.Sprintf("%x", sha256.Sum256(data))}
	for key, selected := range map[string]string{"rootID": root, "prefixID": selection.Prefix, "agentID": selection.Agent} {
		id, err := userRecoveryID(selected)
		if err != nil {
			return err
		}
		binding[key] = id
	}
	for _, name := range []string{"prefix.preimage", "agent.preimage"} {
		stamp, err := userTreeStamp(filepath.Join(filepath.Dir(selectionPath), name))
		if err != nil {
			return err
		}
		identity += ":" + stamp
	}
	identity += ":" + binding["prefixID"] + ":" + binding["agentID"]
	token := fmt.Sprintf("%x", sha256.Sum256(append(data, []byte(identity)...)))
	if args[1] == "inspect" {
		_, err := fmt.Fprintf(stdout, "Recovery confirmation: %s\nSelected prefix: %q\nSelected agent: %q\n", token, selection.Prefix, selection.Agent)
		return err
	}
	if args[1] != token {
		return errors.New("fresh recovery confirmation differs")
	}
	node := filepath.Join(root, "runtime/node/bin/node")
	if _, err := privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return err
	}
	helperBytes, err := assets.ReadUserHelper()
	if err != nil {
		return err
	}
	helper := filepath.Join(root, "provision.mjs")
	if _, err := privateNativeFile(ctx, helper, 0400, int64(len(helperBytes)), fmt.Sprintf("%x", sha256.Sum256(helperBytes))); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	authority, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, node, helper, root, selection.Prefix, selection.Agent, selection.FinalPrefix, selection.FinalRoot, "shared", "restore", string(authority))
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, selection.Prefix, selection.Agent)
	_, err = privateRun(ctx, cmd, cancel)
	if err != nil {
		failure := privateError("uncertain", err)
		failure.Workspace, failure.Destination = root, selection.Prefix
		return failure // The restore command may already have moved shared data.
	}
	_, err = fmt.Fprintf(stdout, "Restored selected shared preimages; preserve recovery evidence at %q\n", root)
	return err
}

const userBinarySize int64 = 17109176
const userBinarySHA = "50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d"
const userNativeManifest = `{"version":"4.0.0","asset":"gentle-ai_4.0.0_linux_amd64.tar.gz","assetSha256":"5f4417cf29c969c86da4799942fd673368840901be1bb09c779a12d7ed6096ea","binarySha256":"50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d"}` + "\n"

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
