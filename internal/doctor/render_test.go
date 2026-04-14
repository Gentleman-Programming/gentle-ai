package doctor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/verify"
)

func TestRenderReportHealthy(t *testing.T) {
	report := Report{
		Checks: []CheckResult{
			{ID: "binary-engram", Category: "deps", Status: verify.CheckStatusPassed, Message: "single copy at /usr/local/bin/engram"},
			{ID: "binary-gga", Category: "deps", Status: verify.CheckStatusPassed, Message: "single copy at /usr/local/bin/gga"},
		},
		Passed:  2,
		Healthy: true,
	}

	rendered := RenderReport(report)

	if !strings.Contains(rendered, "gentle-ai doctor") {
		t.Error("missing header")
	}
	if !strings.Contains(rendered, "[deps] Dependency Health") {
		t.Error("missing deps section header")
	}
	if !strings.Contains(rendered, "[ok]") {
		t.Error("missing [ok] icon")
	}
	if !strings.Contains(rendered, "2 passed, 0 failed") {
		t.Errorf("unexpected summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Status: healthy") {
		t.Error("expected healthy status")
	}
}

func TestRenderReportUnhealthy(t *testing.T) {
	report := Report{
		Checks: []CheckResult{
			{ID: "binary-engram", Category: "deps", Status: verify.CheckStatusFailed, Message: "binary missing"},
		},
		Failed: 1,
	}

	rendered := RenderReport(report)

	if !strings.Contains(rendered, "[!!]") {
		t.Error("missing [!!] icon for failure")
	}
	if !strings.Contains(rendered, "Status: unhealthy") {
		t.Error("expected unhealthy status")
	}
}

func TestRenderReportWithWarnings(t *testing.T) {
	report := Report{
		Checks: []CheckResult{
			{ID: "binary-engram", Category: "deps", Status: verify.CheckStatusPassed, Message: "ok"},
			{
				ID: "binary-gga", Category: "deps", Status: verify.CheckStatusWarning,
				Message: "gga has 2 copies in PATH",
				Details: []string{"  /usr/local/bin/gga (active)", "  /home/user/go/bin/gga (shadowed)"},
			},
		},
		Passed:   1,
		Warnings: 1,
		Healthy:  true,
	}

	rendered := RenderReport(report)

	if !strings.Contains(rendered, "[??]") {
		t.Error("missing [??] icon for warning")
	}
	if !strings.Contains(rendered, "shadowed") {
		t.Error("details should contain shadowed path")
	}
	if !strings.Contains(rendered, "healthy (with warnings)") {
		t.Error("expected 'healthy (with warnings)' status")
	}
}

func TestRenderJSONStructure(t *testing.T) {
	report := Report{
		Checks: []CheckResult{
			{ID: "binary-engram", Category: "deps", Status: verify.CheckStatusPassed, Message: "ok"},
			{ID: "binary-gga", Category: "deps", Status: verify.CheckStatusWarning, Message: "duplicate"},
		},
		Passed:   1,
		Warnings: 1,
		Healthy:  true,
		Duration: 50 * time.Millisecond,
	}

	var buf bytes.Buffer
	if err := RenderJSON(&buf, report); err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	var parsed struct {
		Checks  []struct{ ID string } `json:"checks"`
		Passed  int                   `json:"passed"`
		Healthy bool                  `json:"healthy"`
	}

	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	if len(parsed.Checks) != 2 {
		t.Errorf("checks = %d, want 2", len(parsed.Checks))
	}
	if parsed.Passed != 1 {
		t.Errorf("passed = %d, want 1", parsed.Passed)
	}
	if !parsed.Healthy {
		t.Error("healthy = false, want true (warnings are non-blocking)")
	}
}

func TestRenderJSONUnhealthy(t *testing.T) {
	report := Report{
		Checks: []CheckResult{{Status: verify.CheckStatusFailed}},
		Failed: 1,
	}

	var buf bytes.Buffer
	if err := RenderJSON(&buf, report); err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	var parsed struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Healthy {
		t.Error("healthy = true, want false for failed check")
	}
}
