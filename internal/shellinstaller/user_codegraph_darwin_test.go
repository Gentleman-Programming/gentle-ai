//go:build darwin

package shellinstaller

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// Independent literals: the darwin arm64 archives of the same pinned 1.4.1
// release, each equal to its npm registry dist.integrity.
func TestUserCodeGraphDarwinPins(t *testing.T) {
	if len(userCodeGraphSources) != 2 {
		t.Fatalf("CodeGraph source set differs: %d", len(userCodeGraphSources))
	}
	main, platform := userCodeGraphSources[0], userCodeGraphSources[1]
	if main.name != "codegraph" || main.sri != "sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg==" || main.size != 210435 || main.unpacked != 603273 || main.entries != 164 {
		t.Fatalf("platform-neutral CodeGraph pin changed: %+v", main)
	}
	if platform.name != "codegraph-darwin-arm64" || platform.sri != "sha512-USSpExjuxZWN0OeNexZp2V5fCfOI4ZekqE79CJiBdGdPbqGTB+857ImIqzWGyF/rAyyED94Wc0Cd82t+uWD30w==" || platform.size != 50996796 || platform.unpacked != 231633547 || platform.entries != 854 {
		t.Fatalf("darwin arm64 CodeGraph pin differs: %+v", platform)
	}
	command := userCodeGraphCommandWithNode("/stage", "runtime/node/bin/node")
	if !strings.Contains(command, "/runtime/codegraph/node_modules/@colbymchenry/codegraph-darwin-arm64/lib/dist/bin/codegraph.js") {
		t.Fatalf("CodeGraph command does not name the darwin library: %s", command)
	}
}

// Without retained sources the readback must fail on custody, not on a
// Linux-only platform gate.
func TestUserCodeGraphDarwinReadbackIsNotPlatformRefused(t *testing.T) {
	root := filepath.Join(userUnixTempRoot(t), "missing")
	err := userCodeGraphReadback(context.Background(), root)
	if err == nil || strings.Contains(err.Error(), "requires Linux amd64") {
		t.Fatalf("darwin readback refused by platform: %v", err)
	}
}
