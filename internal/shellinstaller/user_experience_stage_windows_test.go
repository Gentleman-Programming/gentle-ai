//go:build windows

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func windowsExperienceStage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestUserWindowsStageExperienceCanonicalFiles(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			t.Run(persona+"-"+background, func(t *testing.T) {
				root := windowsExperienceStage(t)
				config := filepath.Join(root, "config")
				identity, err := userWindowsIdentity(config, true)
				if err != nil {
					t.Fatal(err)
				}
				profile := UserExperience{Persona: persona, BackgroundSubagents: background}
				if err := userStageExperience(context.Background(), root, &profile); err != nil {
					t.Fatal(err)
				}
				files, err := userStageExperienceFiles(&profile)
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range files {
					data, err := userWindowsRead(filepath.Join(config, file.name), int64(len(file.data)))
					if err != nil || string(data) != string(file.data) {
						t.Fatalf("SID/DACL/custody or canonical readback differs: %q, %v", file.name, err)
					}
				}
				if fresh, err := userWindowsIdentity(config, true); err != nil || fresh != identity {
					t.Fatalf("configuration identity changed: %v", err)
				}
				if persona == "unmanaged" {
					if _, err := os.Lstat(filepath.Join(config, "persona.json")); !os.IsNotExist(err) {
						t.Fatalf("unmanaged created a persona: %v", err)
					}
				}
			})
		}
	}
}

// Main preview stages the same channel-independent RDD state as Stable (the
// TUI always emits GlobalRDD), in parity with the Linux Main route.
func TestUserWindowsStageExperienceAdvancedChoices(t *testing.T) {
	for _, tc := range []struct{ source, rdd, context7, enabled string }{
		{"stable", "off", "on", "true"},
		{"main-preview", "on", "on", "true"},
		{"main-preview", "off", "off", "false"},
	} {
		t.Run(tc.source+"-rdd-"+tc.rdd, func(t *testing.T) {
			root := windowsExperienceStage(t)
			profile := DefaultUserExperience()
			profile.SDKSource = tc.source
			profile.StrictTDD = "on"
			profile.GlobalRDD = tc.rdd
			profile.Context7 = tc.context7
			if err := userStageExperience(context.Background(), root, &profile); err != nil {
				t.Fatal(err)
			}
			for _, item := range []struct {
				path string
				want string
			}{
				{filepath.Join(root, "config", "strict-tdd.json"), `"policy": "on"`},
				{filepath.Join(root, "config", "background-subagents.json"), `"schema": "gentle-pi.background-subagents/v1"`},
				{filepath.Join(root, "home", ".gentle-ai", "state.json"), `"rdd_mode": "` + tc.rdd + `"`},
				{filepath.Join(root, "agent", "mcp.json"), `"enabled": ` + tc.enabled},
				{filepath.Join(root, "agent", "mcp.json"), `"url": "https://mcp.context7.com/mcp"`},
			} {
				data, err := userWindowsRead(item.path, 1<<20)
				if err != nil || !strings.Contains(string(data), item.want) {
					t.Fatalf("advanced preference %q = %q, %v", item.path, data, err)
				}
			}
		})
	}
}

func TestUserWindowsStageExperiencePreservesPreimages(t *testing.T) {
	for _, name := range []string{"background-subagents.json", "persona.json"} {
		t.Run(name, func(t *testing.T) {
			root := windowsExperienceStage(t)
			path := filepath.Join(root, "config", name)
			const preimage = "malformed retired personal bytes\n"
			if err := userWindowsWrite(path, []byte(preimage)); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: "neutral", BackgroundSubagents: "on"}
			if err := userStageExperience(context.Background(), root, &profile); err == nil {
				t.Fatal("existing preference admitted")
			}
			data, readErr := userWindowsRead(path, int64(len(preimage)))
			after, statErr := os.Stat(path)
			if readErr != nil || statErr != nil || string(data) != preimage || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
				t.Fatalf("preimage bytes/identity/mtime changed: %v, %v", readErr, statErr)
			}
			other := "persona.json"
			if name == other {
				other = "background-subagents.json"
			}
			if _, err := os.Lstat(filepath.Join(root, "config", other)); !os.IsNotExist(err) {
				t.Fatalf("wrote before checking every preimage: %v", err)
			}
			if name == "persona.json" {
				profile.Persona = "unmanaged"
				if err := userStageExperience(context.Background(), root, &profile); err != nil {
					t.Fatal(err)
				}
				data, readErr = userWindowsRead(path, int64(len(preimage)))
				after, statErr = os.Stat(path)
				if readErr != nil || statErr != nil || string(data) != preimage || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
					t.Fatal("unmanaged touched malformed persona")
				}
			}
		})
	}
}

func TestUserWindowsStageExperienceRefusesBeforeWrites(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		profile UserExperience
	}{
		{"invalid", context.Background(), UserExperience{}},
		{"explicit GGA", context.Background(), UserExperience{Persona: "neutral", BackgroundSubagents: "on", GGA: "on"}},
		{"canceled", canceled, DefaultUserExperience()},
		{"nil context", nil, DefaultUserExperience()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := windowsExperienceStage(t)
			if err := userStageExperience(tc.ctx, root, &tc.profile); err == nil {
				t.Fatal("unsupported stage admitted")
			}
			entries, err := os.ReadDir(filepath.Join(root, "config"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("refusal wrote configuration: %v", err)
			}
		})
	}
	if err := userStageExperience(nil, "not-a-private-path", nil); err != nil {
		t.Fatalf("nil legacy profile performed a lookup: %v", err)
	}
}

// CodeGraph is a supported Windows combination (the runtime stage was built
// for Windows deliberately): the experience stage admits the selection and
// writes the base preferences, while the CodeGraph runtime itself is staged
// by the install flow's own provision step after this writer returns.
func TestUserWindowsStageExperienceAdmitsCodeGraphSelection(t *testing.T) {
	root := windowsExperienceStage(t)
	profile := UserExperience{Persona: "neutral", BackgroundSubagents: "on", CodeGraph: "on"}
	if err := userStageExperience(context.Background(), root, &profile); err != nil {
		t.Fatalf("CodeGraph selection refused at the experience stage: %v", err)
	}
	config := filepath.Join(root, "config")
	for _, name := range []string{"background-subagents.json", "persona.json"} {
		if _, err := os.Lstat(filepath.Join(config, name)); err != nil {
			t.Fatalf("CodeGraph admission skipped a base preference: %v", err)
		}
	}
}
