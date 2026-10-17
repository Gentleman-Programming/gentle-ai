//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUserGGAProvisionDoesNotActivateImplicitly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "untouched")
	for _, profile := range []*UserExperience{nil, {Persona: "unmanaged", BackgroundSubagents: "off"}, {Persona: "unmanaged", BackgroundSubagents: "off", GGA: "off"}} {
		if err := userGGAProvision(nil, root, "separate", profile); err != nil {
			t.Fatal("OFF/absence performed acquisition", err)
		}
	}
	profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"}
	if err := userGGAProvision(context.Background(), root, "shared", &profile); err == nil {
		t.Fatal("Shared accepted GGA provisioning")
	}
	if err := userGGAProvision(nil, root, "separate", &profile); err == nil {
		t.Fatal("ON accepted missing context")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("refused/absent provisioning changed filesystem", err)
	}
}

func TestUserGGAEarlyCancellationPreservesCause(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer stop()
	profile := UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", GGA: "on", GGAProvider: "codex"}
	for _, tt := range []struct {
		name string
		ctx  context.Context
	}{
		{"canceled", canceled},
		{"deadline", expired},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "untouched")
			if err := userGGAProvision(tt.ctx, root, "separate", &profile); !errors.Is(err, tt.ctx.Err()) {
				t.Fatal("acquisition lost the early cancellation cause", err)
			}
			if err := userStageGGAExecutable(tt.ctx, root, nil); !errors.Is(err, tt.ctx.Err()) {
				t.Fatal("materialization lost the early cancellation cause", err)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("early cancellation changed filesystem", err)
			}
		})
	}
}
