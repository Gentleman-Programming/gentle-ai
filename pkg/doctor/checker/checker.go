package checker

import (
	"context"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
)

// Checker is the interface for health checks.
type Checker interface {
	Name() string
	Category() doctor.Category
	Run(ctx context.Context) []doctor.CheckResult
}

// BaseChecker provides common fields for all checkers.
type BaseChecker struct {
	name     string
	category doctor.Category
}

func (b *BaseChecker) Name() string     { return b.name }
func (b *BaseChecker) Category() doctor.Category { return b.category }

// HardwareChecker performs hardware-related health checks.
type HardwareChecker struct {
	BaseChecker
	// Dependencies for testing - overridden in tests
	hostInfoFn      func() ([]string, error)
	cpuInfoFn       func() ([]string, error)
	memInfoFn       func() (uint64, error)
	diskUsageFn     func(string) (uint64, uint64, uint64, error)
}

// NewHardwareChecker creates a new hardware checker with default implementations.
func NewHardwareChecker() *HardwareChecker {
	h := &HardwareChecker{}
	h.name = "hardware"
	h.category = doctor.CategoryHardware
	// Set default implementations (will be overridden by gopsutil in real implementation)
	h.hostInfoFn = func() ([]string, error) { return nil, nil }
	h.cpuInfoFn = func() ([]string, error) { return nil, nil }
	h.memInfoFn = func() (uint64, error) { return 0, nil }
	h.diskUsageFn = func(string) (uint64, uint64, uint64, error) { return 0, 0, 0, nil }
	return h
}

// WithHostInfoFn sets a custom host info function for testing.
func (h *HardwareChecker) WithHostInfoFn(fn func() ([]string, error)) *HardwareChecker {
	h.hostInfoFn = fn
	return h
}

// WithCPUInfoFn sets a custom CPU info function for testing.
func (h *HardwareChecker) WithCPUInfoFn(fn func() ([]string, error)) *HardwareChecker {
	h.cpuInfoFn = fn
	return h
}

// WithMemInfoFn sets a custom memory info function for testing.
func (h *HardwareChecker) WithMemInfoFn(fn func() (uint64, error)) *HardwareChecker {
	h.memInfoFn = fn
	return h
}

// WithDiskUsageFn sets a custom disk usage function for testing.
func (h *HardwareChecker) WithDiskUsageFn(fn func(string) (uint64, uint64, uint64, error)) *HardwareChecker {
	h.diskUsageFn = fn
	return h
}

// Run executes all hardware checks.
func (h *HardwareChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	results = append(results, h.checkCPU(ctx)...)
	results = append(results, h.checkMemory(ctx)...)
	results = append(results, h.checkDisk(ctx)...)
	results = append(results, h.checkGPU(ctx)...)
	results = append(results, h.checkThermal(ctx)...)
	
	return results
}

// checkCPU verifies CPU core count and model.
func (h *HardwareChecker) checkCPU(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	cpuCount := runtime.NumCPU()
	if cpuCount >= 4 {
		results = append(results, doctor.CheckResult{
			Name:     "hardware:cpu:cores",
			Category: doctor.CategoryHardware,
			Status:   doctor.StatusPass,
			Summary:  "CPU cores sufficient",
			Detail:   "Found " + string(rune(cpuCount)) + " CPU cores (minimum 4 recommended)",
		})
	} else {
		results = append(results, doctor.CheckResult{
			Name:     "hardware:cpu:cores",
			Category: doctor.CategoryHardware,
			Status:   doctor.StatusWarn,
			Summary:  "Low CPU core count",
			Detail:   "Found " + string(rune(cpuCount)) + " CPU cores (minimum 4 recommended for optimal performance)",
			Remediation: &doctor.Remediation{
				Description: "Consider upgrading hardware for better performance",
				ManualSteps: []string{"Check if CPU throttling is enabled in BIOS/power settings"},
			},
		})
	}
	
	return results
}

