//go:build linux && amd64

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestUserNativeProducerPreferences(t *testing.T) {
	for _, profile := range []*UserExperience{
		nil,
		{Persona: "gentleman", BackgroundSubagents: "on"},
		{Persona: "neutral", BackgroundSubagents: "off"},
		{Persona: "unmanaged", BackgroundSubagents: "off"},
	} {
		name := "nil"
		var beforeProfile UserExperience
		if profile != nil {
			name = profile.Persona
			beforeProfile = *profile
		}
		t.Run(name, func(t *testing.T) {
			root := installStagePath(t)
			ctx := context.Background()
			if err := userPrepareNativeProducer(ctx, root, profile); err != nil {
				t.Fatal(err)
			}
			files, err := userExperienceConfigFiles(profile)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range files {
				if _, err := os.Lstat(filepath.Join(root, "config", file.name)); !os.IsNotExist(err) {
					t.Fatalf("producer wrote publication-only preference %s: %v", file.name, err)
				}
			}
			for _, name := range []string{"user.npmrc", "global.npmrc"} {
				info, err := os.Stat(filepath.Join(root, "config", name))
				if err != nil || info.Mode() != 0600 || info.Size() != 0 {
					t.Fatal("npm bootstrap config changed", name, err)
				}
			}
			if err := userStageExperience(ctx, root, profile); err != nil {
				t.Fatal("final materialization refused", err)
			}
			for _, file := range files {
				got, err := os.ReadFile(filepath.Join(root, "config", file.name))
				if err != nil || !reflect.DeepEqual(got, file.data) {
					t.Fatal("final preference differs", file.name, err)
				}
			}
			if profile != nil && *profile != beforeProfile {
				t.Fatal("preparation changed the consent-bound profile")
			}
			if len(files) != 0 {
				before, err := userTreeStamp(root)
				if err != nil {
					t.Fatal(err)
				}
				if err := userStageExperience(ctx, root, profile); err == nil {
					t.Fatal("existing preference collision accepted")
				}
				after, err := userTreeStamp(root)
				if err != nil || after != before {
					t.Fatal("collision changed existing preferences", err)
				}
			}
		})
	}
}

func TestUserNativeProducerPreservesLegacyPreparation(t *testing.T) {
	ctx := context.Background()
	profile := DefaultUserExperience()
	root := installStagePath(t)
	if err := userPrepareInstallStage(ctx, root, "separate", &profile); err != nil {
		t.Fatal(err)
	}
	files, err := userExperienceConfigFiles(&profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		got, err := os.ReadFile(filepath.Join(root, "config", file.name))
		if err != nil || !reflect.DeepEqual(got, file.data) {
			t.Fatal("legacy preferences no longer prepared", file.name, err)
		}
	}
	profile.Persona = "invalid"
	invalid := filepath.Join(t.TempDir(), "invalid")
	if err := userPrepareNativeProducer(ctx, invalid, &profile); err == nil {
		t.Fatal("invalid profile accepted")
	}
	if _, err := os.Lstat(invalid); !os.IsNotExist(err) {
		t.Fatal("invalid profile created DATA", err)
	}
}
