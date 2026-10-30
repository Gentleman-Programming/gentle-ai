//go:build linux

package shellinstaller

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Independent literals: porting CodeGraph to macOS leaves the Linux amd64
// sources, command and refusal text unchanged.
func TestUserCodeGraphLinuxSourcesUnchanged(t *testing.T) {
	want := "[{codegraph sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg== 210435 603273 164 0} {codegraph-linux-x64 sha512-bjfa+0IyjbxpZz7ghznN4qMPoYVSiZafeZu/rXGdg3VI+kEPOPZrk2vmtWIU9tBPGVJbeU2Kwa/0Z2DkUGOJ9A== 56164932 234498802 854 123438592}]"
	if got := fmt.Sprint(userCodeGraphSources); got != want {
		t.Fatalf("Linux CodeGraph pins changed: %s", got)
	}
	if command := userCodeGraphCommandWithNode("/stage", "runtime/node/bin/node"); command != "#!/bin/sh\nexec '/stage/runtime/node/bin/node' '/stage/runtime/codegraph/node_modules/@colbymchenry/codegraph-linux-x64/lib/dist/bin/codegraph.js' \"$@\"\n" {
		t.Fatalf("Linux CodeGraph command changed: %q", command)
	}
	on := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", CodeGraph: "on"}
	err := userCodeGraphProvision(context.Background(), filepath.Join(t.TempDir(), "stage"), "agent", &on)
	if err == nil || !strings.Contains(err.Error(), "CodeGraph library requires a live Linux amd64 Separate or Shared stage") {
		t.Fatalf("Linux refusal text changed: %v", err)
	}
}
