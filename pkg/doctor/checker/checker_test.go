package checker

import (
	"context"
	"runtime"
	"testing"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor"
)

func TestNewHardwareChecker(t *testing.T) {
	h := NewHardwareChecker()
	if h == nil {
		t.Fatal("NewHardwareChecker returned nil")
	}
	if h.Name() != "hardware" {
		t.Errorf("Name() = %q, want %q", h.Name(), "hardware")
	}
	if h.Category() != doctor.CategoryHardware {
		t.Errorf("Category() = %v, want %v", h.Category(), doctor.CategoryHardware)
	}
}

func TestHardwareChecker_WithCustomFunctions(t *testing.T) {
	h := NewHardwareChecker().
		WithHostInfoFn(func() ([]string, error) { return []string{"test"}, nil }).
		WithCPUInfoFn(func() ([]string, error) { return []string{"test"}, nil }).
		WithMemInfoFn(func() (uint64, error) { return 16 * 1024 * 1024 * 1024, nil }).
		WithDiskUsageFn(func(string) (uint64, uint64, uint64, error) { return 100, 50, 50, nil })

	if h == nil {
		t.Fatal("With functions returned nil")
	}
}

func TestHardwareChecker_Run(t *testing.T) {
	h := NewHardwareChecker()
	results := h.Run(context.Background())

	if len(results) == 0 {
		t.Error("Run() should return at least one result")
	}

	// Check that all results have the correct category
	for _, r := range results {
		if r.Category != doctor.CategoryHardware {
			t.Errorf("Result %s has category %v, want %v", r.Name, r.Category, doctor.CategoryHardware)
		}
	}
}

func TestHardwareChecker_checkCPU(t *testing.T) {
	h := NewHardwareChecker()
	results := h.checkCPU(context.Background())

	if len(results) == 0 {
		t.Error("checkCPU should return at least one result")
	}

	for _, r := range results {
		if r.Name != "hardware:cpu:cores" {
			t.Errorf("Unexpected check name: %s", r.Name)
		}
		if r.Category != doctor.CategoryHardware {
			t.Errorf("Wrong category: %v", r.Category)
		}
	}

	// On most systems, we should have enough cores
	if runtime.NumCPU() >= 4 {
		if results[0].Status != doctor.StatusPass {
			t.Errorf("Expected pass for %d cores, got %s", runtime.NumCPU(), results[0].Status)
		}
	}
}

func TestHardwareChecker_checkMemory(t *testing.T) {
	h := NewHardwareChecker()
	results := h.checkMemory(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "hardware:memory:ram" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
	if results[0].Status != doctor.StatusInfo {
		t.Errorf("Expected info status for placeholder, got %s", results[0].Status)
	}
}

func TestHardwareChecker_checkDisk(t *testing.T) {
	h := NewHardwareChecker()
	results := h.checkDisk(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "hardware:disk:space" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestHardwareChecker_checkGPU(t *testing.T) {
	h := NewHardwareChecker()
	results := h.checkGPU(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "hardware:gpu:vendor" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestHardwareChecker_checkThermal(t *testing.T) {
	h := NewHardwareChecker()
	results := h.checkThermal(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "hardware:thermal:throttling" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestNewSoftwareChecker(t *testing.T) {
	s := NewSoftwareChecker()
	if s == nil {
		t.Fatal("NewSoftwareChecker returned nil")
	}
	if s.Name() != "software" {
		t.Errorf("Name() = %q, want %q", s.Name(), "software")
	}
	if s.Category() != doctor.CategorySoftware {
		t.Errorf("Category() = %v, want %v", s.Category(), doctor.CategorySoftware)
	}
}

func TestSoftwareChecker_WithCustomFunctions(t *testing.T) {
	s := NewSoftwareChecker().
		WithLookPathFn(func(string) (string, error) { return "/usr/bin/test", nil }).
		WithHTTPGetFn(func(string, int) (int, error) { return 200, nil }).
		WithPathDirsFn(func() []string { return []string{"/usr/bin"} })

	if s == nil {
		t.Fatal("With functions returned nil")
	}
}

func TestSoftwareChecker_Run(t *testing.T) {
	s := NewSoftwareChecker()
	results := s.Run(context.Background())

	if len(results) == 0 {
		t.Error("Run() should return at least one result")
	}

	for _, r := range results {
		if r.Category != doctor.CategorySoftware {
			t.Errorf("Result %s has category %v, want %v", r.Name, r.Category, doctor.CategorySoftware)
		}
	}
}

func TestSoftwareChecker_checkBinaries(t *testing.T) {
	s := NewSoftwareChecker()
	results := s.checkBinaries(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "software:binaries" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
	if results[0].Status != doctor.StatusInfo {
		t.Errorf("Expected info status for placeholder, got %s", results[0].Status)
	}
}

func TestSoftwareChecker_checkEngramReachable(t *testing.T) {
	s := NewSoftwareChecker()
	results := s.checkEngramReachable(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "software:engram:reachable" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestSoftwareChecker_checkEnvVars(t *testing.T) {
	s := NewSoftwareChecker()
	results := s.checkEnvVars(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "software:env:vars" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestNewConfigChecker(t *testing.T) {
	c := NewConfigChecker()
	if c == nil {
		t.Fatal("NewConfigChecker returned nil")
	}
	if c.Name() != "config" {
		t.Errorf("Name() = %q, want %q", c.Name(), "config")
	}
	if c.Category() != doctor.CategoryConfig {
		t.Errorf("Category() = %v, want %v", c.Category(), doctor.CategoryConfig)
	}
}

func TestConfigChecker_WithCustomFunctions(t *testing.T) {
	c := NewConfigChecker().
		WithReadFileFn(func(string) ([]byte, error) { return []byte("test"), nil }).
		WithStatFn(func(string) (interface{}, error) { return nil, nil }).
		WithUserHomeDirFn(func() (string, error) { return "/home/test", nil })

	if c == nil {
		t.Fatal("With functions returned nil")
	}
}

func TestConfigChecker_Run(t *testing.T) {
	c := NewConfigChecker()
	results := c.Run(context.Background())

	if len(results) == 0 {
		t.Error("Run() should return at least one result")
	}

	for _, r := range results {
		if r.Category != doctor.CategoryConfig {
			t.Errorf("Result %s has category %v, want %v", r.Name, r.Category, doctor.CategoryConfig)
		}
	}
}

func TestConfigChecker_checkStateJSON(t *testing.T) {
	c := NewConfigChecker()
	results := c.checkStateJSON(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "config:state:json" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestConfigChecker_checkSSHConfig(t *testing.T) {
	c := NewConfigChecker()
	results := c.checkSSHConfig(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "config:ssh:config" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestConfigChecker_checkGitConfig(t *testing.T) {
	c := NewConfigChecker()
	results := c.checkGitConfig(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "config:git:config" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}

func TestConfigChecker_checkAgentDirs(t *testing.T) {
	c := NewConfigChecker()
	results := c.checkAgentDirs(context.Background())

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "config:agent:dirs" {
		t.Errorf("Unexpected check name: %s", results[0].Name)
	}
}