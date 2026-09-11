package cli

import (
	"strings"
	"testing"

	componentuninstall "github.com/gentleman-programming/gentle-ai/v2/internal/components/uninstall"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// A batch that failed for one agent must not open with "complete". The user
// reads the header before the manual-cleanup detail printed further down.
func TestRenderUninstallReportHeaderNamesAPartialBatch(t *testing.T) {
	report := RenderUninstallReport(componentuninstall.Result{
		FailedAgents: []model.AgentID{model.AgentHermes},
	})
	header := strings.SplitN(report, "\n", 2)[0]
	if !strings.Contains(header, "partially complete") {
		t.Fatalf("header = %q, want it to name the batch as partial", header)
	}
	if !strings.Contains(header, "Hermes") && !strings.Contains(header, "hermes") {
		t.Fatalf("header = %q, want it to name the failed agent", header)
	}
}

func TestRenderUninstallReportHeaderStaysCompleteWhenNothingFailed(t *testing.T) {
	report := RenderUninstallReport(componentuninstall.Result{})
	header := strings.SplitN(report, "\n", 2)[0]
	if header != "Managed uninstall complete" {
		t.Fatalf("header = %q, want the unchanged complete header", header)
	}
}

func TestRenderUninstallReportAdvisoryOnly(t *testing.T) {
	report := RenderUninstallReport(componentuninstall.Result{OptionalPiPackageCleanupCommands: []string{"pi remove npm:gentle-pi"}})
	if strings.SplitN(report, "\n", 2)[0] != "Managed uninstall complete" || !strings.Contains(report, "pi remove npm:gentle-pi") {
		t.Fatalf("advice must not imply retained resources:\n%s", report)
	}
}

func TestRenderUninstallReportDistinguishesRetainedPiResourcesAndCommands(t *testing.T) {
	report := RenderUninstallReport(componentuninstall.Result{
		RetainedPiResources: []string{"/home/test/.pi/agent/agents"},
		OptionalPiPackageCleanupCommands: []string{
			"pi remove npm:gentle-pi",
			"pi remove npm:gentle-engram",
			"pi remove npm:pi-mcp-adapter",
			"pi remove npm:@juicesharp/rpiv-ask-user-question",
			"pi remove npm:pi-web-access",
			"pi remove npm:pi-btw",
		},
	})
	header := strings.SplitN(report, "\n", 2)[0]
	if header == "Managed uninstall complete" {
		t.Fatalf("header = %q, must distinguish retained Pi state", header)
	}
	for _, want := range []string{
		"Retained Pi resources (not deleted)",
		"/home/test/.pi/agent/agents",
		"Optional Pi package cleanup",
		"Review shared or user-modified packages/resources before removing them",
		"pi remove npm:gentle-pi",
		"pi remove npm:gentle-engram",
		"pi remove npm:pi-mcp-adapter",
		"pi remove npm:@juicesharp/rpiv-ask-user-question",
		"pi remove npm:pi-web-access",
		"pi remove npm:pi-btw",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
