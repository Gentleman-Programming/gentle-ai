//go:build linux

package shellinstaller

import (
	"strings"
	"testing"
)

// The configured limits reach the unit through its own environment entry, so
// the systemd properties and the inner leaf check read one value (#6055).
func TestUserServiceArgsApplyConfiguredLimits(t *testing.T) {
	set := UserUnitLimits{MemoryMaxBytes: 12 << 30, SwapMaxBytes: 8 << 30, CPUs: 6, TasksMax: 2048}
	env := []string{"HOME=/owned/home", set.environment()}
	for name, args := range map[string][]string{
		"user": userServiceArgs("gentle-shell-0123456789abcdef", false, "/owned/supervisor", "/cwd", nil, env),
		"root": rootServiceArgs("gentle-shell-0123456789abcdef", false, "/owned/supervisor", "/cwd", nil, env),
	} {
		joined := strings.Join(args, "\n")
		for _, literal := range []string{
			"--property=MemoryMax=12884901888", "--property=MemorySwapMax=8589934592", "--property=CPUQuota=600%",
			"--property=CPUQuotaPeriodSec=100ms", "--property=TasksMax=2048", "GENTLE_SHELL_UNIT_LIMITS=12884901888/8589934592/6/2048",
		} {
			if !strings.Contains(joined, literal) {
				t.Errorf("%s: missing %q", name, literal)
			}
		}
		if strings.Contains(joined, "MemoryMax=3221225472") || strings.Contains(joined, "CPUQuota=100%") {
			t.Errorf("%s: default limits leaked into a configured unit", name)
		}
	}
}

func TestPrivateKernelDataUsesConfiguredLimits(t *testing.T) {
	mount := "1 0 0:1 / /sys/fs/cgroup ro - cgroup2 cgroup rw\n"
	configured := []string{"12884901888", "8589934592", "600000 100000", "2048"}
	defaults := []string{"3221225472", "0", "100000 100000", "512"}

	t.Setenv(userUnitLimitsEnv, "12884901888/8589934592/6/2048")
	if err := privateKernelData("0::/\n", mount, configured); err != nil {
		t.Fatalf("configured leaf refused: %v", err)
	}
	err := privateKernelData("0::/\n", mount, defaults)
	if err == nil || !strings.Contains(err.Error(), `want 12884901888/8589934592/"600000 100000"/2048`) {
		t.Fatalf("leaf below the configured limits must be refused naming the want, got %v", err)
	}

	t.Setenv(userUnitLimitsEnv, "max/max/6/2048")
	if err := privateKernelData("0::/\n", mount, []string{"max", "max", "600000 100000", "2048"}); err == nil {
		t.Fatal("unbounded limits accepted: the leaf check must keep finite upper bounds")
	}
}
