package opencoderuntimeplugins

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// Ratchet: every embedded plugin asset must stay recognizable as Gentle
// AI-owned after it ships, so changing an asset requires regenerating the
// registry (go generate ./internal/components/opencoderuntimeplugins/).
func TestReleasedPluginDigestsCoverEmbeddedAssets(t *testing.T) {
	names := append(ManagedPluginNames(model.AgentOpenCode), retiredPluginNames(model.AgentOpenCode)...)
	slices.Sort(names)
	if got := slices.Sorted(maps.Keys(releasedPluginDigests)); !slices.Equal(got, names) {
		t.Fatalf("registry names = %v, want lifecycle names %v", got, names)
	}
	for _, name := range names {
		for _, dir := range []string{"opencode/plugins/", "opencode/plugins-v2/"} {
			data, err := assets.Read(dir + name)
			if err != nil {
				continue
			}
			if sum := sha256.Sum256([]byte(data)); !releasedPlugin(name, []byte(data)) {
				t.Errorf("embedded %s%s digest %s missing from releasedPluginDigests; run go generate ./internal/components/opencoderuntimeplugins/", dir, name, hex.EncodeToString(sum[:]))
			}
		}
	}
}
