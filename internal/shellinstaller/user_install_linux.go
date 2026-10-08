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
	"net/http"
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
	if !userSelectionPath(req.Destination) || (req.Mode != "separate" && req.Mode != "shared") {
		return privateError("refused", errors.New("choose separate or shared mode and a canonical absolute target using only ASCII letters, digits, /, _, . or -"))
	}
	parent, err := privateDirectory(filepath.Dir(req.Destination))
	if err != nil {
		return err
	}
	fresh := true
	if err := privateDestination(req.Destination); err != nil {
		fresh = false
		manifest, readErr := userReadManifest(context.Background(), req.Destination)
		if readErr != nil || manifest.Mode != req.Mode || (req.Mode == "shared" && (manifest.Prefix != req.SharedPrefix || manifest.Agent != req.SharedAgent)) {
			return err
		}
	}
	if req.Mode == "separate" {
		if req.SharedPrefix != "" || req.SharedAgent != "" {
			return privateError("refused", errors.New("separate mode cannot select shared paths"))
		}
		return nil
	}
	if userPathsOverlap(req.SharedPrefix, req.SharedAgent) {
		return errors.New("shared prefix and agent must be disjoint")
	}
	for _, path := range []string{req.SharedPrefix, req.SharedAgent} {
		if !userSelectionPath(path) || userPathsOverlap(path, req.Destination) {
			return privateError("refused", errors.New("shared paths must use the supported target character set and remain disjoint from the target in both directions"))
		}
		selected, err := privateDirectory(path)
		if err != nil {
			return err
		}
		if selected.Sys().(*syscall.Stat_t).Dev != parent.Sys().(*syscall.Stat_t).Dev {
			return errors.New("shared recovery requires one physical filesystem")
		}
	}
	if fresh {
		if err := userToolBinAvailable(req.SharedAgent); err != nil {
			return err
		}
	}
	_, err = privatePhysical(filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"))
	return err
}

func InspectUserInstall(req UserInstallRequest) (string, error) {
	if err := ValidateUserInstall(req); err != nil {
		return "", err
	}
	if _, statErr := os.Lstat(req.Destination); statErr == nil {
		data, err := os.ReadFile(filepath.Join(req.Destination, "installation.json"))
		if err != nil {
			return "", err
		}
		identity, err := userIdentity(req.Destination)
		return userConfirmation(req, identity+fmt.Sprintf("%x", sha256.Sum256(data))), err
	}
	identity, err := userIdentity(filepath.Dir(req.Destination))
	if err != nil {
		return "", err
	}
	if req.Mode == "shared" {
		var stamps []string
		for pass := 0; pass < 2; pass++ {
			for index, path := range []string{req.SharedPrefix, req.SharedAgent} {
				stamp, inspectErr := userTreeStamp(path)
				if inspectErr != nil {
					return "", inspectErr
				}
				if pass == 0 {
					stamps = append(stamps, stamp)
					identity += ":" + stamp
				} else if stamps[index] != stamp {
					return "", errors.New("shared selection changed between inspections")
				}
			}
		}
	}
	return userConfirmation(req, identity), nil
}

