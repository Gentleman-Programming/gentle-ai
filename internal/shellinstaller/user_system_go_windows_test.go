//go:build windows

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// userWindowsSystemGoFixture writes a regular fixture go.exe. It is never
// executed: the probe seam below supplies every `go version` output.
func userWindowsSystemGoFixture(t *testing.T, name string) string {
	t.Helper()
	if name == "" {
		name = "go.exe"
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("fixture Go, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// userWindowsSystemGoProbeStub replaces the toolchain probe seam and restores
// it at cleanup, so no test ever executes a foreign go.exe.
func userWindowsSystemGoProbeStub(t *testing.T, output string, probeErr error) {
	t.Helper()
	previous := userWindowsSystemGoProbe
	userWindowsSystemGoProbe = func(ctx context.Context, goExe string) ([]byte, error) {
		return []byte(output), probeErr
	}
	t.Cleanup(func() { userWindowsSystemGoProbe = previous })
}

func TestUserWindowsSystemGoVersionGate(t *testing.T) {
	for _, tt := range []struct {
		output  string
		version string
		ok      bool
	}{
		{"go version go1.27.1 windows/amd64\n", "go1.27.1", true},
		{"go version go1.25.10 windows/amd64", "go1.25.10", true},
		{"go version go1.30 windows/amd64", "go1.30", true},
		{"go version go1.25.9 windows/amd64", "", false},
		{"go version go1.25 windows/amd64", "", false},
		{"go version go1.26rc1 windows/amd64", "", false},
		{"go version go1.27.1-beta1 windows/amd64", "", false},
		{"go version devel +f123abc windows/amd64", "", false},
		{"", "", false},
		{"go env GOROOT", "", false},
		{"go version go1.25.10", "", false},
	} {
		version, ok := userWindowsSystemGoVersion(tt.output)
		if ok != tt.ok || version != tt.version {
			t.Fatalf("system Go version gate %q: got (%q, %v); want (%q, %v)", tt.output, version, ok, tt.version, tt.ok)
		}
	}
}

func TestUserWindowsSystemGoDetectAcceptsCompatibleCallerGo(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	userWindowsSystemGoProbeStub(t, "go version go1.27.1 windows/amd64\n", nil)
	t.Setenv(userWindowsSystemGoCandidateEnv, goExe)
	marker, ok := userWindowsSystemGoDetect(context.Background())
	if !ok || marker.Go != goExe || marker.Version != "go1.27.1" {
		t.Fatalf("compatible caller Go was not accepted: ok=%v marker=%+v", ok, marker)
	}
}

func TestUserWindowsSystemGoDetectFallsBack(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	for _, tt := range []struct {
		name      string
		candidate string
		output    string
		probeErr  error
	}{
		{"absent candidate", "", "", nil},
		{"missing candidate", filepath.Join(t.TempDir(), "absent", "go.exe"), "", nil},
		{"directory candidate", t.TempDir(), "", nil},
		{"relative candidate", "go.exe", "", nil},
		{"wrong name", userWindowsSystemGoFixture(t, "notgo.exe"), "", nil},
		{"probe failure", goExe, "", errors.New("fixture probe failure")},
		{"unparsable version", goExe, "go version devel +f123abc windows/amd64", nil},
		{"prerelease version", goExe, "go version go1.26rc1 windows/amd64", nil},
		{"too old", goExe, "go version go1.25.9 windows/amd64", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			userWindowsSystemGoProbeStub(t, tt.output, tt.probeErr)
			t.Setenv(userWindowsSystemGoCandidateEnv, tt.candidate)
			if _, ok := userWindowsSystemGoDetect(context.Background()); ok {
				t.Fatalf("%s was accepted for reuse", tt.name)
			}
		})
	}
}

func TestUserWindowsSystemGoDetectRefusesAliasedCandidate(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	aliased := filepath.Join(t.TempDir(), "go.exe")
	userWindowsSymlinkOrSkip(t, goExe, aliased)
	userWindowsSystemGoProbeStub(t, "go version go1.27.1 windows/amd64\n", nil)
	t.Setenv(userWindowsSystemGoCandidateEnv, aliased)
	if _, ok := userWindowsSystemGoDetect(context.Background()); ok {
		t.Fatal("aliased go.exe was accepted for reuse")
	}
}

func TestUserWindowsSystemGoDetectRefusesJunctionedDirectory(t *testing.T) {
	directory := t.TempDir()
	real := filepath.Join(directory, "go.exe")
	if err := os.WriteFile(real, []byte("fixture Go, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	userWindowsJunction(t, link, directory)
	userWindowsSystemGoProbeStub(t, "go version go1.27.1 windows/amd64\n", nil)
	t.Setenv(userWindowsSystemGoCandidateEnv, filepath.Join(link, "go.exe"))
	if _, ok := userWindowsSystemGoDetect(context.Background()); ok {
		t.Fatal("go.exe behind a junctioned directory was accepted for reuse")
	}
}

func TestUserWindowsSystemGoStageWritesCanonicalInventorySafeMarker(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	marker := userWindowsSystemGo{Go: goExe, Version: "go1.27.1"}
	if err := userWindowsSystemGoStage(root, marker); err != nil {
		t.Fatalf("system Go marker was not staged: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "runtime", "system-go.json"))
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(marker)
	if string(raw) != string(canonical) {
		t.Fatalf("staged marker is not canonical: %q want %q", raw, canonical)
	}
	read, err := userWindowsSystemGoMarkerRead(root)
	if err != nil || read != marker {
		t.Fatalf("staged marker did not revalidate: %+v %v", read, err)
	}
	// The marker must live inside the target and pass the strict stage
	// inventory: it is an owned regular selection-safe name.
	if err := userWindowsInventory(root, userWindowsInventoryInstalled); err != nil {
		t.Fatalf("stage inventory refused the system Go marker: %v", err)
	}
	if err := userWindowsInventory(root, userWindowsInventoryLaunch); err != nil {
		t.Fatalf("launch inventory refused the system Go marker: %v", err)
	}
}

func TestUserWindowsSystemGoMarkerReadRefusesInvalidMarkers(t *testing.T) {
	goExe := userWindowsSystemGoFixture(t, "")
	valid, _ := json.Marshal(userWindowsSystemGo{Go: goExe, Version: "go1.27.1"})
	foreign, _ := json.Marshal(userWindowsSystemGo{Go: filepath.Join(t.TempDir(), "elsewhere", "go.exe"), Version: "go1.27.1"})
	directory, _ := json.Marshal(userWindowsSystemGo{Go: t.TempDir(), Version: "go1.27.1"})
	notGoExe, _ := json.Marshal(userWindowsSystemGo{Go: userWindowsSystemGoFixture(t, "notgo.exe"), Version: "go1.27.1"})
	missing, _ := json.Marshal(userWindowsSystemGo{Go: filepath.Join(t.TempDir(), "gone", "go.exe"), Version: "go1.27.1"})
	oldVersion, _ := json.Marshal(userWindowsSystemGo{Go: goExe, Version: "go1.25.9"})
	prerelease, _ := json.Marshal(userWindowsSystemGo{Go: goExe, Version: "go1.26rc1"})
	for name, data := range map[string][]byte{
		"unknown field":  append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"Injected":1}`)...),
		"trailing byte":  append(append([]byte{}, valid...), '\n'),
		"empty":          {},
		"not JSON":       []byte("{"),
		"foreign path":   foreign,
		"directory path": directory,
		"not go.exe":     notGoExe,
		"missing file":   missing,
		"old version":    oldVersion,
		"prerelease":     prerelease,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := userWindowsWrite(filepath.Join(root, "runtime", "system-go.json"), data); err != nil {
				t.Fatal(err)
			}
			if _, err := userWindowsSystemGoMarkerRead(root); err == nil || errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s marker was not refused as a distinct error: %v", name, err)
			}
		})
	}
	// A missing marker itself stays exactly os.ErrNotExist so callers can
	// distinguish the pinned-artifact contract from an invalid marker.
	root := t.TempDir()
	if _, err := userWindowsSystemGoMarkerRead(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent marker is not os.ErrNotExist: %v", err)
	}
}

func TestUserWindowsGoZipSkippedFollowsAcceptance(t *testing.T) {
	if !userWindowsGoZipSkipped(true, "go.zip") {
		t.Fatal("accepted system Go did not skip the pinned go.zip artifact")
	}
	for _, archive := range []string{"node.zip", "fd.zip", "rg.zip"} {
		if userWindowsGoZipSkipped(true, archive) {
			t.Fatalf("accepted system Go skipped the unrelated %s artifact", archive)
		}
		if userWindowsGoZipSkipped(false, archive) {
			t.Fatalf("fallback skipped the unrelated %s artifact", archive)
		}
	}
	if userWindowsGoZipSkipped(false, "go.zip") {
		t.Fatal("fallback skipped the pinned go.zip artifact")
	}
}
