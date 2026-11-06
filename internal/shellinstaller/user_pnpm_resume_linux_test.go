//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUserPNPMResume(t *testing.T) {
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
	parent := filepath.Dir(installStagePath(t))
	root := filepath.Join(parent, "final")
	want, err := publishNativePNPMInstallationWithExperienceForTest(ctx, "/out", filepath.Join(parent, "stage"), root, location, nil)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "installation.json")
	saved, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
	if err != nil {
		t.Fatal(err)
	}
	req := UserInstallRequest{Destination: root, Mode: "separate"}
	req.Confirmation, err = InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := RunUserInstall(ctx, req)
	if err != nil || got != want {
		t.Fatal("confirmed PNPM resume failed", got, err)
	}
	after, err := userPNPMPayloadInventory(ctx, root, userPNPMRoots)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("resume changed installed payload", err)
	}
	if fresh, err := os.ReadFile(manifestPath); err != nil || string(fresh) != string(saved) {
		t.Fatal("resume changed manifest", err)
	}
	// Valid JSON whitespace changes the physical confirmation without damaging
	// the installation; stale confirmation must not be accepted or rewritten.
	changed := append(append([]byte(nil), saved...), '\n')
	if err := os.WriteFile(manifestPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := RunUserInstall(ctx, req); err == nil || got.State != "" {
		t.Fatal("stale confirmation accepted", got, err)
	}
	if fresh, err := os.ReadFile(manifestPath); err != nil || string(fresh) != string(changed) {
		t.Fatal("rejected resume rewrote manifest", err)
	}
	if err := os.WriteFile(manifestPath, saved, 0600); err != nil {
		t.Fatal(err)
	}
	unsealed := filepath.Join(root, "pnpm", "unsealed.js")
	if err := os.WriteFile(unsealed, []byte("unsealed code"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := RunUserInstall(ctx, req); err == nil || got.State != "" {
		t.Fatal("damaged payload accepted", got, err)
	}
	if fresh, err := os.ReadFile(unsealed); err != nil || string(fresh) != "unsealed code" {
		t.Fatal("rejected resume changed damaged payload", err)
	}
}