func userEnvironment(root, prefix, agent string) []string {
	return []string{"HOME=" + root + "/home", "TMPDIR=" + root + "/tmp", "XDG_CONFIG_HOME=" + root + "/config", "XDG_STATE_HOME=" + root + "/state",
		"GENTLE_PI_CONFIG_HOME=" + root + "/config", "PI_CODING_AGENT_DIR=" + agent, "GENTLE_PI_AGENT_HOME=" + agent, "GENTLE_PI_NO_SKILL_REGISTRY=1",
		"PATH=" + root + "/runtime/node/bin:/usr/bin:/bin", "NPM_CONFIG_PREFIX=" + prefix, "npm_config_prefix=" + prefix,
		"NPM_CONFIG_IGNORE_SCRIPTS=true", "npm_config_ignore_scripts=true", "NPM_CONFIG_USERCONFIG=" + root + "/config/user.npmrc",
		"NPM_CONFIG_GLOBALCONFIG=" + root + "/config/global.npmrc", "NPM_CONFIG_CACHE=" + root + "/runtime/cache", "NPM_CONFIG_AUDIT=false", "NPM_CONFIG_FUND=false", "NODE_USE_SYSTEM_CA=1"}
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

func RunUserInstall(ctx context.Context, req UserInstallRequest) (result UserInstallResult, err error) {
	if ctx == nil || ctx.Err() != nil {
		return result, privateError("canceled", context.Canceled)
	}
	if err = UserKernelCheck(); err != nil {
		return result, err
	}
	confirmation, err := InspectUserInstall(req)
	if err != nil || req.Confirmation != confirmation {
		return result, privateError("refused", errors.Join(err, errors.New("physical selection confirmation differs")))
	}
	if _, statErr := os.Lstat(req.Destination); statErr == nil {
		manifest, err := userReadManifest(ctx, req.Destination)
		if err != nil {
			return result, err
		}
		if err = userVerifyGlobal(ctx, req.Destination, manifest.Prefix, manifest.Agent, manifest.Prefix, req.Destination, req.Mode); err != nil {
			return result, userGraphRepairError(req.Destination, req.Mode, err)
		}
		if err = userNativeReadback(ctx, filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi/.gentle-ai")); err != nil {
			return result, err
		}
		return UserInstallResult{req.Destination, manifest.Prefix, manifest.Agent, "ComponentInstalled"}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 870*time.Second)
	defer cancel()
	workspace, err := os.MkdirTemp(filepath.Dir(req.Destination), ".gentle-user-")
	if err != nil {
		return result, err
	}
	identity, err := privateDirectory(workspace)
	sharedProvisioningStarted := false
	defer func() {
		result, err = userInstallFinish(ctx, result, err, workspace, identity, req, sharedProvisioningStarted)
	}()
	if err != nil {
		return result, err
	}
	root := filepath.Join(workspace, "installed")
	if err = os.Mkdir(root, 0700); err != nil {
		return result, err
	}
	for _, name := range []string{"home", "tmp", "config", "state", "agent", "project", "bin"} {
		if err = os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			return result, err
		}
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		if err = os.WriteFile(filepath.Join(root, "config", name), nil, 0600); err != nil {
			return result, err
		}
	}
	if err = userBootstrap(ctx, root, workspace); err != nil {
		return result, fmt.Errorf("runtime acquisition: %w", err)
	}
	node := filepath.Join(root, "runtime/node/bin/node")
	if _, err = privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return result, fmt.Errorf("bootstrap Node readback: %w", err)
	}
	if err = userProvisionAssets(ctx, root, true); err != nil {
		return result, err
	}
	helperPath := filepath.Join(root, "provision.mjs")
	prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
	finalPrefix, finalAgent := filepath.Join(req.Destination, "prefix"), filepath.Join(req.Destination, "agent")
	if req.Mode == "shared" {
		prefix, agent, finalPrefix, finalAgent = req.SharedPrefix, req.SharedAgent, req.SharedPrefix, req.SharedAgent
	}
	// Acquire and verify pinned fd/rg inside the private stage before any
	// selected Shared object changes; binding into AGENT/bin comes later.
	if err = userToolsAcquire(ctx, root); err != nil {
		return result, fmt.Errorf("pinned tool acquisition: %w", err)
	}
	freshConfirmation, inspectErr := InspectUserInstall(req)
	if inspectErr != nil || freshConfirmation != req.Confirmation {
		return result, privateError("preimage", errors.Join(inspectErr, errors.New("selected files changed before global provisioning")))
	}
	sharedProvisioningStarted = req.Mode == "shared"
	cmd := exec.CommandContext(ctx, node, helperPath, root, prefix, agent, finalPrefix, req.Destination, req.Mode, "install")
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, prefix, agent)
	if output, err := privateRun(ctx, cmd, cancel); err != nil {
		return result, fmt.Errorf("global provisioning: %w; bounded output: %s", err, output)
	}
	pkg := filepath.Join(prefix, "lib/node_modules/gentle-pi")
	for _, source := range []struct{ name, pin string }{{"scripts/gentle-ai-installer.mjs", userInstallerSHA}, {"runtime/gentle-ai-binary.mjs", privateNativeResolverSHA}} {
		if _, err = userSourceFile(ctx, filepath.Join(pkg, source.name), -1, source.pin); err != nil {
			return result, fmt.Errorf("authenticate native supplier %q: %w", source.name, err)
		}
	}
	cmd = exec.CommandContext(ctx, node, "--input-type=module", "-e", privateNativeInvoke)
	cmd.Dir, cmd.Env = pkg, userEnvironment(root, prefix, agent)
	if output, err := privateRun(ctx, cmd, cancel); err != nil {
		return result, fmt.Errorf("native supplier: %w; bounded output: %s", err, output)
	}
	if err = userNativeReadback(ctx, filepath.Join(pkg, ".gentle-ai")); err != nil {
		return result, fmt.Errorf("native supplier readback: %w", err)
	}
	if err = userVerifyGlobal(ctx, root, prefix, agent, finalPrefix, req.Destination, req.Mode); err != nil {
		return result, fmt.Errorf("post-native global readback: %w", err)
	}
	// A bind refusal here follows Shared provisioning: it stays uncertain and
	// keeps the staged recovery root instead of claiming an atomic rollback.
	if err = userTools(ctx, root, agent, true); err != nil {
		return result, privateError("source", err)
	}
	self, err := os.Executable()
	if err != nil {
		return result, err
	}
	selfSHA, err := userCopySupervisor(ctx, self, filepath.Join(root, "supervisor"))
	if err != nil {
		return result, fmt.Errorf("supervisor provenance: %w", err)
	}
	manifest := userManifest{Schema: userSchema, Destination: req.Destination, Prefix: finalPrefix, Agent: finalAgent, Mode: req.Mode, SupervisorSHA: selfSHA, NodeSHA: privateNativeNodeSHA}
	manifest.PrefixIdentity, err = userIdentity(prefix)
	if err == nil {
		manifest.AgentIdentity, err = userIdentity(agent)
	}
	if err != nil {
		return result, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return result, err
	}
	if err = userToolWrite(filepath.Join(root, "installation.json"), data, 0600); err != nil {
		return result, err
	}
	for _, name := range []string{"gentle-shell", "pi"} {
		binding := userBinding(req.Destination, name)
		if err = userToolWrite(filepath.Join(root, "bin", name), []byte(binding), 0700); err != nil {
			return result, err
		}
	}
	for selected, expected := range map[string]string{prefix: manifest.PrefixIdentity, agent: manifest.AgentIdentity} {
		fresh, identityErr := userIdentity(selected)
		if identityErr != nil || fresh != expected {
			return result, privateError("preimage", identityErr)
		}
	}
	if err = privateDestination(req.Destination); err != nil || ctx.Err() != nil {
		return result, privateError("preimage", errors.Join(err, ctx.Err()))
	}
	if err = userRenameNoReplace(root, req.Destination); err != nil {
		return result, err
	}
	if err = userDirectorySync(filepath.Dir(req.Destination)); err != nil {
		return result, privateError("uncertain", err)
	}
	if _, err = userReadManifest(ctx, req.Destination); err != nil {
		return result, privateError("uncertain", err)
	}
	return UserInstallResult{req.Destination, finalPrefix, finalAgent, "ComponentInstalled"}, nil
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

