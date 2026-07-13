package checks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

// HardwareChecker performs hardware-related health checks.
type HardwareChecker struct{}

// NewHardwareChecker creates a new hardware checker.
func NewHardwareChecker() *HardwareChecker {
	return &HardwareChecker{}
}

func (h *HardwareChecker) Name() string          { return "hardware" }
func (h *HardwareChecker) Category() doctor.Category { return doctor.CategoryHardware }

func (h *HardwareChecker) Run(ctx context.Context) []doctor.CheckResult {
	var results []doctor.CheckResult

	results = append(results, h.checkCPUCores(ctx)...)
	results = append(results, h.checkCPUArchitecture(ctx)...)
	results = append(results, h.checkRAM(ctx)...)
	results = append(results, h.checkDiskSpace(ctx)...)
	results = append(results, h.checkGPU(ctx)...)

	return results
}

func (h *HardwareChecker) checkCPUCores(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	cores := runtime.NumCPU()

	status := doctor.StatusPass
	if cores < 2 {
		status = doctor.StatusWarn
	}

	return []doctor.CheckResult{{
		Name:        "hw/cpu/cores",
		Category:    doctor.CategoryHardware,
		Status:      status,
		Summary:     fmt.Sprintf("CPU cores: %d", cores),
		Detail:      fmt.Sprintf("Architecture: %s", runtime.GOARCH),
		Duration:    time.Since(start),
	}}
}

func (h *HardwareChecker) checkCPUArchitecture(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	info, err := cpu.Info()
	var detail string
	if err == nil && len(info) > 0 {
		detail = fmt.Sprintf("Model: %s, Cores per CPU: %d", info[0].ModelName, info[0].Cores)
	} else {
		detail = "Could not retrieve CPU details"
	}

	return []doctor.CheckResult{{
		Name:        "hw/cpu/architecture",
		Category:    doctor.CategoryHardware,
		Status:      doctor.StatusInfo,
		Summary:     fmt.Sprintf("CPU Architecture: %s", runtime.GOARCH),
		Detail:      detail,
		Duration:    time.Since(start),
	}}
}

func (h *HardwareChecker) checkRAM(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	v, err := mem.VirtualMemory()
	if err != nil {
		return []doctor.CheckResult{{
			Name:        "hw/ram/total",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusWarn,
			Summary:     "Could not read memory info",
			Detail:      err.Error(),
			Duration:    time.Since(start),
		}}
	}

	totalGB := float64(v.Total) / (1024 * 1024 * 1024)
	availGB := float64(v.Available) / (1024 * 1024 * 1024)

	var statusTotal, statusAvail doctor.Status
	switch {
	case totalGB < 2:
		statusTotal = doctor.StatusFail
	case totalGB < 4:
		statusTotal = doctor.StatusWarn
	default:
		statusTotal = doctor.StatusPass
	}

	switch {
	case availGB < 1:
		statusAvail = doctor.StatusFail
	case availGB < 2:
		statusAvail = doctor.StatusWarn
	default:
		statusAvail = doctor.StatusPass
	}

	return []doctor.CheckResult{
		{
			Name:        "hw/ram/total",
			Category:    doctor.CategoryHardware,
			Status:      statusTotal,
			Summary:     fmt.Sprintf("Total RAM: %.1f GB", totalGB),
			Duration:    time.Since(start),
		},
		{
			Name:        "hw/ram/available",
			Category:    doctor.CategoryHardware,
			Status:      statusAvail,
			Summary:     fmt.Sprintf("Available RAM: %.1f GB", availGB),
			Duration:    time.Since(start),
		},
	}
}

func (h *HardwareChecker) checkDiskSpace(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	homeDir, _ := os.UserHomeDir()
	target := filepath.Join(homeDir, ".config", "gentle-ai")

	u, err := disk.Usage(target)
	if err != nil {
		// Fallback to home directory
		u, _ = disk.Usage(homeDir)
	}

	freeGB := float64(u.Free) / (1024 * 1024 * 1024)
	var status doctor.Status
	switch {
	case freeGB < 0.5:
		status = doctor.StatusFail
	case freeGB < 1:
		status = doctor.StatusWarn
	default:
		status = doctor.StatusPass
	}

	remediation := &doctor.Remediation{
		Description: "Free up disk space in ~/.config/gentle-ai",
		Commands: map[string]string{
			"linux":   "rm -rf ~/.cache/* && journalctl --vacuum-time=7d",
			"darwin":  "rm -rf ~/Library/Caches/*",
			"windows": "cleanmgr /sagerun:1",
		},
	}

	return []doctor.CheckResult{{
		Name:         "hw/disk/config",
		Category:     doctor.CategoryHardware,
		Status:       status,
		Summary:      fmt.Sprintf("Disk free (~/.config/gentle-ai): %.1f GB", freeGB),
		Remediation:  remediation,
		Duration:     time.Since(start),
	}}
}

func (h *HardwareChecker) checkGPU(ctx context.Context) []doctor.CheckResult {
	start := time.Now()
	var results []doctor.CheckResult

	// Host info
	info, err := host.Info()
	if err == nil {
		results = append(results, doctor.CheckResult{
			Name:        "hw/gpu/host",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusInfo,
			Summary:     fmt.Sprintf("Host: %s (%s)", info.Platform, info.PlatformVersion),
			Duration:    time.Since(start),
		})
	}

	// NVIDIA
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		out, _ := exec.Command("nvidia-smi", "--query-gpu=name,driver_version", "--format=csv,noheader").Output()
		results = append(results, doctor.CheckResult{
			Name:        "hw/gpu/nvidia",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusPass,
			Summary:     "NVIDIA GPU detected",
			Detail:      strings.TrimSpace(string(out)),
			Duration:    time.Since(start),
		})
	}

	// AMD ROCm
	if _, err := exec.LookPath("rocm-smi"); err == nil {
		results = append(results, doctor.CheckResult{
			Name:        "hw/gpu/amd",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusPass,
			Summary:     "AMD ROCm detected",
			Duration:    time.Since(start),
		})
	}

	// Intel
	if _, err := exec.LookPath("intel_gpu_top"); err == nil {
		results = append(results, doctor.CheckResult{
			Name:        "hw/gpu/intel",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusPass,
			Summary:     "Intel GPU tools detected",
			Duration:    time.Since(start),
		})
	}

	if len(results) == 0 {
		results = append(results, doctor.CheckResult{
			Name:        "hw/gpu/none",
			Category:    doctor.CategoryHardware,
			Status:      doctor.StatusInfo,
			Summary:     "No GPU detected",
			Duration:    time.Since(start),
		})
	}

	return results
}