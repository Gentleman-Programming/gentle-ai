//go:build linux

package shellinstaller

import "runtime"

// Fixed complete npm archives, including the Node member we deliberately omit.
var userCodeGraphSources = []userCodeGraphSource{
	{"codegraph", "sha512-xZayboJt6T3QBzBsBsI0eJ3J/tN4vYs6LMuvrxJVdM71yQFjrFKFeV3KsbicAgkj6AHnTXxUNk26g9BYwkNMgg==", 210435, 603273, 164, 0},
	{"codegraph-linux-x64", "sha512-bjfa+0IyjbxpZz7ghznN4qMPoYVSiZafeZu/rXGdg3VI+kEPOPZrk2vmtWIU9tBPGVJbeU2Kwa/0Z2DkUGOJ9A==", 56164932, 234498802, 854, 123438592},
}

const userCodeGraphPlatformName = "Linux amd64"

func userCodeGraphPlatform() bool { return runtime.GOARCH == "amd64" }
