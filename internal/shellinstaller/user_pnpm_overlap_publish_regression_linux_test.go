//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIndependentPNPMOverlapPublish(t *testing.T) {
	if testing.Short() {
		t.Skip("requires the qualified retained producer fixture")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before := independentOverlapHash(t)
	receipt := independentOverlapReceipt(t)
	loc := independentOverlapLocation(t)
	parent := "/report/publish-parent"
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	stage, root := parent+"/stage", parent+"/final"
	profile := UserExperience{Persona: "neutral", BackgroundSubagents: "off"}
	start := time.Now()
	result, err := publishNativePNPMInstallationWithExperienceForTest(ctx, independentOverlapSource, stage, root, loc, &profile)
	seconds := time.Since(start).Seconds()
	if err != nil || result.State != "ComponentInstalled" {
		t.Fatal(result, err)
	}
	if _, err := os.Lstat(root + "/home/.npm"); !os.IsNotExist(err) {
		t.Fatal("producer acquisition cache was published", err)
	}
	if _, err := os.Lstat(root + "/pnpm/store/v11/files"); !os.IsNotExist(err) {
		t.Fatal("PNPM acquisition cache was published", err)
	}
	sourceHome, err := userPNPMPayloadInventory(ctx, independentOverlapSource, []string{"home", "pnpm"})
	if err != nil {
		t.Fatal(err)
	}
	finalHome, err := userPNPMPayloadInventory(ctx, root, []string{"home", "pnpm"})
	if err != nil {
		t.Fatal(err)
	}
	expectedHome := map[string]string{}
	for path, identity := range sourceHome {
		if userPNPMAcquisitionCache(path) {
			continue
		}
		if strings.Contains(identity, ":file:") {
			data, changed, err := userPNPMRelocation(ctx, filepath.Join(independentOverlapSource, path), path, independentOverlapSource, root)
			if err != nil {
				t.Fatal(err)
			}
			if changed {
				identity = strings.SplitN(identity, ":", 2)[0] + fmt.Sprintf(":file:%x", sha256.Sum256(data))
			}
		}
		identity = strings.Replace(identity, ":link:"+independentOverlapSource+"/", ":link:"+root+"/", 1)
		expectedHome[path] = identity
	}
	if !reflect.DeepEqual(expectedHome, finalHome) {
		t.Fatal("non-cache HOME or PNPM payload changed")
	}
	manifest, err := userReadManifest(ctx, root)
	if err != nil || manifest.PNPM == nil {
		t.Fatal("final manifest", err)
	}
	loc.PackageRoot = filepath.Join(root, manifest.PNPM.PackageRoot)
	loc.PNPMHome, loc.GlobalBin = root+"/pnpm", root+"/pnpm/bin"
	installed, err := ReadNativePNPMInstallation(ctx, root, loc)
	if err != nil {
		t.Fatal(err)
	}
	consumer := parent + "/consumer-package"
	cp := exec.CommandContext(ctx, "/bin/cp", "-a", "--no-preserve=ownership", "--", loc.PackageRoot, consumer)
	if out, err := cp.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	inventories := []map[string]string{}
	for _, pair := range [][2]string{{root, manifest.PNPM.PackageRoot}, {parent, "consumer-package"}} {
		entries, err := userPNPMPayloadInventory(ctx, pair[0], []string{pair[1]})
		if err != nil {
			t.Fatal(err)
		}
		normalized := map[string]string{}
		for p, id := range entries {
			normalized[strings.TrimPrefix(p, pair[1])] = id
		}
		inventories = append(inventories, normalized)
	}
	if !reflect.DeepEqual(inventories[0], inventories[1]) {
		t.Fatal("consumer copy differs")
	}
	if err := os.Symlink(filepath.Dir(loc.PackageRoot), parent+"/node_modules"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, filepath.Join(root, installed.Node), "--experimental-strip-types", "/source/e2e/shell-experience-canonical.mjs", root, "neutral", "off", consumer)
	cmd.Env = append(userEnvironment(root, root+"/pnpm", root+"/agent"), "NODE_OPTIONS=--v8-pool-size=1", "UV_THREADPOOL_SIZE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal("canonical consumer", err, string(out))
	}
	t.Log(string(out))
	if _, err := os.Lstat(stage); !os.IsNotExist(err) {
		t.Fatal("stage remains", err)
	}
	if before != independentOverlapHash(t) || !bytes.Equal(receipt, independentOverlapReceipt(t)) {
		t.Fatal("source changed")
	}
	b, err := json.MarshalIndent(map[string]any{"publisherSeconds": seconds, "result": result, "sourceEntries": 84345, "sourceInventoryHash": before, "receiptUnchanged": true, "consumer": string(out)}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/report/publication.json", b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("FULL_PUBLIC_PUBLISHER_SECONDS", seconds)
}
