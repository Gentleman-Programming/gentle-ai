package upgrade

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
	"github.com/gentleman-programming/gentle-ai/v4/internal/update"
)

func TestGentleAISourceBuildRoutesToPinnedGoInstall(t *testing.T) {
	originalKeys := releaseMinisignPublicKeys
	releaseMinisignPublicKeys = unsetReleaseMinisignPublicKeys
	t.Cleanup(func() { releaseMinisignPublicKeys = originalKeys })
	originalBrew := homebrewPackageInstalled
	homebrewPackageInstalled = func(string) bool { return false }
	t.Cleanup(func() { homebrewPackageInstalled = originalBrew })

	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			gobin := t.TempDir()
			active := writeFakeBinary(t, gobin, "gentle-ai")
			originalLookPath := lookPathFn
			lookPathFn = func(string) (string, error) { return active, nil }
			t.Cleanup(func() { lookPathFn = originalLookPath })
			originalOS := detectOS
			detectOS = func() string { return goos }
			t.Cleanup(func() { detectOS = originalOS })
			originalExec := execCommand
			t.Cleanup(func() { execCommand = originalExec })
			installs := 0
			execCommand = func(name string, args ...string) *exec.Cmd {
				if name == "go" && len(args) == 2 && args[0] == "env" {
					return mockCmd("echo", gobin)
				}
				if name != "go" || len(args) != 2 || args[0] != "install" || args[1] != "github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0" {
					t.Fatalf("unexpected command: %s %v", name, args)
				}
				installs++
				cmd := mockCmd("true")
				t.Cleanup(func() {
					if cmd.Env != nil {
						t.Errorf("stable source upgrade overrides Go verification environment: %v", cmd.Env)
					}
				})
				return cmd
			}

			report := ExecuteWithOptions(context.Background(), []update.UpdateResult{{
				Tool: registryGentleAI(t), LatestVersion: "4.0.0", Status: update.UpdateAvailable,
			}}, system.PlatformProfile{OS: goos, GoAvailable: true, Supported: true}, t.TempDir(), false, ExecuteOptions{SkipBackup: true})
			if len(report.Results) != 1 || report.Results[0].Status != UpgradeSucceeded || report.Results[0].Err != nil || report.ExitRequested {
				t.Fatalf("source upgrade = %#v, want success", report)
			}
			if installs != 1 || report.Results[0].Method != update.InstallGoInstall {
				t.Fatalf("installs = %d, method = %q, want one go install", installs, report.Results[0].Method)
			}
		})
	}
}

func TestGentleAISourceBuildWithoutGoPreservesState(t *testing.T) {
	originalKeys := releaseMinisignPublicKeys
	releaseMinisignPublicKeys = unsetReleaseMinisignPublicKeys
	t.Cleanup(func() { releaseMinisignPublicKeys = originalKeys })
	originalExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("manual refusal executed %s %v", name, args)
		return nil
	}
	t.Cleanup(func() { execCommand = originalExec })
	for _, tc := range []struct {
		name          string
		goAvailable   bool
		missingImport bool
	}{
		{name: "without Go"},
		{name: "without import path", goAvailable: true, missingImport: true},
	} {
		for _, goos := range []string{"linux", "darwin"} {
			t.Run(goos+"/"+tc.name, func(t *testing.T) {
				tool := registryGentleAI(t)
				if tc.missingImport {
					tool.GoImportPath = ""
				}
				home := t.TempDir()
				active := writeFakeBinary(t, home, "gentle-ai")
				before, err := os.ReadFile(active)
				if err != nil {
					t.Fatal(err)
				}
				report := ExecuteWithOptions(context.Background(), []update.UpdateResult{{
					Tool: tool, LatestVersion: "4.0.0", Status: update.UpdateAvailable,
				}}, system.PlatformProfile{OS: goos, Supported: true, GoAvailable: tc.goAvailable}, home, false, ExecuteOptions{})
				wantHint := "This source build has no embedded release trust anchor. Automatic source upgrade requires Go on PATH and a declared Go import path. No files were changed. Install/update from source with Go 1.25.10+:\n  go install github.com/gentleman-programming/gentle-ai/v4/cmd/gentle-ai@v4.0.0"
				if len(report.Results) != 1 || report.Results[0].Status != UpgradeSkipped || report.Results[0].Err != nil || report.Results[0].ManualHint != wantHint {
					t.Fatalf("source refusal = %#v, want skip with hint %q", report, wantHint)
				}
				if report.BackupID != "" || report.BackupWarning != "" || report.ExitRequested {
					t.Fatalf("manual refusal changed backup/exit state: %#v", report)
				}
				if _, err := os.Stat(filepath.Join(home, ".gentle-ai")); !os.IsNotExist(err) {
					t.Fatalf("manual refusal created managed state: %v", err)
				}
				after, err := os.ReadFile(active)
				if err != nil || string(after) != string(before) {
					t.Fatalf("binary changed: %v", err)
				}
			})
		}
	}
}

