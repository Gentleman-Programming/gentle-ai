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
}

type userManifest struct {
	Schema, Destination, Prefix, Agent, Mode string
	SupervisorSHA, NodeSHA                   string
	PrefixIdentity, AgentIdentity            string
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

func userServiceArgs(unit string, interactive bool, self, cwd string, args, env []string) []string {
	result := []string{"--user", "--quiet", "--wait", "--collect", "--service-type=exec", "--expand-environment=no", "--description=Gentle Shell owned runtime", "--unit=" + unit, "--working-directory=" + cwd,
		"--property=MemoryMax=3221225472", "--property=MemorySwapMax=0", "--property=CPUQuota=100%", "--property=CPUQuotaPeriodSec=100ms",
		"--property=TasksMax=64", "--property=NoNewPrivileges=yes", "--property=UMask=0077", "--property=KillMode=control-group", "--property=TimeoutStopSec=2s",
		"--property=UnsetEnvironment=LD_PRELOAD LD_LIBRARY_PATH LD_AUDIT NODE_OPTIONS NODE_PATH"}
	if interactive {
		result = append(result, "--pty")
	} else {
		result = append(result, "--pipe")
	}
	result = append(result, "/usr/bin/env", "-i")
	result = append(result, env...)
	return append(append(result, userCapabilityDrop, "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs", "--", self, "shell"), args...)
}
