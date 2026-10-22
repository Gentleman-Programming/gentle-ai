//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// The POSIX installation flow shared by Linux and darwin. Each platform keeps
// its public entry points and supplies the per-OS hooks this flow calls:
// userBootstrapPublish, userOwnedRun, userLaunchLimits and userExecutable.

// userCanonicalRequest resolves each operator-supplied path once with
// userCanonicalPath before the strict checks. Paths that are not clean and
// absolute stay verbatim so those checks refuse them with their own message.
func userCanonicalRequest(req UserInstallRequest) (UserInstallRequest, error) {
	for _, path := range []*string{&req.Destination, &req.SharedPrefix, &req.SharedAgent} {
		if !privateHierarchyPath(*path) {
			continue
		}
		canonical, err := userCanonicalPath(*path)
		if err != nil {
			return req, err
		}
		*path = canonical
	}
	return req, nil
}

func userValidateInstall(req UserInstallRequest) error {
	if err := userExperienceSupported(req.Experience); err != nil {
		return err
	}
	if req.Experience != nil && req.Mode != "separate" {
		switch req.Mode {
		case "shared":
			if req.Experience.SDKSource == "main-preview" {
				return experienceError("Main preview is not available with the retained Shared prefix; use On my existing Pi or Separate")
			}
		case "agent":
			// Every selection is staged privately; the agent is only the settings home.
		default:
			return experienceError("explicit experience requires a fresh Separate, Shared or agent-reuse runtime")
		}
	}
	req, err := userCanonicalRequest(req)
	if err != nil {
		return err
	}
	if !userSelectionPath(req.Destination) || (req.Mode != "separate" && req.Mode != "shared" && req.Mode != "agent") {
		return privateError("refused", errors.New("choose separate or shared mode and a canonical absolute target using only ASCII letters, digits, /, _, . or -"))
	}
	parent, err := privateDirectory(filepath.Dir(req.Destination))
	if err != nil {
		return err
	}
	fresh := true
	reconfigure := req.Experience != nil && userReconfigureTarget() == req.Destination
	if err := privateDestination(req.Destination); err != nil {
		if req.Experience != nil && !reconfigure {
			return experienceError("explicit experience requires a new empty target; existing preferences are preserved")
		}
		fresh = false
		if reconfigure {
			manifest, readErr := userReadInstallationManifest(req.Destination)
			if readErr != nil || manifest.Mode != req.Mode || (req.Mode == "shared" && (manifest.Prefix != req.SharedPrefix || manifest.Agent != req.SharedAgent)) {
				return err
			}
		} else {
			manifest, readErr := userReadManifest(context.Background(), req.Destination)
			if readErr != nil || manifest.Mode != req.Mode || (req.Mode == "shared" && (manifest.Prefix != req.SharedPrefix || manifest.Agent != req.SharedAgent)) {
				return err
			}
		}
	}
	if req.Mode == "separate" {
		if req.SharedPrefix != "" || req.SharedAgent != "" {
			return privateError("refused", errors.New("separate mode cannot select shared paths"))
		}
		return nil
	}
	if req.Mode == "agent" {
		if req.SharedPrefix != "" || req.SharedAgent == "" || !privateHierarchyPath(req.SharedAgent) || userPathsOverlap(req.SharedAgent, req.Destination) {
			return privateError("refused", errors.New("agent reuse requires one absolute existing agent disjoint from the target"))
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
	// The selected stock Pi is the operator's file, not one this installer wrote.
	_, err = privateForeignPhysical(filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js"))
	return err
}

func userInspectInstall(req UserInstallRequest) (string, error) {
	if err := userExperienceSupported(req.Experience); err != nil {
		return "", err
	}
	req, err := userCanonicalRequest(req)
	if err != nil {
		return "", err
	}
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
	if req.Mode == "shared" || req.Mode == "agent" {
		paths := []string{req.SharedPrefix, req.SharedAgent}
		if req.Mode == "agent" {
			paths = []string{req.SharedAgent}
		}
		var stamps []string
		for pass := 0; pass < 2; pass++ {
			for index, path := range paths {
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

type userDirectInstallKey struct{}

func userDirectInstall(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	direct, _ := ctx.Value(userDirectInstallKey{}).(bool)
	return direct
}

func userRunInstall(ctx context.Context, req UserInstallRequest) (result UserInstallResult, err error) {
	if ctx == nil || ctx.Err() != nil {
		return result, privateError("canceled", context.Canceled)
	}
	// GGA is retired from the shell installer. Force it inert before any
	// inspection, staging or manifest recording so an old profile or a crafted
	// transport cannot silently enable it; such a request then fails its
	// physical confirmation check below.
	userForceGGAInert(req.Experience)
	if userDirectInstall(ctx) {
		if err = userDirectKernelCheck(); err != nil {
			return result, err
		}
	} else if err = UserKernelCheck(); err != nil {
		return result, err
	}
	if req, err = userCanonicalRequest(req); err != nil {
		return result, err
	}
	userShellStepBegin(ctx, 1, 5, "Validate the selection and target")
	userShellDetail(ctx, 1, 5, "Checking the destination and private parent")
	userShellDetail(ctx, 1, 5, "Verifying the inspected selection")
	userShellStep(ctx, 1, 5, "Selection and target validated")
	confirmation, err := InspectUserInstall(req)
	if err != nil || req.Confirmation != confirmation {
		return result, privateError("refused", errors.Join(err, errors.New("physical selection confirmation differs")))
	}
	if _, statErr := os.Lstat(req.Destination); statErr == nil && userReconfigureTarget() != req.Destination {
		manifest, err := userReadManifest(ctx, req.Destination)
		if err != nil {
			return result, err
		}
		if manifest.PNPM != nil {
			return UserInstallResult{req.Destination, manifest.Prefix, manifest.Agent, "ComponentInstalled"}, nil
		}
		if err = userVerifyInstalledGlobal(ctx, req.Destination, manifest); err != nil {
			return result, userGraphRepairError(req.Destination, req.Mode, err)
		}
		if err = userNativeReadback(ctx, filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi/.gentle-ai")); err != nil {
			return result, err
		}
		return UserInstallResult{req.Destination, manifest.Prefix, manifest.Agent, "ComponentInstalled"}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 870*time.Second)
	defer cancel()
	userShellStepBegin(ctx, 2, 5, "Prepare the private stage and preferences")
	userShellDetail(ctx, 2, 5, "Creating the private staging workspace")
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
	userShellDetail(ctx, 2, 5, "Writing persona and recommended preferences")
	if err = userPrepareInstallStage(ctx, root, req.Mode, req.Experience); err != nil {
		return result, err
	}
	userShellStep(ctx, 2, 5, "Private stage and preferences prepared")
	userShellStepBegin(ctx, 3, 5, "Install the runtime and pinned suppliers")
	userShellDetail(ctx, 3, 5, "Downloading Node 24.18.0")
	if err = userBootstrap(ctx, root, workspace); err != nil {
		return result, fmt.Errorf("runtime acquisition: %w", err)
	}
	userShellDetail(ctx, 3, 5, "Verifying the pinned Node runtime")
	node := filepath.Join(root, "runtime/node/bin/node")
	if _, err = privateNativeFile(ctx, node, 0700, privateNativeNodeSize, privateNativeNodeSHA); err != nil {
		return result, fmt.Errorf("bootstrap Node readback: %w", err)
	}
	userShellDetail(ctx, 3, 5, "Provisioning bundled assets")
	if err = userProvisionAssets(ctx, root, true); err != nil {
		return result, err
	}
	helperPath := filepath.Join(root, "provision.mjs")
	prefix, agent := filepath.Join(root, "prefix"), filepath.Join(root, "agent")
	finalPrefix, finalAgent := filepath.Join(req.Destination, "prefix"), filepath.Join(req.Destination, "agent")
	if req.Mode == "shared" {
		prefix, agent, finalPrefix, finalAgent = req.SharedPrefix, req.SharedAgent, req.SharedPrefix, req.SharedAgent
	}
	if req.Mode == "agent" {
		agent, finalAgent = req.SharedAgent, req.SharedAgent
		if err = userSnapshotOwnedAgentSettings(ctx, agent, root); err != nil {
			return result, fmt.Errorf("agent settings preimage: %w", err)
		}
	}
	// Acquire and verify pinned fd/rg inside the private stage before any
	// selected Shared object changes; binding into AGENT/bin comes later.
	userShellDetail(ctx, 3, 5, "Downloading pinned tools")
	if err = userToolsAcquire(ctx, root); err != nil {
		return result, fmt.Errorf("pinned tool acquisition: %w", err)
	}
	userShellDetail(ctx, 3, 5, "Preparing optional CodeGraph")
	if err = userCodeGraphProvision(ctx, root, req.Mode, req.Experience); err != nil {
		return result, fmt.Errorf("optional CodeGraph provisioning: %w", err)
	}
	if req.Experience != nil && req.Experience.CodeGraph == "on" {
		if err = userStageCodeGraphExecutable(ctx, root, req.Destination); err != nil {
			return result, err
		}
	}
	freshConfirmation, inspectErr := InspectUserInstall(req)
	if inspectErr != nil || freshConfirmation != req.Confirmation {
		return result, privateError("preimage", errors.Join(inspectErr, errors.New("selected files changed before global provisioning")))
	}
	sharedProvisioningStarted = req.Mode == "shared"
	userShellDetail(ctx, 3, 5, "Installing the global runtime")
	cmd := exec.CommandContext(ctx, node, helperPath, root, prefix, agent, finalPrefix, req.Destination, req.Mode, "install")
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, prefix, agent)
	if output, err := userOwnedRun(ctx, cmd, cancel); err != nil {
		return result, fmt.Errorf("global provisioning: %w; bounded output: %s", err, output)
	}
	pkg := filepath.Join(prefix, "lib/node_modules/gentle-pi")
	for _, source := range []struct{ name, pin string }{{"scripts/gentle-ai-installer.mjs", userInstallerSHA}, {"runtime/gentle-ai-binary.mjs", privateNativeResolverSHA}} {
		if _, err = userSourceFile(ctx, filepath.Join(pkg, source.name), -1, source.pin); err != nil {
			return result, fmt.Errorf("authenticate native supplier %q: %w", source.name, err)
		}
	}
	userShellDetail(ctx, 3, 5, "Verifying the native supplier")
	cmd = exec.CommandContext(ctx, node, "--input-type=module", "-e", privateNativeInvoke)
	cmd.Dir, cmd.Env = pkg, userEnvironment(root, prefix, agent)
	if output, err := userOwnedRun(ctx, cmd, cancel); err != nil {
		return result, fmt.Errorf("native supplier: %w; bounded output: %s", err, output)
	}
	if err = userNativeReadback(ctx, filepath.Join(pkg, ".gentle-ai")); err != nil {
		return result, fmt.Errorf("native supplier readback: %w", err)
	}
	if err = userVerifyGlobal(ctx, root, prefix, agent, finalPrefix, req.Destination, req.Mode); err != nil {
		return result, fmt.Errorf("post-native global readback: %w", err)
	}
	if req.Mode == "shared" && req.Experience != nil {
		if err = userStageContext7Shared(ctx, req.SharedAgent, req.Experience.Context7); err != nil {
			return result, fmt.Errorf("shared Context7 staging: %w", err)
		}
	}
	if req.Mode == "agent" && req.Experience != nil {
		if err = userStageOwnedAgentContext7(ctx, agent, req.Experience.Context7, root); err != nil {
			return result, fmt.Errorf("agent Context7: %w", err)
		}
	}
	userShellStep(ctx, 3, 5, "Runtime and pinned suppliers installed")
	userShellStepBegin(ctx, 4, 5, "Apply optional runtimes and the SDK channel")
	userShellDetail(ctx, 4, 5, "Applying the selected SDK channel")
	mainSDK, err := userInstallMainSDK(ctx, root, req.Destination, prefix, agent, req.Experience)
	if err != nil {
		return result, fmt.Errorf("Main SDK + stock provider: %w", err)
	}
	// A bind refusal here follows Shared provisioning: it stays uncertain and
	// keeps the staged recovery root instead of claiming an atomic rollback.
	userShellDetail(ctx, 4, 5, "Binding pinned tools into the agent")
	if err = userTools(ctx, root, agent, true); err != nil {
		return result, privateError("source", err)
	}
	userShellDetail(ctx, 4, 5, "Recording supervisor provenance")
	self, err := userExecutable()
	if err != nil {
		return result, err
	}
	selfSHA, err := userCopySupervisor(ctx, self, filepath.Join(root, "supervisor"))
	if err != nil {
		return result, fmt.Errorf("supervisor provenance: %w", err)
	}
	userShellDetail(ctx, 4, 5, "Preparing the installation manifest")
	manifest := userManifest{Schema: userSchema, Destination: req.Destination, Prefix: finalPrefix, Agent: finalAgent, Mode: req.Mode, SupervisorSHA: selfSHA, NodeSHA: privateNativeNodeSHA}
	manifest.MainSDK = mainSDK
	// Shell installs never provision or record GGA; the bit stays false.
	manifest.GGA = false
	manifest.CodeGraph = req.Experience != nil && req.Experience.CodeGraph == "on"
	manifest.PrefixIdentity, err = userIdentity(prefix)
	if err == nil {
		manifest.AgentIdentity, err = userIdentity(agent)
	}
	if err != nil {
		return result, err
	}
	userShellStep(ctx, 4, 5, "Optional runtimes and SDK channel applied")
	if req.Mode == "agent" {
		if err = userCaptureAgentPostImages(agent, root); err != nil {
			return result, fmt.Errorf("agent post-image evidence: %w", err)
		}
	}
	userShellStepBegin(ctx, 5, 5, "Publish, verify and register the installation")
	userShellDetail(ctx, 5, 5, "Publishing the staged installation")
	result, err = userPublishManifest(ctx, root, prefix, agent, manifest)
	if err == nil {
		userShellDetail(ctx, 5, 5, "Registering the published installation")
		userShellStep(ctx, 5, 5, "Published, verified and registered")
	}
	return result, err
}

// userPublishManifest is the shared publication boundary. A renamed destination
// is retained on sync/readback failure; success requires final physical readback.
func userPublishManifest(ctx context.Context, root, prefix, agent string, manifest userManifest) (result UserInstallResult, err error) {
	backup, replaced, err := userBeginShellReplacement(manifest.Destination)
	if err != nil {
		return result, err
	}
	switched := false
	if replaced {
		manifest.Replaced = backup
		defer func() {
			if err != nil && !switched {
				err = errors.Join(err, userRestoreShellReplacement(manifest.Destination, backup))
			}
		}()
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return result, err
	}
	if err = userToolWrite(filepath.Join(root, "installation.json"), data, 0600); err != nil {
		return result, err
	}
	for _, name := range []string{"gentle-shell", "pi"} {
		binding := userManifestBinding(manifest.Destination, name, manifest)
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
	if err = privateDestination(manifest.Destination); err != nil || ctx.Err() != nil {
		return result, privateError("preimage", errors.Join(err, ctx.Err()))
	}
	if err = userRenameNoReplace(root, manifest.Destination); err != nil {
		return result, err
	}
	switched = true
	if err = userDirectorySync(filepath.Dir(manifest.Destination)); err != nil {
		return result, privateError("uncertain", err)
	}
	if _, err = userReadManifest(ctx, manifest.Destination); err != nil {
		return result, privateError("uncertain", err)
	}
	userRecordPublishedShell(manifest)
	if manifest.Mode == "separate" {
		if err := PublishUserShellLauncher(manifest.Destination); err != nil {
			return result, fmt.Errorf("installation published but launcher registration failed: %w", err)
		}
	}
	// The publication is renamed, read back and registered; drop only the
	// ephemeral caches. Failure is reported but never retracts the install.
	if cleanupErr := userPublishEphemeralCleanup(manifest.Destination); cleanupErr != nil {
		fmt.Fprintf(os.Stderr, "Installation is published and verified, but ephemeral cleanup failed; installer caches may remain in %q: %v\n", manifest.Destination, cleanupErr)
	}
	return UserInstallResult{manifest.Destination, manifest.Prefix, manifest.Agent, "ComponentInstalled"}, nil
}

// userPublishEphemeralCleanup drops the installer's ephemeral caches after a
// verified publication. It is a variable only so tests can observe whether
// the publish tail calls it; production always uses userInstallEphemeralCleanup.
var userPublishEphemeralCleanup = userInstallEphemeralCleanup

// userRecover restores the saved Shared preimages of ROOT after a fresh printed
// confirmation. The restore command runs as an owned installer command:
// inside the qualified unit on Linux, under userSupervise on darwin.
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
	_, err = userOwnedRun(ctx, cmd, cancel)
	if err != nil {
		failure := privateError("uncertain", err)
		failure.Workspace, failure.Destination = root, selection.Prefix
		return failure // The restore command may already have moved shared data.
	}
	_, err = fmt.Fprintf(stdout, "Restored selected shared preimages; preserve recovery evidence at %q\n", root)
	return err
}

const userInstallerSHA = "8679f49d3675323bf7ff5c24321f08016bb0f8e79ded307b5900c37648f52470"
const privateNativeResolverSHA = "61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80"
const privateNativeInvoke = `import {installGentleAi} from './scripts/gentle-ai-installer.mjs'; const packageRoot=process.cwd(); await installGentleAi({packageRoot});`

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
	output, err := userOwnedRun(ctx, cmd, cancel)
	if err != nil {
		return fmt.Errorf("Node bootstrap: %w; bounded output: %s", err, output)
	}
	return userBootstrapPublish(ctx, output, filepath.Join(root, "runtime/node"))
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
	return userVerifyGlobalAction(ctx, root, prefix, agent, finalPrefix, finalRoot, mode, "verify")
}

func userVerifyGlobalAction(ctx context.Context, root, prefix, agent, finalPrefix, finalRoot, mode, action string) error {
	if err := userProvisionAssets(ctx, root, false); err != nil {
		return err
	}
	helper := filepath.Join(root, "provision.mjs")
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), helper, root, prefix, agent, finalPrefix, finalRoot, mode, action)
	cmd.Dir, cmd.Env = filepath.Join(root, "project"), userEnvironment(root, prefix, agent)
	output, err := userOwnedRun(ctx, cmd, cancel)
	if err != nil {
		return fmt.Errorf("%w; bounded output: %s", err, output)
	}
	return nil
}

func userNativeReadback(ctx context.Context, root string) error {
	if _, err := privateDirectory(root); err != nil {
		return err
	}
	entries, err := privateNativeEntries(root)
	if err != nil || len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "v") {
		return privateError("readback", err)
	}
	// The stock installer names the bundle after its own pinned Gentle AI
	// release; the version is derived from the layout and cross-checked against
	// the manifest instead of a frozen literal.
	version := strings.TrimPrefix(entries[0].Name(), "v")
	if !userNativeSemver(version) {
		return privateError("readback", nil)
	}
	bundle := filepath.Join(root, entries[0].Name())
	if _, err := privateDirectory(bundle); err != nil {
		return err
	}
	members, err := privateNativeEntries(bundle)
	if err != nil || len(members) != 2 {
		return privateError("readback", err)
	}
	var manifest struct {
		Version      string `json:"version"`
		Asset        string `json:"asset"`
		AssetSha256  string `json:"assetSha256"`
		Method       string `json:"method"`
		BinarySha256 string `json:"binarySha256"`
	}
	integrity := filepath.Join(bundle, "integrity.json")
	if _, err := privateNativeFile(ctx, integrity, 0600, -1, ""); err != nil {
		return err
	}
	data, err := os.ReadFile(integrity)
	if err != nil || len(data) > 4096 || !strings.HasSuffix(string(data), "}\n") || strings.Contains(strings.TrimSuffix(string(data), "\n"), "\n") || json.Unmarshal(data, &manifest) != nil {
		return privateError("readback", err)
	}
	digest := func(value string) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == sha256.Size
	}
	if manifest.Version != version || !digest(manifest.BinarySha256) {
		return privateError("readback", nil)
	}
	canonical := func(shape any) bool {
		var buffer bytes.Buffer
		encoder := json.NewEncoder(&buffer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(shape) == nil && bytes.Equal(data, buffer.Bytes())
	}
	if manifest.Method == "" {
		// Stock signed-release shape: the pinned archive and its extracted
		// binary, in the installer's exact canonical byte form.
		release := map[string]string{"linux": "linux_amd64", "darwin": "darwin_arm64"}[runtime.GOOS]
		canonicalSigned := struct {
			Version      string `json:"version"`
			Asset        string `json:"asset"`
			AssetSha256  string `json:"assetSha256"`
			BinarySha256 string `json:"binarySha256"`
		}{manifest.Version, manifest.Asset, manifest.AssetSha256, manifest.BinarySha256}
		if manifest.Asset != "gentle-ai_"+version+"_"+release+".tar.gz" || !digest(manifest.AssetSha256) || !canonical(canonicalSigned) {
			return privateError("readback", nil)
		}
	} else {
		// The stock Windows Go SumDB source-build shape is also admitted, again
		// only in its exact canonical byte form; any other method or field set
		// is a foreign manifest.
		var canonicalSource struct {
			Version        string          `json:"version"`
			Method         string          `json:"method"`
			Package        string          `json:"package"`
			Module         string          `json:"module"`
			Tag            string          `json:"tag"`
			Architecture   string          `json:"architecture"`
			BinarySha256   string          `json:"binarySha256"`
			ModuleChecksum string          `json:"moduleChecksum"`
			GoVersion      string          `json:"goVersion"`
			Goos           string          `json:"goos"`
			Goarch         string          `json:"goarch"`
			BuildMode      string          `json:"buildMode"`
			Compiler       string          `json:"compiler"`
			CgoEnabled     json.RawMessage `json:"cgoEnabled"`
		}
		if manifest.Method != "go-sumdb-source-build" || json.Unmarshal(data, &canonicalSource) != nil || !canonical(canonicalSource) {
			return privateError("readback", nil)
		}
	}
	if _, err := privateNativeFile(ctx, filepath.Join(bundle, "gentle-ai"), 0700, -1, manifest.BinarySha256); err != nil {
		return err
	}
	return nil
}

// userNativeSemver admits only canonical stock release versions: three
// dot-separated unsigned integer components with an optional prerelease,
// rejecting leading zeros, build metadata and every other spelling.
func userNativeSemver(version string) bool {
	const component = "(0|[1-9][0-9]*)"
	pattern := "^" + component + "\\." + component + "\\." + component + "(?:-[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$"
	matched, err := regexp.MatchString(pattern, version)
	return err == nil && matched
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
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &manifest) != nil || manifest.Schema != userSchema || manifest.Destination != root || (manifest.Mode != "separate" && manifest.Mode != "shared" && manifest.Mode != "agent") {
		return manifest, privateError("refused", err)
	}
	if manifest.PNPM != nil {
		return userReadPNPMManifest(ctx, root, manifest)
	}
	if manifest.CodeGraph {
		if manifest.Mode != "separate" && manifest.Mode != "shared" && manifest.Mode != "agent" {
			return manifest, privateError("refused", errors.New("CodeGraph requires a Separate, Shared or agent manifest"))
		}
		if err := userCodeGraphReadback(ctx, root); err != nil {
			return manifest, fmt.Errorf("CodeGraph retained integrity: %w", err)
		}
	}
	for _, name := range []string{"gentle-shell", "pi"} {
		binding := userManifestBinding(root, name, manifest)
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
	if manifest.MainSDK != nil {
		if err := userVerifyInstalledGlobal(ctx, root, manifest); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

func userLaunchCommand(ctx context.Context, root string, manifest userManifest, cli string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(root, "runtime/node/bin/node"), append([]string{cli}, args...)...)
	// Read-only stock Git probes must not refresh the caller's index. Explicit
	// Git writes still take their mandatory locks.
	cmd.Env = append(userEnvironment(root, manifest.Prefix, manifest.Agent), "GIT_OPTIONAL_LOCKS=0")
	// GGA is retired from the shell installer: no runtime PATH is extended.
	userCodeGraphLaunchEnv(root, manifest, cmd)
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
	if err := userVerifyInstalledGlobal(ctx, root, manifest); err != nil {
		return userGraphRepairError(root, manifest.Mode, err)
	}
	if err := userNativeReadback(ctx, filepath.Join(manifest.Prefix, "lib/node_modules/gentle-pi/.gentle-ai")); err != nil {
		return err
	}
	cmd := userLaunchCommand(ctx, root, manifest, cli, args)
	limited, err := userLaunchLimits(cmd)
	if err != nil {
		return err
	}
	settled := false
	defer func() {
		if !settled {
			_ = limited(nil) // Start failed: release the limits attestation.
		}
	}()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return userLaunchGroup(cmd, stdin, func(err error) error {
		settled = true
		if manifest.CodeGraph {
			if readbackErr := userCodeGraphReadback(context.Background(), root); readbackErr != nil {
				failure := privateError("uncertain", errors.Join(err, readbackErr))
				failure.Workspace, failure.Destination = root, root
				return failure
			}
		}
		if readbackErr := userVerifyInstalledGlobal(context.Background(), root, manifest); readbackErr != nil {
			failure := privateError("uncertain", errors.Join(err, readbackErr))
			failure.Workspace, failure.Destination = root, manifest.Prefix
			return failure
		}
		return err
	})
}
