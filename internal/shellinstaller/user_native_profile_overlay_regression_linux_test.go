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

func TestIndependentNativeProfileOverlayPrepare(t *testing.T) {
	if testing.Short() {
		t.Skip("requires qualified producer config overlay")
	}
	for _, name := range []string{"gentleman", "neutral", "unmanaged", "prewritten", "malformed"} {
		profile := UserExperience{Persona: name, BackgroundSubagents: "off"}
		if name == "gentleman" {
			profile.BackgroundSubagents = "on"
		}
		if name == "prewritten" || name == "malformed" {
			profile.Persona = "neutral"
		}
		path := "/report/overlay-" + name
		var err error
		if name == "prewritten" {
			err = userPrepareInstallStage(context.Background(), path, "separate", &profile)
		} else {
			err = userPrepareNativeProducer(context.Background(), path, &profile)
		}
		if err != nil {
			t.Fatal(name, err)
		}
		if name == "malformed" {
			if err := os.WriteFile(filepath.Join(path, "config/background-subagents.json"), []byte("malformed existing preference\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if name != "prewritten" && name != "malformed" {
			for _, file := range []string{"persona.json", "background-subagents.json"} {
				if _, err := os.Lstat(filepath.Join(path, "config", file)); !os.IsNotExist(err) {
					t.Fatal("native producer prematurely materialized", name, file, err)
				}
			}
		}
	}
	t.Log("NATIVE_PREPARED_OVERLAYS_NO_FINAL_PREFERENCES")
}
func TestIndependentNativeProfilePublicCollision(t *testing.T) {
	if testing.Short() {
		t.Skip("requires qualified producer config overlay")
	}
	ctx := context.Background()
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	var location NativeInstallationLocation
	data, err := os.ReadFile("/out/verified-location.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &location); err != nil {
		t.Fatal(err)
	}
	before, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
	if err != nil {
		t.Fatal(err)
	}
	parent := "/tmp/profile-collision"
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	profile := UserExperience{Persona: "neutral", BackgroundSubagents: "off"}
	started := time.Now()
	result, callErr := publishNativePNPMInstallationWithExperienceForTest(ctx, "/out", parent+"/stage", parent+"/final", location, &profile)
	if callErr == nil || !strings.Contains(callErr.Error(), "staged preference already exists") || result.State != "" {
		t.Fatal("existing preference overwritten or accepted", result, callErr)
	}
	for _, path := range []string{parent + "/stage", parent + "/final"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("refused publication path remains", path, err)
		}
	}
	after, err := userPNPMPayloadInventory(ctx, "/out", userPNPMRoots)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("collision changed producer", err)
	}
	t.Log("PUBLIC_COLLISION_PRESERVED", time.Since(started), len(before))
}
