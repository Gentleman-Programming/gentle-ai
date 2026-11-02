//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestUserWindowsLaunchBoundaryTiming measures the launch trust boundary
// against a production-sized supervisor image. The launch boundary reads and
// hashes the whole supervisor.exe on every start; this probe reports how long
// that actually takes on a real Windows runner (cold first read, then warm
// repeats) so any caching decision is made from measured cost, not guessed
// cost. The numbers land in the CI log: this test never fails on timing.
func TestUserWindowsLaunchBoundaryTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("timing probe")
	}
	root := userWindowsLaunchFixture(t)
	// Resize the supervisor image to the production supervisor size so the
	// read+SHA cost matches what every real launch pays.
	const productionSupervisorBytes = 40 << 20
	image := make([]byte, productionSupervisorBytes)
	pattern := []byte("gentle-shell launch boundary timing fixture\r\n")
	for i := 0; i < len(image); i += len(pattern) {
		copy(image[i:], pattern)
	}
	supervisorPath := filepath.Join(root, "supervisor.exe")
	if err := os.WriteFile(supervisorPath, image, 0600); err != nil {
		t.Fatal(err)
	}
	if err := userWindowsLaunchFixtureRepinSupervisor(t, root); err != nil {
		t.Fatal(err)
	}
	cold, err := timeUserWindowsVerifyLaunch(root)
	if err != nil {
		t.Fatalf("cold launch boundary refused the owned tree: %v", err)
	}
	t.Logf("launch boundary cold: %v", cold)
	warmTotal := time.Duration(0)
	var warmBest time.Duration
	for i := 0; i < 3; i++ {
		warm, err := timeUserWindowsVerifyLaunch(root)
		if err != nil {
			t.Fatalf("warm launch boundary refused the owned tree: %v", err)
		}
		warmTotal += warm
		if warmBest == 0 || warm < warmBest {
			warmBest = warm
		}
		t.Logf("launch boundary warm %d: %v", i+1, warm)
	}
	t.Logf("launch boundary warm mean: %v best: %v (supervisor image %d bytes)",
		warmTotal/3, warmBest, productionSupervisorBytes)
}

// userWindowsLaunchFixtureRepinSupervisor rewrites manifest.json so its
// SupervisorSHA matches the resized supervisor image, exactly as the real
// installer records the hash of the supervisor it publishes.
func userWindowsLaunchFixtureRepinSupervisor(t *testing.T, root string) error {
	t.Helper()
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest userWindowsManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	image, err := userWindowsRead(filepath.Join(root, "supervisor.exe"), 256<<20)
	if err != nil {
		return err
	}
	manifest.SupervisorSHA = userWindowsSHA(image)
	updated, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, append(updated, '\n'), 0600)
}

func timeUserWindowsVerifyLaunch(root string) (time.Duration, error) {
	start := time.Now()
	if err := userWindowsVerifyLaunch(context.Background(), root); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}
