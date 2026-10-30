package shellinstaller

import (
	"fmt"
	"strings"
)

// UserInstallRequest, userConfirmation and UserInstallFromEntry are platform
// contracts: Windows binds a channel, Linux keeps its five-value selection.
// See user_selection_windows.go and user_selection_other.go.

type UserInstallResult struct {
	Destination, Prefix, Agent, State string
	// LauncherNote carries the user-facing launcher publication report
	// (#5740): published commands, occupied names and the PATH remediation
	// hint. Empty when no launcher was published.
	LauncherNote string
}

type userManifest struct {
	PNPM                                     *userPNPMManifest `json:"pnpm,omitempty"`
	Schema, Destination, Prefix, Agent, Mode string
	SupervisorSHA, NodeSHA                   string
	// Component pins recorded at install time. An empty pin on an otherwise
	// valid manifest is the pre-pin legacy layout: the update check then
	// reports the component as changed so the next update refreshes it.
	SDKLockSHA string `json:"sdk_lock_sha,omitempty"`
	// SDKVersion records the exact installed gentle-pi version of a stable
	// install, read from the lock the install generated (empty for Main SDK
	// installs, whose authority is the snapshot). A manifest without the field
	// is a pre-feature install: the update check reports the SDK as changed
	// once, and the next update records it.
	SDKVersion                    string `json:"sdk_version,omitempty"`
	ToolsSHA                      string `json:"tools_sha,omitempty"`
	CodeGraphSHA                  string `json:"codegraph_sha,omitempty"`
	Context7SHA                   string `json:"context7_sha,omitempty"`
	PrefixIdentity, AgentIdentity string
	GGA                           bool                  `json:"gga,omitempty"`
	CodeGraph                     bool                  `json:"codegraph,omitempty"`
	MainSDK                       *userSnapshotArtifact `json:"main_sdk,omitempty"`
	Replaced                      string                `json:"replaced,omitempty"`
	// RootRisk records that root published this installation with the explicit
	// acceptance binding; older manifests simply omit the field.
	RootRisk bool `json:"rootRisk,omitempty"`
}

const userSchema = "gentle-shell-user-install/v1"
const userCapabilityDrop = "/usr/bin/setpriv"

func userBinding(root, name string) string {
	root = strings.ReplaceAll(root, "'", "'\\''")
	binding := "#!/bin/sh\nexec '" + root + "/supervisor' shell launch '" + root + "' \"$@\"\n"
	if name == "gentle-shell" {
		binding = "#!/bin/sh\nif test \"${1-}\" = install; then shift; exec '" + root + "/supervisor' shell install \"$@\"; fi\n" + binding[len("#!/bin/sh\n"):]
	}
	return binding
}

func userGraphRepairError(root, mode string, err error) error {
	if mode == "separate" && err != nil {
		return fmt.Errorf("%w\nSeparate repair: preserve target %q and its agent data; install into a different empty target with separate mode. Run gentle-ai shell install --help for inspection and confirmation flags; do not delete or reuse the damaged target", err, root)
	}
	return err
}

// userServicePropertyBase holds the identical sandbox properties for the user
// and system delegation paths: both create the same bounded transient unit.
func userServicePropertyBase(unit string, interactive bool, cwd string) []string {
	result := []string{"--quiet", "--wait", "--collect", "--service-type=exec", "--expand-environment=no", "--description=Gentle Shell owned runtime", "--unit=" + unit, "--working-directory=" + cwd,
		"--property=MemoryMax=3221225472", "--property=MemorySwapMax=0", "--property=CPUQuota=100%", "--property=CPUQuotaPeriodSec=100ms",
		"--property=TasksMax=64", "--property=NoNewPrivileges=yes", "--property=UMask=0077", "--property=KillMode=control-group", "--property=TimeoutStopSec=2s",
		"--property=UnsetEnvironment=LD_PRELOAD LD_LIBRARY_PATH LD_AUDIT NODE_OPTIONS NODE_PATH"}
	if interactive {
		result = append(result, "--pty")
	} else {
		result = append(result, "--pipe")
	}
	return result
}

func userServiceArgs(unit string, interactive bool, self, cwd string, args, env []string) []string {
	result := append([]string{"--user"}, userServicePropertyBase(unit, interactive, cwd)...)
	result = append(result, "/usr/bin/env", "-i")
	result = append(result, env...)
	return append(append(result, userCapabilityDrop, "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs", "--", self, "shell"), args...)
}

// rootServiceArgs mirrors the user unit for the system manager: root has no
// user manager to inherit, and none is created, so the identical sandbox
// properties are applied by a transient system unit instead.
func rootServiceArgs(unit string, interactive bool, self, cwd string, args, env []string) []string {
	result := userServicePropertyBase(unit, interactive, cwd)
	result = append(result, "/usr/bin/env", "-i")
	result = append(result, env...)
	return append(append(result, userCapabilityDrop, "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs", "--", self, "shell"), args...)
}

// Keep npm bytes intact; construct the PNPM command without rewriting path text.
func userManifestBinding(root, name string, manifest userManifest) string {
	if manifest.PNPM == nil {
		return userBinding(root, name)
	}
	root = strings.ReplaceAll(root, "'", "'\\''")
	binding := "#!/bin/sh\nexec '" + root + "/supervisor' shell launch-pnpm '" + root + "' '" + name + "' \"$@\"\n"
	if name == "gentle-shell" {
		binding = "#!/bin/sh\nif test \"${1-}\" = install; then shift; exec '" + root + "/supervisor' shell install \"$@\"; fi\n" + binding[len("#!/bin/sh\n"):]
	}
	return binding
}