const userInstallerSHA = "bc2da0585026fa538f0c6ae0cf50463767c175b71d0dfdb582a88cbe894c84ca"
const userBinarySHA = "50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d"
const userNativeManifest = `{"version":"4.0.0","asset":"gentle-ai_4.0.0_linux_amd64.tar.gz","assetSha256":"5f4417cf29c969c86da4799942fd673368840901be1bb09c779a12d7ed6096ea","binarySha256":"50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d"}` + "\n"

func userBootstrap(ctx context.Context, root, workspace string) error {
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		return err
	}
	client, err := privateColdClient()
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	archive := filepath.Join(workspace, "node.tgz")
	if err := privateColdFetch(ctx, client, archive, privateColdSize, privateColdSHA); err != nil {
		return err
	}
	for _, name := range []string{"bootstrap-gentle-shell-private-node.sh", "complete-generated-lock-sri.mjs", "normalize-private-optional-platform-closure.mjs"} {
		data, err := assets.ReadPrivateHelper(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, name), data, 0400); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", filepath.Join(root, "bootstrap-gentle-shell-private-node.sh"), "--destination", filepath.Join(root, "runtime/node"), "--node-archive", archive)
	cmd.Dir, cmd.Env = root, []string{"PATH=/usr/bin:/bin", "HOME=" + root, "TMPDIR=" + root}
	if output, err := privateRun(ctx, cmd, cancel); err != nil {
		return fmt.Errorf("Node bootstrap: %w; bounded output: %s", err, output)
	}
	return nil
}

