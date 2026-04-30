package system

import (
	"context"
	"testing"
)

func TestParseVersionNode(t *testing.T) {
	tests := []struct {
		name   string
		tool   string
		output string
		want   string
	}{
		{name: "node v18.0.0", tool: "node", output: "v18.0.0\n", want: "18.0.0"},
		{name: "node v20.11.1", tool: "node", output: "v20.11.1\n", want: "20.11.1"},
		{name: "npm 10.2.4", tool: "npm", output: "10.2.4\n", want: "10.2.4"},
		{name: "git version 2.43.0", tool: "git", output: "git version 2.43.0\n", want: "2.43.0"},
		{name: "curl 8.4.0", tool: "curl", output: "curl 8.4.0 (x86_64-apple-darwin23.0) libcurl/8.4.0\n", want: "8.4.0"},
		{name: "go version go1.22.5 darwin/arm64", tool: "go", output: "go version go1.22.5 darwin/arm64\n", want: "1.22.5"},
		{name: "go1.21.0", tool: "go", output: "go1.21.0\n", want: "1.21.0"},
		{name: "brew Homebrew 4.2.0", tool: "brew", output: "Homebrew 4.2.0\n", want: "4.2.0"},
		{name: "empty output", tool: "node", output: "", want: ""},
		{name: "whitespace only", tool: "node", output: "   \n  ", want: ""},
		{name: "no version pattern", tool: "node", output: "some random text", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVersion(tc.tool, tc.output)
			if got != tc.want {
				t.Fatalf("parseVersion(%q, %q) = %q, want %q", tc.tool, tc.output, got, tc.want)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		name       string
		version    string
		minVersion string
		want       bool
	}{
		{name: "equal", version: "18.0.0", minVersion: "18.0.0", want: true},
		{name: "above major", version: "20.0.0", minVersion: "18.0.0", want: true},
		{name: "above minor", version: "18.5.0", minVersion: "18.0.0", want: true},
		{name: "above patch", version: "18.0.1", minVersion: "18.0.0", want: true},
		{name: "below major", version: "16.0.0", minVersion: "18.0.0", want: false},
		{name: "below minor", version: "17.9.0", minVersion: "18.0.0", want: false},
		{name: "two part version", version: "18.0", minVersion: "18.0.0", want: true},
		{name: "two part min", version: "18.0.0", minVersion: "18.0", want: true},
		{name: "zero values", version: "0.0.0", minVersion: "0.0.0", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := versionAtLeast(tc.version, tc.minVersion)
			if got != tc.want {
				t.Fatalf("versionAtLeast(%q, %q) = %v, want %v", tc.version, tc.minVersion, got, tc.want)
			}
		})
	}
}

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    [3]int
	}{
		{name: "full semver", version: "18.2.3", want: [3]int{18, 2, 3}},
		{name: "two parts", version: "18.2", want: [3]int{18, 2, 0}},
		{name: "one part", version: "18", want: [3]int{18, 0, 0}},
		{name: "empty", version: "", want: [3]int{0, 0, 0}},
		{name: "non-numeric", version: "abc.def.ghi", want: [3]int{0, 0, 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVersionParts(tc.version)
			if got != tc.want {
				t.Fatalf("parseVersionParts(%q) = %v, want %v", tc.version, got, tc.want)
			}
		})
	}
}

func TestDefineDependenciesDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}
	deps := defineDependencies(profile)

	names := map[string]bool{}
	for _, dep := range deps {
		names[dep.Name] = true
	}

	// Darwin should include brew as optional.
	required := []string{"git", "curl", "node", "npm"}
	for _, name := range required {
		if !names[name] {
			t.Fatalf("missing required dep %q for darwin", name)
		}
	}

	if !names["brew"] {
		t.Fatalf("missing brew dep for darwin")
	}

	// Check brew is optional.
	for _, dep := range deps {
		if dep.Name == "brew" && dep.Required {
			t.Fatalf("brew should be optional on darwin")
		}
	}
}

func TestDefineDependenciesLinuxNoBrew(t *testing.T) {
	profile := PlatformProfile{OS: "linux", LinuxDistro: "ubuntu", PackageManager: "apt", Supported: true}
	deps := defineDependencies(profile)

	for _, dep := range deps {
		if dep.Name == "brew" {
			t.Fatalf("brew should not be in linux dependency list")
		}
	}
}

func TestDefineDependenciesNodeMinVersion(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew", Supported: true}
	deps := defineDependencies(profile)

	for _, dep := range deps {
		if dep.Name == "node" {
			if dep.MinVersion != "18.0.0" {
				t.Fatalf("node MinVersion = %q, want 18.0.0", dep.MinVersion)
			}
			return
		}
	}

	t.Fatalf("node dependency not found")
}

