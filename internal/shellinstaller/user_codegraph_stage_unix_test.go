//go:build linux || darwin

package shellinstaller

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUserCodeGraphStageAcceptsQualifiedModes(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("CodeGraph qualification is Linux amd64 only")
	}
	for _, mode := range []string{"separate", "shared"} {
		for _, choice := range []string{"on", "off"} {
			t.Run(mode+"/"+choice, func(t *testing.T) {
				profile, err := DecodeUserExperience(`{"persona":"unmanaged","background_subagents":"off","codegraph":"` + choice + `"}`)
				if err != nil {
					t.Fatal(err)
				}
				root := installStagePath(t)
				if err := userPrepareInstallStage(context.Background(), root, mode, &profile); err != nil {
					t.Fatalf("qualified %s CodeGraph stage refused: %v", mode, err)
				}
				if _, err := os.Lstat(filepath.Join(root, "runtime/codegraph")); !os.IsNotExist(err) {
					t.Fatal("stage materialized the provisioned runtime")
				}
			})
		}
	}
	profile, err := DecodeUserExperience(`{"persona":"unmanaged","background_subagents":"off","codegraph":"on"}`)
	if err != nil {
		t.Fatal(err)
	}
	if files, err := userStageExperienceFiles(&profile); err != nil || len(files) == 0 {
		t.Fatalf("qualified stock profile lost generic staging: %v %v", files, err)
	}
	main := profile
	main.SDKSource = "main-preview"
	if files, err := userStageExperienceFiles(&main); err == nil || files != nil {
		t.Fatalf("stock PNPM writer admitted Main: %v %v", files, err)
	}
	if err := userExperienceSupported(&main); err != nil {
		t.Fatalf("Main CodeGraph parity lost: %v", err)
	}
}
