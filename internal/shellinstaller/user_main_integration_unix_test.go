//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func mainStoppedContext(expired bool) (context.Context, context.CancelFunc, error) {
	if expired {
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
		return ctx, cancel, context.DeadlineExceeded
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, cancel, context.Canceled
}

// Authored before integrating the producer. No live request or archive executes.
func TestUserMainIntegrationPreservesContextCauses(t *testing.T) {
	for _, expired := range []bool{false, true} {
		ctx, cancel, cause := mainStoppedContext(expired)
		defer cancel()
		for _, snapshot := range []func() (userSnapshotArtifact, []byte, error){
			func() (userSnapshotArtifact, []byte, error) { return userMainSnapshot(ctx) },
			func() (userSnapshotArtifact, []byte, error) { return userMainSnapshotUsing(ctx, &http.Client{}) },
		} {
			artifact, data, err := snapshot()
			if !errors.Is(err, cause) || artifact != (userSnapshotArtifact{}) || data != nil {
				t.Fatalf("stopped producer lost cause or returned usable data: %v %v", cause, err)
			}
		}
		if reader, err := userSnapshotZIP(ctx, nil, userSnapshotArtifact{}); !errors.Is(err, cause) || reader != nil {
			t.Fatalf("stopped ZIP parser lost context cause: %v", err)
		}
		if data, err := userReadMainArchive(ctx, "not-a-private-root", userSnapshotArtifact{}); !errors.Is(err, cause) || data != nil {
			t.Fatalf("stopped retained reader lost cause: %v", err)
		}
		profile := DefaultUserExperience()
		profile.SDKSource = "main-preview"
		before := profile
		root := installStagePath(t)
		if err := userPrepareInstallStage(ctx, root, "separate", &profile); !errors.Is(err, cause) {
			t.Fatalf("stopped preparer lost context cause: %v", err)
		}
		if profile != before {
			t.Fatal("stopped preparer changed the original complete selection")
		}
		if _, err := os.Lstat(root); !os.IsNotExist(err) {
			t.Fatal("stopped preparer touched its destination", err)
		}
	}
}

func TestUserMainIntegrationRejectsBodyCloseFailure(t *testing.T) {
	t.Setenv("GENTLE_AI_MAIN_FALLBACK", "1")
	for _, failingCall := range []int{1} {
		cause := errors.New("controlled response close failure")
		calls, closes := 0, 0
		client := &http.Client{Transport: userMainResponse(func(req *http.Request) (*http.Response, error) {
			calls++
			data := "opaque response DATA, not ZIP or publisher evidence"
			var closeErr error
			if calls == failingCall {
				closeErr = cause
			}
			body := userMainCloseErrorBody{Reader: strings.NewReader(data), cause: closeErr, closed: &closes}
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, ContentLength: int64(len(data)), Body: body}, nil
		})}
		artifact, data, err := userMainSnapshotUsing(context.Background(), client)
		if !errors.Is(err, cause) || artifact != (userSnapshotArtifact{}) || data != nil || calls != failingCall || closes != calls {
			t.Fatalf("close failure became usable snapshot or did not settle: err=%v calls=%d closes=%d", err, calls, closes)
		}
	}
}

func TestUserMainIntegrationStagesOriginalCompleteProfile(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			profile := UserExperience{Persona: persona, BackgroundSubagents: background, SDKSource: "main-preview"}
			before := profile
			root := installStagePath(t)
			err := userPrepareInstallStage(context.Background(), root, "separate", &profile)
			if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
				if err == nil {
					t.Fatal("unqualified Main stage admitted")
				}
				if _, lookup := os.Lstat(root); !os.IsNotExist(lookup) {
					t.Fatal("unqualified Main stage touched destination", lookup)
				}
			} else {
				if err != nil {
					t.Fatal("complete Main profile could not reach its real base writer", err)
				}
				files, err := userExperienceConfigFiles(&profile)
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range files {
					data, err := os.ReadFile(filepath.Join(root, "config", file.name))
					if err != nil || !bytes.Equal(data, file.data) {
						t.Fatal("native base preference readback differs", file.name, err)
					}
				}
				if _, err := os.Lstat(filepath.Join(root, "runtime/main-sdk")); !os.IsNotExist(err) {
					t.Fatal("base preparer invented the Main runtime consumer", err)
				}
			}
			if profile != before {
				t.Fatal("preparer projected, cleared or replaced original preferences")
			}
			if files, err := userStageExperienceFiles(&profile); err == nil || files != nil {
				t.Fatal("generic preference writer pretended to install Main SDK")
			}
		}
	}
}

func TestUserMainIntegrationRefusesSharedWithoutChangingProfile(t *testing.T) {
	profile := DefaultUserExperience()
	profile.SDKSource = "main-preview"
	before := profile
	root := installStagePath(t)
	if err := userPrepareInstallStage(context.Background(), root, "shared", &profile); err == nil {
		t.Fatal("Main SDK reached Shared preparation")
	}
	if profile != before {
		t.Fatal("Shared refusal changed original selection")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("Shared refusal created the destination", err)
	}
}