var userToolSources = []userToolSource{
	{"fd", "sharkdp/fd", "v10.5.0", "fd-v10.5.0-x86_64-unknown-linux-musl", "761c72dc8e120d85b22292063be8a796e2eeb20eb3e4f38b8fa2343ccf3514a7", 1573549},
	{"rg", "BurntSushi/ripgrep", "15.2.0", "ripgrep-15.2.0-x86_64-unknown-linux-musl", "33e15bcf1624b25cdd2a55813a47a2f95dbe126268203e76aa6a585d1e7b149c", 2265718},
}

// Selected personal tools are never replaced. AGENT/bin must be absent or an
// owned physical 0700/0755 directory without fd or rg; links are not followed.
func userToolBinAvailable(agent string) error {
	bin := filepath.Join(agent, "bin")
	if _, err := os.Lstat(bin); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return privateError("refused", fmt.Errorf("inspect agent bin %q: %w", bin, err))
	}
	if err := privateNativeDirectory(bin); err != nil {
		return privateError("refused", fmt.Errorf("agent bin %q must be absent or an owned physical 0700/0755 directory: %w", bin, err))
	}
	for _, source := range userToolSources {
		if _, err := os.Lstat(filepath.Join(bin, source.name)); !errors.Is(err, os.ErrNotExist) {
			return privateError("refused", errors.Join(fmt.Errorf("agent bin %q already contains %s; move it before Shared installation", bin, source.name), err))
		}
	}
	return nil
}

// Pinned archives land only in the private stage. Nothing here touches the
// selected agent, so acquisition failure precedes every Shared mutation.
func userToolsAcquire(ctx context.Context, root string) error {
	archives := filepath.Join(root, "runtime/tools")
	if err := os.Mkdir(archives, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := privateNativeDirectory(archives); err != nil {
		return err
	}
	client, err := privateColdClient()
	if err != nil {
		return err
	}
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "" // Verify each actual HTTPS host.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Fragment != "" || req.URL.Host != "release-assets.githubusercontent.com" {
			return privateError("acquisition", nil)
		}
		return nil
	}
	defer client.CloseIdleConnections()
	for _, source := range userToolSources {
		url := "https://github.com/" + source.repo + "/releases/download/" + source.tag + "/" + source.stem + ".tar.gz"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err != nil {
			return privateError("acquisition", err)
		}
		encoding := response.Header.Values("Content-Encoding")
		if response.StatusCode != 200 || response.ContentLength != source.size || response.Uncompressed || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || len(encoding) > 1 || (len(encoding) == 1 && encoding[0] != "" && encoding[0] != "identity") {
			return privateError("acquisition", errors.Join(response.Body.Close(), errors.New("helper response refused")))
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, source.size+1))
		err = errors.Join(readErr, response.Body.Close(), ctx.Err())
		if err != nil || int64(len(data)) != source.size || fmt.Sprintf("%x", sha256.Sum256(data)) != source.pin {
			return privateError("acquisition", err)
		}
		if err := userToolWrite(filepath.Join(archives, source.name+".tgz"), data, 0600); err != nil {
			return err
		}
		if _, err := userToolBinary(ctx, archives, source); err != nil {
			return err
		}
	}
	return nil
}

