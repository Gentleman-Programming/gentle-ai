//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUserPNPMPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("requires original installer DATA in the qualified private sandbox")
	}
	ctx := context.Background()
	data, err := os.ReadFile("/out/verified-location.json")
	if err != nil {
		t.Fatal(err)
	}
	var location NativeInstallationLocation
	if err := json.Unmarshal(data, &location); err != nil {
		t.Fatal(err)
	}
	before, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(installStagePath(t))
	stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	got, err := publishNativePNPMInstallationWithExperienceForTest(ctx, "/out", stage, root, location, nil)
	want := UserInstallResult{root, root + "/pnpm", root + "/agent", "ComponentInstalled"}
	if err != nil || got != want {
		t.Fatal("publication failed", got, err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatal("stage remains after publication", err)
	}
	manifest, err := userReadManifest(ctx, root)
	if err != nil || manifest.PNPM == nil {
		t.Fatal("final manifest rejected", err)
	}
	saved, err := os.ReadFile(root + "/installation.json")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := publishNativePNPMInstallationWithExperienceForTest(ctx, "/out", stage, root, location, nil); err == nil || result.State != "" {
		t.Fatal("occupied destination accepted", result, err)
	}
	if fresh, err := os.ReadFile(root + "/installation.json"); err != nil || string(fresh) != string(saved) {
		t.Fatal("occupied destination changed", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if result, err := publishNativePNPMInstallationWithExperienceForTest(canceled, "/out", stage, root+"-canceled", location, nil); err == nil || result.State != "" {
		t.Fatal("canceled publication accepted", result, err)
	}
	if _, err := os.Lstat(root + "-canceled"); !os.IsNotExist(err) {
		t.Fatal("canceled destination exists", err)
	}
	if err := os.WriteFile(root+"/pnpm/extra-code.js", []byte("unsealed code"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := userReadManifest(ctx, root); err == nil {
		t.Fatal("unsealed final payload accepted")
	}
	after, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("original DATA changed", err)
	}
}

// Exercise the shared commit boundary without a second full DATA copy. This
// deliberately incomplete stage must never be reported installed or rolled back
// after rename: it is retained for inspection with an uncertain result.
func TestUserPNPMPublishReadbackFailure(t *testing.T) {
	ctx := context.Background()
	parent := filepath.Dir(installStagePath(t))
	stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	for _, dir := range []string{stage, stage + "/pnpm", stage + "/agent", stage + "/bin"} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	m := userManifest{Schema: userSchema, Destination: root, Prefix: root + "/pnpm", Agent: root + "/agent", Mode: "separate", PNPM: &userPNPMManifest{}}
	var err error
	m.PrefixIdentity, err = userIdentity(stage + "/pnpm")
	if err != nil {
		t.Fatal(err)
	}
	m.AgentIdentity, err = userIdentity(stage + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	got, err := userPublishManifest(ctx, stage, stage+"/pnpm", stage+"/agent", m)
	var failure *PrivateRuntimeError
	if !errors.As(err, &failure) || failure.Kind != "uncertain" || got.State != "" {
		t.Fatal("false publication success or certain rollback", got, err)
	}
	if _, err := os.Stat(root + "/installation.json"); err != nil {
		t.Fatal("published evidence was removed", err)
	}
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatal("stage not renamed", err)
	}
}
