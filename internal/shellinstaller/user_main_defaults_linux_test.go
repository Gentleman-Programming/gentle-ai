//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mainDefaultsProfile() UserExperience {
	return UserExperience{
		Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on",
		GlobalRDD: "on", CodeGraph: "on", Context7: "on", GGA: "off", SDKSource: "main-preview",
	}
}

// Main preview reuses the same private preference consumers as Stable on the
// legacy authenticated Main worker; CodeGraph and GGA remain incompatible.
func TestUserMainDefaultsAdmission(t *testing.T) {
	profile := mainDefaultsProfile()
	if err := userExperienceSupported(&profile); err != nil {
		t.Fatalf("qualified Main defaults refused: %v", err)
	}
	// Every preference that the stock path consumes also works with Main, and
	// GGA ON is delegated to the legacy installer that owns it.
	withGGA := profile
	withGGA.GGA, withGGA.GGAProvider = "on", "codex"
	if err := userExperienceSupported(&withGGA); err != nil {
		t.Fatalf("qualified GGA selection was refused: %v", err)
	}
	stock := profile
	stock.SDKSource = ""
	if err := userExperienceSupported(&stock); err != nil {
		t.Fatalf("qualified stock selection was refused: %v", err)
	}
	stockGGA := stock
	stockGGA.GGA, stockGGA.GGAProvider = "on", "codex"
	if err := userExperienceSupported(&stockGGA); err != nil {
		t.Fatalf("stock GGA selection was refused: %v", err)
	}
	unprovided := withGGA
	unprovided.GGAProvider = ""
	if err := userExperienceSupported(&unprovided); err != nil {
		t.Fatalf("GGA ON without a provider was refused: %v", err)
	}
	// The PNPM publication route has no Main producer; that refusal must stay.
	err := userValidateStageExperience(&profile)
	if err == nil || !strings.Contains(err.Error(), "Main SDK requires its authenticated runtime producer") {
		t.Fatalf("Main must stay out of the stock PNPM publication route: %v", err)
	}
}

func TestUserMainDefaultsStageConsumesSelections(t *testing.T) {
	profile := mainDefaultsProfile()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "installed")
	if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
		t.Fatalf("Main stage refused its qualified selections: %v", err)
	}
	assertExperienceFile(t, filepath.Join(root, "config/strict-tdd.json"), "{\n  \"schema\": \"gentle-pi.strict-tdd/v1\",\n  \"policy\": \"on\"\n}\n")
	assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \"on\"\n}\n")
	assertExperienceFile(t, filepath.Join(root, "config/persona.json"), "{\n  \"mode\": \"gentleman\"\n}\n")

	state, err := os.ReadFile(filepath.Join(root, "home/.gentle-ai/state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(state, &fields); err != nil {
		t.Fatalf("RDD state is not canonical JSON: %v", err)
	}
	var mode string
	if err := json.Unmarshal(fields["rdd_mode"], &mode); err != nil || mode != "on" {
		t.Fatalf("Main did not consume the selected RDD mode: %s, %v", state, err)
	}
	if _, ok := fields["rdd_mode_recorded_at"]; !ok {
		t.Fatalf("RDD cutoff missing: %s", state)
	}

	mcp, err := os.ReadFile(filepath.Join(root, "agent/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var servers struct {
		MCPServers map[string]struct {
			URL      string `json:"url"`
			Enabled  bool   `json:"enabled"`
			Exposure string `json:"exposure"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(mcp, &servers); err != nil {
		t.Fatalf("Main MCP selection is not canonical JSON: %v", err)
	}
	context7, ok := servers.MCPServers["context7"]
	if !ok || context7.URL != "https://mcp.context7.com/mcp" || !context7.Enabled || context7.Exposure != "codemode" {
		t.Fatalf("Main did not consume the selected Context7 entry: %s", mcp)
	}
	if _, err := os.Lstat(filepath.Join(root, "runtime/codegraph")); !os.IsNotExist(err) {
		t.Fatalf("Main staged an unsupported CodeGraph runtime: %v", err)
	}
}
