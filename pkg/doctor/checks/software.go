package checks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
)

// SoftwareChecker performs software-related health checks.
type SoftwareChecker struct {
	RequiredBins []string
	OptionalBins []string
	MinVersions  map[string]string
}

// NewSoftwareChecker creates a new software checker with defaults.
func NewSoftwareChecker() *SoftwareChecker {
	return &SoftwareChecker{
		RequiredBins: []string{"git", "go"},
		OptionalBins: []string{"docker", "kubectl", "helm", "terraform", "node", "npm", "python3", "pip", "curl", "wget"},
		MinVersions: map[string]string{
			"git":    "2.30.0",
			"go":     "1.21.0",
			"docker": "24.0.0",
		},
	}
}

func (s *SoftwareChecker) Name() string          { return "software" }
func (s *SoftwareChecker) Category() doctor.Category { return doctor.CategorySoftware }

func (s *SoftwareChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult

	// Required binaries
	for _, bin := range s.RequiredBins {
		results = append(results, s.checkBinary(ctx, bin, true))
	}
	// Optional binaries
	for _, bin := range s.OptionalBins {
		results = append(results, s.checkBinary(ctx, bin, false))
	}
	// Environment variables
	results = append(results, s.checkEnvVars(ctx)...)
	// Gentle AI tools
	results = append(results, s.checkGentleAITools(ctx)...)
	// Shell detection
	results = append(results, s.checkShell(ctx)...)

	return results
}

func (s *SoftwareChecker) checkBinary(ctx context.Context, name string, required bool) doctor.CheckResult {
	start := time.Now()
	path, err := exec.LookPath(name)
	if err != nil {
		status := doctor.StatusWarn
		if required {
			status = doctor.StatusFail
		}
		return doctor.CheckResult{
			Name:        "sw/bin/" + name,
			Category:    doctor.CategorySoftware,
			Status:      status,
			Summary:     fmt.Sprintf("Binary '%s': NOT FOUND", name),
			Remediation: &doctor.Remediation{
				Description: fmt.Sprintf("Install %s", name),
				Commands:    s.installCommands(name),
			},
			Duration: time.Since(start),
		}
	}

	// Get version
	version := s.getVersion(name)
	minVer := s.MinVersions[name]
	status := doctor.StatusPass
	detail := fmt.Sprintf("Found at %s", path)
	if version != "" {
		detail += fmt.Sprintf(" (version: %s)", version)
		if minVer != "" && compareVersions(version, minVer) < 0 {
			status = doctor.StatusWarn
			detail += fmt.Sprintf(" — minimum recommended: %s", minVer)
		}
	}

	return doctor.CheckResult{
		Name:        "sw/bin/" + name,
		Category:    doctor.CategorySoftware,
		Status:      status,
		Summary:     fmt.Sprintf("Binary '%s': OK", name),
		Detail:      detail,
		Duration:    time.Since(start),
	}
}

func (s *SoftwareChecker) getVersion(bin string) string {
	// Common version flags
	for _, flag := range []string{"--version", "-v", "version"} {
		out, err := exec.Command(bin, flag).Output()
		if err == nil {
			return parseVersion(string(out))
		}
	}
	return ""
}

func parseVersion(output string) string {
	// Extract version from common output formats
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for patterns like "version 1.2.3", "v1.2.3", "1.2.3"
		fields := strings.Fields(line)
		for _, f := range fields {
			f = strings.TrimPrefix(f, "v")
			f = strings.TrimPrefix(f, "V")
			if strings.Contains(f, ".") && (f[0] >= '0' && f[0] <= '9') {
				return f
			}
		}
	}
	return ""
}

func compareVersions(current, minimum string) int {
	// Simple semantic version comparison
	cParts := strings.Split(current, ".")
	mParts := strings.Split(minimum, ".")
	maxLen := len(cParts)
	if len(mParts) > maxLen {
		maxLen = len(mParts)
	}
	for i := 0; i < maxLen; i++ {
		c := 0
		m := 0
		if i < len(cParts) {
			fmt.Sscanf(cParts[i], "%d", &c)
		}
		if i < len(mParts) {
			fmt.Sscanf(mParts[i], "%d", &m)
		}
		if c < m {
			return -1
		}
		if c > m {
			return 1
		}
	}
	return 0
}

