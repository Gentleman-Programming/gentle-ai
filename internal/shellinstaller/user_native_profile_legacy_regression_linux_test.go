//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentLegacyPreparationBytes(t *testing.T) {
	root := installStagePath(t)
	profile := DefaultUserExperience()
	if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	for _, name := range []string{"persona.json", "background-subagents.json", "user.npmrc", "global.npmrc"} {
		data, err := os.ReadFile(filepath.Join(root, "config", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		values[name] = fmtHex(sum[:])
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("LEGACY_PROFILE_SIGNATURE", string(data))
}
func fmtHex(data []byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, len(data)*2)
	for i, b := range data {
		result[2*i] = hex[b>>4]
		result[2*i+1] = hex[b&15]
	}
	return string(result)
}