// checkMemory verifies available RAM.
func (h *HardwareChecker) checkMemory(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	// This will be implemented with gopsutil in Phase 2
	// For now, return a placeholder
	results = append(results, doctor.CheckResult{
		Name:     "hardware:memory:ram",
		Category: doctor.CategoryHardware,
		Status:   doctor.StatusInfo,
		Summary:  "Memory check not yet implemented",
		Detail:   "Will use gopsutil to check total/available RAM in Phase 2",
	})
	
	return results
}

// checkDisk verifies available disk space.
func (h *HardwareChecker) checkDisk(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	// This will be implemented with gopsutil in Phase 2
	// For now, return a placeholder
	results = append(results, doctor.CheckResult{
		Name:     "hardware:disk:space",
		Category: doctor.CategoryHardware,
		Status:   doctor.StatusInfo,
		Summary:  "Disk space check not yet implemented",
		Detail:   "Will use gopsutil to check disk usage in Phase 2",
	})
	
	return results
}

// checkGPU detects GPU vendor.
func (h *HardwareChecker) checkGPU(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	// Placeholder for gopsutil GPU detection
	results = append(results, doctor.CheckResult{
		Name:     "hardware:gpu:vendor",
		Category: doctor.CategoryHardware,
		Status:   doctor.StatusInfo,
		Summary:  "GPU detection not yet implemented",
		Detail:   "Will use gopsutil to detect GPU vendor in Phase 2",
	})
	
	return results
}

// checkThermal checks for thermal throttling.
func (h *HardwareChecker) checkThermal(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	// Placeholder for thermal throttling detection
	results = append(results, doctor.CheckResult{
		Name:     "hardware:thermal:throttling",
		Category: doctor.CategoryHardware,
		Status:   doctor.StatusInfo,
		Summary:  "Thermal throttling check not yet implemented",
		Detail:   "Will check CPU frequency scaling and thermal zones in Phase 2",
	})
	
	return results
}

// SoftwareChecker performs software-related health checks.
type SoftwareChecker struct {
	BaseChecker
	// Dependencies for testing
	lookPathFn   func(string) (string, error)
	execLookPath func(string) (string, error)
	httpGetFn    func(string, int) (int, error)
	pathDirsFn   func() []string
}

// NewSoftwareChecker creates a new software checker.
func NewSoftwareChecker() *SoftwareChecker {
	s := &SoftwareChecker{}
	s.name = "software"
	s.category = doctor.CategorySoftware
	s.lookPathFn = func(s string) (string, error) { return "", nil }
	s.execLookPath = func(s string) (string, error) { return "", nil }
	s.httpGetFn = func(string, int) (int, error) { return 0, nil }
	s.pathDirsFn = func() []string { return nil }
	return s
}

// WithLookPathFn sets a custom LookPath function for testing.
func (s *SoftwareChecker) WithLookPathFn(fn func(string) (string, error)) *SoftwareChecker {
	s.lookPathFn = fn
	return s
}

// WithHTTPGetFn sets a custom HTTP GET function for testing.
func (s *SoftwareChecker) WithHTTPGetFn(fn func(string, int) (int, error)) *SoftwareChecker {
	s.httpGetFn = fn
	return s
}

// WithPathDirsFn sets a custom PATH directories function for testing.
func (s *SoftwareChecker) WithPathDirsFn(fn func() []string) *SoftwareChecker {
	s.pathDirsFn = fn
	return s
}

// Run executes all software checks.
func (s *SoftwareChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	results = append(results, s.checkBinaries(ctx)...)
	results = append(results, s.checkEngramReachable(ctx)...)
	results = append(results, s.checkEnvVars(ctx)...)
	
	return results
}

// checkBinaries verifies required tools are in PATH.
func (s *SoftwareChecker) checkBinaries(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "software:binaries",
		Category: doctor.CategorySoftware,
		Status:   doctor.StatusInfo,
		Summary:  "Binary checks not yet implemented",
		Detail:   "Will check PATH for required tools in Phase 2",
	}}
}

