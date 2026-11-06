package cli

import (
	"encoding/json"
	"fmt"
	"github.com/gentleman-programming/gentle-ai/v4/internal/installcmd"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPiInstallPreservesEngramSourceBeforePersistence(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		rejected          bool
	}{
		{"pin", `["npm:gentle-engram@0.1.16"]`, `["npm:gentle-engram@0.1.16"]`, false},
		{"object", `[{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, `[{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, false},
		{"bare and pin", `["npm:gentle-engram","npm:gentle-engram@0.1.16"]`, `["npm:gentle-engram@0.1.16"]`, false},
		{"bare and object", `["npm:gentle-engram",{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, `[{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, false},
		{"conflicting pins", `["npm:gentle-engram@0.1.16","npm:gentle-engram@0.2.0"]`, "", true},
		{"conflicting options", `[{"source":"npm:gentle-engram@0.1.16","extensions":[]},{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"]}]`, "", true},
		{"malformed settings", `invalid`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("PI_CODING_AGENT_DIR", "")
			path := filepath.Join(home, ".pi", "agent", "settings.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			initial := []byte(`{"theme":"kanagawa","packages":` + tt.input + `}`)
			if err := os.WriteFile(path, initial, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(installcmd.OverrideLookPath(func(string) (string, error) { return "pi", nil }))
			previous := runCommand
			t.Cleanup(func() { runCommand = previous })
			var commands []string
			runCommand = func(name string, args ...string) error {
				commands = append(commands, strings.Join(append([]string{name}, args...), " "))
				if tt.rejected {
					t.Fatal("rejected configuration executed a command")
				}
				var settings map[string]any
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(data, &settings); err != nil {
					t.Fatal(err)
				}
				// Mirror Pi 1.0.3's reproduced same-identity source replacement.
				if len(args) == 2 && strings.HasPrefix(args[1], "npm:gentle-engram") {
					for i, pkg := range settings["packages"].([]any) {
						source, _ := pkg.(string)
						object, isObject := pkg.(map[string]any)
						if isObject {
							source, _ = object["source"].(string)
						}
						if source != "npm:gentle-engram" && !strings.HasPrefix(source, "npm:gentle-engram@") {
							continue
						}
						if isObject {
							object["source"] = args[1]
						} else {
							settings["packages"].([]any)[i] = args[1]
						}
						break
					}
					updated, err := json.Marshal(settings)
					if err != nil {
						t.Fatal(err)
					}
					return os.WriteFile(path, updated, 0o644)
				}
				return nil
			}
			err := (agentInstallStep{agent: model.AgentPi, homeDir: home}).Run()
			actual, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tt.rejected {
				if err == nil || len(commands) != 0 || string(actual) != string(initial) {
					t.Fatalf("rejection changed state: error=%v commands=%v settings=%s", err, commands, actual)
				}
				if tt.input == "invalid" {
					wantErr := fmt.Sprintf("resolve install command for %q: unmarshal pi json file %q: invalid character 'i' looking for beginning of value", model.AgentPi, path)
					if err.Error() != wantErr {
						t.Fatalf("error = %q, want %q", err, wantErr)
					}
				} else {
					wantErr := fmt.Sprintf("resolve install command for %q: conflicting Engram package declarations in %q; resolve them before installing Pi packages", model.AgentPi, path)
					if err.Error() != wantErr {
						t.Fatalf("error = %q, want %q", err, wantErr)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(actual, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(`{"theme":"kanagawa","packages":`+tt.want+`}`), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("after Pi persistence: %s, want packages %s", actual, tt.want)
			}
		})
	}
}
