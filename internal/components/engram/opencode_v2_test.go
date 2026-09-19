package engram

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestExistingNativeEngramCommand(t *testing.T) {
	raw := []byte(`{"mcp":{"engram":{"command":["stale"]},"servers":{"engram":{"type":"local","command":["/custom/engram","mcp"],"disabled":true}}}}`)
	got, ok := existingMergedEngramCommand(raw, model.AgentOpenCode)
	if !ok || got != "/custom/engram" {
		t.Fatalf("command=(%q,%v)", got, ok)
	}
}

func TestNativeEngramOverlayPreservesUserSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	raw := []byte(`{"mcp":{"engram":{"type":"local","command":["stale"]},"servers":{"engram":{"type":"local","command":["/custom/engram","mcp"],"disabled":true,"environment":{"CUSTOM":"kept"}},"other":{"type":"remote","url":"https://example.invalid"}}}}`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	overlay, err := nativeOpenCodeEngramOverlay(path, engramOverlayJSON(model.AgentOpenCode, "/custom/engram"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mergeJSONFile(path, overlay); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	root, err := filemerge.UnmarshalJSONObject(data)
	if err != nil {
		t.Fatal(err)
	}
	mcp := root["mcp"].(map[string]any)
	servers := mcp["servers"].(map[string]any)
	entry := servers["engram"].(map[string]any)
	if entry["disabled"] != true || entry["environment"].(map[string]any)["CUSTOM"] != "kept" || servers["other"] == nil {
		t.Fatalf("user native fields lost: %#v", mcp)
	}
	if got := mcp["engram"].(map[string]any)["command"].([]any)[0]; got != "stale" {
		t.Fatalf("unrelated legacy server modified: %v", got)
	}
}
