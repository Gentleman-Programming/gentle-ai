package system

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/gentleman-programming/gentle-ai/internal/sysproc"
)

// Dependency represents a system prerequisite with detection and install metadata.
type Dependency struct {
	Name        string   // "node", "git", "curl", "brew", "go", "npm"
	Required    bool     // true = must have, false = nice to have
	MinVersion  string   // e.g., "18.0.0" for Node.js (empty = any version)
	DetectCmd   []string // e.g., ["node", "--version"]
	Installed   bool
	Version     string // detected version (parsed, e.g., "18.2.0")
	InstallHint string // platform-specific install hint
}

// DependencyReport summarizes the result of dependency detection.
type DependencyReport struct {
	Dependencies    []Dependency
	AllPresent      bool
	MissingRequired []string
	MissingOptional []string
}

// versionRegexp extracts a semver-like version from command output.
// Handles: "v18.0.0", "git version 2.43.0", "curl 8.4.0", "go1.22.5", "4.2.0" etc.
var versionRegexp = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

// goVersionRegexp handles "go version go1.22.5 darwin/arm64" and "go1.22.5".
var goVersionRegexp = regexp.MustCompile(`go(\d+\.\d+(?:\.\d+)?)`)

// defineDependencies returns the canonical dependency list for the installer.
func defineDependencies(profile PlatformProfile) []Dependency {
	deps := []Dependency{
		{
			Name:        "git",
			Required:    true,
			DetectCmd:   []string{"git", "--version"},
			InstallHint: installHintGit(profile),
		},
		{
			Name:        "curl",
			Required:    true,
			DetectCmd:   []string{"curl", "--version"},
			InstallHint: installHintCurl(profile),
		},
		{
			Name:        "node",
			Required:    true,
			MinVersion:  "18.0.0",
			DetectCmd:   []string{"node", "--version"},
			InstallHint: installHintNode(profile),
		},
		{
			Name:        "npm",
			Required:    true,
			DetectCmd:   []string{"npm", "--version"},
			InstallHint: installHintNpm(profile),
		},
	}

	// brew is optional and only relevant on macOS.
	if profile.OS == "darwin" {
		deps = append(deps, Dependency{
			Name:        "brew",
			Required:    false,
			DetectCmd:   []string{"brew", "--version"},
			InstallHint: installHintBrew(),
		})
	}

	// go is optional (needed for Engram on Linux via go install).
	deps = append(deps, Dependency{
		Name:        "go",
		Required:    false,
		DetectCmd:   []string{"go", "version"},
		InstallHint: installHintGo(profile),
	})

	return deps
}

// DetectDependencies checks all prerequisites for the installer.
// Checks run concurrently for speed.
func DetectDependencies(ctx context.Context, profile PlatformProfile) DependencyReport {
	deps := defineDependencies(profile)
	return detectDeps(ctx, deps)
}

// detectDeps runs detection on the provided dependency list (testable).
func detectDeps(ctx context.Context, deps []Dependency) DependencyReport {
	var wg sync.WaitGroup
	results := make([]Dependency, len(deps))

	for i, dep := range deps {
		wg.Add(1)
		go func(idx int, d Dependency) {
			defer wg.Done()
			results[idx] = detectSingleDep(ctx, d)
		}(i, dep)
	}

	wg.Wait()

	report := DependencyReport{
		Dependencies: results,
		AllPresent:   true,
	}

	for _, dep := range results {
		if dep.Required && !dep.Installed {
			report.AllPresent = false
			report.MissingRequired = append(report.MissingRequired, dep.Name)
		}
		if !dep.Required && !dep.Installed {
			report.MissingOptional = append(report.MissingOptional, dep.Name)
		}
	}

	return report
}

// detectSingleDep probes a single dependency using exec.LookPath + version command.
func detectSingleDep(ctx context.Context, dep Dependency) Dependency {
	if len(dep.DetectCmd) == 0 {
		return dep
	}

	// First check if binary exists on PATH.
	binary := dep.DetectCmd[0]
	if _, err := exec.LookPath(binary); err != nil {
		return dep
	}

	dep.Installed = true

	// Run version command to extract version string.
	cmd := exec.CommandContext(ctx, dep.DetectCmd[0], dep.DetectCmd[1:]...)
	sysproc.HideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		// Binary exists but version command failed — still mark as installed.
		return dep
	}

	dep.Version = parseVersion(dep.Name, string(out))

	// Check minimum version if specified.
	if dep.MinVersion != "" && dep.Version != "" {
		if !versionAtLeast(dep.Version, dep.MinVersion) {
			dep.Installed = false
		}
	}

	return dep
}

// parseVersion extracts a clean version string from command output.
func parseVersion(name, output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	// Go has a special format: "go version go1.22.5 darwin/arm64"
	if name == "go" {
		match := goVersionRegexp.FindStringSubmatch(output)
		if len(match) >= 2 {
			return match[1]
		}
	}

	// Generic: extract first semver-like pattern.
	match := versionRegexp.FindStringSubmatch(output)
	if len(match) >= 2 {
		return match[1]
	}

	return ""
}

// versionAtLeast returns true if version >= minVersion (semver comparison).
func versionAtLeast(version, minVersion string) bool {
	vParts := parseVersionParts(version)
	mParts := parseVersionParts(minVersion)

	for i := 0; i < 3; i++ {
		if vParts[i] > mParts[i] {
			return true
		}
		if vParts[i] < mParts[i] {
			return false
		}
	}

	return true // equal
}

// parseVersionParts splits "1.2.3" into [1, 2, 3], padding with zeros.
func parseVersionParts(version string) [3]int {
	parts := strings.SplitN(version, ".", 3)
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		result[i] = n
	}
	return result
}

// RenderDependencyReport formats a DependencyReport for CLI output.
func RenderDependencyReport(report DependencyReport) string {
	var b strings.Builder

	b.WriteString("Dependencies:\n")

	for _, dep := range report.Dependencies {
		status := "NOT FOUND"
		marker := "x"
		if dep.Installed {
			marker = "v"
			status = dep.Version
			if status == "" {
				status = "found"
			}
		}

		suffix := ""
		if !dep.Installed && dep.Required {
			suffix = " (required)"
		} else if !dep.Required {
			suffix = " (optional)"
		}

		b.WriteString(fmt.Sprintf("  %s: %s %s%s\n", dep.Name, marker, status, suffix))
	}

	if len(report.MissingRequired) > 0 {
		b.WriteString(fmt.Sprintf("Missing required: %s\n", strings.Join(report.MissingRequired, ", ")))
	}

	if len(report.MissingOptional) > 0 {
		b.WriteString(fmt.Sprintf("Missing optional: %s\n", strings.Join(report.MissingOptional, ", ")))
	}

	return strings.TrimRight(b.String(), "\n")
}