// checkEngramReachable verifies engram HTTP health endpoint.
func (s *SoftwareChecker) checkEngramReachable(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "software:engram:reachable",
		Category: doctor.CategorySoftware,
		Status:   doctor.StatusInfo,
		Summary:  "Engram reachability check not yet implemented",
		Detail:   "Will check engram health endpoint in Phase 2",
	}}
}

// checkEnvVars validates required environment variables.
func (s *SoftwareChecker) checkEnvVars(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "software:env:vars",
		Category: doctor.CategorySoftware,
		Status:   doctor.StatusInfo,
		Summary:  "Environment variable validation not yet implemented",
		Detail:   "Will validate ENGRAM_BASE_URL, PATH, etc. in Phase 2",
	}}
}

// ConfigChecker performs configuration-related health checks.
type ConfigChecker struct {
	BaseChecker
	// Dependencies for testing
	readFileFn     func(string) ([]byte, error)
	statFn         func(string) (interface{}, error)
	userHomeDirFn  func() (string, error)
}

// NewConfigChecker creates a new config checker.
func NewConfigChecker() *ConfigChecker {
	c := &ConfigChecker{}
	c.name = "config"
	c.category = doctor.CategoryConfig
	c.readFileFn = func(string) ([]byte, error) { return nil, nil }
	c.statFn = func(string) (interface{}, error) { return nil, nil }
	c.userHomeDirFn = func() (string, error) { return "", nil }
	return c
}

// WithReadFileFn sets a custom file read function for testing.
func (c *ConfigChecker) WithReadFileFn(fn func(string) ([]byte, error)) *ConfigChecker {
	c.readFileFn = fn
	return c
}

// WithStatFn sets a custom stat function for testing.
func (c *ConfigChecker) WithStatFn(fn func(string) (interface{}, error)) *ConfigChecker {
	c.statFn = fn
	return c
}

// WithUserHomeDirFn sets a custom home dir function for testing.
func (c *ConfigChecker) WithUserHomeDirFn(fn func() (string, error)) *ConfigChecker {
	c.userHomeDirFn = fn
	return c
}

// Run executes all config checks.
func (c *ConfigChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult
	
	results = append(results, c.checkStateJSON(ctx)...)
	results = append(results, c.checkSSHConfig(ctx)...)
	results = append(results, c.checkGitConfig(ctx)...)
	results = append(results, c.checkAgentDirs(ctx)...)
	
	return results
}

// checkStateJSON validates ~/.gentle-ai/state.json and agent config dirs.
func (c *ConfigChecker) checkStateJSON(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "config:state:json",
		Category: doctor.CategoryConfig,
		Status:   doctor.StatusInfo,
		Summary:  "State JSON validation not yet implemented",
		Detail:   "Will validate state.json and agent config dirs in Phase 2",
	}}
}

// checkSSHConfig validates ~/.ssh/config permissions and syntax.
func (c *ConfigChecker) checkSSHConfig(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "config:ssh:config",
		Category: doctor.CategoryConfig,
		Status:   doctor.StatusInfo,
		Summary:  "SSH config validation not yet implemented",
		Detail:   "Will parse and validate ~/.ssh/config in Phase 2",
	}}
}

// checkGitConfig validates ~/.gitconfig user/email.
func (c *ConfigChecker) checkGitConfig(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "config:git:config",
		Category: doctor.CategoryConfig,
		Status:   doctor.StatusInfo,
		Summary:  "Git config validation not yet implemented",
		Detail:   "Will check ~/.gitconfig for user.name and user.email in Phase 2",
	}}
}

// checkAgentDirs verifies agent configuration directories exist.
func (c *ConfigChecker) checkAgentDirs(ctx context.Context) []doctor.CheckResult {
	// Placeholder - will be implemented in Phase 2
	return []doctor.CheckResult{{
		Name:     "config:agent:dirs",
		Category: doctor.CategoryConfig,
		Status:   doctor.StatusInfo,
		Summary:  "Agent directory checks not yet implemented",
		Detail:   "Will verify agent config directories exist in Phase 2",
	}}
}