func userToolBinary(ctx context.Context, archives string, source userToolSource) ([]byte, error) {
	archive := filepath.Join(archives, source.name+".tgz")
	if _, err := privateNativeFile(ctx, archive, 0600, source.size, source.pin); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return nil, err
	}
	return userToolMember(ctx, data, source.pin, source.stem+"/"+source.name)
}

// Stock Pi prefers agent/bin before probing PATH or downloading helpers.
// Retained archive authority also verifies every ordinary launch and idempotence.
func userTools(ctx context.Context, root, agent string, install bool) error {
	archives, bin := filepath.Join(root, "runtime/tools"), filepath.Join(agent, "bin")
	if install {
		// Guarded reinspection at bind time; exclusive writes below still refuse races.
		if err := userToolBinAvailable(agent); err != nil {
			return err
		}
		if err := os.Mkdir(bin, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	for _, dir := range []string{archives, bin} {
		if err := privateNativeDirectory(dir); err != nil {
			return err
		}
	}
	for _, source := range userToolSources {
		tool, err := userToolBinary(ctx, archives, source)
		if err != nil {
			return err
		}
		path := filepath.Join(bin, source.name)
		if install {
			if err := userToolWrite(path, tool, 0700); err != nil {
				return err
			}
		}
		if _, err := privateNativeFile(ctx, path, 0700, int64(len(tool)), fmt.Sprintf("%x", sha256.Sum256(tool))); err != nil {
			return err
		}
	}
	return nil
}

func userProvisionAssets(ctx context.Context, root string, stage bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	for _, name := range []string{"user-locks", "user-locks/modern", "user-locks/prior"} {
		directory := filepath.Join(root, name)
		if stage {
			if err := os.Mkdir(directory, 0700); err != nil {
				return err
			}
		}
		if _, err := privateDirectory(directory); err != nil {
			return err
		}
	}
	files, err := assets.ReadUserAssets()
	if err != nil {
		return err
	}
	for name, data := range files {
		filename := filepath.Join(root, name)
		if stage {
			if err := userToolWrite(filename, data, 0400); err != nil {
				return err
			}
		}
		if _, err := privateNativeFile(ctx, filename, 0400, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256(data))); err != nil {
			return err
		}
	}
	return nil
}

func userVerifyGlobal(ctx context.Context, root, prefix, agent, finalPrefix, finalRoot, mode string) error {
	if err := userProvisionAssets(ctx, root, false); err != nil {
		return err
	}
	helper := filepath.Join(root, "provision.mjs")
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), helper, root, prefix, agent, finalPrefix, finalRoot, mode, "verify")
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, prefix, agent)
	_, err := privateRun(ctx, cmd, cancel)
	return err
}

