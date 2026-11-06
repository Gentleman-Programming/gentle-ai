//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Each case runs in its own bounded sandbox; namespace teardown owns payload
// cleanup. These are actual retained-package consumers, not a duplicate codec.
func TestUserPNPMExperience(t *testing.T) {
	if testing.Short() {
		t.Skip("requires original installer DATA in the qualified private sandbox")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []UserExperience{
		{Persona: "gentleman", BackgroundSubagents: "on"},
		{Persona: "neutral", BackgroundSubagents: "off"},
		{Persona: "unmanaged", BackgroundSubagents: "off"},
	} {
		t.Run(profile.Persona+"-"+profile.BackgroundSubagents, func(t *testing.T) {
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
			parent := filepath.Join("/tmp", "pnpm-experience-"+profile.Persona)
			if err := os.Mkdir(parent, 0700); err != nil {
				t.Fatal(err)
			}
			stage, root := filepath.Join(parent, "stage"), filepath.Join(parent, "final")
			got, err := publishNativePNPMInstallationWithExperienceForTest(ctx, "/out", stage, root, location, &profile)
			if err != nil || got.State != "ComponentInstalled" || got.Destination != root {
				t.Fatal("profiled publication failed", got, err)
			}
			manifest, err := userReadManifest(ctx, root)
			if err != nil || manifest.PNPM == nil {
				t.Fatal("final profiled manifest rejected", err)
			}
			location.PackageRoot = filepath.Join(root, manifest.PNPM.PackageRoot)
			location.PNPMHome, location.GlobalBin = root+"/pnpm", root+"/pnpm/bin"
			installed, err := ReadNativePNPMInstallation(ctx, root, location)
			if err != nil {
				t.Fatal(err)
			}
			// Node refuses native type stripping below node_modules. Reuse the
			// canonical isolated-package consumer pattern with byte-identical
			// package DATA, not rewritten code or a rebuilt supplier.
			consumer := filepath.Join(parent, "consumer-package")
			copy := exec.CommandContext(ctx, "/bin/cp", "-a", "--no-preserve=ownership", "--", location.PackageRoot, consumer)
			if output, err := copy.CombinedOutput(); err != nil {
				t.Fatal("private consumer package copy", err, string(output))
			}
			inventories := make([]map[string]string, 0, 2)
			for _, selection := range [][2]string{{root, manifest.PNPM.PackageRoot}, {parent, "consumer-package"}} {
				entries, err := userPNPMPayloadInventory(ctx, selection[0], []string{selection[1]})
				if err != nil {
					t.Fatal(err)
				}
				normalized := map[string]string{}
				for path, identity := range entries {
					normalized[strings.TrimPrefix(path, selection[1])] = identity
				}
				inventories = append(inventories, normalized)
			}
			if !reflect.DeepEqual(inventories[0], inventories[1]) {
				t.Fatal("consumer package bytes, modes or links differ")
			}
			// Retain the authenticated PNPM peer graph for bare package imports;
			// the copied supplier files themselves remain byte-identical.
			if err := os.Symlink(filepath.Dir(location.PackageRoot), filepath.Join(parent, "node_modules")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, filepath.Join(root, installed.Node), "--experimental-strip-types", "/source/e2e/shell-experience-canonical.mjs", root, profile.Persona, profile.BackgroundSubagents, consumer)
			cmd.Env = append(userEnvironment(root, root+"/pnpm", root+"/agent"), "NODE_OPTIONS=--v8-pool-size=1", "UV_THREADPOOL_SIZE=1")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("actual PNPM preference consumer: %v\n%s", err, output)
			} else {
				t.Log(string(output))
			}
			if _, err := os.Lstat(stage); !os.IsNotExist(err) {
				t.Fatal("unpublished stage remains", err)
			}
			after, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("producer DATA changed", err)
			}
		})
	}
}

func TestUserPNPMExperienceRefusal(t *testing.T) {
	for _, name := range []string{"invalid", "rdd", "tdd", "context7", "gga", "codegraph", "sdk"} {
		t.Run(name, func(t *testing.T) {
			profile := DefaultUserExperience()
			switch name {
			case "invalid":
				profile.Persona = "invalid"
			case "rdd":
				profile.GlobalRDD = "off"
			case "tdd":
				profile.StrictTDD = "off"
			case "context7":
				profile.Context7 = "off"
			case "gga":
				profile.GGA = "off"
			case "codegraph":
				profile.CodeGraph = "off"
			case "sdk":
				profile.SDKSource = "main-preview"
			}
			parent := t.TempDir()
			before, err := userTreeStamp(parent)
			if err != nil {
				t.Fatal(err)
			}
			got, err := publishNativePNPMInstallationWithExperienceForTest(context.Background(), parent+"/missing", parent+"/stage", parent+"/final", NativeInstallationLocation{}, &profile)
			if err == nil || got.State != "" {
				t.Fatal("unsupported profile accepted", got, err)
			}
			if after, err := userTreeStamp(parent); err != nil || after != before {
				t.Fatal("refused profile changed DATA", err)
			}
		})
	}
}
