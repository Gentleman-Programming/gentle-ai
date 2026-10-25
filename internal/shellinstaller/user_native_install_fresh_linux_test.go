//go:build linux && amd64 && shell_native_install_fresh

package shellinstaller

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const nativeInstallSourceCommit = "8983b43bd52210a9242a056d291fb2bf30e73cc5"
const nativeInstallSourceSHA = "f4b964eda29f2ff9b4fb6b8329a2f1eb60e7b5313984518cf6109159b18a71e9"

func TestUserNativeInstallPrepare(t *testing.T) {
	ctx := context.Background()
	root := "/producer/worker"
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	artifact, err := userMainDescriptor(nativeInstallSourceCommit, nativeInstallSourceSHA)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("/native-source.zip")
	if err != nil {
		t.Fatal(err)
	}
	if err := userStageMainArchive(ctx, root, artifact, data); err != nil {
		t.Fatal(err)
	}
	if err := userStageMainSource(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
	nodeRoot := filepath.Join(root, "runtime/node")
	if err := os.Mkdir(nodeRoot, 0700); err != nil {
		t.Fatal(err)
	}
	copy := exec.CommandContext(ctx, "/bin/cp", "-a", "--no-preserve=ownership", "/opt/node/.", nodeRoot)
	if output, err := copy.CombinedOutput(); err != nil {
		t.Fatal("private temporary Node copy", err, string(output))
	}
	if err := os.Chmod(filepath.Join(nodeRoot, "bin/node"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := userNativeRuntimeFromSnapshot(ctx, root, artifact); err != nil {
		t.Fatal(err)
	}
}

func TestUserNativeInstallFresh(t *testing.T) {
	ctx := context.Background()
	artifact, err := userMainDescriptor(nativeInstallSourceCommit, nativeInstallSourceSHA)
	if err != nil {
		t.Fatal(err)
	}
	w, err := userNativeRuntimeFromSnapshot(ctx, "/producer/worker", artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	// This dedicated fixture lives on the qualified sandbox's capped tmpfs.
	// Settled namespace teardown owns disposal, not synchronous test cleanup.
	parent := "/tmp/native-publication"
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	req := UserInstallRequest{Destination: filepath.Join(parent, "final"), Mode: "separate", Experience: &UserExperience{Persona: "neutral", BackgroundSubagents: "off"}}
	req.Confirmation, err = InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	native, location, runErr := w.produceForDestination(ctx, req, stage)
	var installed UserInstallResult
	if runErr == nil && location != nil {
		req, runErr = userCanonicalRequest(req)
		if runErr == nil {
			installed, runErr = publishNativePNPMInstallationWithExperienceForTest(ctx, w.root, stage, req.Destination, *location, req.Experience)
		}
	}
	message := ""
	if runErr != nil {
		message = runErr.Error()
	}
	report, _ := json.Marshal(struct {
		Native    NativeInstallerResult
		Installed UserInstallResult
		Error     string
	}{native, installed, message})
	if err := os.WriteFile("/report/native-publication.json", report, 0600); err != nil {
		t.Fatal(err)
	}
	if runErr != nil || native.Outcome != "terminal-action-required" || len(native.Completed) != 13 || installed.State != "ComponentInstalled" || installed.Destination != req.Destination {
		t.Fatal("fresh original publication incomplete", native, installed, runErr)
	}
	files, err := userStageExperienceFiles(req.Experience)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(req.Destination, "config", file.name))
		if err != nil || string(data) != string(file.data) {
			t.Fatal("published preference differs", file.name, err)
		}
	}
	manifest, err := userReadManifest(ctx, req.Destination)
	if err != nil || manifest.PNPM == nil {
		t.Fatal("published PNPM manifest rejected", err)
	}
}