func userNativeReadback(ctx context.Context, root string) error {
	version := filepath.Join(root, "v4.0.0")
	for _, directory := range []string{root, version} {
		if _, err := privateDirectory(directory); err != nil {
			return err
		}
	}
	entries, err := privateNativeEntries(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "v4.0.0" {
		return privateError("readback", err)
	}
	binary := filepath.Join(version, "gentle-ai")
	if _, err := privateNativeFile(ctx, binary, 0700, 17109176, userBinarySHA); err != nil {
		return err
	}
	if _, err := privateNativeFile(ctx, filepath.Join(version, "integrity.json"), 0600, int64(len(userNativeManifest)), fmt.Sprintf("%x", sha256.Sum256([]byte(userNativeManifest)))); err != nil {
		return err
	}
	entries, err = privateNativeEntries(version)
	if err != nil || len(entries) != 2 || !((entries[0].Name() == "gentle-ai" && entries[1].Name() == "integrity.json") || (entries[1].Name() == "gentle-ai" && entries[0].Name() == "integrity.json")) {
		return privateError("readback", err)
	}
	return nil
}

func userReadManifest(ctx context.Context, root string) (userManifest, error) {
	var manifest userManifest
	if _, err := privateDirectory(filepath.Dir(root)); err != nil {
		return manifest, err
	}
	if _, err := privateDirectory(root); err != nil {
		return manifest, err
	}
	path := filepath.Join(root, "installation.json")
	if _, err := privateNativeFile(ctx, path, 0600, -1, ""); err != nil {
		return manifest, err
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &manifest) != nil || manifest.Schema != userSchema || manifest.Destination != root || (manifest.Mode != "separate" && manifest.Mode != "shared") {
		return manifest, privateError("refused", err)
	}
	for _, name := range []string{"gentle-shell", "pi"} {
		binding := userBinding(root, name)
		if _, err := privateNativeFile(ctx, filepath.Join(root, "bin", name), 0700, int64(len(binding)), fmt.Sprintf("%x", sha256.Sum256([]byte(binding)))); err != nil {
			return manifest, err
		}
	}
	for path, expected := range map[string]string{manifest.Prefix: manifest.PrefixIdentity, manifest.Agent: manifest.AgentIdentity} {
		actual, err := userIdentity(path)
		if err != nil || actual != expected {
			return manifest, privateError("preimage", err)
		}
	}
	for _, source := range []struct{ path, pin string }{{"supervisor", manifest.SupervisorSHA}, {"runtime/node/bin/node", privateNativeNodeSHA}} {
		if len(source.pin) != 64 {
			return manifest, privateError("source", nil)
		}
		if _, err := privateNativeFile(ctx, filepath.Join(root, source.path), 0700, -1, source.pin); err != nil {
			return manifest, err
		}
	}
	if err := userTools(ctx, root, manifest.Agent, false); err != nil {
		return manifest, privateError("source", err)
	}
	return manifest, nil
}

func userLaunchCommand(ctx context.Context, root string, manifest userManifest, cli string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), append([]string{cli}, args...)...)
	// Read-only stock Git probes must not refresh the caller's index. Explicit
	// Git writes still take their mandatory locks.
	cmd.Env = append(userEnvironment(root, manifest.Prefix, manifest.Agent), "GIT_OPTIONAL_LOCKS=0")
	return cmd // Empty Dir inherits the caller's project, not the installer stage.
}

func userLaunch(ctx context.Context, root string, args []string, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	manifest, err := userReadManifest(ctx, root)
	if err != nil {
		return err
	}
	cli := filepath.Join(manifest.Prefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	if _, err := userSourceFile(ctx, cli, -1, ""); err != nil {
		return userGraphRepairError(root, manifest.Mode, err)
	}
	if err := userVerifyGlobal(ctx, root, manifest.Prefix, manifest.Agent, manifest.Prefix, root, manifest.Mode); err != nil {
		return userGraphRepairError(root, manifest.Mode, err)
	}
	if err := userNativeReadback(ctx, filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi/.gentle-ai")); err != nil {
		return err
	}
	cmd := userLaunchCommand(ctx, root, manifest, cli, args)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	restore, err := userForeground(cmd, stdin)
	if err != nil {
		return err
	}
	defer func() {
		if restoreErr := restore(); restoreErr != nil {
			err = privateError("uncertain", errors.Join(err, restoreErr))
		}
	}()
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	if err := cmd.Start(); err != nil {
		return err
	}
	err = cmd.Wait()
	if reapErr := userReapGroup(cmd.Process.Pid, err); reapErr != nil {
		return reapErr
	}
	if readbackErr := userVerifyGlobal(context.Background(), root, manifest.Prefix, manifest.Agent, manifest.Prefix, root, manifest.Mode); readbackErr != nil {
		failure := privateError("uncertain", errors.Join(err, readbackErr))
		failure.Workspace, failure.Destination = root, manifest.Prefix
		return failure
	}
	return err
}