func TestGentleAISignatureFailureNeverFallsBackToGo(t *testing.T) {
	private := useTestReleaseKey(t)
	manifest := []byte("original manifest")
	signature := signTestManifest(t, private, manifest, "Gentleman-Programming", "gentle-ai", "4.0.0")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/signature" {
			_, _ = w.Write(signature)
		} else if r.URL.Path == "/manifest" {
			_, _ = fmt.Fprint(w, "tampered manifest")
		} else {
			t.Errorf("unverified archive requested: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	originalChecksum, originalSignature := resolveChecksumURLFn, resolveSignatureURLFn
	resolveChecksumURLFn = func(string, string, string) string { return server.URL + "/manifest" }
	resolveSignatureURLFn = func(string, string, string) string { return server.URL + "/signature" }
	t.Cleanup(func() { resolveChecksumURLFn, resolveSignatureURLFn = originalChecksum, originalSignature })
	originalExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("signature failure executed %s %v", name, args)
		return nil
	}
	t.Cleanup(func() { execCommand = originalExec })
	active := writeFakeBinary(t, t.TempDir(), "gentle-ai")
	before, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	originalLookPath := lookPathFn
	lookPathFn = func(string) (string, error) { return active, nil }
	t.Cleanup(func() { lookPathFn = originalLookPath })
	for _, goos := range []string{"linux", "darwin"} {
		result := executeOne(context.Background(), update.UpdateResult{
			Tool: registryGentleAI(t), LatestVersion: "4.0.0", Status: update.UpdateAvailable,
		}, system.PlatformProfile{OS: goos, GoAvailable: true}, false)
		if result.Method != update.InstallBinary || result.Status != UpgradeFailed || !errors.Is(result.Err, ErrSignatureVerificationFailed) {
			t.Fatalf("signature failure = %#v, want fail-closed binary upgrade", result)
		}
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want manifest/signature only for both platforms", requests)
	}
	after, err := os.ReadFile(active)
	if err != nil || string(after) != string(before) {
		t.Fatalf("binary changed: %v", err)
	}
}

func TestGentleAIInvalidAnchorNeverFallsBackToGo(t *testing.T) {
	originalKeys := releaseMinisignPublicKeys
	t.Cleanup(func() { releaseMinisignPublicKeys = originalKeys })
	originalExec := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		t.Fatalf("invalid anchor executed %s %v", name, args)
		return nil
	}
	t.Cleanup(func() { execCommand = originalExec })
	for _, keys := range []string{"", "malformed", legacyZeroMinisignKeyPlaceholder} {
		for _, goos := range []string{"linux", "darwin"} {
			t.Run(goos+"/"+keys, func(t *testing.T) {
				releaseMinisignPublicKeys = keys
				result := executeOne(context.Background(), update.UpdateResult{
					Tool: registryGentleAI(t), LatestVersion: "4.0.0", Status: update.UpdateAvailable,
				}, system.PlatformProfile{OS: goos, GoAvailable: true}, false)
				if result.Method != update.InstallBinary || result.Status != UpgradeFailed || !errors.Is(result.Err, ErrReleaseTrustUnavailable) {
					t.Fatalf("invalid anchor result = %#v, want fail-closed binary upgrade", result)
				}
			})
		}
	}
}
