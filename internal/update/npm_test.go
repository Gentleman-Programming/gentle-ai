package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

// --- TestFetchLatestNpmRelease ---

func TestFetchLatestNpmRelease(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		pkg         string
		wantVersion string
		wantErrPart string
	}{
		{
			name: "success returns version from latest dist-tag document",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"name":"@colbymchenry/codegraph","version":"1.5.0"}`)
			},
			pkg:         "@colbymchenry/codegraph",
			wantVersion: "1.5.0",
		},
		{
			name: "non-200 status is an error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			pkg:         "@colbymchenry/codegraph",
			wantErrPart: "HTTP 500",
		},
		{
			name: "malformed JSON is an error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{not json`)
			},
			pkg:         "@colbymchenry/codegraph",
			wantErrPart: "decode npm registry response",
		},
		{
			name: "missing version field is an error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"name":"@colbymchenry/codegraph"}`)
			},
			pkg:         "@colbymchenry/codegraph",
			wantErrPart: "no version field",
		},
		{
			name:        "empty package name is an error",
			handler:     func(w http.ResponseWriter, r *http.Request) {},
			pkg:         "  ",
			wantErrPart: "must not be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			origBaseURL := npmRegistryBaseURL
			t.Cleanup(func() { npmRegistryBaseURL = origBaseURL })
			npmRegistryBaseURL = server.URL

			release, err := fetchLatestNpmRelease(context.Background(), tc.pkg)
			if tc.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
					t.Fatalf("fetchLatestNpmRelease() err = %v, want error containing %q", err, tc.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchLatestNpmRelease() unexpected error: %v", err)
			}
			if release.TagName != tc.wantVersion {
				t.Fatalf("TagName = %q, want %q", release.TagName, tc.wantVersion)
			}
			if !strings.Contains(release.HTMLURL, tc.pkg) {
				t.Fatalf("HTMLURL = %q, want it to reference package %q", release.HTMLURL, tc.pkg)
			}
		})
	}
}

// TestFetchLatestNpmReleaseRequestPath verifies the fetcher hits the canonical
// /<pkg>/latest endpoint, preserving the slash inside scoped package names.
func TestFetchLatestNpmReleaseRequestPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"version":"1.5.0"}`)
	}))
	defer server.Close()

	origBaseURL := npmRegistryBaseURL
	t.Cleanup(func() { npmRegistryBaseURL = origBaseURL })
	npmRegistryBaseURL = server.URL

	if _, err := fetchLatestNpmRelease(context.Background(), "@colbymchenry/codegraph"); err != nil {
		t.Fatalf("fetchLatestNpmRelease() unexpected error: %v", err)
	}
	if want := "/@colbymchenry/codegraph/latest"; gotPath != want {
		t.Fatalf("request path = %q, want %q", gotPath, want)
	}
}

// --- TestCheckSingleToolNpmGlobal ---

// TestCheckSingleToolNpmGlobal exercises the full check pipeline for an
// npm-global tool: local detection via DetectCmd, remote resolution via the
// npm registry fetcher, and version comparison.
func TestCheckSingleToolNpmGlobal(t *testing.T) {
	tool := ToolInfo{
		Name:          "codegraph",
		Owner:         "colbymchenry",
		Repo:          "codegraph",
		DetectCmd:     []string{"codegraph", "--version"},
		VersionPrefix: "v",
		InstallMethod: InstallNpmGlobal,
		NpmPackage:    "@colbymchenry/codegraph",
	}

	tests := []struct {
		name          string
		localVersion  string // empty = binary not found on PATH
		remoteVersion string
		wantStatus    UpdateStatus
		wantInstalled string
		wantLatest    string
	}{
		{
			name:          "up-to-date",
			localVersion:  "1.5.0",
			remoteVersion: "1.5.0",
			wantStatus:    UpToDate,
			wantInstalled: "1.5.0",
			wantLatest:    "1.5.0",
		},
		{
			name:          "update available",
			localVersion:  "1.4.1",
			remoteVersion: "1.5.0",
			wantStatus:    UpdateAvailable,
			wantInstalled: "1.4.1",
			wantLatest:    "1.5.0",
		},
		{
			name:          "not installed",
			localVersion:  "",
			remoteVersion: "1.5.0",
			wantStatus:    NotInstalled,
			wantInstalled: "",
			wantLatest:    "1.5.0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"version":%q}`, tc.remoteVersion)
			}))
			defer server.Close()

			origBaseURL := npmRegistryBaseURL
			origLookPath := lookPath
			origExecCommand := execCommand
			t.Cleanup(func() {
				npmRegistryBaseURL = origBaseURL
				lookPath = origLookPath
				execCommand = origExecCommand
			})

			npmRegistryBaseURL = server.URL
			if tc.localVersion == "" {
				lookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
				execCommand = func(name string, args ...string) *exec.Cmd { return mockCmd("false") }
			} else {
				lookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
				execCommand = func(name string, args ...string) *exec.Cmd {
					return mockCmd("echo", tc.localVersion)
				}
			}

			result := checkSingleTool(context.Background(), tool, "dev", system.PlatformProfile{OS: "linux", PackageManager: "apt"})
			assertResult(t, result, "codegraph", tc.wantStatus, tc.wantInstalled, tc.wantLatest)
		})
	}
}

// TestCheckSingleToolNpmGlobalRegistryFailure verifies a remote fetch failure
// surfaces as CheckFailed for npm-global tools.
func TestCheckSingleToolNpmGlobalRegistryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	origBaseURL := npmRegistryBaseURL
	origLookPath := lookPath
	origExecCommand := execCommand
	t.Cleanup(func() {
		npmRegistryBaseURL = origBaseURL
		lookPath = origLookPath
		execCommand = origExecCommand
	})

	npmRegistryBaseURL = server.URL
	lookPath = func(name string) (string, error) { return "/usr/local/bin/" + name, nil }
	execCommand = func(name string, args ...string) *exec.Cmd { return mockCmd("echo", "1.4.1") }

	tool := ToolInfo{
		Name:          "codegraph",
		DetectCmd:     []string{"codegraph", "--version"},
		InstallMethod: InstallNpmGlobal,
		NpmPackage:    "@colbymchenry/codegraph",
	}

	result := checkSingleTool(context.Background(), tool, "dev", system.PlatformProfile{OS: "linux", PackageManager: "apt"})
	if result.Status != CheckFailed {
		t.Fatalf("status = %q, want %q", result.Status, CheckFailed)
	}
	if result.Err == nil {
		t.Fatal("expected a fetch error, got nil")
	}
}

// --- TestRegistryCodegraphEntry ---

// TestRegistryCodegraphEntry pins the shipped codegraph registry declaration so
// routing cannot drift away from the npm-global design (issue #984).
func TestRegistryCodegraphEntry(t *testing.T) {
	var found *ToolInfo
	for i := range Tools {
		if Tools[i].Name == "codegraph" {
			found = &Tools[i]
			break
		}
	}
	if found == nil {
		t.Fatal("codegraph is missing from the tool registry")
	}

	if got, want := found.InstallMethod, InstallNpmGlobal; got != want {
		t.Errorf("InstallMethod = %q, want %q", got, want)
	}
	if got, want := found.NpmPackage, "@colbymchenry/codegraph"; got != want {
		t.Errorf("NpmPackage = %q, want %q", got, want)
	}
	if len(found.DetectCmd) != 2 || found.DetectCmd[0] != "codegraph" || found.DetectCmd[1] != "--version" {
		t.Errorf("DetectCmd = %v, want [codegraph --version]", found.DetectCmd)
	}
}
