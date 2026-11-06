//go:build darwin && arm64

package shellinstaller

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// macOS arm64 carries the same Main composition as Linux amd64: a
// channel-only reconfigure of a Separate install takes the in-place overlay
// in both directions instead of the full retire-and-reinstall, and the
// installed Main tree verifies through the selected-SDK provisioner action.
func TestUserDarwinChannelOverlayAppliesInPlace(t *testing.T) {
	root, _ := userUnixOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	var calls []string
	refusals := 0
	userUnixOverlayRealActions(t, artifact, &calls, &refusals)
	for _, source := range []string{"main-preview", "stable"} {
		calls = nil
		result, applied, err := userUnixChannelOverlay(context.Background(), userUnixOverlayRequest(root, source))
		if err != nil || !applied || result.State != "channel-overlay" {
			t.Fatalf("%s reconfigure did not take the in-place overlay: %+v applied=%v err=%v (actions %v)", source, result, applied, err, calls)
		}
		if !slices.Contains(calls, userMainSDKSelectedAction) {
			t.Fatalf("%s overlay never verified the selected Main SDK: %v", source, calls)
		}
	}
	if packages := userUnixOverlayPackages(t, root); len(packages) != 1 || packages[0] != filepath.Join(root, "prefix/lib/node_modules/gentle-pi") {
		t.Fatalf("Stable declaration was not restored: %v", packages)
	}
}

// An installed Main manifest on macOS arm64 is verified by the selected-SDK
// action, never refused as a Linux-only binding.
func TestUserDarwinVerifiesInstalledMainSDK(t *testing.T) {
	root, manifest := userUnixOverlayFixture(t)
	artifact := userUnixOverlayMainArtifact(t, strings.Repeat("a", 40))
	manifest.MainSDK = &artifact
	previous := userMainSDKAction
	t.Cleanup(func() { userMainSDKAction = previous })
	var calls []string
	userMainSDKAction = func(_ context.Context, gotRoot, finalRoot, prefix, agent string, got userSnapshotArtifact, action string) error {
		if gotRoot != root || finalRoot != root || prefix != manifest.Prefix || agent != manifest.Agent || got != artifact {
			t.Fatalf("selected-SDK verification bound different paths: %s %s %s %s %+v", gotRoot, finalRoot, prefix, agent, got)
		}
		calls = append(calls, action)
		return nil
	}
	if err := userVerifyInstalledGlobal(context.Background(), root, manifest); err != nil {
		t.Fatalf("installed Main SDK refused on macOS: %v", err)
	}
	if !slices.Equal(calls, []string{userMainSDKSelectedAction}) {
		t.Fatalf("installed Main SDK verification actions differ: %v", calls)
	}
}
