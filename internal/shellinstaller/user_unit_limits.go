package shellinstaller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// UserUnitLimits is the resource budget of the transient Gentle Shell unit.
// The defaults are the historical fixed budget; a user may raise or lower it
// with `gentle-ai shell limits` (#6055). Every value stays finite, so the
// exact leaf-limit check keeps enforcing upper bounds.
type UserUnitLimits struct {
	MemoryMaxBytes uint64 `json:"memory_max_bytes"`
	SwapMaxBytes   uint64 `json:"swap_max_bytes"`
	CPUs           uint64 `json:"cpus"`
	TasksMax       uint64 `json:"tasks_max"`
}

const (
	userUnitLimitsEnv     = "GENTLE_SHELL_UNIT_LIMITS"
	userUnitLimitsRelPath = ".gentle-ai/shell-limits.json"

	userUnitMinMemoryBytes uint64 = 1 << 30
	userUnitMaxBytes       uint64 = 1 << 40
	userUnitMaxCPUs        uint64 = 1024
	userUnitMaxTasks       uint64 = 1 << 16
	userUnitCPUPeriodUSec  uint64 = 100000
)

// DefaultUserUnitLimits returns the budget used when no setting exists.
func DefaultUserUnitLimits() UserUnitLimits {
	return UserUnitLimits{MemoryMaxBytes: userUnitMemoryMaxBytes, SwapMaxBytes: 0, CPUs: 1, TasksMax: userUnitTasksMax}
}

func (l UserUnitLimits) validate() error {
	switch {
	case l.MemoryMaxBytes < userUnitMinMemoryBytes || l.MemoryMaxBytes > userUnitMaxBytes:
		return fmt.Errorf("memory must be between 1G and 1T, got %d bytes", l.MemoryMaxBytes)
	case l.SwapMaxBytes > userUnitMaxBytes:
		return fmt.Errorf("swap must be at most 1T, got %d bytes", l.SwapMaxBytes)
	case l.CPUs < 1 || l.CPUs > userUnitMaxCPUs:
		return fmt.Errorf("cpus must be between 1 and %d, got %d", userUnitMaxCPUs, l.CPUs)
	case l.TasksMax < userUnitTasksMax || l.TasksMax > userUnitMaxTasks:
		// Below the default the managed runtime cannot fork (#5815).
		return fmt.Errorf("tasks must be between %d and %d, got %d", userUnitTasksMax, userUnitMaxTasks, l.TasksMax)
	}
	return nil
}

// cpuMax is the cgroup v2 cpu.max value systemd writes for CPUQuota=N*100%
// with a 100ms period.
func (l UserUnitLimits) cpuMax() string {
	return fmt.Sprintf("%d %d", l.CPUs*userUnitCPUPeriodUSec, userUnitCPUPeriodUSec)
}

// leaf is the exact memory.max, memory.swap.max, cpu.max and pids.max content.
func (l UserUnitLimits) leaf() []string {
	return []string{strconv.FormatUint(l.MemoryMaxBytes, 10), strconv.FormatUint(l.SwapMaxBytes, 10), l.cpuMax(), strconv.FormatUint(l.TasksMax, 10)}
}

func (l UserUnitLimits) environment() string {
	return fmt.Sprintf("%s=%d/%d/%d/%d", userUnitLimitsEnv, l.MemoryMaxBytes, l.SwapMaxBytes, l.CPUs, l.TasksMax)
}

// userUnitLimitsFromEnv reads the budget a unit was created with from its
// environment list. An absent entry means the default budget.
func userUnitLimitsFromEnv(env []string) (UserUnitLimits, error) {
	value, found := "", false
	for _, entry := range env {
		if key, v, ok := strings.Cut(entry, "="); ok && key == userUnitLimitsEnv {
			value, found = v, true
		}
	}
	if !found {
		return DefaultUserUnitLimits(), nil
	}
	parts := strings.Split(value, "/")
	if len(parts) != 4 {
		return UserUnitLimits{}, fmt.Errorf("%s=%q: want memory/swap/cpus/tasks", userUnitLimitsEnv, value)
	}
	var numbers [4]uint64
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return UserUnitLimits{}, fmt.Errorf("%s=%q: %w", userUnitLimitsEnv, value, err)
		}
		numbers[i] = n
	}
	limits := UserUnitLimits{MemoryMaxBytes: numbers[0], SwapMaxBytes: numbers[1], CPUs: numbers[2], TasksMax: numbers[3]}
	if err := limits.validate(); err != nil {
		return UserUnitLimits{}, fmt.Errorf("%s=%q: %w", userUnitLimitsEnv, value, err)
	}
	return limits, nil
}

// UserUnitLimitsPath is the per-user setting file.
func UserUnitLimitsPath(home string) string {
	return filepath.Join(home, filepath.FromSlash(userUnitLimitsRelPath))
}

// LoadUserUnitLimits returns the saved budget, or the defaults when none is
// saved. An unreadable or invalid setting refuses instead of falling back.
func LoadUserUnitLimits(home string) (UserUnitLimits, error) {
	path := UserUnitLimitsPath(home)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultUserUnitLimits(), nil
	}
	if err != nil {
		return UserUnitLimits{}, fmt.Errorf("read shell limits %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var limits UserUnitLimits
	if err := decoder.Decode(&limits); err != nil {
		return UserUnitLimits{}, fmt.Errorf("parse shell limits %s: %w; fix it or run gentle-ai shell limits --reset", path, err)
	}
	if err := limits.validate(); err != nil {
		return UserUnitLimits{}, fmt.Errorf("invalid shell limits %s: %w; fix it or run gentle-ai shell limits --reset", path, err)
	}
	return limits, nil
}

// SaveUserUnitLimits validates and atomically writes the setting.
func SaveUserUnitLimits(home string, limits UserUnitLimits) error {
	if err := limits.validate(); err != nil {
		return err
	}
	path := UserUnitLimitsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(limits, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".shell-limits-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// ResetUserUnitLimits removes the setting so the defaults apply again.
func ResetUserUnitLimits(home string) error {
	if err := os.Remove(UserUnitLimitsPath(home)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// userUnitLimitsEnvironment loads the caller's setting for a new unit. The
// default budget adds no entry, so default units keep their exact arguments.
func userUnitLimitsEnvironment(home string) ([]string, error) {
	limits, err := LoadUserUnitLimits(home)
	if err != nil || limits == DefaultUserUnitLimits() {
		return nil, err
	}
	return []string{limits.environment()}, nil
}

// ParseUserUnitSize parses a byte size such as 512M, 12G, 1.5G or 1T
// (binary units); a bare number is bytes.
func ParseUserUnitSize(original string) (uint64, error) {
	text := strings.TrimSpace(original)
	multiplier := uint64(1)
	if text != "" {
		switch strings.ToUpper(text[len(text)-1:]) {
		case "K":
			multiplier = 1 << 10
		case "M":
			multiplier = 1 << 20
		case "G":
			multiplier = 1 << 30
		case "T":
			multiplier = 1 << 40
		}
		if multiplier != 1 {
			text = text[:len(text)-1]
		}
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) || strings.ContainsAny(text, "eE+-") {
		return 0, fmt.Errorf("invalid size %q: use a number with an optional K, M, G or T suffix", original)
	}
	bytes := value * float64(multiplier)
	if bytes > float64(userUnitMaxBytes) {
		return 0, fmt.Errorf("size %q is above 1T", original)
	}
	return uint64(bytes), nil
}