func (s *SoftwareChecker) checkEnvVars(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	critical := []string{"HOME", "PATH", "SHELL"}
	var results []doctor.CheckResult
	for _, v := range critical {
		val, ok := os.LookupEnv(v)
		status := doctor.StatusPass
		detail := "Set"
		if !ok {
			status = doctor.StatusFail
			detail = "NOT SET"
		} else if v == "PATH" && val == "" {
			status = doctor.StatusWarn
			detail = "EMPTY"
		}
		results = append(results, doctor.CheckResult{
			Name:        "sw/env/" + strings.ToLower(v),
			Category:    doctor.CategorySoftware,
			Status:      status,
			Summary:     fmt.Sprintf("Env var %s: %s", v, detail),
			Duration:    time.Since(start),
		})
	}
	return results
}

func (s *SoftwareChecker) checkGentleAITools(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	tools := []string{"gentle-ai", "engram", "gga"}
	var results []doctor.CheckResult
	for _, t := range tools {
		_, err := exec.LookPath(t)
		status := doctor.StatusPass
		if err != nil {
			status = doctor.StatusWarn // Not FAIL — optional tools
		}
		results = append(results, doctor.CheckResult{
			Name:        "sw/tool/" + t,
			Category:    doctor.CategorySoftware,
			Status:      status,
			Summary:     fmt.Sprintf("gentle-ai tool '%s': %s", t, status),
			Duration:    time.Since(start),
		})
	}
	return results
}

func (s *SoftwareChecker) checkShell(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	shell := os.Getenv("SHELL")
	status := doctor.StatusInfo
	detail := "Not detected"
	if shell != "" {
		status = doctor.StatusPass
		detail = shell
		// Check if shell binary exists
		if _, err := exec.LookPath(shell); err != nil {
			status = doctor.StatusWarn
			detail += " (binary not in PATH)"
		}
	}
	return []doctor.CheckResult{{
		Name:        "sw/env/shell",
		Category:    doctor.CategorySoftware,
		Status:      status,
		Summary:     fmt.Sprintf("Shell: %s", detail),
		Duration:    time.Since(start),
	}}
}

// installCommands returns OS-specific install commands for a binary.
func (s *SoftwareChecker) installCommands(bin string) map[string]string {
	cmds := map[string]map[string]string{
		"git": {
			"linux":   "sudo apt-get update && sudo apt-get install -y git",
			"darwin":  "brew install git",
			"windows": "winget install Git.Git",
		},
		"go": {
			"linux":   "sudo apt-get update && sudo apt-get install -y golang-go",
			"darwin":  "brew install go",
			"windows": "winget install GoLang.Go",
		},
		"docker": {
			"linux":   "curl -fsSL https://get.docker.com | sh",
			"darwin":  "brew install --cask docker",
			"windows": "winget install Docker.DockerDesktop",
		},
		"kubectl": {
			"linux":   "curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl && sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl",
			"darwin":  "brew install kubectl",
			"windows": "winget install Kubernetes.kubectl",
		},
		"terraform": {
			"linux":   "wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg && echo 'deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main' | sudo tee /etc/apt/sources.list.d/hashicorp.list && sudo apt update && sudo apt install terraform",
			"darwin":  "brew install terraform",
			"windows": "winget install HashiCorp.Terraform",
		},
		"node": {
			"linux":   "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs",
			"darwin":  "brew install node",
			"windows": "winget install OpenJS.NodeJS",
		},
		"python3": {
			"linux":   "sudo apt-get update && sudo apt-get install -y python3",
			"darwin":  "brew install python",
			"windows": "winget install Python.Python.3",
		},
	}
	if m, ok := cmds[bin]; ok {
		return m
	}
	return map[string]string{
		"linux":   "Check your package manager for " + bin,
		"darwin":  "brew install " + bin,
		"windows": "winget install " + bin,
	}
}