//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestUserCodeGraphPinSHADeterministic(t *testing.T) {
	first, err := userCodeGraphPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	second, err := userCodeGraphPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("CodeGraph pin SHA differs across calls: %q vs %q", first, second)
	}
}

func TestUserSDKLockSHAMatchesEmbeddedLock(t *testing.T) {
	assets, err := userReadUserAssets()
	if err != nil {
		t.Fatal(err)
	}
	lock, ok := assets["user-locks/modern/package-lock.json"]
	if !ok {
		t.Fatal("embedded modern package lock is absent")
	}
	want := fmtSHA256(lock)
	got, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("SDK lock SHA differs: got %q want %q", got, want)
	}
}

func TestUserContext7EntrySHA(t *testing.T) {
	on, err := userContext7EntrySHA("on")
	if err != nil {
		t.Fatal(err)
	}
	again, err := userContext7EntrySHA("on")
	if err != nil {
		t.Fatal(err)
	}
	off, err := userContext7EntrySHA("off")
	if err != nil {
		t.Fatal(err)
	}
	if on == "" || on != again || on == off {
		t.Fatalf("Context7 entry SHA differs: on=%q again=%q off=%q", on, again, off)
	}
}

// An installation whose manifest records every current pin and the newest
// listed gentle-pi release is up to date: the version the Stable install just
// recorded never reports an endless "update available" loop (S17(c)).
func TestCheckUserShellUpdateAllPinsCurrent(t *testing.T) {
	list := userStubSelectedList(t)
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, SDKVersion: list.newestReviewedRelease(), ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Available {
		t.Fatalf("expected up to date, got: %+v", update)
	}
	if len(update.Scope) != 0 {
		t.Fatalf("expected empty scope, got %v", update.Scope)
	}
}

// S19 detection: the Stable sdk is outdated when the recorded gentle-pi is
// strictly older than the newest listed release — including a version that
// matches the frozen lock but not the selected list, which an existing build
// detects without being rebuilt.
func TestCheckUserShellUpdateFrozenVersionReportsSDKOutdated(t *testing.T) {
	list := userStubSelectedList(t)
	frozen, err := userFrozenLockGentlePiVersion()
	if err != nil {
		t.Fatal(err)
	}
	if frozen == list.newestReviewedRelease() {
		t.Skip("frozen lock already pins the newest listed release")
	}
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, SDKVersion: frozen, ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSDK}) {
		t.Fatalf("expected the SDK outdated for the frozen-lock version, got: %+v", update)
	}
}

// S14 legacy detection: a manifest that records every current pin but predates
// the recorded gentle-pi version has no version field, so the stable SDK is
// reported outdated once; the next update installs and records the version,
// and the check converges (no endless "update available" loop).
func TestCheckUserShellUpdateLegacyVersionReportsSDKOnce(t *testing.T) {
	userStubSelectedList(t)
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSDK}) {
		t.Fatalf("expected the SDK outdated once for a pre-version manifest, got: %+v", update)
	}
}

// S17(c): a recorded version strictly newer than this build's stable target
// (the frozen lock's gentle-pi until the live listed release replaces it) —
// e.g. after a fallback to an older embedded list — never offers a downgrade.
func TestCheckUserShellUpdateNewerSDKVersionSkipsDowngrade(t *testing.T) {
	userStubSelectedList(t)
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, SDKVersion: "9.9.9", ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if update.Available {
		t.Fatalf("expected no downgrade offer for a newer recorded version, got: %+v", update)
	}
	if len(update.Scope) != 0 {
		t.Fatalf("expected empty scope, got %v", update.Scope)
	}
}

// A recorded version that differs from this build's stable target (the frozen
// lock's gentle-pi until the live listed release replaces it) reports the SDK
// outdated.
func TestCheckUserShellUpdateChangedSDKVersionReportsSDK(t *testing.T) {
	userStubSelectedList(t)
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, SDKVersion: "0.0.1", ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSDK}) {
		t.Fatalf("expected the SDK outdated for a differing recorded version, got: %+v", update)
	}
}

