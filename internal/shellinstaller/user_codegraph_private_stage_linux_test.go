//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Private preparation is not public inspection, acquisition or installed Ready.
// ON reaches the real producer's cancellation boundary without network effects;
// OFF reaches its no-acquisition path. No selected preference is cleared.
func TestUserCodeGraphPrivateSeparateStage(t *testing.T) {
	for _, choice := range []string{"on", "off"} {
		for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
			for _, background := range []string{"on", "off"} {
				t.Run(choice+"/"+persona+"/"+background, func(t *testing.T) {
					root := installStagePath(t)
					profile := UserExperience{Persona: persona, BackgroundSubagents: background, CodeGraph: choice}
					before := profile
					encoded, err := profile.Encode()
					if err != nil {
						t.Fatal(err)
					}
					if err := userPrepareInstallStage(context.Background(), root, "separate", &profile); err != nil {
						t.Fatal("qualified private preparation refused", err)
					}
					assertExperienceFile(t, filepath.Join(root, "config/background-subagents.json"), "{\n  \"schema\": \"gentle-pi.background-subagents/v1\",\n  \"policy\": \""+background+"\"\n}\n")
					if persona == "unmanaged" {
						if _, err := os.Lstat(filepath.Join(root, "config/persona.json")); !os.IsNotExist(err) {
							t.Fatal("unmanaged acquired a persona", err)
						}
					} else {
						assertExperienceFile(t, filepath.Join(root, "config/persona.json"), "{\n  \"mode\": \""+persona+"\"\n}\n")
					}
					stamp, err := userTreeStamp(root)
					if err != nil {
						t.Fatal(err)
					}
					if choice == "off" {
						if err := userCodeGraphProvision(context.Background(), root, "separate", &profile); err != nil {
							t.Fatal("OFF attempted acquisition", err)
						}
					} else {
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						if err := userCodeGraphProvision(ctx, root, "separate", &profile); !errors.Is(err, context.Canceled) {
							t.Fatal("ON failed before the real producer cancellation boundary", err)
						}
					}
					if files, err := userStageExperienceFiles(&profile); err != nil || len(files) == 0 {
						t.Fatal("qualified stock profile lost the generic writer", files, err)
					}
					if err := userExperienceSupported(&profile); err != nil {
						t.Fatalf("qualified stock profile was refused: %v", err)
					}
					main := profile
					main.SDKSource = "main-preview"
					if err := userExperienceSupported(&main); err != nil {
						t.Fatalf("Main lost the qualified CodeGraph profile: %v", err)
					}
					if files, err := userStageExperienceFiles(&main); err == nil || files != nil || !strings.Contains(err.Error(), "Main SDK") {
						t.Fatal("stock PNPM stage admitted Main without its producer", files, err)
					}
					after, err := userTreeStamp(root)
					if err != nil || after != stamp {
						t.Fatal("non-acquiring producer/refusal paths changed the stage", err)
					}
					if _, err := os.Lstat(filepath.Join(root, "runtime")); !os.IsNotExist(err) {
						t.Fatal("preparation or refused acquisition created a runtime", err)
					}
					afterEncoded, err := profile.Encode()
					if err != nil || profile != before || afterEncoded != encoded {
						t.Fatal("private composition changed selected confirmation bytes", err)
					}
				})
			}
		}
	}
}

func TestUserCodeGraphPrivateStageRefusesNonPrivateParent(t *testing.T) {
	for _, choice := range []string{"on", "off"} {
		t.Run(choice, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0755); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(parent)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: "neutral", BackgroundSubagents: "off", CodeGraph: choice}
			original := profile
			root := filepath.Join(parent, "stage")
			err = userPrepareInstallStage(context.Background(), root, "separate", &profile)
			var refusal *PrivateRuntimeError
			if !errors.As(err, &refusal) || refusal.Kind != "refused" || !strings.Contains(err.Error(), "filesystem") {
				t.Fatal("nonprivate parent bypassed physical admission", err)
			}
			if profile != original {
				t.Fatal("parent refusal changed original CodeGraph selection")
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("parent refusal created its destination", err)
			}
			after, err := os.Lstat(parent)
			if err != nil || privateStamp(before) != privateStamp(after) {
				t.Fatal("parent refusal changed parent custody or metadata", err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("parent refusal wrote before admission", entries, err)
			}
		})
	}
}
