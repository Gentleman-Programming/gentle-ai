//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUserPNPMStage(t *testing.T) {
	if testing.Short() {
		t.Skip("requires original installer DATA in the qualified private sandbox")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
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
	stage, destination := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
	if _, err := StageNativePNPMInstallation(ctx, "/out", stage, destination+"$HOME", location); err == nil {
		t.Fatal("unsafe generated shell reference accepted")
	}
	got, err := StageNativePNPMInstallation(ctx, "/out", stage, destination, location)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodeSeal == "" || got.Installation.Node == "" {
		t.Fatal("missing closure or targets")
	}
	after, err := userPNPMPayloadInventory(ctx, stage, userPNPMRoots)
	if err != nil {
		t.Fatal(err)
	}
	expectedEntries := len(before)
	for rel := range before {
		if userPNPMAcquisitionCache(rel) {
			expectedEntries--
		}
	}
	if _, err := os.Lstat(filepath.Join(stage, "home/.npm")); !os.IsNotExist(err) {
		t.Fatal("acquisition cache was published", err)
	}
	if _, err := os.Lstat(filepath.Join(stage, "pnpm/store/v11/files")); !os.IsNotExist(err) {
		t.Fatal("PNPM acquisition cache was published", err)
	}
	if len(after) != expectedEntries {
		t.Fatal("incomplete runtime payload", expectedEntries, len(after))
	}
	changes := 0
	for rel, original := range before {
		if userPNPMAcquisitionCache(rel) {
			continue
		}
		if after[rel] != original {
			changes++
		}
		a, err := os.Lstat(filepath.Join("/out", rel))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.Lstat(filepath.Join(stage, rel))
		if err != nil {
			t.Fatal(err)
		}
		if a.Mode().IsRegular() && os.SameFile(a, b) {
			t.Fatal("shared source inode", rel)
		}
	}
	if changes != 96 {
		t.Fatal("unexpected fixture transformations", changes)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatal("premature publication", err)
	}
	if _, err := StageNativePNPMInstallation(ctx, "/out", stage, destination, location); err == nil {
		t.Fatal("existing stage overwritten")
	}
	if seal, err := PNPMCodeSeal(ctx, stage); err != nil || seal != got.CodeSeal {
		t.Fatal("unstable seal", err)
	}
	if err := os.Rename(stage, destination); err != nil {
		t.Fatal(err)
	}
	location.PackageRoot = strings.Replace(location.PackageRoot, "/out/", destination+"/", 1)
	location.PNPMHome, location.GlobalBin = destination+"/pnpm", destination+"/pnpm/bin"
	if _, err := ReadNativePNPMInstallation(ctx, destination, location); err != nil {
		t.Fatal("final targets", err)
	}
	if seal, err := PNPMCodeSeal(ctx, destination); err != nil || seal != got.CodeSeal {
		t.Fatal("relocation changed seal", err)
	}
	dependency := filepath.Join(destination, filepath.Dir(got.Installation.PiCLI), "../package.json")
	// Select a genuine unselected dependency instead of only rechecking entrypoints.
	for rel := range before {
		if strings.HasSuffix(rel, "/package.json") && strings.HasPrefix(rel, "pnpm/") && got.Installation.Files[rel] == "" {
			dependency = filepath.Join(destination, rel)
			break
		}
	}
	original, err := os.ReadFile(dependency)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"modify", "remove", "add"} {
		t.Run(action, func(t *testing.T) {
			path := dependency
			if action == "add" {
				path += ".extra"
			}
			if action == "remove" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte("changed"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if seal, err := PNPMCodeSeal(ctx, destination); err == nil && seal == got.CodeSeal {
				t.Fatal("dependency change escaped closure")
			}
			if action == "add" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, original, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(destination, "agent", "session.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if seal, err := PNPMCodeSeal(ctx, destination); err != nil || seal != got.CodeSeal {
		t.Fatal("mutable agent data sealed", err)
	}
	if _, err := StageNativePNPMInstallation(ctx, destination, destination+"/pnpm/nested-stage", destination+"/pnpm/nested-final", location); err == nil {
		t.Fatal("source overlap accepted")
	}
	bad := filepath.Join(destination, "pnpm/bin/0-unexpected")
	if err := os.WriteFile(bad, []byte(destination+"/pnpm/private"), 0600); err != nil {
		t.Fatal(err)
	}
	badStage, badFinal := filepath.Join(parent, "bad-stage"), filepath.Join(parent, "bad-final")
	if _, err := StageNativePNPMInstallation(ctx, destination, badStage, badFinal, location); err == nil {
		t.Fatal("unexpected operational reference accepted")
	}
	if _, err := os.Lstat(badFinal); !os.IsNotExist(err) {
		t.Fatal("rejected payload published")
	}
	if b, err := os.ReadFile(bad); err != nil || string(b) != destination+"/pnpm/private" {
		t.Fatal("rejected source changed", err)
	}
	unchanged, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
	if err != nil || !reflect.DeepEqual(before, unchanged) {
		t.Fatal("original DATA changed", err)
	}
}