// A manifest written by an older build without the component SHA fields reports
// the missing components as changed, with canonical scope names for the update.
func TestCheckUserShellUpdateLegacyManifestReportsComponents(t *testing.T) {
	userStubSelectedList(t)
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	manifest := userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		CodeGraph: true,
	}
	writeInstallFixture(t, target, manifest)
	// Context7 on inside the agent mcp.json.
	agent := filepath.Join(target, "agent")
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	mcp := map[string]any{"mcpServers": map[string]any{"context7": map[string]any{"url": "https://mcp.context7.com/mcp", "enabled": true, "exposure": "codemode"}}}
	data, err := json.Marshal(mcp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "mcp.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Available {
		t.Fatalf("expected update available for legacy manifest, got: %+v", update)
	}
	want := map[string]bool{"sdk": true, "codegraph": true, "context7": true}
	for _, component := range update.Scope {
		if !want[component] {
			t.Fatalf("unexpected scope component %q in %+v", component, update.Scope)
		}
		delete(want, component)
	}
	if len(want) != 0 {
		t.Fatalf("missing scope components: %v", want)
	}
}

// A newly reviewed release is detected by an already-built consumer without a
// gentle-ai rebuild: a list holding a newer release reports the recorded
// version outdated, and an install that recorded that same version converges
// (no endless loop).
func TestCheckUserShellUpdateNewerListedReleaseReportsSDK(t *testing.T) {
	list, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	recorded := list.newestReviewedRelease()
	newer := newerListedReleaseForTest(t, list)
	list.Releases[newer] = userReviewedSupplier{
		Integrity: list.Releases[list.newestReviewedRelease()].Integrity,
		Installer: list.Releases[list.newestReviewedRelease()].Installer,
		Binary:    list.Releases[list.newestReviewedRelease()].Binary,
	}
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return list, nil, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	ctx := context.Background()
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := userSupervisorSHA(ctx, self)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := userSDKLockSHA()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := userToolsPinSHA()
	if err != nil {
		t.Fatal(err)
	}
	target := userUnixTempRoot(t)
	writeInstallFixture(t, target, userManifest{
		Schema: userSchema, Destination: target,
		Prefix: filepath.Join(target, "prefix"), Agent: filepath.Join(target, "agent"),
		Mode: "separate", SupervisorSHA: supervisor, NodeSHA: privateNativeNodeSHA,
		SDKLockSHA: lock, SDKVersion: recorded, ToolsSHA: tools,
	})
	update, err := CheckUserShellUpdate(ctx, UserShellInstall{Target: target, Mode: "separate"})
	if err != nil {
		t.Fatal(err)
	}
	if !update.Available || !slices.Equal(update.Scope, []string{UserComponentSDK}) {
		t.Fatalf("expected the SDK outdated for a newer listed release, got: %+v", update)
	}
	// The version that update would record satisfies the same list: no loop.
	if userReviewedVersionOlder(newer, list.newestReviewedRelease()) {
		t.Fatalf("recorded newest listed release %q would report outdated again", newer)
	}
}

// newerListedReleaseForTest returns an exact semver strictly newer than every
// release of the given list.
func newerListedReleaseForTest(t *testing.T, list userReviewedList) string {
	t.Helper()
	newest := list.newestReviewedRelease()
	parts := strings.Split(newest, ".")
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d.%s.%s", major+1, parts[1], parts[2])
}

func fmtSHA256(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// userStubSelectedList pins the update check's list authority to the embedded
// copy, so the detection tests never touch the network.
func userStubSelectedList(t *testing.T) userReviewedList {
	t.Helper()
	list, _, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	previous := userSelectedReviewedListFunc
	userSelectedReviewedListFunc = func(context.Context) (userReviewedList, []byte, error) { return list, nil, nil }
	t.Cleanup(func() { userSelectedReviewedListFunc = previous })
	return list
}
