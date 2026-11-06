//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skipUnqualifiedStableHost keeps these Stable profile tests on the two
// qualified unix distributions: Linux amd64 and macOS arm64.
func skipUnqualifiedStableHost(t *testing.T) {
	t.Helper()
	if !(runtime.GOOS == "linux" && runtime.GOARCH == "amd64") && !(runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") {
		t.Skip("Stable experience is qualified on Linux amd64 and macOS arm64")
	}
}

// The TUI always emits explicit ON/OFF values with CodeGraph OFF and GGA OFF.
// Every such Stable profile reaches physical review and stages the same
// canonical files on Linux amd64 and macOS arm64.
func TestUserStableTUIProfileStagesCanonicalFiles(t *testing.T) {
	skipUnqualifiedStableHost(t)
	toggles := []string{"on", "off"}
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range toggles {
			for _, tdd := range toggles {
				for _, rdd := range toggles {
					for _, context7 := range toggles {
						for _, sdk := range []string{"", "stable"} {
							name := strings.Join([]string{persona, background, tdd, rdd, context7, sdk}, "-")
							t.Run(name, func(t *testing.T) {
								profile := UserExperience{Persona: persona, BackgroundSubagents: background, StrictTDD: tdd, GlobalRDD: rdd, Context7: context7, CodeGraph: "off", GGA: "off", SDKSource: sdk}
								target := filepath.Join(userUnixTempRoot(t), "target")
								req := UserInstallRequest{Destination: target, Mode: "separate", Experience: &profile}
								if token, err := InspectUserInstall(req); err != nil || token == "" {
									t.Fatalf("Stable TUI profile refused at physical review: %q, %v", token, err)
								}
								if _, err := os.Lstat(target); !os.IsNotExist(err) {
									t.Fatal("inspection created the target before consent")
								}
								if err := userPrepareInstallStage(context.Background(), target, req.Mode, req.Experience); err != nil {
									t.Fatalf("Stable TUI profile refused at the stage: %v", err)
								}
								assertStableCanonicalFiles(t, target, profile)
							})
						}
					}
				}
			}
		}
	}
}

func assertStableCanonicalFiles(t *testing.T, root string, profile UserExperience) {
	t.Helper()
	assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \""+profile.BackgroundSubagents+"\"\n}\n")
	assertExperienceFile(t, filepath.Join(root, "config/strict-tdd.json"), "{\n  \"schema\": \"gentle-pi.strict-tdd/v1\",\n  \"policy\": \""+profile.StrictTDD+"\"\n}\n")
	personaPath := filepath.Join(root, "config/persona.json")
	if profile.Persona == "unmanaged" {
		if _, err := os.Lstat(personaPath); !os.IsNotExist(err) {
			t.Fatalf("unmanaged persona wrote a file: %v", err)
		}
	} else {
		assertExperienceFile(t, personaPath, "{\n  \"mode\": \""+profile.Persona+"\"\n}\n")
	}
	var state struct {
		RDD        string `json:"rdd_mode"`
		RecordedAt string `json:"rdd_mode_recorded_at"`
	}
	data, err := os.ReadFile(filepath.Join(root, "home/.gentle-ai/state.json"))
	if err != nil || json.Unmarshal(data, &state) != nil || state.RDD != profile.GlobalRDD || state.RecordedAt == "" {
		t.Fatalf("global RDD state differs: %s, %v", data, err)
	}
	var mcp struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	data, err = os.ReadFile(filepath.Join(root, "agent/mcp.json"))
	if err != nil || json.Unmarshal(data, &mcp) != nil {
		t.Fatalf("Context7 MCP entry is missing: %s, %v", data, err)
	}
	entry := mcp.Servers["context7"]
	if len(mcp.Servers) != 1 || len(entry) != 3 || entry["url"] != "https://mcp.context7.com/mcp" || entry["enabled"] != (profile.Context7 == "on") || entry["exposure"] != "codemode" {
		t.Fatalf("Context7 MCP entry differs: %s", data)
	}
}

// Main preview reuses the Stable private writers on both qualified unix
// distributions: the TUI defaults (CodeGraph ON) and the all-OFF Custom
// profile reach physical review and stage the same canonical files. The Main
// composition itself is produced after the runtime, never by the stage, and
// the retained Shared prefix keeps refusing Main.
func TestUserMainPreviewStagesCanonicalFiles(t *testing.T) {
	skipUnqualifiedStableHost(t)
	for _, profile := range []UserExperience{
		{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", Context7: "on", CodeGraph: "on", GGA: "off", SDKSource: "main-preview"},
		{Persona: "unmanaged", BackgroundSubagents: "off", StrictTDD: "off", GlobalRDD: "off", Context7: "off", CodeGraph: "off", GGA: "off", SDKSource: "main-preview"},
	} {
		t.Run(profile.Persona, func(t *testing.T) {
			target := filepath.Join(userUnixTempRoot(t), "target")
			req := UserInstallRequest{Destination: target, Mode: "separate", Experience: &profile}
			if token, err := InspectUserInstall(req); err != nil || token == "" {
				t.Fatalf("Main preview refused at physical review: %q, %v", token, err)
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("inspection created the target before consent")
			}
			if err := userPrepareInstallStage(context.Background(), target, req.Mode, req.Experience); err != nil {
				t.Fatalf("Main preview refused at the stage: %v", err)
			}
			assertStableCanonicalFiles(t, target, profile)
			for _, relative := range userUnixOverlayMainPaths {
				if _, err := os.Lstat(filepath.Join(target, relative)); !os.IsNotExist(err) {
					t.Fatalf("the stage invented the Main composition %s: %v", relative, err)
				}
			}
			shared := filepath.Join(userUnixTempRoot(t), "shared")
			if err := userPrepareInstallStage(context.Background(), shared, "shared", &profile); err == nil || !strings.Contains(err.Error(), "Main SDK requires a Separate or agent stage") {
				t.Fatalf("Main reached the Shared stage: %v", err)
			}
			if _, err := os.Lstat(shared); !os.IsNotExist(err) {
				t.Fatal("refused Shared stage created the target")
			}
		})
	}
}

// The TUI defaults (CodeGraph ON) reach physical review and stage the same
// canonical files on macOS arm64 as on Linux amd64. The library itself is
// provisioned after the runtime, never by the stage.
func TestUserStableTUIDefaultsAdmitCodeGraph(t *testing.T) {
	skipUnqualifiedStableHost(t)
	for _, mode := range []string{"separate", "shared"} {
		t.Run(mode, func(t *testing.T) {
			profile := UserExperience{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", Context7: "on", CodeGraph: "on", GGA: "off"}
			if err := userExperienceSupported(&profile); err != nil {
				t.Fatalf("CodeGraph ON refused at admission: %v", err)
			}
			target := filepath.Join(userUnixTempRoot(t), "target")
			if mode == "separate" {
				req := UserInstallRequest{Destination: target, Mode: mode, Experience: &profile}
				if token, err := InspectUserInstall(req); err != nil || token == "" {
					t.Fatalf("CodeGraph ON refused at physical review: %q, %v", token, err)
				}
			}
			if err := userPrepareInstallStage(context.Background(), target, mode, &profile); err != nil {
				t.Fatalf("CodeGraph ON refused at the stage: %v", err)
			}
			if mode == "separate" {
				// Shared stages Context7 into the selected agent later.
				assertStableCanonicalFiles(t, target, profile)
			}
			if _, err := os.Lstat(filepath.Join(target, "runtime/codegraph")); !os.IsNotExist(err) {
				t.Fatal("stage materialized the provisioned CodeGraph runtime")
			}
		})
	}
}
