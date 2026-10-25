//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testing.TempDir creates its numbered directory with the process umask, so a
// strict private parent must be normalized before staging.
func agreedStageParent(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return parent
}

func agreedDefaultsProfile() UserExperience {
	return UserExperience{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", CodeGraph: "on", Context7: "on", GGA: "off"}
}

func TestUserAgreedDefaultsReachInspection(t *testing.T) {
	profile := agreedDefaultsProfile()
	parent := agreedStageParent(t)
	before, err := userTreeStamp(parent)
	if err != nil {
		t.Fatal(err)
	}
	req := UserInstallRequest{Destination: filepath.Join(parent, "target"), Mode: "separate", Experience: &profile}
	token, err := InspectUserInstall(req)
	if err != nil || token == "" {
		t.Errorf("agreed defaults cannot reach physical review: %v", err)
	}
	if profile != agreedDefaultsProfile() {
		t.Fatal("inspection cleared an explicit choice")
	}
	if after, err := userTreeStamp(parent); err != nil || after != before {
		t.Fatal("inspection changed the selected parent", err)
	}
	for _, scenario := range []string{"shared", "main-combination", "gga-without-provider", "gga-on-with-provider"} {
		t.Run(scenario, func(t *testing.T) {
			changed, selection := profile, req
			selection.Experience = &changed
			switch scenario {
			case "shared":
				selection.Mode = "shared"
			case "main-combination":
				changed.SDKSource = "main-preview"
			case "gga-without-provider":
				changed.GGA = "on"
			case "gga-on-with-provider":
				changed.GGA, changed.GGAProvider = "on", "codex"
			}
			_, err := InspectUserInstall(selection)
			switch scenario {
			case "main-combination", "gga-on-with-provider", "gga-without-provider":
				if err != nil {
					t.Fatalf("qualified selection refused (%s): %v", scenario, err)
				}
			default:
				if err == nil {
					t.Fatal("defaults bypassed an unsupported selection")
				}
			}
		})
	}
}

func TestUserAgreedDefaultsKeepProducerUnprofiled(t *testing.T) {
	profile := agreedDefaultsProfile()
	root := filepath.Join(agreedStageParent(t), "producer")
	if err := userPrepareNativeProducer(context.Background(), root, &profile); err != nil {
		t.Fatalf("agreed defaults cannot prepare the native producer: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "config", "strict-tdd.json")); !os.IsNotExist(err) {
		t.Fatal("producer materialized final preferences before the copied stage", err)
	}
	if profile != agreedDefaultsProfile() {
		t.Fatal("producer preparation discarded the reviewed profile")
	}
}

func TestUserAgreedDefaultsPrivateConsumers(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(agreedStageParent(t), "stage")
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	privateState := filepath.Join(root, "home", ".gentle-ai")
	if err := os.Mkdir(privateState, 0700); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(privateState, "state.json")
	if err := os.WriteFile(stateFile, []byte(`{"installed_agents":["pi"],"producer_owned":17}`), 0600); err != nil {
		t.Fatal(err)
	}
	mcpFile := filepath.Join(root, "agent", "mcp.json")
	if err := os.WriteFile(mcpFile, []byte(`{"mcpServers":{"engram":{"command":"producer-owned"}},"producer_owned":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	t.Setenv("GENTLE_AI_HOME", outside)
	profile := agreedDefaultsProfile()
	profile.CodeGraph = "off" // Library acquisition is a different, authenticated stage.
	if err := userStageExperience(ctx, root, &profile); err != nil {
		t.Fatalf("agreed defaults have no private preference consumers: %v", err)
	}
	read := func(path string) map[string]json.RawMessage {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	persisted := read(stateFile)
	if string(persisted["rdd_mode"]) != `"on"` || persisted["rdd_mode_recorded_at"] == nil || string(persisted["producer_owned"]) != "17" {
		t.Fatal("private RDD selection or producer state was lost", persisted)
	}
	var recorded time.Time
	if err := json.Unmarshal(persisted["rdd_mode_recorded_at"], &recorded); err != nil || recorded.IsZero() {
		t.Fatal("private RDD cutoff is missing or invalid", err)
	}
	if got := read(filepath.Join(root, "config", "strict-tdd.json")); string(got["policy"]) != `"on"` {
		t.Fatal("Strict TDD did not reach its canonical consumer", got)
	}
	mcp := read(mcpFile)
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcp["mcpServers"], &servers); err != nil || servers["context7"] == nil || servers["engram"] == nil || string(mcp["producer_owned"]) != "true" {
		t.Fatal("Context7 did not preserve the generated agent config", mcp, err)
	}
	var context7 struct {
		URL, Exposure string
		Enabled       bool
	}
	if err := json.Unmarshal(servers["context7"], &context7); err != nil || context7.URL != "https://mcp.context7.com/mcp" || !context7.Enabled || context7.Exposure != "codemode" {
		t.Fatal("Context7 selection did not reach the existing MCP contract", context7, err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("private staging followed the operator-state override", err)
	}
}

func TestUserAgreedDefaultsUnsafeStateRefuses(t *testing.T) {
	for _, scenario := range []string{"malformed", "unknown-mode", "bad-cutoff", "duplicate", "case-alias", "symlink", "hardlink"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(agreedStageParent(t), "stage")
			if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(root, "home", ".gentle-ai")
			if err := os.Mkdir(directory, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "state.json")
			outside := filepath.Join(t.TempDir(), "untouched")
			if err := os.WriteFile(outside, []byte(`{"producer_owned":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			invalid := map[string]string{"malformed": "{", "unknown-mode": `{"rdd_mode":"broken"}`, "bad-cutoff": `{"rdd_mode_recorded_at":"broken"}`, "duplicate": `{"rdd_mode":"on","RDD_MODE":"off"}`, "case-alias": `{"RDD_MODE":"off"}`}
			var err error
			switch scenario {
			case "symlink":
				err = os.Symlink(outside, path)
			case "hardlink":
				err = os.Link(outside, path)
			default:
				err = os.WriteFile(path, []byte(invalid[scenario]), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			profile := DefaultUserExperience()
			profile.GlobalRDD = "on"
			if err := userStageExperience(ctx, root, &profile); err == nil {
				t.Fatal("unsafe generated state was silently repaired")
			}
			if data, err := os.ReadFile(outside); err != nil || string(data) != `{"producer_owned":true}` {
				t.Fatal("unsafe preimage affected an unrelated file", err)
			}
		})
	}
}

func TestUserAgreedDefaultsUnsafeMCPRefuses(t *testing.T) {
	for index, raw := range []string{
		`[]`, `{"mcpServers":[]}`, `{"mcpServers":{"context7":{"url":"https://untrusted.invalid"}}}`,
		`{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","enabled":"broken"}}}`,
		`{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","exposure":17}}}`,
		`{"mcpServers":{"context7":{"url":"https://mcp.context7.com/mcp","enabled":true,"Enabled":false}}}`,
	} {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0700); err != nil { // testing.TempDir uses the process umask.
				t.Fatal(err)
			}
			root := filepath.Join(parent, "stage")
			if err := userPrepareInstallStage(context.Background(), root, "separate", nil); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "agent", "mcp.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			profile := DefaultUserExperience()
			profile.Context7 = "on"
			if err := userStageExperience(context.Background(), root, &profile); err == nil {
				t.Fatal("unsafe MCP preimage was silently repaired")
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != raw {
				t.Fatal("refusal changed the MCP preimage", err)
			}
		})
	}
}

func TestUserAgreedDefaultsPreparedSourcesRefuseBeforeEffects(t *testing.T) {
	for _, requested := range []bool{true, false} {
		root := t.TempDir()
		before, err := userTreeStamp(root)
		if err != nil {
			t.Fatal(err)
		}
		profile := DefaultUserExperience()
		var archives [][]byte
		if requested {
			profile.CodeGraph = "on"
		} else {
			archives = [][]byte{[]byte("unrequested")}
		}
		_, err = userPublishNativePNPMPrepared(context.Background(), root+"/missing", root+"/stage", root+"/final", NativeInstallationLocation{}, &profile, archives)
		if err == nil || !strings.Contains(err.Error(), "prepared CodeGraph source set differs") {
			t.Fatal("prepared source set reached publication effects", err)
		}
		if after, err := userTreeStamp(root); err != nil || before != after {
			t.Fatal("refused source set changed private paths", err)
		}
	}
}