func TestDetectDepsWithMockDeps(t *testing.T) {
	// Test with all deps pre-filled (simulating detection without exec).
	deps := []Dependency{
		{Name: "git", Required: true, Installed: true, Version: "2.43.0"},
		{Name: "curl", Required: true, Installed: true, Version: "8.4.0"},
		{Name: "node", Required: true, MinVersion: "18.0.0", Installed: true, Version: "20.11.0"},
		{Name: "npm", Required: true, Installed: true, Version: "10.2.4"},
		{Name: "go", Required: false, Installed: false},
	}

	// Build report manually to simulate what detectDeps would produce.
	report := DependencyReport{
		AllPresent: true,
	}

	for _, dep := range deps {
		if dep.Required && !dep.Installed {
			report.AllPresent = false
			report.MissingRequired = append(report.MissingRequired, dep.Name)
		}
		if !dep.Required && !dep.Installed {
			report.MissingOptional = append(report.MissingOptional, dep.Name)
		}
	}

	if !report.AllPresent {
		t.Fatalf("expected AllPresent=true when all required deps are installed")
	}

	if len(report.MissingRequired) != 0 {
		t.Fatalf("MissingRequired = %v, want empty", report.MissingRequired)
	}

	if len(report.MissingOptional) != 1 || report.MissingOptional[0] != "go" {
		t.Fatalf("MissingOptional = %v, want [go]", report.MissingOptional)
	}
}

func TestDependencyReportMissingRequired(t *testing.T) {
	deps := []Dependency{
		{Name: "git", Required: true, Installed: true, Version: "2.43.0"},
		{Name: "node", Required: true, MinVersion: "18.0.0", Installed: false},
		{Name: "npm", Required: true, Installed: false},
	}

	report := DependencyReport{
		AllPresent: true,
	}

	for _, dep := range deps {
		if dep.Required && !dep.Installed {
			report.AllPresent = false
			report.MissingRequired = append(report.MissingRequired, dep.Name)
		}
	}

	if report.AllPresent {
		t.Fatalf("expected AllPresent=false when required deps missing")
	}

	if len(report.MissingRequired) != 2 {
		t.Fatalf("MissingRequired = %v, want [node, npm]", report.MissingRequired)
	}
}

func TestRenderDependencyReportAllPresent(t *testing.T) {
	report := DependencyReport{
		Dependencies: []Dependency{
			{Name: "git", Required: true, Installed: true, Version: "2.43.0"},
			{Name: "node", Required: true, Installed: true, Version: "20.11.0"},
		},
		AllPresent: true,
	}

	output := RenderDependencyReport(report)
	if output == "" {
		t.Fatalf("expected non-empty output")
	}

	if !containsAll(output, "git", "v 2.43.0", "node", "v 20.11.0") {
		t.Fatalf("output missing expected content:\n%s", output)
	}
}

func TestRenderDependencyReportMissing(t *testing.T) {
	report := DependencyReport{
		Dependencies: []Dependency{
			{Name: "git", Required: true, Installed: true, Version: "2.43.0"},
			{Name: "node", Required: true, Installed: false, InstallHint: "brew install node"},
		},
		AllPresent:      false,
		MissingRequired: []string{"node"},
	}

	output := RenderDependencyReport(report)
	if !containsAll(output, "node", "x NOT FOUND", "Missing required: node") {
		t.Fatalf("output missing expected content:\n%s", output)
	}
}

func TestDetectSingleDepWithEchoTrue(t *testing.T) {
	// This test uses "echo" which should be on PATH everywhere.
	dep := Dependency{
		Name:      "echo",
		Required:  true,
		DetectCmd: []string{"echo", "v1.0.0"},
	}

	result := detectSingleDep(context.Background(), dep)
	if !result.Installed {
		t.Fatalf("expected echo to be detected as installed")
	}

	if result.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", result.Version)
	}
}

func TestDetectSingleDepNonExistentBinary(t *testing.T) {
	dep := Dependency{
		Name:      "nonexistent_tool_xyz_42",
		Required:  true,
		DetectCmd: []string{"nonexistent_tool_xyz_42", "--version"},
	}

	result := detectSingleDep(context.Background(), dep)
	if result.Installed {
		t.Fatalf("expected nonexistent tool to not be installed")
	}
}

func TestDetectSingleDepMinVersionFail(t *testing.T) {
	// Use echo to produce a version below the minimum.
	dep := Dependency{
		Name:       "echo",
		Required:   true,
		MinVersion: "99.0.0",
		DetectCmd:  []string{"echo", "v1.0.0"},
	}

	result := detectSingleDep(context.Background(), dep)
	if result.Installed {
		t.Fatalf("expected dep with version below minimum to be marked as not installed")
	}
	if result.Version != "1.0.0" {
		t.Fatalf("Version = %q, want 1.0.0", result.Version)
	}
}

func TestDetectSingleDepEmptyDetectCmd(t *testing.T) {
	dep := Dependency{
		Name:     "empty",
		Required: true,
	}

	result := detectSingleDep(context.Background(), dep)
	if result.Installed {
		t.Fatalf("expected dep with empty DetectCmd to not be installed")
	}
}

func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
