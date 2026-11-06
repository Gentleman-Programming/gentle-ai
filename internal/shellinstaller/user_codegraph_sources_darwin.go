//go:build darwin

package shellinstaller

import "runtime"

// Fixed complete npm archives of the same 1.4.1 release as Linux, other
// platform bundle. Each pin equals its registry.npmjs.org dist.integrity; the
// bundled Mach-O Node is omitted in favor of the pinned runtime Node.
var userCodeGraphSources = []userCodeGraphSource{
	{"codegraph", "sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg==", 210435, 603273, 164, 0},
	{"codegraph-darwin-arm64", "sha512-USSpExjuxZWN0OeNexZp2V5fCfOI4ZekqE79CJiBdGdPbqGTB+857ImIqzWGyF/rAyyED94Wc0Cd82t+uWD30w==", 50996796, 231633547, 854, 120573328},
}

const userCodeGraphPlatformName = "macOS arm64"

func userCodeGraphPlatform() bool { return runtime.GOARCH == "arm64" }
