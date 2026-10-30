//go:build linux

package shellinstaller

import (
	"context"
	"net/url"
	"os"
	"strings"
)

// userEngramLaunchContext attaches the outer entry decision to a launch entry
// only; install, recovery and every other entry keep their environment. The
// decision runs while the unit environment is built, before systemd-run.
func userEngramLaunchContext(ctx context.Context, args []string) context.Context {
	if len(args) < 2 || args[0] != "launch" {
		return ctx
	}
	root := args[1]
	return userEngramWithLaunch(ctx, func() []string { return userEngramUnitSafe(userEngramLaunchEnv(ctx, root)) })
}

// userEngramUnitSafe keeps credentials off the systemd-run command line,
// which carries the unit environment and is readable by local processes. A
// database selection, or a server URL with userinfo (or one that does not
// parse), disables memory for the launch instead of crossing it. Windows and
// darwin pass the environment to the process, never on argv.
func userEngramUnitSafe(entries []string) []string {
	for _, entry := range entries {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "ENGRAM_DATABASE_URL":
			return userEngramDisabled(userEngramArgvNotice(key))
		case "ENGRAM_URL":
			if parsed, err := url.Parse(value); err != nil || parsed.User != nil {
				return userEngramDisabled(userEngramArgvNotice(key))
			}
		}
	}
	return entries
}

func userEngramArgvNotice(key string) string {
	if key == "ENGRAM_DATABASE_URL" {
		return "Engram memory disabled: ENGRAM_DATABASE_URL is not carried into a Separate launch on Linux, because the launch unit receives its environment on a command line other local processes can read. Gentle Shell remains usable without memory."
	}
	return "Engram memory disabled: an ENGRAM_URL with credentials is not carried into a Separate launch on Linux, because the launch unit receives its environment on a command line other local processes can read. Use a URL without credentials and relaunch. Gentle Shell remains usable without memory."
}

// userEngramLaunchScope is the worker side: inside the unit the environment
// is exactly the list the outer entry built, so its Engram entries are the
// outer decision (or the user's own selection) and nothing else.
func userEngramLaunchScope(ctx context.Context, _ string) context.Context {
	return userEngramWithLaunch(ctx, func() []string { return userEngramForwarded(os.LookupEnv) })
}
